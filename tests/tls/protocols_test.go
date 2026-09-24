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

// TLS protocol versions, workspace-09n.9 and workspace-09n.11.
//
// Asserted by dialling with a client pinned to one version, never by reading
// configuration back. What a server accepts is a property of the handshake,
// and restating the flags would prove only that they were parsed.

package tls

import (
	"crypto/tls"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

var tlsVersionNames = map[uint16]string{
	tls.VersionTLS10: "TLS1_0",
	tls.VersionTLS11: "TLS1_1",
	tls.VersionTLS12: "TLS1_2",
	tls.VersionTLS13: "TLS1_3",
}

// Which versions each server accepts out of the box, with nothing disabled.
// The old ones being refused is the answer a compliance question is really
// asking, and it is worth recording whether it is true by configuration or
// merely by the library's defaults.
func TestTLSProtocols_DefaultsAgree(t *testing.T) {
	f := harness.NewTLSFixture(t)
	mongod := harness.StartTLSMongod(t, f, harness.TLSOptions{})
	dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{})
	if mongod.StartFailed || dumbodb.StartFailed {
		t.Fatal("a server would not start with valid material")
	}

	for _, version := range []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13} {
		name := tlsVersionNames[version]
		t.Run(name, func(t *testing.T) {
			_, mongoErr := mongod.ConnectWithVersion(t, version)
			_, dumboErr := dumbodb.ConnectWithVersion(t, version)

			mongoAccepted := mongoErr == nil
			dumboAccepted := dumboErr == nil
			t.Logf("%s: mongod accepted=%v, dumbodb accepted=%v", name, mongoAccepted, dumboAccepted)

			if mongoAccepted != dumboAccepted {
				t.Errorf("%s: mongod accepted=%v but dumbodb accepted=%v; the servers disagree about which protocol versions they speak",
					name, mongoAccepted, dumboAccepted)
			}
			if !mongoAccepted && version <= tls.VersionTLS11 {
				return // both correctly refuse the obsolete versions
			}
		})
	}
}

// Disabling a version has to actually refuse it, and leave the others alone.
//
// TLS1_2 is the interesting one to disable: it leaves a hole rather than
// moving a floor, so a server implementing this as a minimum version cannot
// express it and will either serve 1.2 anyway or refuse 1.3 as well.
func TestTLSProtocols_DisablingAVersionRefusesIt(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{DisabledProtocols: "TLS1_2"}

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with --tlsDisabledProtocols:\n%s", mongod.FailureOutput)
	}
	if _, err := mongod.ConnectWithVersion(t, tls.VersionTLS12); err == nil {
		t.Fatal("premise failed: mongod served TLS 1.2 after being told to disable it, so this test cannot tell enforcement from its absence")
	}
	if _, err := mongod.ConnectWithVersion(t, tls.VersionTLS13); err != nil {
		t.Fatalf("premise failed: mongod refused TLS 1.3 when only 1.2 was disabled: %v", err)
	}

	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start with --tlsDisabledProtocols, which mongod accepts: %s", firstLine(dumbodb.FailureOutput))
	}
	if _, err := dumbodb.ConnectWithVersion(t, tls.VersionTLS12); err == nil {
		t.Error("dumbodb served TLS 1.2 despite --tlsDisabledProtocols TLS1_2")
	}
	if _, err := dumbodb.ConnectWithVersion(t, tls.VersionTLS13); err != nil {
		t.Errorf("dumbodb refused TLS 1.3 when only 1.2 was disabled: %v", err)
	}
}

// A client offering everything must be given the highest version the server
// still allows, not merely some allowed one. Negotiating downwards when a
// better version was on offer is the downgrade this flag exists to prevent.
func TestTLSProtocols_NegotiatesTheHighestEnabledVersion(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{DisabledProtocols: "TLS1_3"}

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Fatalf("premise failed: mongod would not start with --tlsDisabledProtocols TLS1_3:\n%s", mongod.FailureOutput)
	}
	dumbodb := harness.StartTLSDumboDB(t, f, opts)
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb would not start with --tlsDisabledProtocols TLS1_3: %s", firstLine(dumbodb.FailureOutput))
	}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
		t.Run(s.name, func(t *testing.T) {
			version, err := s.server.ConnectAnyVersion(t)
			if err != nil {
				t.Fatalf("%s refused a client offering every version with only TLS 1.3 disabled: %v", s.name, err)
			}
			t.Logf("%s negotiated %s", s.name, tlsVersionNames[version])
			if version != tls.VersionTLS12 {
				t.Errorf("%s negotiated %s with TLS 1.3 disabled; TLS 1.2 was on offer and is the highest version still enabled",
					s.name, tlsVersionNames[version])
			}
		})
	}
}

