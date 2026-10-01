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

// Certificate policy against mechanism, workspace-61n.7 group 2b.
//
// Seven meaningful cells across server certificate policy, whether a
// certificate is presented, and which mechanism is attempted. The one that
// matters is --tlsAllowConnectionsWithoutCertificates WITH a certificate
// presented: that mode sets ClientAuth to VerifyClientCertIfGiven rather than
// RequireAndVerifyClientCert, and an implementation that reads the peer
// identity only on the Require path works perfectly by default and
// authenticates nobody once optional certificates are turned on. That is a
// flag operators enable for migrations.

package tls

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestX509Matrix_CertificatePolicyAgainstMechanism(t *testing.T) {
	for _, policy := range []struct {
		name         string
		allowWithout bool
	}{
		{"certificates required", false},
		{"certificates optional", true},
	} {
		t.Run(policy.name, func(t *testing.T) {
			ctx := tlsContext(t)
			f := harness.NewTLSFixture(t)
			certFile, dn := harness.ClientPEMWithSubjectString(t, f,
				"matrix-"+sanitize(policy.name)+".pem", "/CN=matrix-user/O=Example")

			opts := harness.TLSOptions{Auth: true, AllowConnectionsWithoutCertificates: policy.allowWithout}
			mongod := harness.StartTLSMongod(t, f, opts)
			if mongod.StartFailed {
				t.Fatalf("premise failed: mongod would not start:\n%s", mongod.FailureOutput)
			}
			dumbodb := harness.StartTLSDumboDB(t, f, opts)
			if dumbodb.StartFailed {
				t.Fatalf("dumbodb would not start where mongod does: %s", firstLine(dumbodb.FailureOutput))
			}
			for _, s := range []*harness.TLSServer{mongod, dumbodb} {
				bootstrapRoot(ctx, t, s)
				if err := createUserAs(ctx, t, s, dn); err != nil {
					t.Logf("note: %s would not create %q: %v", s.Addr, dn, err)
				}
				createSCRAMUser(ctx, t, s)
			}

			for _, s := range []struct {
				name   string
				server *harness.TLSServer
				oracle bool
			}{{"mongod", mongod, true}, {"dumbodb", dumbodb, false}} {
				t.Run(s.name, func(t *testing.T) {
					// With a certificate: both mechanisms must work.
					t.Run("certificate presented, X509", func(t *testing.T) {
						cli, err := s.server.ConnectX509(ctx, t, certFile)
						if err != nil {
							t.Fatalf("%s refused X.509 for a certificate naming an existing user: %v", s.name, err)
						}
						got := authenticatedAs(ctx, t, cli)
						_ = cli.Disconnect(context.Background())
						if got != dn {
							t.Errorf("%s authenticated as %q, expected %q", s.name, got, dn)
						}
					})
					t.Run("certificate presented, SCRAM", func(t *testing.T) {
						// The certificate is transport here and must play no
						// part in the identity.
						cli, err := s.server.ConnectAsUser(ctx, t, "scramuser", "scrampass")
						if err != nil {
							t.Fatalf("%s refused SCRAM from a client that also presented a certificate: %v", s.name, err)
						}
						got := authenticatedAs(ctx, t, cli)
						_ = cli.Disconnect(context.Background())
						if got != "scramuser" {
							t.Errorf("%s authenticated as %q where the password names scramuser; the certificate leaked into the identity",
								s.name, got)
						}
					})

					if !policy.allowWithout {
						// Without a certificate the connection never reaches
						// authentication at all.
						t.Run("no certificate, connection refused", func(t *testing.T) {
							cli, err := s.server.Connect(ctx, t, false)
							if err == nil {
								_ = cli.Disconnect(context.Background())
								t.Errorf("%s accepted a connection with no client certificate while requiring one", s.name)
							}
						})
						return
					}

					t.Run("no certificate, X509 must fail", func(t *testing.T) {
						cli, err := s.server.ConnectX509(ctx, t, "")
						if err == nil {
							_ = cli.Disconnect(context.Background())
							t.Errorf("%s authenticated an X.509 login from a client that presented no certificate", s.name)
							return
						}
						t.Logf("%s refused: %v", s.name, err)
					})
					t.Run("no certificate, SCRAM works", func(t *testing.T) {
						cli, err := s.server.ConnectAsUserWithout(ctx, t, "scramuser", "scrampass")
						if err != nil {
							t.Errorf("%s refused SCRAM from a certificate-free client, which is the entire point of allowing them: %v",
								s.name, err)
							return
						}
						_ = cli.Disconnect(context.Background())
					})
				})
			}
		})
	}
}

func bootstrapRoot(ctx context.Context, t *testing.T, s *harness.TLSServer) {
	t.Helper()
	cli, err := s.Connect(ctx, t, true)
	if err != nil {
		t.Fatalf("connecting to bootstrap root on %s: %v", s.Addr, err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()
	if err := cli.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: "root"}, {Key: "pwd", Value: "root"},
		{Key: "roles", Value: bson.A{"root"}},
	}).Err(); err != nil {
		t.Fatalf("bootstrapping root on %s: %v", s.Addr, err)
	}
}

func createSCRAMUser(ctx context.Context, t *testing.T, s *harness.TLSServer) {
	t.Helper()
	admin, err := s.ConnectAsUser(ctx, t, "root", "root")
	if err != nil {
		t.Fatalf("connecting as root on %s: %v", s.Addr, err)
	}
	defer func() { _ = admin.Disconnect(context.Background()) }()
	if err := admin.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: "scramuser"}, {Key: "pwd", Value: "scrampass"},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: "shop"}}}},
	}).Err(); err != nil {
		t.Fatalf("creating the SCRAM user on %s: %v", s.Addr, err)
	}
}
