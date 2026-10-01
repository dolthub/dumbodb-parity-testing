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

// X.509 client certificate authentication, workspace-61n.
//
// Written before the feature exists. Each case establishes mongod's behavior
// as a hard premise, then grades DumboDB XFail against workspace-61n.2 so the
// implementation landing trips promotion rather than passing quietly.
//
// The authentication source is $external throughout and is not a choice: the
// drivers specification fixes it for this mechanism and rejects any other
// value before contacting a server. Where DumboDB STORES these users is a
// separate decision and is not what these tests pin.

package tls

import (
	"context"
	"crypto/x509/pkix"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// authedPair starts both servers with --auth and TLS and bootstraps a root
// user on each through the localhost exception.
func authedPair(t *testing.T, f *harness.TLSFixture) (mongod, dumbodb *harness.TLSServer) {
	t.Helper()
	opts := harness.TLSOptions{Auth: true}
	mongod = harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with --auth and TLS:\n%s", mongod.FailureOutput)
	}
	dumbodb = harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start with --auth and TLS, which mongod accepts: %s",
			firstLine(dumbodb.FailureOutput))
	}
	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		ctx := tlsContext(t)
		cli, err := s.server.Connect(ctx, t, true)
		if err != nil {
			t.Fatalf("%s: connecting to bootstrap root: %v", s.name, err)
		}
		err = cli.Database("admin").RunCommand(ctx, bson.D{
			{Key: "createUser", Value: "root"},
			{Key: "pwd", Value: "root"},
			{Key: "roles", Value: bson.A{"root"}},
		}).Err()
		_ = cli.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("%s: bootstrapping root through the localhost exception: %v", s.name, err)
		}
	}
	return mongod, dumbodb
}

