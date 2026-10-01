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

// A bad certificate naming a good user, and the properties of certificate
// identity. workspace-61n.7 groups 5 and 5b.
//
// TestX509_UnknownSubjectIsRefused covers the other half of this axis, a good
// certificate naming no user. These are the cases where getting it wrong is
// dangerous rather than merely broken: the TLS layer has to refuse before the
// name is ever looked up.

package tls

import (
	"context"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Group 5. An expired certificate whose subject names a real user. The
// expectation is that the handshake fails and authentication is never
// reached; a server that resolved the name first would authenticate it.
func TestX509CertState_ExpiredCertificateNamingAGoodUser(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ExpiredClientPEM(t, f, "expired-client.pem", "/CN=expired-user/O=Example")
	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
		oracle bool
	}{{"mongod", mongod, true}, {"dumbodb", dumbodb, false}} {
		t.Run(s.name, func(t *testing.T) {
			if err := createUserAs(ctx, t, s.server, dn); err != nil && s.oracle {
				t.Fatalf("premise failed: mongod would not create %q: %v", dn, err)
			}
			cli, err := s.server.ConnectX509(ctx, t, certFile)
			if err == nil {
				_ = cli.Disconnect(context.Background())
				t.Errorf("%s authenticated an EXPIRED certificate whose subject names a real user; the name was resolved before the certificate was checked",
					s.name)
				return
			}
			t.Logf("%s refused: %v", s.name, err)
		})
	}
}

// Group 5. The same shape with revocation, which is the one that matters
// operationally: a compromised certificate is revoked, and the user it names
// is still perfectly valid. Revocation has to win.
func TestX509CertState_RevokedCertificateNamingAGoodUser(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithSubjectString(t, f, "revoked-client.pem", "/CN=revoked-user/O=Example")
	crl := harness.RevocationListFor(t, f, f.Dir+"/revoked-client.pem.crt")

	opts := harness.TLSOptions{Auth: true, CRLFile: crl}
	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with a CRL and --auth:\n%s", mongod.FailureOutput)
	}
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start where mongod does: %s", firstLine(dumbodb.FailureOutput))
	}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
		oracle bool
	}{{"mongod", mongod, true}, {"dumbodb", dumbodb, false}} {
		t.Run(s.name, func(t *testing.T) {
			bootstrapRoot(ctx, t, s.server)
			if err := createUserAs(ctx, t, s.server, dn); err != nil && s.oracle {
				t.Fatalf("premise failed: mongod would not create %q: %v", dn, err)
			}
			cli, err := s.server.ConnectX509(ctx, t, certFile)
			if err == nil {
				_ = cli.Disconnect(context.Background())
				t.Errorf("%s authenticated a REVOKED certificate whose subject names a real user; revocation did not survive the addition of X.509",
					s.name)
				return
			}
			t.Logf("%s refused: %v", s.name, err)
		})
	}
}

// Group 5b. Certificate identity is NAME-based, not key-based. Two
// certificates with one subject are one user.
//
// This is the property behind rotation working at all: reissue a certificate
// with the same subject and the user continues. It is also the hazard behind
// putting a public CA in --tlsCAFile, since anyone who can have that subject
// signed becomes that user.
func TestX509CertState_SameSubjectDifferentKeyIsTheSameUser(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	const subj = "/CN=twins/O=Example"
	firstCert, dn := harness.ClientPEMWithSubjectString(t, f, "twin-a.pem", subj)
	secondCert, secondDN := harness.ClientPEMWithSubjectString(t, f, "twin-b.pem", subj)
	if dn != secondDN {
		t.Fatalf("premise failed: two certificates from the same subject rendered differently, %q and %q", dn, secondDN)
	}
	if sameFile(t, firstCert, secondCert) {
		t.Fatal("premise failed: the two certificates are the same file, so nothing is being compared")
	}
	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
		oracle bool
	}{{"mongod", mongod, true}, {"dumbodb", dumbodb, false}} {
		t.Run(s.name, func(t *testing.T) {
			if err := createUserAs(ctx, t, s.server, dn); err != nil && s.oracle {
				t.Fatalf("premise failed: mongod would not create %q: %v", dn, err)
			}
			for i, cert := range []string{firstCert, secondCert} {
				cli, err := s.server.ConnectX509(ctx, t, cert)
				if err != nil {
					if !s.oracle && mechanismMissing(err) {
						t.Skipf("XFAIL %s: dumbodb does not implement MONGODB-X509", x509Bead)
					}
					t.Fatalf("%s refused certificate %d of two sharing the subject %q: %v", s.name, i+1, dn, err)
				}
				got := authenticatedAs(ctx, t, cli)
				_ = cli.Disconnect(context.Background())
				if got != dn {
					t.Errorf("%s authenticated certificate %d as %q, expected %q", s.name, i+1, got, dn)
				}
			}
			t.Logf("%s: both certificates authenticate as %q; identity is the subject, not the key", s.name, dn)
		})
	}
}

