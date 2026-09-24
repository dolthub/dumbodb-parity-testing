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

// Certificate revocation, workspace-09n.8 and workspace-09n.10.
//
// Mutual TLS without revocation means a compromised client certificate stays
// valid until it expires. The case that matters here is the revoked
// certificate being SERVED: a server with no revocation support at all passes
// every other check in this file.

package tls

import (
	"context"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestTLSRevocation_RevokedClientCertificateIsRefused(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	crl := harness.RevocationListFor(t, f, f.ClientCertFile)
	opts := harness.TLSOptions{CRLFile: crl}

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with a CRL, so there is no oracle:\n%s", mongod.FailureOutput)
	}
	client, err := mongod.Connect(ctx, t, true)
	if err == nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("premise failed: mongod served a certificate listed in its own CRL, so this test cannot tell revocation from its absence")
	}

	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start with --tlsCRLFile, which mongod accepts: %s", firstLine(dumbodb.FailureOutput))
	}
	served, err := dumbodb.Connect(ctx, t, true)
	if err == nil {
		_ = served.Disconnect(context.Background())
		t.Error("dumbodb served a client certificate listed in the CRL, so a compromised certificate stays valid until it expires")
	}
}

// Revocation must not become a blanket refusal. A certificate absent from the
// list is still good, and a server that rejected everything would satisfy the
// case above while being useless.
func TestTLSRevocation_UnrevokedCertificateStillWorks(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	// A CRL revoking the SERVER certificate, which no client presents, so the
	// client certificate is legitimately absent from the list.
	crl := harness.RevocationListFor(t, f, f.ServerCertFile)
	opts := harness.TLSOptions{CRLFile: crl}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start with a CRL:\n%s", s.name, s.server.FailureOutput)
			}
			client, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Errorf("%s refused a client certificate that is not in the revocation list: %v", s.name, err)
				return
			}
			_ = client.Disconnect(context.Background())
		})
	}
}

// A revocation list that has itself expired is a question with opposite
// security consequences either way: treat it as unusable and refuse every
// client, or keep honouring a list nobody has refreshed. Whatever mongod does
// is the answer DumboDB has to give.
//
// Startup is the lesser half. Both servers may well come up, and what then
// happens to a client carrying a perfectly good certificate is the part an
// operator finds out about at the worst moment.
func TestTLSRevocation_ExpiredRevocationList(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{CRLFile: harness.ExpiredRevocationList(t, f)}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)

	t.Logf("expired CRL at startup: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Fatalf("the servers disagree about starting with an expired revocation list: mongod startFailed=%v, dumbodb startFailed=%v.\nmongod said: %s\ndumbodb said: %s",
			mongod.StartFailed, dumbodb.StartFailed, firstLine(mongod.FailureOutput), firstLine(dumbodb.FailureOutput))
	}
	if mongod.StartFailed {
		return // both refused; there is nothing left to connect to
	}

	mongodServed := connectSucceeds(ctx, t, mongod)
	dumbodbServed := connectSucceeds(ctx, t, dumbodb)
	t.Logf("expired CRL, unrevoked client certificate: mongod served=%v, dumbodb served=%v", mongodServed, dumbodbServed)
	if mongodServed != dumbodbServed {
		t.Errorf("mongod served=%v but dumbodb served=%v for a client whose certificate is not revoked, with a CRL that has expired; "+
			"one of these fails every client on a list nobody refreshed and the other ignores the expiry",
			mongodServed, dumbodbServed)
	}
}

// Revocation with nothing to verify against. --tlsCRLFile only means anything
// alongside --tlsCAFile, and a server that accepts the pair without one is
// quietly enforcing nothing.
func TestTLSRevocation_WithoutACA(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{
		CRLFile:  harness.RevocationListFor(t, f, f.ClientCertFile),
		NoCAFile: true,
	}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)

	t.Logf("--tlsCRLFile without --tlsCAFile: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v for a revocation list with no CA to verify against.\nmongod said: %s\ndumbodb said: %s",
			mongod.StartFailed, dumbodb.StartFailed, firstLine(mongod.FailureOutput), firstLine(dumbodb.FailureOutput))
	}
}

// --tlsCRLFile pointed at the wrong PEM. Operators do this, and the failure is
// silent in the worst case: revocation configured, nothing enforced.
func TestTLSRevocation_FileIsNotARevocationList(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{CRLFile: harness.NotARevocationList(t, f)}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)

	t.Logf("--tlsCRLFile naming a certificate: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v when --tlsCRLFile names a file that is not a revocation list.\nmongod said: %s\ndumbodb said: %s",
			mongod.StartFailed, dumbodb.StartFailed, firstLine(mongod.FailureOutput), firstLine(dumbodb.FailureOutput))
	}
}

func connectSucceeds(ctx context.Context, t *testing.T, s *harness.TLSServer) bool {
	t.Helper()
	client, err := s.Connect(ctx, t, true)
	if err != nil {
		return false
	}
	_ = client.Disconnect(context.Background())
	return true
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	if len(s) > 160 {
		return s[:160]
	}
	return s
}
