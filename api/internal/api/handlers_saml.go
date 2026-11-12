package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	saml2 "github.com/russellhaering/gosaml2"
	samltypes "github.com/russellhaering/gosaml2/types"
	dsig "github.com/russellhaering/goxmldsig"
	"go.uber.org/zap"
)

// SAML 2.0 sign-in: a provider of kind "saml" in the same list as the
// OpenID Connect ones. Orbit is the service provider: its metadata at
// /api/v1/auth/saml/<id>/metadata (entity id and audience), its assertion
// consumer service at …/acs, its signing key made once and kept sealed.
// The identity provider's metadata comes from its URL (fetched through the
// guarded client, cached a while) or pasted XML. A response is accepted only
// signed by the provider's certificate, by the expected issuer, for this
// audience and recipient, in its time window, and answering a request Orbit
// made (InResponseTo, single use) — no unsolicited (IdP-initiated) sign-ins.

const samlSPKeySetting = "saml_sp_key"

// samlKeyStore is the service provider's key pair.
type samlKeyStore struct {
	key  *rsa.PrivateKey
	cert []byte // DER
}

func (k samlKeyStore) GetKeyPair() (*rsa.PrivateKey, []byte, error) { return k.key, k.cert, nil }

type sealedSPKey struct {
	Key  string `json:"key"`  // PKCS#1, base64
	Cert string `json:"cert"` // DER, base64
}

var samlKeyMu sync.Mutex

// samlSPKey is the instance's SAML key, made the first time and kept sealed.
func (h *Handlers) samlSPKey() (samlKeyStore, error) {
	samlKeyMu.Lock()
	defer samlKeyMu.Unlock()
	var row models.Setting
	if h.db.Where(&models.Setting{Key: samlSPKeySetting}).Limit(1).Find(&row).RowsAffected == 1 {
		raw, err := base64.StdEncoding.DecodeString(row.Value)
		if err != nil {
			return samlKeyStore{}, err
		}
		plain, err := h.sealer.Unseal(raw)
		if err != nil {
			return samlKeyStore{}, err
		}
		var s sealedSPKey
		if err := json.Unmarshal(plain, &s); err != nil {
			return samlKeyStore{}, err
		}
		kb, _ := base64.StdEncoding.DecodeString(s.Key)
		cb, _ := base64.StdEncoding.DecodeString(s.Cert)
		key, err := x509.ParsePKCS1PrivateKey(kb)
		return samlKeyStore{key: key, cert: cb}, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return samlKeyStore{}, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Orbit SAML"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	cert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return samlKeyStore{}, err
	}
	b, _ := json.Marshal(sealedSPKey{Key: base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key)), Cert: base64.StdEncoding.EncodeToString(cert)})
	sealed, err := h.sealer.Seal(b)
	if err != nil {
		return samlKeyStore{}, err
	}
	if err := h.db.Save(&models.Setting{Key: samlSPKeySetting, Value: base64.StdEncoding.EncodeToString(sealed), UpdatedAt: time.Now()}).Error; err != nil {
		return samlKeyStore{}, err
	}
	return samlKeyStore{key: key, cert: cert}, nil
}

// ---- The identity provider's metadata ----

type samlMetaEntry struct {
	at time.Time
	ed *samltypes.EntityDescriptor
}

var (
	samlMetaMu    sync.Mutex
	samlMetaCache = map[string]samlMetaEntry{} // by metadata URL
)

const samlMetaTTL = 10 * time.Minute

func (h *Handlers) samlMetadata(ctx context.Context, p auth.OIDCSettings) (*samltypes.EntityDescriptor, error) {
	raw := []byte(p.MetadataXML)
	if len(raw) == 0 {
		samlMetaMu.Lock()
		if m, ok := samlMetaCache[p.Issuer]; ok && time.Since(m.at) < samlMetaTTL {
			samlMetaMu.Unlock()
			return m.ed, nil
		}
		samlMetaMu.Unlock()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Issuer, nil)
		if err != nil {
			return nil, err
		}
		resp, err := auth.OIDCClient().Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("metadata: HTTP %d", resp.StatusCode)
		}
		if raw, err = io.ReadAll(io.LimitReader(resp.Body, 2<<20)); err != nil {
			return nil, err
		}
	}
	var ed samltypes.EntityDescriptor
	if err := xml.Unmarshal(raw, &ed); err != nil {
		return nil, fmt.Errorf("metadata: %w", err)
	}
	if ed.IDPSSODescriptor == nil || ed.EntityID == "" {
		return nil, errors.New("metadata: not an identity provider's (no IDPSSODescriptor)")
	}
	if len(p.MetadataXML) == 0 {
		samlMetaMu.Lock()
		samlMetaCache[p.Issuer] = samlMetaEntry{at: time.Now(), ed: &ed}
		samlMetaMu.Unlock()
	}
	return &ed, nil
}

