package xray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// certPEM is a generated cert/key pair plus the PEM of the CA that signed it.
type certPEM struct {
	cert string
	key  string
}

func mustGenCA(t *testing.T) (caPEM string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ = x509.ParseCertificate(der)
	caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return caPEM, caCert, key
}

func mustGenLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, server bool) certPEM {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.DNSNames = []string{cn}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	return certPEM{
		cert: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		key:  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
	}
}

func TestBuildTLSConfigValidation(t *testing.T) {
	caPEM, ca, caKey := mustGenCA(t)
	client := mustGenLeaf(t, ca, caKey, "control-plane", false)

	// Valid: CA + client cert/key.
	if _, err := buildTLSConfig(DialOptions{
		TLS: true, CACertPEM: caPEM, ClientCertPEM: client.cert, ClientKeyPEM: client.key,
	}); err != nil {
		t.Fatalf("valid mTLS material rejected: %v", err)
	}
	// Cert without key is an error.
	if _, err := buildTLSConfig(DialOptions{TLS: true, ClientCertPEM: client.cert}); err == nil {
		t.Error("cert without key should fail")
	}
	// Garbage CA is an error.
	if _, err := buildTLSConfig(DialOptions{TLS: true, CACertPEM: "not a pem"}); err == nil {
		t.Error("invalid CA PEM should fail")
	}
	// Mismatched cert/key is an error.
	other := mustGenLeaf(t, ca, caKey, "other", false)
	if _, err := buildTLSConfig(DialOptions{
		TLS: true, ClientCertPEM: client.cert, ClientKeyPEM: other.key,
	}); err == nil {
		t.Error("mismatched cert/key should fail")
	}
}

// TestMutualTLSHandshake proves the client config actually performs mutual auth
// against a server that requires and verifies client certificates — the real
// behavior mTLS must guarantee, not just that the PEM parses.
func TestMutualTLSHandshake(t *testing.T) {
	caPEM, ca, caKey := mustGenCA(t)
	server := mustGenLeaf(t, ca, caKey, "node.local", true)
	client := mustGenLeaf(t, ca, caKey, "control-plane", false)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM([]byte(caPEM))
	serverCert, err := tls.X509KeyPair([]byte(server.cert), []byte(server.key))
	if err != nil {
		t.Fatal(err)
	}

	run := func(clientCfg *tls.Config) error {
		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert, // node enforces mTLS
			ClientCAs:    caPool,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		errCh := make(chan error, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()
			tconn := conn.(*tls.Conn)
			errCh <- tconn.Handshake()
		}()

		clientCfg.ServerName = "node.local"
		c, err := tls.Dial("tcp", ln.Addr().String(), clientCfg)
		if err != nil {
			return err
		}
		c.Close()
		return <-errCh
	}

	// With a valid client cert, both sides complete the handshake.
	good, err := buildTLSConfig(DialOptions{
		TLS: true, CACertPEM: caPEM, ClientCertPEM: client.cert, ClientKeyPEM: client.key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := run(good); err != nil {
		t.Fatalf("valid mTLS handshake failed: %v", err)
	}

	// Without a client cert, the server rejects the connection.
	noCert, _ := buildTLSConfig(DialOptions{TLS: true, CACertPEM: caPEM})
	if err := run(noCert); err == nil {
		t.Error("server accepted a client with no certificate")
	}
}
