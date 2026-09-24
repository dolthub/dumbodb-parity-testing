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

const protocolsBead = "workspace-09n.9"

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

// Disabling a version has to actually refuse it. Until --tlsDisabledProtocols
// exists, DumboDB cannot even be asked.
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
		t.Logf("XFAIL %s: dumbodb will not start with --tlsDisabledProtocols: %s", protocolsBead, firstLine(dumbodb.FailureOutput))
		return
	}
	if _, err := dumbodb.ConnectWithVersion(t, tls.VersionTLS12); err == nil {
		t.Errorf("dumbodb served TLS 1.2 despite --tlsDisabledProtocols TLS1_2")
		return
	}
	if _, err := dumbodb.ConnectWithVersion(t, tls.VersionTLS13); err != nil {
		t.Errorf("dumbodb refused TLS 1.3 when only 1.2 was disabled: %v", err)
		return
	}
	t.Logf("XPASS %s: dumbodb honours --tlsDisabledProtocols; remove the exemption", protocolsBead)
}

// A non-contiguous set cannot be expressed as a min and max version, so it
// decides whether the simple mapping is enough or a connection callback is
// needed. mongod's answer is what DumboDB has to match.
func TestTLSProtocols_NonContiguousDisabledSet(t *testing.T) {
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{DisabledProtocols: "TLS1_2"}

	mongod := harness.StartTLSMongod(t, f, opts)
	if mongod.StartFailed {
		t.Skipf("mongod would not start with --tlsDisabledProtocols TLS1_2")
	}
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		_, err := mongod.ConnectWithVersion(t, version)
		t.Logf("oracle: mongod with TLS1_2 disabled, client pinned to %s, accepted=%v",
			tlsVersionNames[version], err == nil)
	}
	t.Logf("recorded for %s: a set that leaves a hole cannot be a min/max range, so this is the case that decides the implementation",
		protocolsBead)
}
