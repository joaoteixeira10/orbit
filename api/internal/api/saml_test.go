package api

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"

	"github.com/beevik/etree"
	"github.com/gin-gonic/gin"
	dsig "github.com/russellhaering/goxmldsig"
)

// fakeSAMLIdP signs responses as a SAML identity provider would.
type fakeSAMLIdP struct {
	ks       dsig.X509KeyStore
	metadata string
}

func newFakeSAMLIdP(t *testing.T) *fakeSAMLIdP {
	t.Helper()
	ks := dsig.RandomKeyStoreForTest()
	_, cert, err := ks.GetKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	md := `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.test">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing"><KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#"><X509Data><X509Certificate>` + base64.StdEncoding.EncodeToString(cert) + `</X509Certificate></X509Data></KeyInfo></KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.test/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`
	return &fakeSAMLIdP{ks: ks, metadata: md}
}

type samlAnswer struct {
	issuer, audience, inResponseTo, nameID string
	notOnOrAfter                           time.Time
	attrs                                  map[string][]string
	unsigned                               bool
}

// response is a SAML Response with one assertion, signed (unless unsigned).
func (f *fakeSAMLIdP) response(t *testing.T, acs string, a samlAnswer) string {
	t.Helper()
	now := time.Now().UTC()
	ts := func(x time.Time) string { return x.Format("2006-01-02T15:04:05Z") }
	doc := etree.NewDocument()
	resp := doc.CreateElement("samlp:Response")
	resp.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	resp.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	resp.CreateAttr("ID", "_r"+randHex(8))
	resp.CreateAttr("Version", "2.0")
	resp.CreateAttr("IssueInstant", ts(now))
	resp.CreateAttr("Destination", acs)
	resp.CreateAttr("InResponseTo", a.inResponseTo)
	resp.CreateElement("saml:Issuer").SetText(a.issuer)
	resp.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", "urn:oasis:names:tc:SAML:2.0:status:Success")

	as := etree.NewElement("saml:Assertion")
	as.CreateAttr("ID", "_a"+randHex(8))
	as.CreateAttr("Version", "2.0")
	as.CreateAttr("IssueInstant", ts(now))
	as.CreateElement("saml:Issuer").SetText(a.issuer)
	sub := as.CreateElement("saml:Subject")
	nid := sub.CreateElement("saml:NameID")
	nid.CreateAttr("Format", "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified")
	nid.SetText(a.nameID)
	sc := sub.CreateElement("saml:SubjectConfirmation")
	sc.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	scd := sc.CreateElement("saml:SubjectConfirmationData")
	scd.CreateAttr("InResponseTo", a.inResponseTo)
	scd.CreateAttr("NotOnOrAfter", ts(a.notOnOrAfter))
	scd.CreateAttr("Recipient", acs)
	cond := as.CreateElement("saml:Conditions")
	cond.CreateAttr("NotBefore", ts(now.Add(-time.Minute)))
	cond.CreateAttr("NotOnOrAfter", ts(a.notOnOrAfter))
	cond.CreateElement("saml:AudienceRestriction").CreateElement("saml:Audience").SetText(a.audience)
	an := as.CreateElement("saml:AuthnStatement")
	an.CreateAttr("AuthnInstant", ts(now))
	an.CreateAttr("SessionIndex", "_s1")
	an.CreateElement("saml:AuthnContext").CreateElement("saml:AuthnContextClassRef").SetText("urn:oasis:names:tc:SAML:2.0:ac:classes:PasswordProtectedTransport")
	st := as.CreateElement("saml:AttributeStatement")
	for name, vals := range a.attrs {
		at := st.CreateElement("saml:Attribute")
		at.CreateAttr("Name", name)
		for _, v := range vals {
			at.CreateElement("saml:AttributeValue").SetText(v)
		}
	}
	resp.AddChild(as)
	root := resp
	if !a.unsigned { // the whole response signed, as many identity providers do
		signed, err := dsig.NewDefaultSigningContext(f.ks).SignEnveloped(resp)
		if err != nil {
			t.Fatal(err)
		}
		root = signed
	}
	out := etree.NewDocument()
	out.SetRoot(root)
	out.WriteSettings = etree.WriteSettings{CanonicalAttrVal: true, CanonicalEndTags: true, CanonicalText: true}
	b, err := out.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func randHex(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = "0123456789abcdef"[time.Now().UnixNano()>>uint(i)&15]
	}
	return string(b) + strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
}

var samlReqID = regexp.MustCompile(`ID="([^"]+)"`)

