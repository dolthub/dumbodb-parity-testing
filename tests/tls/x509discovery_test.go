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

// Where certificate users live, how they are discovered, and the refusals
// that are configuration errors rather than bad credentials.
// workspace-61n.7 groups 2, 3 and 4.

package tls

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Group 2. The storage decision recorded on workspace-61n.3 says certificate
// users are kept the way MongoDB keeps them: in admin.system.users carrying
// db "$external", with no database of that name on disk. All three halves are
// observable, so all three are asserted.
func TestX509Storage_MatchesMongoDB(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	_, dn := harness.ClientPEMWithSubjectString(t, f, "storage.pem", "/CN=storage-user/O=Example")
	mongod, dumbodb := authedPair(t, f)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
		oracle bool
	}{{"mongod", mongod, true}, {"dumbodb", dumbodb, false}} {
		t.Run(s.name, func(t *testing.T) {
			if err := createUserAs(ctx, t, s.server, dn); err != nil {
				t.Fatalf("%s would not create %q: %v", s.name, dn, err)
			}
			admin, err := s.server.ConnectAsUser(ctx, t, "root", "root")
			if err != nil {
				t.Fatalf("%s: connecting as root: %v", s.name, err)
			}
			defer func() { _ = admin.Disconnect(context.Background()) }()

			external := usersIn(ctx, t, admin, "$external")
			inAdmin := usersIn(ctx, t, admin, "admin")
			t.Logf("%s: usersInfo on $external=%v, on admin=%v", s.name, external, inAdmin)

			if !contains(external, dn) {
				t.Errorf("%s: usersInfo against $external does not list %q, so a certificate user cannot be found where MongoDB puts it",
					s.name, dn)
			}
			if contains(inAdmin, dn) {
				t.Errorf("%s: usersInfo against admin lists %q; a certificate user is showing up under the wrong database",
					s.name, dn)
			}

			dbs := databaseNames(ctx, t, admin)
			t.Logf("%s: databases %v", s.name, dbs)
			if contains(dbs, "$external") {
				t.Errorf("%s created a database called $external; it is a value in the db field, not a database",
					s.name)
			}
		})
	}
}

// Group 3. X.509 attempted over a connection carrying no TLS. allowTLS makes
// that reachable without a second kind of server: the port accepts plaintext,
// so the client gets far enough to try authenticating, and there is no
// certificate to identify it with.
//
// A configuration error on a route the operator believed was set up, and it
// must fail rather than fall through to something else.
func TestX509Refusal_OverPlaintext(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{Auth: true, Mode: "allowTLS"}

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with --tlsMode allowTLS and --auth:\n%s", mongod.FailureOutput)
	}
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start where mongod does: %s", firstLine(dumbodb.FailureOutput))
	}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			cli, err := s.server.ConnectX509Plaintext(ctx, t)
			if err == nil {
				_ = cli.Disconnect(context.Background())
				t.Errorf("%s authenticated an X.509 login over a plaintext connection, where no certificate was ever presented", s.name)
				return
			}
			t.Logf("%s refused: %v", s.name, err)
		})
	}
}

// Group 4. MONGODB-X509 IS NOT ADVERTISED, and that is the finding.
//
// MEASURED: mongod 8.0.28 returns an EMPTY saslSupportedMechs for a
// certificate user, while returning the two SCRAM mechanisms for a password
// user. The name of the field is the explanation: X.509 is not a SASL
// mechanism. It is negotiated through the legacy `authenticate` command, as
// workspace-61n.2 records, so it never appears in a list of SASL mechanisms.
//
// The consequence for a client is that there is no discovery: the mechanism
// has to be named explicitly, which is why --authenticationMechanism
// MONGODB-X509 is not optional the way --authenticationDatabase is.
//
// So this pins an ABSENCE. An implementation that helpfully advertised
// MONGODB-X509 here would diverge, and might send a driver down a SASL path
// that does not exist.
func TestX509Discovery_NotAdvertisedAsASASLMechanism(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	_, dn := harness.ClientPEMWithSubjectString(t, f, "discovery.pem", "/CN=discovery-user/O=Example")
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
			cli, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Fatalf("%s: connecting: %v", s.name, err)
			}
			defer func() { _ = cli.Disconnect(context.Background()) }()

			certMechs := saslMechsFor(ctx, t, cli, "$external."+dn)
			scramMechs := saslMechsFor(ctx, t, cli, "admin.root")
			t.Logf("%s: saslSupportedMechs for the certificate user=%v, for admin.root=%v",
				s.name, certMechs, scramMechs)

			if contains(certMechs, "MONGODB-X509") {
				t.Errorf("%s advertises MONGODB-X509 in saslSupportedMechs; mongod does not, because it is not a SASL mechanism",
					s.name)
			}
			// The SCRAM half is the control. Without it this case would pass
			// against a server whose hello ignores saslSupportedMechs
			// entirely.
			if !contains(scramMechs, "SCRAM-SHA-256") {
				t.Errorf("%s does not advertise SCRAM-SHA-256 for a password user, so its saslSupportedMechs says nothing and the absence above proves nothing",
					s.name)
			}
		})
	}
}

func usersIn(ctx context.Context, t *testing.T, cli *mongo.Client, db string) []string {
	t.Helper()
	var res struct {
		Users []struct {
			User string `bson:"user"`
		} `bson:"users"`
	}
	if err := cli.Database(db).RunCommand(ctx, bson.D{{Key: "usersInfo", Value: 1}}).Decode(&res); err != nil {
		t.Logf("usersInfo against %s: %v", db, err)
		return nil
	}
	out := make([]string, 0, len(res.Users))
	for _, u := range res.Users {
		out = append(out, u.User)
	}
	return out
}

func saslMechsFor(ctx context.Context, t *testing.T, cli *mongo.Client, user string) []string {
	t.Helper()
	var res struct {
		SaslSupportedMechs []string `bson:"saslSupportedMechs"`
	}
	if err := cli.Database("admin").RunCommand(ctx, bson.D{
		{Key: "hello", Value: 1},
		{Key: "saslSupportedMechs", Value: user},
	}).Decode(&res); err != nil {
		t.Logf("hello with saslSupportedMechs=%s: %v", user, err)
		return nil
	}
	return res.SaslSupportedMechs
}

func databaseNames(ctx context.Context, t *testing.T, cli *mongo.Client) []string {
	t.Helper()
	names, err := cli.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		t.Logf("listDatabases: %v", err)
		return nil
	}
	return names
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