// createX509User adds a user named by a certificate subject, and reports
// whether the server accepted it. MongoDB creates these against $external;
// DumboDB's storage location is undecided, so both are tried and the one that
// worked is reported.
func createX509User(ctx context.Context, t *testing.T, s *harness.TLSServer, dn string) (string, error) {
	t.Helper()
	admin, err := s.ConnectAsUser(ctx, t, "root", "root")
	if err != nil {
		return "", err
	}
	defer func() { _ = admin.Disconnect(context.Background()) }()

	cmd := bson.D{
		{Key: "createUser", Value: dn},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: "shop"}}}},
	}
	var lastErr error
	for _, db := range []string{"$external", "admin"} {
		if err := admin.Database(db).RunCommand(ctx, cmd).Err(); err == nil {
			return db, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}

// The name MongoDB gives a certificate is the RFC 2253 rendering of its
// subject, and attribute order is part of that name. A multi-attribute
// subject is the only way to see it: Go's pkix.Name.String() reverses the
// order relative to the certificate's own encoding, so an implementation that
// formats the name itself can produce a string that looks right and matches
// nothing.
func TestX509_SubjectNameMatchesOpenSSL(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithSubject(t, f, "x509-multi.pem", pkix.Name{
		CommonName:         "dumbo-client",
		OrganizationalUnit: []string{"engineering"},
		Organization:       []string{"Example"},
	})
	t.Logf("certificate subject, RFC 2253: %q", dn)

	mongod, dumbodb := authedPair(t, f)

	where, err := createX509User(ctx, t, mongod, dn)
	if err != nil {
		t.Fatalf("premise failed: mongod would not create a user named %q: %v", dn, err)
	}
	t.Logf("mongod stores certificate users in %s", where)

	cli, err := mongod.ConnectX509(ctx, t, certFile)
	if err != nil {
		t.Fatalf("premise failed: mongod refused a certificate whose subject names an existing user (%q): %v", dn, err)
	}
	if got := authenticatedAs(ctx, t, cli); got != dn {
		t.Errorf("premise failed: mongod authenticated %q as %q; the oracle for the name format is not what this test assumed", dn, got)
	}
	_ = cli.Disconnect(context.Background())

	if _, err := createX509User(ctx, t, dumbodb, dn); err != nil {
		t.Fatalf("dumbodb would not create a certificate-named user: %v", err)
	}
	served, err := dumbodb.ConnectX509(ctx, t, certFile)
	if err != nil {
		t.Fatalf("dumbodb refused MONGODB-X509 for a certificate naming an existing user: %v", err)
	}
	got := authenticatedAs(ctx, t, served)
	_ = served.Disconnect(context.Background())
	if got != dn {
		t.Errorf("dumbodb authenticated the certificate as %q where mongod says %q; the subject is rendered differently and users will not match",
			got, dn)
	}
}

// THE CASE THAT MATTERS. A client must be authenticated as the certificate it
// presented, never as a name it asked for. A test that only checks the happy
// path passes against a server that trusts the client's claim.
func TestX509_IdentityComesFromTheCertificateNotTheClient(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	holderCert, holderDN := harness.ClientPEMWithSubject(t, f, "x509-holder.pem",
		pkix.Name{CommonName: "low-privilege", Organization: []string{"Example"}})
	_, victimDN := harness.ClientPEMWithSubject(t, f, "x509-victim.pem",
		pkix.Name{CommonName: "high-privilege", Organization: []string{"Example"}})

	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			// Both users exist, so the only thing standing between the holder
			// and the victim's identity is the server checking the certificate.
			for _, dn := range []string{holderDN, victimDN} {
				where, err := createX509User(ctx, t, s.server, dn)
				if err != nil {
					t.Fatalf("%s would not create %q: %v", s.name, dn, err)
				}
				t.Logf("%s accepted a user named %q in %s", s.name, dn, where)
			}

			cli, err := s.server.ConnectX509Claiming(ctx, t, holderCert, victimDN)
			if err != nil {
				t.Logf("%s refused the impersonation outright: %v", s.name, err)
				return
			}
			got := authenticatedAs(ctx, t, cli)
			_ = cli.Disconnect(context.Background())
			if got == victimDN {
				t.Errorf("%s authenticated a client holding %q as %q because the client asked for it; the certificate is not the identity",
					s.name, holderDN, victimDN)
				return
			}
			if got != holderDN {
				t.Errorf("%s authenticated a client holding %q as %q, which is neither the certificate nor the claim", s.name, holderDN, got)
				return
			}
			t.Logf("%s ignored the claim of %q and authenticated the certificate holder %q", s.name, victimDN, got)
		})
	}
}

// X.509 with nothing to identify. Both are configuration errors on a path the
// operator believed was set up, so the refusal should say which.
func TestX509_RefusedWithoutACertificate(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	mongod := harness.StartTLSMongod(t, f, harness.TLSOptions{Auth: true, AllowConnectionsWithoutCertificates: true})
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start:\n%s", mongod.FailureOutput)
	}
	dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{Auth: true, AllowConnectionsWithoutCertificates: true})
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start: %s", firstLine(dumbodb.FailureOutput))
	}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			cli, err := s.server.ConnectX509(ctx, t, "")
			if err == nil {
				_ = cli.Disconnect(context.Background())
				t.Errorf("%s authenticated an X.509 login from a client that presented no certificate", s.name)
				return
			}
			t.Logf("%s refused: %v", s.name, err)
		})
	}
}

// A certificate nobody made a user for is an ordinary unknown user and should
// look like one.
func TestX509_UnknownSubjectIsRefused(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithSubject(t, f, "x509-stranger.pem",
		pkix.Name{CommonName: "nobody-made-this-user", Organization: []string{"Example"}})
	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			cli, err := s.server.ConnectX509(ctx, t, certFile)
			if err == nil {
				_ = cli.Disconnect(context.Background())
				t.Errorf("%s authenticated %q, for which no user was ever created", s.name, dn)
				return
			}
			t.Logf("%s refused an unknown subject: %v", s.name, err)
		})
	}
}

