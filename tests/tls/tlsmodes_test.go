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

// tlsMode, workspace-09n.4.
//
// allowTLS and preferTLS serve plaintext and TLS on one port, which is what
// workspace-lkd was filed as a P1 for doing under requireTLS. The mode that
// must NOT do it is therefore tested here beside the modes that must, so the
// two can never drift apart unnoticed.

package tls

import (
	"context"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestTLSModes_PlaintextAcceptedOnlyWhereTheModeSaysSo(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)

	for _, mode := range []string{"requireTLS", "allowTLS", "preferTLS"} {
		t.Run(mode, func(t *testing.T) {
			opts := harness.TLSOptions{Mode: mode}
			mongod := harness.StartTLSMongod(t, f, opts)
			if mongod.StartFailed {
				t.Fatalf("premise failed: mongod would not start with --tlsMode %s:\n%s", mode, mongod.FailureOutput)
			}
			dumbodb := harness.StartTLSDumboDB(t, f, opts)
			if dumbodb.StartFailed {
				t.Fatalf("dumbodb would not start with --tlsMode %s, which mongod accepts: %s",
					mode, firstLine(dumbodb.FailureOutput))
			}

			mongodPlaintext := mongod.ConnectPlaintext(ctx, t) == nil
			dumbodbPlaintext := dumbodb.ConnectPlaintext(ctx, t) == nil
			t.Logf("--tlsMode %s: plaintext accepted by mongod=%v, dumbodb=%v", mode, mongodPlaintext, dumbodbPlaintext)

			if mode == "requireTLS" && dumbodbPlaintext {
				t.Error("dumbodb served a plaintext client under requireTLS; this is workspace-lkd returning")
			}
			if mongodPlaintext != dumbodbPlaintext {
				t.Errorf("--tlsMode %s: mongod accepted plaintext=%v but dumbodb accepted=%v",
					mode, mongodPlaintext, dumbodbPlaintext)
			}

			// Whatever the mode does about plaintext, TLS has to keep working.
			for _, s := range []struct {
				name   string
				server *harness.TLSServer
			}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
				client, err := s.server.Connect(ctx, t, true)
				if err != nil {
					t.Errorf("%s refused a TLS client under --tlsMode %s: %v", s.name, mode, err)
					continue
				}
				_ = client.Disconnect(context.Background())
			}
		})
	}
}

// A plaintext client and a TLS client against the same running server, so the
// routing is shown to work per connection rather than being fixed at the
// first one.
func TestTLSModes_AllowTLSServesBothOnOneListener(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{Mode: "allowTLS"}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start with --tlsMode allowTLS: %s", s.name, firstLine(s.server.FailureOutput))
			}
			if err := s.server.ConnectPlaintext(ctx, t); err != nil {
				t.Errorf("%s refused a plaintext client under allowTLS: %v", s.name, err)
			}
			client, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Errorf("%s refused a TLS client under allowTLS: %v", s.name, err)
				return
			}
			_ = client.Disconnect(context.Background())
			// Again, after the TLS connection, to show one does not poison
			// the listener for the other.
			if err := s.server.ConnectPlaintext(ctx, t); err != nil {
				t.Errorf("%s refused a plaintext client after serving a TLS one: %v", s.name, err)
			}
		})
	}
}
