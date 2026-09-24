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

// TLS material that is wrong in ways the first three cases do not reach,
// workspace-09n.12.
//
// Every case asks the same question: does the server refuse at startup, or
// start and fail every client afterwards. The second is worse than it looks,
// because a process that is up with an open port looks healthy to anything
// watching it, and the symptom then appears on the clients instead.

package tls

import (
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestTLSFailureModes_StartupAgreesWithMongod(t *testing.T) {
	f := harness.NewTLSFixture(t)
	// A second fixture is a whole unrelated trust root, which is how a CA
	// comes to name a certificate that did not sign the server's.
	stranger := harness.NewTLSFixture(t)

	cases := []struct {
		name string
		opts harness.TLSOptions
		// bead names a tracked divergence; empty means the servers must agree.
		bead string
	}{
		{
			name: "certificate not yet valid",
			opts: harness.TLSOptions{CertificateKeyFile: harness.NotYetValidPEM(t, f)},
		},
		{
			name: "CA file is not a certificate",
			opts: harness.TLSOptions{CAFile: harness.GarbageCAFile(t, f)},
		},
		{
			name: "key file is world readable",
			opts: harness.TLSOptions{CertificateKeyFile: harness.WorldReadableServerPEM(t, f)},
		},
		{
			name: "CA did not sign the server certificate",
			opts: harness.TLSOptions{CAFile: stranger.CAFile},
		},
		{
			name: "certificate names a different host",
			opts: harness.TLSOptions{CertificateKeyFile: harness.WrongHostPEM(t, f)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mongod := harness.StartTLSMongod(t, f, c.opts)
			dumbodb := harness.StartTLSDumboDB(t, f, c.opts)

			t.Logf("mongod startFailed=%v, dumbodb startFailed=%v", mongod.StartFailed, dumbodb.StartFailed)

			if mongod.StartFailed == dumbodb.StartFailed {
				if c.bead != "" {
					t.Errorf("XPASS %s: the servers now agree on %q; remove the exemption", c.bead, c.name)
				}
				return
			}
			if c.bead != "" {
				t.Logf("XFAIL %s: mongod startFailed=%v, dumbodb startFailed=%v for %q",
					c.bead, mongod.StartFailed, dumbodb.StartFailed, c.name)
				return
			}
			if mongod.StartFailed {
				t.Errorf("dumbodb started with %q where mongod refused to; every client will fail instead of the server saying why.\nmongod said: %s",
					c.name, firstLine(mongod.FailureOutput))
				return
			}
			t.Errorf("dumbodb refused to start with %q where mongod accepted it, so a configuration MongoDB supports is rejected here.\ndumbodb said: %s",
				c.name, firstLine(dumbodb.FailureOutput))
		})
	}
}