// Group 5b. Whether the clientAuth extended key usage is enforced. Measured
// rather than assumed: this suite had no idea what either server does.
//
// MEASURED: mongod 8.0.28 ACCEPTS a client certificate carrying no extended
// key usage at all. The constraint is not enforced.
func TestX509CertState_WithoutClientAuthExtendedKeyUsage(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithoutClientAuth(t, f, "no-eku.pem", "/CN=no-eku/O=Example")
	mongod, dumbodb := authedPair(t, f)

	if err := createUserAs(ctx, t, mongod, dn); err != nil {
		t.Fatalf("premise failed: mongod would not create %q: %v", dn, err)
	}
	oracleAccepts := x509Accepts(ctx, t, mongod, certFile)
	t.Logf("mongod: certificate with no extended key usage accepted=%v", oracleAccepts)

	if err := createUserAs(ctx, t, dumbodb, dn); err != nil {
		t.Logf("XFAIL %s: dumbodb would not create %q: %v", x509Bead, dn, err)
		return
	}
	cli, err := dumbodb.ConnectX509(ctx, t, certFile)
	if err != nil && mechanismMissing(err) {
		t.Logf("XFAIL %s: dumbodb does not implement MONGODB-X509", x509Bead)
		return
	}
	accepted := err == nil
	if accepted {
		_ = cli.Disconnect(context.Background())
	}
	t.Logf("dumbodb: accepted=%v (err=%v)", accepted, err)
	if accepted != oracleAccepts {
		t.Errorf("mongod accepted=%v but dumbodb accepted=%v for a certificate carrying no clientAuth extended key usage",
			oracleAccepts, accepted)
	}
}

// Group 5b. Revocation latency. Dropping a user does not close the
// connections it already has, but it does stop them working.
//
// MEASURED: mongod 8.0.28 leaves the socket up and answers the next command
// with (Unauthorized) Command find requires authentication. So the
// de-authorization is immediate; what survives is the TCP connection, not the
// session's authority.
func TestX509CertState_DropUserDeauthorizesLiveConnections(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithSubjectString(t, f, "dropped.pem", "/CN=dropped-user/O=Example")
	mongod, dumbodb := authedPair(t, f)

	oracleStillWorks, ok := dropAndRetry(ctx, t, mongod, certFile, dn, true)
	if !ok {
		t.Fatal("premise failed: mongod could not run this case")
	}
	t.Logf("mongod: connection still usable after its user was dropped: %v", oracleStillWorks)

	stillWorks, ok := dropAndRetry(ctx, t, dumbodb, certFile, dn, false)
	if !ok {
		t.Logf("XFAIL %s: dumbodb does not implement MONGODB-X509", x509Bead)
		return
	}
	t.Logf("dumbodb: connection still usable after its user was dropped: %v", stillWorks)
	if stillWorks != oracleStillWorks {
		t.Errorf("mongod kept the connection usable=%v but dumbodb=%v after dropUser; revocation latency differs",
			oracleStillWorks, stillWorks)
	}
}

// x509Accepts reports whether the server authenticated the certificate,
// without failing the test either way.
func x509Accepts(ctx context.Context, t *testing.T, s *harness.TLSServer, certFile string) bool {
	t.Helper()
	cli, err := s.ConnectX509(ctx, t, certFile)
	if err != nil {
		return false
	}
	_ = cli.Disconnect(context.Background())
	return true
}

// dropAndRetry authenticates, drops the user out from under the live
// connection, and reports whether that connection still works. ok is false
// when the server could not get far enough to answer.
func dropAndRetry(ctx context.Context, t *testing.T, s *harness.TLSServer, certFile, dn string, oracle bool) (stillWorks, ok bool) {
	t.Helper()
	if err := createUserAs(ctx, t, s, dn); err != nil {
		if oracle {
			t.Fatalf("premise failed: mongod would not create %q: %v", dn, err)
		}
		return false, false
	}
	cli, err := s.ConnectX509(ctx, t, certFile)
	if err != nil {
		if oracle {
			t.Fatalf("premise failed: mongod refused a valid certificate user: %v", err)
		}
		return false, false
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	if err := cli.Database("shop").Collection("c").FindOne(ctx, bson.D{}).Err(); err != nil && err != mongo.ErrNoDocuments {
		t.Fatalf("the connection was not usable before the drop: %v", err)
	}

	admin, err := s.ConnectAsUser(ctx, t, "root", "root")
	if err != nil {
		t.Fatalf("connecting as root: %v", err)
	}
	dropErr := admin.Database("$external").RunCommand(ctx, bson.D{{Key: "dropUser", Value: dn}}).Err()
	_ = admin.Disconnect(context.Background())
	if dropErr != nil {
		t.Fatalf("dropUser %q: %v", dn, dropErr)
	}

	err = cli.Database("shop").Collection("c").FindOne(ctx, bson.D{}).Err()
	return err == nil || err == mongo.ErrNoDocuments, true
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ab, err := os.ReadFile(a)
	if err != nil {
		t.Fatalf("reading %s: %v", a, err)
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		t.Fatalf("reading %s: %v", b, err)
	}
	return string(ab) == string(bb)
}