func authenticatedAs(ctx context.Context, t *testing.T, cli *mongo.Client) string {
	t.Helper()
	var res struct {
		AuthInfo struct {
			AuthenticatedUsers []struct {
				User string `bson:"user"`
				DB   string `bson:"db"`
			} `bson:"authenticatedUsers"`
		} `bson:"authInfo"`
	}
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}}).Decode(&res); err != nil {
		t.Fatalf("connectionStatus: %v", err)
	}
	if len(res.AuthInfo.AuthenticatedUsers) == 0 {
		return ""
	}
	return res.AuthInfo.AuthenticatedUsers[0].User
}

// Roles on a certificate-identified user. Covered separately from the SCRAM
// cross-database cases in tests/auth_crossdb_self_test.go, because for an
// $external user EVERY role is cross-database: there is no natural "own"
// database to fall back on. An implementation that resolves roles relative to
// the user's authentication database passes every SCRAM test and gives an
// X.509 user nothing at all.
func TestX509_RolesApplyToTheDatabasesTheyName(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	certFile, dn := harness.ClientPEMWithSubject(t, f, "x509-roles.pem",
		pkix.Name{CommonName: "role-holder", Organization: []string{"Example"}})
	const granted, ungranted = "granteddb", "ungranteddb"

	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			admin, err := s.server.ConnectAsUser(ctx, t, "root", "root")
			if err != nil {
				t.Fatalf("%s: connecting as root: %v", s.name, err)
			}
			err = admin.Database("$external").RunCommand(ctx, bson.D{
				{Key: "createUser", Value: dn},
				{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: granted}}}},
			}).Err()
			_ = admin.Disconnect(context.Background())
			if err != nil {
				t.Fatalf("%s would not create %q: %v", s.name, dn, err)
			}

			cli, err := s.server.ConnectX509(ctx, t, certFile)
			if err != nil {
				t.Fatalf("%s refused a certificate naming an existing user: %v", s.name, err)
			}
			defer func() { _ = cli.Disconnect(context.Background()) }()

			_, grantedErr := cli.Database(granted).Collection("c").InsertOne(ctx, bson.D{{Key: "v", Value: 1}})
			_, ungrantedErr := cli.Database(ungranted).Collection("c").InsertOne(ctx, bson.D{{Key: "v", Value: 1}})
			t.Logf("%s: write to the granted database err=%v; write to the ungranted one err=%v",
				s.name, grantedErr, ungrantedErr)

			if grantedErr != nil {
				t.Errorf("%s denied a write to %q, which the user holds readWrite on; roles granted to a certificate user are not being applied",
					s.name, granted)
			}
			if ungrantedErr == nil {
				t.Errorf("%s allowed a write to %q, which the user holds no role on; a certificate user is reaching past its roles",
					s.name, ungranted)
			}
		})
	}
}

// The role shorthand cannot work for a certificate user. "readWrite" means
// readWrite on the user's own authentication database, and that database is
// $external, where no role is defined. mongod refuses by name; a server that
// accepted it would store a role that grants nothing and say nothing.
func TestX509_RoleShorthandIsRefused(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			admin, err := s.server.ConnectAsUser(ctx, t, "root", "root")
			if err != nil {
				t.Fatalf("%s: connecting as root: %v", s.name, err)
			}
			defer func() { _ = admin.Disconnect(context.Background()) }()

			err = admin.Database("$external").RunCommand(ctx, bson.D{
				{Key: "createUser", Value: "CN=shorthand"},
				{Key: "roles", Value: bson.A{"readWrite"}},
			}).Err()
			t.Logf("%s: createUser with a shorthand role: %v", s.name, err)

			if err == nil {
				t.Errorf("%s accepted a shorthand role for an $external user; it resolves to readWrite@$external, which is not a role, so the grant can never apply",
					s.name)
				return
			}
			if !strings.Contains(err.Error(), "$external") {
				t.Errorf("%s refused the shorthand without naming $external, so the message does not say why it cannot work: %v",
					s.name, err)
			}
		})
	}
}
