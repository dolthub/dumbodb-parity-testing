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

// Standalone TLS parity. Every case runs the same configuration against mongod
// and DumboDB and requires the same outcome, because mongod is the oracle.
//
// A server refusing a configuration is as much of a result as one accepting
// it: which bad setups are caught at startup, rather than at the first client,
// is most of what separates a usable TLS deployment from a confusing one.

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func tlsContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// A CA without the allow flag is mutual TLS: a client presenting a certificate
// is served and one without is refused.
func TestTLS_CARequiresAClientCertificate(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s refused to start with a valid certificate and CA:\n%s", s.name, s.server.FailureOutput)
			}
			client, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Fatalf("%s rejected a client presenting a valid certificate: %v", s.name, err)
			}
			_ = client.Disconnect(context.Background())

			if _, err := s.server.Connect(ctx, t, false); err == nil {
				t.Errorf("%s served a client with no certificate while a CA was configured and the allow flag was not set", s.name)
			}
		})
	}
}

// The allow flag is the difference between "trust this CA" and "demand a
// certificate from everyone".
func TestTLS_AllowConnectionsWithoutCertificates(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{AllowConnectionsWithoutCertificates: true}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s refused to start:\n%s", s.name, s.server.FailureOutput)
			}
			for _, withCert := range []bool{true, false} {
				client, err := s.server.Connect(ctx, t, withCert)
				if err != nil {
					t.Errorf("%s rejected a client (certificate presented: %v) despite the allow flag: %v", s.name, withCert, err)
					continue
				}
				_ = client.Disconnect(context.Background())
			}
		})
	}
}

// A TLS port that still answers plaintext is protecting nothing. This is the
// regression guard for workspace-lkd, where TLS lived on a second port and the
// main one kept serving unencrypted traffic.
func TestTLS_PlaintextIsRefusedOnTheTLSPort(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, harness.TLSOptions{})},
		{"dumbodb", harness.StartTLSDumboDB(t, f, harness.TLSOptions{})},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s refused to start:\n%s", s.name, s.server.FailureOutput)
			}
			if err := s.server.ConnectPlaintext(ctx, t); err == nil {
				t.Errorf("%s served a plaintext client on a requireTLS port", s.name)
			}
		})
	}
}

// Bad certificate material should be caught when the server starts, not by
// every client afterwards. A server that comes up and then fails every
// handshake looks healthy to anything watching the process or the port.
func TestTLS_BadMaterialIsRejectedAtStartup(t *testing.T) {
	f := harness.NewTLSFixture(t)

	cases := []struct {
		name string
		opts harness.TLSOptions
		// bead names the tracked divergence, empty when both servers agree.
		bead string
	}{
		{
			name: "nonexistent certificate file",
			opts: harness.TLSOptions{CertificateKeyFile: f.Dir + "/does-not-exist.pem"},
		},
		{
			name: "certificate and key do not match",
			opts: harness.TLSOptions{CertificateKeyFile: harness.MismatchedPEM(t, f)},
		},
		{
			name: "expired certificate",
			opts: harness.TLSOptions{CertificateKeyFile: harness.ExpiredPEM(t, f)},
			bead: "workspace-zg3",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mongod := harness.StartTLSMongod(t, f, c.opts)
			dumbodb := harness.StartTLSDumboDB(t, f, c.opts)

			if !mongod.StartFailed {
				t.Skipf("premise failed: mongod accepted %s, so there is no oracle behavior to match", c.name)
			}
			if dumbodb.StartFailed {
				if c.bead != "" {
					t.Errorf("XPASS %s: dumbodb now refuses %s at startup like mongod; remove the exemption", c.bead, c.name)
				}
				return
			}
			if c.bead == "" {
				t.Errorf("dumbodb started with %s where mongod refused to; every client will fail instead", c.name)
				return
			}
			t.Logf("XFAIL %s: dumbodb starts with %s where mongod refuses", c.bead, c.name)
		})
	}
}