// Disabling every version leaves nothing to negotiate. A server that starts
// anyway is one whose port answers and whose every client fails, which is the
// shape of failure this suite exists to catch.
func TestTLSProtocols_AllVersionsDisabled(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{DisabledProtocols: "TLS1_0,TLS1_1,TLS1_2,TLS1_3"}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)

	t.Logf("every version disabled: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v when every TLS version is disabled.\nmongod said: %s\ndumbodb said: %s",
			mongod.StartFailed, dumbodb.StartFailed, firstLine(mongod.FailureOutput), firstLine(dumbodb.FailureOutput))
		return
	}
	if mongod.StartFailed {
		return
	}
	if _, err := mongod.ConnectAnyVersion(t); err == nil {
		t.Error("premise failed: mongod served a client with every TLS version disabled")
	}
	if _, err := dumbodb.ConnectAnyVersion(t); err == nil {
		t.Error("dumbodb served a client with every TLS version disabled")
	}
}

// Naming versions to disable must not enable any others.
//
// TestTLSProtocols_DefaultsAgree fixes what each server speaks with the flag
// absent. This asks whether passing the flag at all moves that floor, which is
// the failure mode where a hardening setting loosens the server: an operator
// disabling TLS 1.0 has said nothing about TLS 1.1, and must not get it.
//
// "none" is mongod's spelling for disable nothing. It is the most permissive
// value the flag takes, and even it does not reach below what the server was
// built to speak.
func TestTLSProtocols_DisablingSomeEnablesNoOthers(t *testing.T) {
	f := harness.NewTLSFixture(t)

	for _, disabled := range []string{"TLS1_0", "TLS1_3", "none"} {
		t.Run(disabled, func(t *testing.T) {
			opts := harness.TLSOptions{DisabledProtocols: disabled}
			mongod := harness.StartTLSMongod(t, f, opts)
			if mongod.StartFailed {
				t.Fatalf("premise failed: mongod would not start with --tlsDisabledProtocols %s:\n%s", disabled, mongod.FailureOutput)
			}
			dumbodb := harness.StartTLSDumboDB(t, f, opts)
			if dumbodb.StartFailed {
				t.Fatalf("dumbodb would not start with --tlsDisabledProtocols %s, which mongod accepts: %s",
					disabled, firstLine(dumbodb.FailureOutput))
			}

			for _, version := range []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13} {
				name := tlsVersionNames[version]
				t.Run(name, func(t *testing.T) {
					_, mongoErr := mongod.ConnectWithVersion(t, version)
					_, dumboErr := dumbodb.ConnectWithVersion(t, version)
					mongoAccepted, dumboAccepted := mongoErr == nil, dumboErr == nil
					t.Logf("--tlsDisabledProtocols %s, client pinned to %s: mongod accepted=%v, dumbodb accepted=%v",
						disabled, name, mongoAccepted, dumboAccepted)
					if mongoAccepted == dumboAccepted {
						return
					}
					if dumboAccepted {
						t.Errorf("dumbodb served %s under --tlsDisabledProtocols %s where mongod refuses it; "+
							"naming versions to disable must not enable versions the server would otherwise refuse",
							name, disabled)
						return
					}
					t.Errorf("dumbodb refused %s under --tlsDisabledProtocols %s where mongod serves it", name, disabled)
				})
			}
		})
	}
}

// A misspelled version is a configuration error, and silently disabling
// nothing is the dangerous reading of it: the operator believes a version is
// off and it is not.
func TestTLSProtocols_UnrecognizedVersionName(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{DisabledProtocols: "TLS1_4"}

	mongod := harness.StartTLSMongod(t, f, opts)
	dumbodb := harness.StartTLSDumboDB(t, f, opts)

	t.Logf("--tlsDisabledProtocols TLS1_4: mongod startFailed=%v, dumbodb startFailed=%v",
		mongod.StartFailed, dumbodb.StartFailed)
	if mongod.StartFailed != dumbodb.StartFailed {
		t.Errorf("mongod startFailed=%v but dumbodb startFailed=%v for an unrecognized protocol name.\nmongod said: %s\ndumbodb said: %s",
			mongod.StartFailed, dumbodb.StartFailed, firstLine(mongod.FailureOutput), firstLine(dumbodb.FailureOutput))
	}
}
