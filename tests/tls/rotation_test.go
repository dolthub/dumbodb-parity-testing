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

// TLS material rotation, workspace-09n.13.
//
// Certificates are replaced on a schedule nobody is watching at the time, and
// the replacement is where deployments break. workspace-zg3 covered the
// startup half, a server refusing material that is already expired. These
// cover what happens either side of that moment: material expiring under a
// running server, a restart onto its replacement, and a CA rotation that
// strands every client certificate the old one signed.

package tls

import (
	"context"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// The window has to outlast two server starts and a connection, and the test
// then waits it out, so it is the floor on this file's runtime.
const shortLife = 25 * time.Second

// A certificate that expires while the server is running. The validity window
// is checked when the configuration is built; whether anything checks it
// again is the question, and the answer decides what an operator sees at the
// moment a renewal is missed.
func TestRotation_CertificateExpiresWhileRunning(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{
		CertificateKeyFile: harness.ShortLivedPEM(t, f, "expiring.pem", shortLife),
	}
	deadline := time.Now().Add(shortLife)

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start on material valid for %s:\n%s", shortLife, mongod.FailureOutput)
	}
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start on material valid for %s, which mongod accepts: %s",
			shortLife, firstLine(dumbodb.FailureOutput))
	}

	// Established before expiry, and held across it.
	mongodHeld, err := mongod.Connect(ctx, t, true)
	if err != nil {
		t.Fatalf("premise failed: mongod served no client while its certificate was still valid: %v", err)
	}
	defer func() { _ = mongodHeld.Disconnect(context.Background()) }()
	dumbodbHeld, err := dumbodb.Connect(ctx, t, true)
	if err != nil {
		t.Fatalf("dumbodb served no client while its certificate was still valid: %v", err)
	}
	defer func() { _ = dumbodbHeld.Disconnect(context.Background()) }()

	if wait := time.Until(deadline) + 3*time.Second; wait > 0 {
		t.Logf("waiting %s for the certificate to expire under both servers", wait.Round(time.Second))
		time.Sleep(wait)
	}

	// Does a connection opened before expiry keep working after it?
	mongodStillUp := mongodHeld.Ping(ctx, nil) == nil
	dumbodbStillUp := dumbodbHeld.Ping(ctx, nil) == nil
	t.Logf("connection established before expiry, used after: mongod alive=%v, dumbodb alive=%v",
		mongodStillUp, dumbodbStillUp)
	if mongodStillUp != dumbodbStillUp {
		t.Errorf("a connection opened before the certificate expired: mongod alive=%v but dumbodb alive=%v",
			mongodStillUp, dumbodbStillUp)
	}

	// And can a new client still get in?
	mongodNew := connectSucceeds(ctx, t, mongod)
	dumbodbNew := connectSucceeds(ctx, t, dumbodb)
	t.Logf("new connection after expiry: mongod accepted=%v, dumbodb accepted=%v", mongodNew, dumbodbNew)
	if mongodNew != dumbodbNew {
		t.Errorf("after the certificate expired under a running server: mongod accepted a new client=%v but dumbodb accepted=%v",
			mongodNew, dumbodbNew)
	}

	// Neither server should have died over it. A process that exits on expiry
	// is a different failure from one that stops accepting.
	t.Logf("still running after expiry: mongod=%v, dumbodb=%v", !mongod.Exited(), !dumbodb.Exited())
	if mongod.Exited() != dumbodb.Exited() {
		t.Errorf("mongod exited=%v but dumbodb exited=%v when the certificate expired underneath it",
			mongod.Exited(), dumbodb.Exited())
	}
}

// The recovery path. Whatever expiry does, replacing the file and restarting
// has to fix it, because that is what an automated renewal does.
func TestRotation_RestartPicksUpReplacedMaterial(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	path := harness.ShortLivedPEM(t, f, "rotating.pem", shortLife)
	opts := harness.TLSOptions{CertificateKeyFile: path}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start on short-lived material: %s", s.name, firstLine(s.server.FailureOutput))
			}
			// Replace the file under the running server, then restart onto it.
			harness.ShortLivedPEM(t, f, "rotating.pem", 24*time.Hour)
			restarted := s.server.Restart(t)
			if restarted.StartFailed {
				t.Fatalf("%s would not restart onto replaced material, so a renewal would not take effect: %s",
					s.name, firstLine(restarted.FailureOutput))
			}
			client, err := restarted.Connect(ctx, t, true)
			if err != nil {
				t.Fatalf("%s restarted onto fresh material but served no client: %v", s.name, err)
			}
			_ = client.Disconnect(context.Background())
		})
	}
}

// Rotating the CA strands every client certificate the old one signed. The
// blast radius is every client at once, so the question is whether the
// refusal is legible and whether both servers agree on it.
func TestRotation_ReplacingTheCAStrandsOldClientCertificates(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	// The replacement authority: a whole new trust root, which is what
	// rotating a CA amounts to.
	replacement := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{CAFile: replacement.CAFile}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start with a replaced CA: %s", s.name, firstLine(s.server.FailureOutput))
			}
			old, err := s.server.Connect(ctx, t, true)
			if err == nil {
				_ = old.Disconnect(context.Background())
				t.Error("a client certificate signed by the replaced CA still authenticated; rotating the authority revoked nothing")
			}
			fresh, err := s.server.ConnectAs(ctx, t, replacement)
			if err != nil {
				t.Errorf("a client certificate signed by the new CA was refused, so the rotation locks everyone out: %v", err)
				return
			}
			_ = fresh.Disconnect(context.Background())
		})
	}
}