// samlSP is the service provider for a provider, from its metadata.
func (h *Handlers) samlSP(c *gin.Context, p auth.OIDCSettings) (*saml2.SAMLServiceProvider, error) {
	ed, err := h.samlMetadata(c.Request.Context(), p)
	if err != nil {
		return nil, err
	}
	idp := ed.IDPSSODescriptor
	ssoURL, binding := "", ""
	for _, s := range idp.SingleSignOnServices {
		if s.Binding == saml2.BindingHttpRedirect || ssoURL == "" {
			ssoURL, binding = s.Location, s.Binding
		}
	}
	if ssoURL == "" {
		return nil, errors.New("metadata: no SingleSignOnService")
	}
	certs := &dsig.MemoryX509CertificateStore{}
	for _, kd := range idp.KeyDescriptors {
		if kd.Use != "" && kd.Use != "signing" {
			continue
		}
		for _, xc := range kd.KeyInfo.X509Data.X509Certificates {
			der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(xc.Data), ""))
			if err != nil {
				continue
			}
			if cert, err := x509.ParseCertificate(der); err == nil {
				certs.Roots = append(certs.Roots, cert)
			}
		}
	}
	if len(certs.Roots) == 0 {
		return nil, errors.New("metadata: no signing certificate")
	}
	ks, err := h.samlSPKey()
	if err != nil {
		return nil, err
	}
	base := h.baseURL(c) + "/api/v1/auth/saml/" + p.ID
	return &saml2.SAMLServiceProvider{
		IdentityProviderSSOURL:      ssoURL,
		IdentityProviderSSOBinding:  binding,
		IdentityProviderIssuer:      ed.EntityID,
		AssertionConsumerServiceURL: base + "/acs",
		ServiceProviderIssuer:       base + "/metadata",
		AudienceURI:                 base + "/metadata",
		IDPCertificateStore:         certs,
		SPKeyStore:                  ks,
		SignAuthnRequests:           idp.WantAuthnRequestsSigned,
	}, nil
}

func (h *Handlers) samlProvider(c *gin.Context) (auth.OIDCSettings, bool) {
	p, ok := h.oidc.Provider(c.Param("id"))
	if !ok || p.Kind != "saml" || !p.Usable() {
		return p, false
	}
	return p, true
}

// samlMetadataOut serves Orbit's service-provider metadata for a provider,
// to give the identity provider.
//
//	@Summary	SAML service-provider metadata
//	@Tags		auth
//	@Produce	xml
//	@Param		id	path	string	true	"Provider id"
//	@Router		/auth/saml/{id}/metadata [get]
func (h *Handlers) samlMetadataOut(c *gin.Context) {
	p, ok := h.oidc.Provider(c.Param("id"))
	if !ok || p.Kind != "saml" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such SAML provider"})
		return
	}
	ks, err := h.samlSPKey()
	if err != nil {
		h.internalError(c, err, "no SAML key")
		return
	}
	base := h.baseURL(c) + "/api/v1/auth/saml/" + p.ID
	// Without the IdP's metadata yet, still what to give it.
	sp := &saml2.SAMLServiceProvider{AssertionConsumerServiceURL: base + "/acs", ServiceProviderIssuer: base + "/metadata", AudienceURI: base + "/metadata", SPKeyStore: ks}
	md, err := sp.Metadata()
	if err != nil {
		h.internalError(c, err, "could not make the metadata")
		return
	}
	b, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		h.internalError(c, err, "could not make the metadata")
		return
	}
	c.Data(http.StatusOK, "application/samlmetadata+xml", append([]byte(xml.Header), b...))
}

type samlState struct {
	Provider  string `json:"provider"`
	RequestID string `json:"request_id"`
}

