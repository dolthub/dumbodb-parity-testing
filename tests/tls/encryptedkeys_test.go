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

// Encrypted private keys, workspace-09n.5 and workspace-09n.15.
//
// A wrong password must be an error and never a server that starts on garbage.
// Go's deprecated RFC 1423 path can return random bytes instead of failing for
// some wrong passwords, so "refuses to start" is asserted here rather than
// assumed from the implementation not using it.

package tls

import (
	"context"
	"strings"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const keyPassword = "hunter2"

func TestEncryptedKey_CorrectPasswordServesClients(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{
		CertificateKeyFile: harness.EncryptedPKCS8PEM(t, f, keyPassword),
		KeyPassword:        keyPassword,
	}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start with an encrypted PKCS#8 key and the right password: %s",
					s.name, firstLine(s.server.FailureOutput))
			}
			client, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Fatalf("%s started with an encrypted key but served no client: %v", s.name, err)
			}
			_ = client.Disconnect(context.Background())
		})
	}
}

// The failure that matters. A wrong password must stop the server, not be
// absorbed into material nobody can use.
func TestEncryptedKey_WrongPasswordRefusesToStart(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{
		CertificateKeyFile: harness.EncryptedPKCS8PEM(t, f, keyPassword),
		KeyPassword:        "not-the-password",
	}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	t.Logf("wrong password: mongod startFailed=%v, dumbodb startFailed=%v", mongod.StartFailed, dumbodb.StartFailed)

	if !dumbodb.StartFailed {
		t.Error("dumbodb started with the wrong key password; the decryption either succeeded on garbage or was skipped")
	}
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v for a wrong key password", mongod.StartFailed, dumbodb.StartFailed)
	}
}

// workspace-09n.15: an encrypted key with no password given used to report a
// parse failure, which sent the operator off to regenerate a file that was
// fine. The refusal has to name the cause.
func TestEncryptedKey_MissingPasswordNamesTheCause(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{CertificateKeyFile: harness.EncryptedPKCS8PEM(t, f, keyPassword)}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	t.Logf("no password: mongod startFailed=%v, dumbodb startFailed=%v", mongod.StartFailed, dumbodb.StartFailed)

	if !dumbodb.StartFailed {
		t.Fatal("dumbodb started with an encrypted key and no password")
	}
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v for an encrypted key with no password given",
			mongod.StartFailed, dumbodb.StartFailed)
	}
	// mongod is not the oracle for the wording; the point is that ours says
	// what is wrong rather than reporting a broken file.
	said := dumbodb.FailureOutput
	if !strings.Contains(said, "encrypted") || !strings.Contains(said, "tlsCertificateKeyFilePassword") {
		t.Errorf("dumbodb refused an encrypted key without saying it is encrypted or naming the password flag.\ndumbodb said: %s",
			firstLine(said))
	}
}

// TestEncryptedKey_LegacyFormatDeviates pins a deliberate deviation: mongod
// accepts RFC 1423 encrypted keys, the ones carrying a DEK-Info header, and
// DumboDB refuses them.
//
// WHY WE DEVIATE: the format is unauthenticated and vulnerable to padding
// oracle attacks, which is why Go deprecated the only standard-library
// function that reads it. That function also cannot reliably tell a wrong
// password from a right one; for some passwords it returns random bytes and
// no error, which would mean a server starting on a key nobody can use.
// Refusing is the choice; reading it badly is the alternative.
//
// WHAT IT COSTS: an operator whose key works on mongod cannot start DumboDB
// until they convert it, so the refusal has to say that and say how.
//
// Both halves are asserted. If mongod ever stops accepting these, or DumboDB
// starts, this is no longer a deviation and the decision should be revisited
// rather than the test quietly following along.
func TestEncryptedKey_LegacyFormatDeviates(t *testing.T) {
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{
		CertificateKeyFile: harness.LegacyEncryptedPEM(t, f, keyPassword),
		KeyPassword:        keyPassword,
	}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	t.Logf("legacy DEK-Info key: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)

	// The MongoDB side of the deviation. Starting is not enough: the key has
	// to be in use, or this would also pass against a mongod that ignored it.
	if mongod.StartFailed {
		t.Fatalf("the deviation no longer holds: mongod refused a legacy RFC 1423 key, so DumboDB refusing one now matches it. Revisit workspace-09n.5.\nmongod said: %s",
			firstLine(mongod.FailureOutput))
	}
	client, err := mongod.Connect(ctx, t, true)
	if err != nil {
		t.Fatalf("mongod started with a legacy encrypted key but served no TLS client, so it is not demonstrably using it: %v", err)
	}
	_ = client.Disconnect(context.Background())

	// The DumboDB side.
	if !dumbodb.StartFailed {
		t.Fatal("the deviation no longer holds: dumbodb started with a legacy RFC 1423 encrypted key. If that is intended, this test should be deleted and the behaviour asserted as parity instead")
	}
	said := dumbodb.FailureOutput
	if !strings.Contains(said, "legacy") {
		t.Errorf("dumbodb refused legacy material without calling it legacy, so it reads like a corrupt file.\ndumbodb said: %s",
			firstLine(said))
	}
	if !strings.Contains(said, "pkcs8") {
		t.Errorf("dumbodb refused legacy material without naming the conversion, leaving the operator with a file mongod accepts and no way forward.\ndumbodb said: %s",
			firstLine(said))
	}
	t.Logf("DEVIATION CONFIRMED: mongod serves clients with a legacy RFC 1423 key; dumbodb refuses at startup and says how to convert it")
}