// begin starts a sign-in: the request's id and the relay state.
func samlBegin(t *testing.T, r *gin.Engine) (string, string) {
	const provider = "corp"
	t.Helper()
	w := do(r, "GET", "/api/v1/auth/saml/"+provider+"/login", "")
	if w.Code != http.StatusFound {
		t.Fatalf("saml login: %d %s", w.Code, w.Body.String())
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Host != "idp.test" {
		t.Fatalf("redirect: %s", loc)
	}
	raw, _ := base64.StdEncoding.DecodeString(loc.Query().Get("SAMLRequest"))
	xmlReq, _ := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	m := samlReqID.FindStringSubmatch(string(xmlReq))
	if m == nil {
		t.Fatalf("request: %s", xmlReq)
	}
	return m[1], loc.Query().Get("RelayState")
}

// samlPost posts the IdP's answer to the ACS: the access token, or the sso_error.
func samlPost(t *testing.T, r *gin.Engine, relay, resp string) (string, string) {
	const provider = "corp"
	t.Helper()
	form := url.Values{"SAMLResponse": {resp}, "RelayState": {relay}}
	req := httptest.NewRequest("POST", "/api/v1/auth/saml/"+provider+"/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "" // as the other test requests
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	loc, _ := url.Parse(w.Header().Get("Location"))
	if e := loc.Query().Get("sso_error"); e != "" {
		return "", e
	}
	w = do(r, "POST", "/api/v1/auth/oidc/exchange", `{"code":"`+loc.Query().Get("sso")+`"}`)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok.AccessToken == "" {
		t.Fatalf("exchange: %d %s (acs %s)", w.Code, w.Body.String(), loc)
	}
	return tok.AccessToken, ""
}

// TestSAML: sign-in through a SAML identity provider — the request answered,
// signed, for this audience, in time; its groups kept; anything else refused.
func TestSAML(t *testing.T) {
	r, db, owner, _, _ := plannerSetupDB(t)
	idp := newFakeSAMLIdP(t)
	setProviders(t, db, []auth.OIDCSettings{{ID: "corp", Name: "Corp", Kind: "saml", Enabled: true, MetadataXML: idp.metadata, GroupClaim: "groups", Domains: []string{"corp.test"}}})
	base := "http:///api/v1/auth/saml/corp" // the test requests have no host
	acs, audience := base+"/acs", base+"/metadata"

	// Orbit's metadata, for the IdP.
	if md := do(r, "GET", "/api/v1/auth/saml/corp/metadata", "").Body.String(); !strings.Contains(md, acs) || !strings.Contains(md, `entityID="`+audience+`"`) {
		t.Fatalf("sp metadata: %s", md)
	}
	var b struct {
		Providers []struct{ ID, Kind string } `json:"sso_providers"`
	}
	decode(t, do(r, "GET", "/api/v1/branding", ""), &b)
	if len(b.Providers) != 1 || b.Providers[0].Kind != "saml" {
		t.Fatalf("buttons: %+v", b.Providers)
	}

	good := func(reqID string) samlAnswer {
		return samlAnswer{issuer: "https://idp.test", audience: audience, inResponseTo: reqID, nameID: "kim-1", notOnOrAfter: time.Now().Add(5 * time.Minute),
			attrs: map[string][]string{"email": {"kim@corp.test"}, "displayName": {"Kim Park"}, "groups": {"Ops", "Sales"}}}
	}

	// A good answer signs in, with the groups.
	reqID, relay := samlBegin(t, r)
	tok, fail := samlPost(t, r, relay, idp.response(t, acs, good(reqID)))
	if fail != "" {
		t.Fatalf("good answer refused: %s", fail)
	}
	var me struct {
		User struct {
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"user"`
	}
	decode(t, authdo(r, "GET", "/api/v1/auth/me", "", tok), &me)
	if me.User.Email != "kim@corp.test" || me.User.DisplayName != "Kim Park" {
		t.Fatalf("me: %+v", me.User)
	}
	if g := authdo(r, "GET", "/api/v1/groups", "", owner).Body.String(); !strings.Contains(g, `"name":"Ops"`) || !strings.Contains(g, `"name":"Sales"`) {
		t.Fatalf("groups: %s", g)
	}
	// The same answer again (its relay state spent): refused.
	if _, fail := samlPost(t, r, relay, idp.response(t, acs, good(reqID))); fail == "" {
		t.Fatal("replayed")
	}

	refused := map[string]func(a *samlAnswer){
		"unsigned":         func(a *samlAnswer) { a.unsigned = true },
		"another audience": func(a *samlAnswer) { a.audience = "https://elsewhere.test" },
		"another request":  func(a *samlAnswer) { a.inResponseTo = "_not-ours" },
		"another issuer":   func(a *samlAnswer) { a.issuer = "https://evil.test" },
		"expired":          func(a *samlAnswer) { a.notOnOrAfter = time.Now().Add(-time.Minute) },
	}
	for name, change := range refused {
		reqID, relay := samlBegin(t, r)
		a := good(reqID)
		change(&a)
		if _, fail := samlPost(t, r, relay, idp.response(t, acs, a)); fail == "" {
			t.Errorf("%s: signed in", name)
		}
	}
	// Signed by someone else's key: refused.
	other := newFakeSAMLIdP(t)
	reqID, relay = samlBegin(t, r)
	if _, fail := samlPost(t, r, relay, other.response(t, acs, good(reqID))); fail == "" {
		t.Error("another key: signed in")
	}
	// Not of the allowed domain.
	reqID, relay = samlBegin(t, r)
	a := good(reqID)
	a.attrs["email"] = []string{"kim@gmail.test"}
	if _, fail := samlPost(t, r, relay, idp.response(t, acs, a)); fail != "domain" {
		t.Errorf("other domain: %q", fail)
	}
}
