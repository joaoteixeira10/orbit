// Package tlsfiles loads TLS certificates and CA bundles from files (or PEM
// handed over by the secrets backend). A certificate loaded from files is
// loaded again when they change, so a renewed certificate needs no restart.
// Orbit uses it for its own HTTPS listener (TLS_CERT_FILE / TLS_KEY_FILE).
package tlsfiles

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Loader serves one certificate and its key.
type Loader struct {
	certFile, keyFile string
	certPEM, keyPEM   []byte

	mu    sync.Mutex
	cert  *tls.Certificate
	stamp string
}

// NewLoader loads a certificate from files, or from PEM when a file is "".
// It fails if the pair can't be loaded now.
func NewLoader(certFile, keyFile, certPEM, keyPEM string) (*Loader, error) {
	l := &Loader{certFile: certFile, keyFile: keyFile, certPEM: []byte(certPEM), keyPEM: []byte(keyPEM)}
	if _, err := l.Get(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Loader) fileStamp() string {
	var parts []string
	for _, f := range []string{l.certFile, l.keyFile} {
		if f == "" {
			continue
		}
		if st, err := os.Stat(f); err == nil {
			parts = append(parts, fmt.Sprintf("%d.%d", st.ModTime().UnixNano(), st.Size()))
		}
	}
	return strings.Join(parts, "/")
}

func (l *Loader) load() (*tls.Certificate, error) {
	certPEM, keyPEM := l.certPEM, l.keyPEM
	var err error
	if l.certFile != "" {
		if certPEM, err = os.ReadFile(l.certFile); err != nil {
			return nil, err
		}
	}
	if l.keyFile != "" {
		if keyPEM, err = os.ReadFile(l.keyFile); err != nil {
			return nil, err
		}
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// Get returns the certificate, loading it again if its files changed. A
// renewal that can't be loaded (half written) keeps the old one.
func (l *Loader) Get() (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s := l.fileStamp(); l.cert == nil || s != l.stamp {
		cert, err := l.load()
		if err != nil {
			if l.cert != nil {
				log.Printf("tls: can't load the new certificate, keeping the old one: %v", err)
				return l.cert, nil
			}
			return nil, err
		}
		l.cert, l.stamp = cert, s
	}
	return l.cert, nil
}

// ServerCertificate is for tls.Config.GetCertificate.
func (l *Loader) ServerCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) { return l.Get() }

// ClientCertificate is for tls.Config.GetClientCertificate.
func (l *Loader) ClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return l.Get()
}

// Pool reads a PEM CA bundle; with system true it adds to the system's CAs.
func Pool(file string, system bool) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if system {
		if sp, err := x509.SystemCertPool(); err == nil && sp != nil {
			pool = sp
		}
	}
	pem, err := os.ReadFile(file) //nolint:gosec // the operator's own CA file
	if err != nil {
		return nil, err
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New(file + " holds no PEM certificate")
	}
	return pool, nil
}
