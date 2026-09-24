// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TLSFixture is a throwaway certificate authority and the material signed by
// it, generated fresh for one test and removed with it.
//
// Generated in process rather than shelled out to openssl, and never checked
// in. A certificate in a repository is a secret in a repository even when it is
// worthless, and the checked-in kind is the sort that outlives its purpose and
// turns up in a scanner years later.
type TLSFixture struct {
	Dir string

	// CAFile is the certificate authority, passed to a server that must verify
	// client certificates and to a client that must verify the server.
	CAFile string

	// ServerCertFile and ServerKeyFile are the separate PEM files that
	// DumboDB's tlsutil.Config takes.
	ServerCertFile string
	ServerKeyFile  string

	// ServerPEMFile is the certificate and key concatenated, which is the form
	// mongod's --tlsCertificateKeyFile requires.
	ServerPEMFile string

	// ClientCertFile, ClientKeyFile and ClientPEMFile are for mutual TLS.
	// Supplying a CA to DumboDB's listener turns on RequireAndVerifyClientCert,
	// so any test that gives the server a CA needs these to connect at all.
	ClientCertFile string
	ClientKeyFile  string
	ClientPEMFile  string
}

// NewTLSFixture generates a CA, a server certificate and a client certificate.
//
// The server certificate carries both 127.0.0.1 and localhost, because a
// replica set member is addressed by whatever string is in its configuration
// and a certificate that covers only one spelling fails verification for the
// other.
func NewTLSFixture(t *testing.T) *TLSFixture {
	t.Helper()

	dir, err := os.MkdirTemp("", "tls-fixture-")
	if err != nil {
		t.Fatalf("NewTLSFixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	f := &TLSFixture{
		Dir:            dir,
		CAFile:         filepath.Join(dir, "ca.crt"),
		ServerCertFile: filepath.Join(dir, "server.crt"),
		ServerKeyFile:  filepath.Join(dir, "server.key"),
		ServerPEMFile:  filepath.Join(dir, "server.pem"),
		ClientCertFile: filepath.Join(dir, "client.crt"),
		ClientKeyFile:  filepath.Join(dir, "client.key"),
		ClientPEMFile:  filepath.Join(dir, "client.pem"),
	}

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("NewTLSFixture: generating CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serialNumber(t),
		Subject:               pkix.Name{CommonName: "dumbodb parity test CA", Organization: []string{"dumbodb-parity-testing"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("NewTLSFixture: signing CA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("NewTLSFixture: parsing CA: %v", err)
	}
	writePEM(t, f.CAFile, "CERTIFICATE", caDER)
	writeKey(t, f.caKeyFile(), caKey)

	serverDER, serverKey := signLeaf(t, caCert, caKey, "127.0.0.1", true)
	writePEM(t, f.ServerCertFile, "CERTIFICATE", serverDER)
	writeKey(t, f.ServerKeyFile, serverKey)
	writeCombined(t, f.ServerPEMFile, serverDER, serverKey)

	clientDER, clientKey := signLeaf(t, caCert, caKey, "dumbodb parity test client", false)
	writePEM(t, f.ClientCertFile, "CERTIFICATE", clientDER)
	writeKey(t, f.ClientKeyFile, clientKey)
	writeCombined(t, f.ClientPEMFile, clientDER, clientKey)

	return f
}

// signLeaf issues a certificate under the CA. A server certificate carries the
// loopback names a test lab is reachable by; a client certificate carries none.
func signLeaf(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, commonName string, server bool) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key for %s: %v", commonName, err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber(t),
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"dumbodb-parity-testing"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("signing %s: %v", commonName, err)
	}
	return der, key
}

func serialNumber(t *testing.T) *big.Int {
	t.Helper()
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		t.Fatalf("generating serial number: %v", err)
	}
	return serial
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func writeKey(t *testing.T, path string, key *rsa.PrivateKey) {
	t.Helper()
	writePEM(t, path, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
}

// writeCombined writes the certificate followed by its key, which is what
// mongod's --tlsCertificateKeyFile expects.
//
// The file is 0600: mongod refuses a key file that is readable by anyone else.
func writeCombined(t *testing.T, path string, der []byte, key *rsa.PrivateKey) {
	t.Helper()
	body := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})...,
	)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// ExpiredPEM returns a combined certificate and key whose validity window
// closed yesterday, signed by the fixture's CA so that expiry is the only
// thing wrong with it.
func ExpiredPEM(t *testing.T, f *TLSFixture) string {
	t.Helper()
	path := filepath.Join(f.Dir, "expired.pem")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	caCert, caKey := f.ca(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("ExpiredPEM: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber(t),
		Subject:      pkix.Name{CommonName: "expired.localhost"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ExpiredPEM: signing: %v", err)
	}
	writeCombined(t, path, der, key)
	return path
}

// MismatchedPEM returns a combined file holding the fixture's real server
// certificate beside a private key that does not belong to it. Both halves
// parse; only the pairing is wrong.
func MismatchedPEM(t *testing.T, f *TLSFixture) string {
	t.Helper()
	path := filepath.Join(f.Dir, "mismatched.pem")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	certPEM, err := os.ReadFile(f.ServerCertFile)
	if err != nil {
		t.Fatalf("MismatchedPEM: %v", err)
	}
	strangerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("MismatchedPEM: %v", err)
	}
	body := append(certPEM,
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(strangerKey)})...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("MismatchedPEM: %v", err)
	}
	return path
}

