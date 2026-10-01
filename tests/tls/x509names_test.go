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

// X.509 subject rendering, workspace-61n.7 group 1.
//
// Identity here is a string comparison, so the rendering rules ARE the
// feature. Two independent traps live in one certificate: values containing
// commas are escaped and the escape is part of the user name, and the RDN
// order of the rendered string is not the order the subject was written in.
// An implementation that formats the name from parsed fields produces
// something that looks right and matches nothing.

package tls

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestX509Name_RenderingRoundTrips(t *testing.T) {
	f := harness.NewTLSFixture(t)

	cases := []struct {
		name string
		subj string
	}{
		{"comma in values", "/CN=Smith, Alice/O=Example, Inc."},
		{"equals in a value", "/CN=a=b/O=Example"},
		{"plus in a value", `/CN=a\+b/O=Example`},
		{"leading and trailing spaces", "/CN= padded /O=Example"},
		{"non-ascii", "/CN=Zoë Müller/O=Example"},
		{"four attributes, order fixed", "/C=US/O=Example/OU=engineering/CN=ordered"},
		{"only an organization", "/O=Example"},
	}

	mongod, dumbodb := authedPair(t, f)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := tlsContext(t)
			certFile, dn := harness.ClientPEMWithSubjectString(t, f, "name-"+sanitize(c.name)+".pem", c.subj)
			t.Logf("subj %q renders RFC 2253 as %q", c.subj, dn)

			// mongod is the oracle for both the name and for it working.
			if err := createUserAs(ctx, t, mongod, dn); err != nil {
				t.Fatalf("premise failed: mongod would not create a user named %q: %v", dn, err)
			}
			cli, err := mongod.ConnectX509(ctx, t, certFile)
			if err != nil {
				t.Fatalf("premise failed: mongod refused a certificate whose subject names an existing user %q: %v", dn, err)
			}
			got := authenticatedAs(ctx, t, cli)
			_ = cli.Disconnect(context.Background())
			if got != dn {
				t.Fatalf("premise failed: mongod authenticated %q as %q; openssl's rendering is not the name mongod uses", dn, got)
			}

			if err := createUserAs(ctx, t, dumbodb, dn); err != nil {
				t.Logf("XFAIL %s: dumbodb would not create %q: %v", x509Bead, dn, err)
				return
			}
			served, err := dumbodb.ConnectX509(ctx, t, certFile)
			if err != nil {
				if mechanismMissing(err) {
					t.Logf("XFAIL %s: dumbodb does not implement MONGODB-X509", x509Bead)
					return
				}
				t.Errorf("dumbodb refused a certificate naming the user %q that mongod accepts: %v", dn, err)
				return
			}
			dumboGot := authenticatedAs(ctx, t, served)
			_ = served.Disconnect(context.Background())
			if dumboGot != dn {
				t.Errorf("dumbodb rendered the subject as %q where mongod says %q; users created from one will not match the other",
					dumboGot, dn)
				return
			}
			t.Errorf("XPASS %s: dumbodb agrees on %q; remove the exemption", x509Bead, dn)
		})
	}
}

// The RDN order of the rendered name is not the order the subject was given
// in, and that difference is the whole trap. Recorded as a standalone fact so
// an implementation has something to compare against without running the
// whole suite.
func TestX509Name_RDNOrderIsReversed(t *testing.T) {
	f := harness.NewTLSFixture(t)
	_, forward := harness.ClientPEMWithSubjectString(t, f, "order-forward.pem", "/CN=leaf/OU=unit/O=org")
	_, reverse := harness.ClientPEMWithSubjectString(t, f, "order-reverse.pem", "/O=org/OU=unit/CN=leaf")

	t.Logf("subj /CN=leaf/OU=unit/O=org renders as %q", forward)
	t.Logf("subj /O=org/OU=unit/CN=leaf renders as %q", reverse)

	if forward == reverse {
		t.Errorf("two subjects written in opposite orders rendered identically as %q; "+
			"this test's premise, that RDN order is preserved and merely reversed, is wrong", forward)
	}
	if forward != "O=org,OU=unit,CN=leaf" {
		t.Errorf("RFC 2253 rendering of /CN=leaf/OU=unit/O=org is %q, not the reversal this suite assumes; "+
			"every name-matching expectation here should be rechecked", forward)
	}
}

func createUserAs(ctx context.Context, t *testing.T, s *harness.TLSServer, dn string) error {
	t.Helper()
	admin, err := s.ConnectAsUser(ctx, t, "root", "root")
	if err != nil {
		return err
	}
	defer func() { _ = admin.Disconnect(context.Background()) }()
	return admin.Database("$external").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: dn},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: "shop"}}}},
	}).Err()
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