// samlLogin sends the browser to the identity provider with a request.
//
//	@Summary	Begin SAML login
//	@Tags		auth
//	@Param		id	path	string	true	"Provider id"
//	@Router		/auth/saml/{id}/login [get]
func (h *Handlers) samlLogin(c *gin.Context) {
	p, ok := h.samlProvider(c)
	if !ok {
		ssoFail(c, "disabled")
		return
	}
	sp, err := h.samlSP(c, p)
	if err != nil {
		h.log.Warn("saml begin failed", zap.String("provider", p.ID), zap.Error(err))
		ssoFail(c, "unavailable")
		return
	}
	doc, err := sp.BuildAuthRequestDocument()
	if err != nil {
		ssoFail(c, "unavailable")
		return
	}
	state, err := h.ephemeral.Put(ephemeral.KindSAMLState, samlState{Provider: p.ID, RequestID: doc.Root().SelectAttrValue("ID", "")}, 10*time.Minute)
	if err != nil {
		ssoFail(c, "unavailable")
		return
	}
	if sp.IdentityProviderSSOBinding == saml2.BindingHttpPost {
		body, err := sp.BuildAuthBodyPostFromDocument(state, doc)
		if err != nil {
			ssoFail(c, "unavailable")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", body)
		return
	}
	u, err := sp.BuildAuthURLFromDocument(state, doc)
	if err != nil {
		ssoFail(c, "unavailable")
		return
	}
	c.Redirect(http.StatusFound, u)
}

// samlNames are the attributes a person's email and name come in (as Entra,
// Okta, Google, ADFS and Keycloak name them).
var (
	samlEmailAttrs = []string{"email", "mail", "Email", "emailaddress", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress", "urn:oid:0.9.2342.19200300.100.1.3"}
	samlNameAttrs  = []string{"displayName", "name", "http://schemas.microsoft.com/identity/claims/displayname", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name", "urn:oid:2.16.840.1.113730.3.1.241"}
	samlUserAttrs  = []string{"username", "uid", "preferred_username", "Login", "urn:oid:0.9.2342.19200300.100.1.1"}
)

func firstValue(v saml2.Values, names []string) string {
	for _, n := range names {
		if s := v.Get(n); s != "" {
			return s
		}
	}
	return ""
}

func allValues(v saml2.Values, name string) []string {
	if name == "" {
		return nil
	}
	a, ok := v[name]
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, x := range a.Values {
		if s := strings.TrimSpace(x.Value); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// samlACS takes the identity provider's answer, checks it, and signs in.
//
//	@Summary	SAML assertion consumer service
//	@Tags		auth
//	@Accept		x-www-form-urlencoded
//	@Param		id	path	string	true	"Provider id"
//	@Router		/auth/saml/{id}/acs [post]
func (h *Handlers) samlACS(c *gin.Context) {
	p, ok := h.samlProvider(c)
	if !ok {
		ssoFail(c, "disabled")
		return
	}
	var st samlState
	if took, err := h.ephemeral.Take(ephemeral.KindSAMLState, c.PostForm("RelayState"), &st); err != nil || !took || st.Provider != p.ID {
		ssoFail(c, "auth_failed") // unsolicited, replayed, or another provider's
		return
	}
	sp, err := h.samlSP(c, p)
	if err != nil {
		ssoFail(c, "unavailable")
		return
	}
	a, err := sp.RetrieveAssertionInfo(c.PostForm("SAMLResponse"))
	if err != nil {
		h.log.Warn("saml response refused", zap.String("provider", p.ID), zap.Error(err))
		ssoFail(c, "auth_failed")
		return
	}
	inResponseTo := ""
	if len(a.Assertions) > 0 && a.Assertions[0].Subject != nil && a.Assertions[0].Subject.SubjectConfirmation != nil && a.Assertions[0].Subject.SubjectConfirmation.SubjectConfirmationData != nil {
		inResponseTo = a.Assertions[0].Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo
	}
	switch {
	case a.WarningInfo != nil && (a.WarningInfo.InvalidTime || a.WarningInfo.NotInAudience):
		h.log.Warn("saml assertion out of time or audience", zap.String("provider", p.ID))
		ssoFail(c, "auth_failed")
		return
	case inResponseTo == "" || inResponseTo != st.RequestID:
		h.log.Warn("saml response to another request", zap.String("provider", p.ID))
		ssoFail(c, "auth_failed")
		return
	case strings.TrimSpace(a.NameID) == "":
		ssoFail(c, "auth_failed")
		return
	}
	info := &auth.OIDCUserInfo{Provider: p.ID, Subject: p.ID + "|" + a.NameID}
	info.Email = firstValue(a.Values, samlEmailAttrs)
	if info.Email == "" && strings.Contains(a.NameID, "@") {
		info.Email = a.NameID
	}
	info.Name = firstValue(a.Values, samlNameAttrs)
	if info.Name == "" {
		info.Name = strings.TrimSpace(firstValue(a.Values, []string{"givenName", "FirstName", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname"}) + " " +
			firstValue(a.Values, []string{"sn", "surname", "LastName", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname"}))
	}
	info.Username = firstValue(a.Values, samlUserAttrs)
	if info.Username == "" {
		info.Username = info.Email
	}
	if info.Username == "" {
		info.Username = a.NameID
	}
	if !auth.DomainAllowed(p, info.Email) {
		ssoFail(c, "domain")
		return
	}
	if p.RoleClaim != "" {
		info.Roles = allValues(a.Values, p.RoleClaim)
	}
	if p.GroupClaim != "" {
		info.Groups = allValues(a.Values, p.GroupClaim)
	}
	h.ssoComplete(c, info, "auth.saml_login")
}