// ca reloads the fixture's authority so derived material can be signed by it.
func (f *TLSFixture) ca(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	certPEM, err := os.ReadFile(f.CAFile)
	if err != nil {
		t.Fatalf("reading CA certificate: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("the CA certificate is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing CA certificate: %v", err)
	}
	keyPEM, err := os.ReadFile(f.caKeyFile())
	if err != nil {
		t.Fatalf("reading CA key: %v", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("the CA key is not valid PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("parsing CA key: %v", err)
	}
	return cert, key
}

func (f *TLSFixture) caKeyFile() string { return filepath.Join(f.Dir, "ca.key") }

// RevocationListFor writes a CRL, signed by the fixture's CA, revoking the
// certificates in the given PEM files.
//
// Revoking the fixture's own client certificate is the case worth testing:
// a server with no revocation support at all serves it happily, so a test
// that only checks an unrevoked certificate passes against a server that
// cannot revoke anything.
func RevocationListFor(t *testing.T, f *TLSFixture, revokedCertFiles ...string) string {
	t.Helper()
	caCert, caKey := f.ca(t)

	entries := make([]x509.RevocationListEntry, 0, len(revokedCertFiles))
	for _, path := range revokedCertFiles {
		certPEM, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("RevocationListFor: reading %s: %v", path, err)
		}
		block, _ := pem.Decode(certPEM)
		if block == nil {
			t.Fatalf("RevocationListFor: %s is not valid PEM", path)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("RevocationListFor: parsing %s: %v", path, err)
		}
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   cert.SerialNumber,
			RevocationTime: time.Now().Add(-time.Hour),
		})
	}

	template := &x509.RevocationList{
		Number:                    serialNumber(t),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                time.Now().Add(24 * time.Hour),
		RevokedCertificateEntries: entries,
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, caCert, caKey)
	if err != nil {
		t.Fatalf("RevocationListFor: %v", err)
	}
	path := filepath.Join(f.Dir, fmt.Sprintf("crl-%d.pem", len(revokedCertFiles)))
	writePEM(t, path, "X509 CRL", der)
	return path
}

// ExpiredRevocationList writes a CRL whose own NextUpdate is in the past.
//
// A stale revocation list is its own question: a server may treat it as
// unusable and refuse, or keep honouring it, and those have opposite security
// consequences. mongod's answer is the one to match.
func ExpiredRevocationList(t *testing.T, f *TLSFixture) string {
	t.Helper()
	caCert, caKey := f.ca(t)
	template := &x509.RevocationList{
		Number:     serialNumber(t),
		ThisUpdate: time.Now().Add(-48 * time.Hour),
		NextUpdate: time.Now().Add(-24 * time.Hour),
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, caCert, caKey)
	if err != nil {
		t.Fatalf("ExpiredRevocationList: %v", err)
	}
	path := filepath.Join(f.Dir, "crl-expired.pem")
	writePEM(t, path, "X509 CRL", der)
	return path
}
