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

// Unimplemented mongod TLS flags, workspace-09n.6.
//
// This is the one place in the suite where matching mongod is the wrong goal.
// mongod accepts these and honours them; DumboDB does not implement them, and
// the choice made was to refuse by name rather than accept and ignore. An
// ignored --tlsAllowInvalidCertificates is a security setting the operator
// believes is in force, so the deviation is deliberate and is asserted here so
// nobody quietly turns it back into silence.

package tls

import (
	"strings"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// implementedTLSFlags are the mongod TLS options DumboDB actually honours.
// Everything else mongod offers has to be refused by name, and which list a
// flag belongs to is the only thing this file needs to know about it.
var implementedTLSFlags = map[string]bool{
	"tlsMode":                                true,
	"tlsCertificateKeyFile":                  true,
	"tlsCAFile":                              true,
	"tlsCRLFile":                             true,
	"tlsDisabledProtocols":                   true,
	"tlsAllowConnectionsWithoutCertificates": true,
	"tlsCertificateKeyFilePassword":          true,
	"tlsClusterFile":                         true,
	"tlsClusterCAFile":                       true,
	"tlsClusterPassword":                     true,
	"tlsClusterAuthX509Attributes":           true,
	"tlsClusterAuthX509ExtensionValue":       true,
}

func unsupportedFlagArgs() map[string][]string {
	return map[string][]string{
		"tlsAllowInvalidCertificates": {"--tlsAllowInvalidCertificates"},
		"tlsAllowInvalidHostnames":    {"--tlsAllowInvalidHostnames"},
		"tlsLogVersions":              {"--tlsLogVersions", "TLS1_2"},
		"tlsOnNormalPorts":            {"--tlsOnNormalPorts"},
	}
}

// Every TLS option mongod offers is either implemented here or refused by
// name. Asking mongod itself for the list means a flag MongoDB adds later
// arrives as a failing test rather than as a silently unhandled option.
func TestTLSUnsupportedFlags_EveryMongodFlagIsAccountedFor(t *testing.T) {
	t.Parallel()
	known := unsupportedFlagArgs()

	for _, flag := range harness.MongodTLSFlags(t) {
		if implementedTLSFlags[flag] || known[flag] != nil {
			continue
		}
		t.Errorf("mongod offers --%s and this suite accounts for it neither as implemented nor as refused by name; "+
			"add it to one of the two lists in this file, having decided which it is",
			flag)
	}
}

func TestTLSUnsupportedFlags_RefusedByName(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	for flag, args := range unsupportedFlagArgs() {
		c := struct {
			flag string
			args []string
		}{flag, args}
		t.Run(c.flag, func(t *testing.T) {
			dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: c.args})
			if !dumbodb.StartFailed {
				t.Errorf("dumbodb started with --%s, which it does not implement; the operator has no way to learn the setting is not in force",
					c.flag)
				return
			}
			if !strings.Contains(dumbodb.FailureOutput, c.flag) {
				t.Errorf("dumbodb refused --%s without naming it, so the message does not say which option is missing.\ndumbodb said: %s",
					c.flag, firstLine(dumbodb.FailureOutput))
			}
		})
	}
}

// The deviation only holds if mongod really does accept these, which is what
// makes refusing them a choice rather than a shared limitation. Recorded
// rather than asserted: mongod's answer is the fact, not the requirement.
func TestTLSUnsupportedFlags_MongodAcceptsThem(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)

	for _, c := range []struct {
		flag string
		args []string
	}{
		{"tlsAllowInvalidCertificates", []string{"--tlsAllowInvalidCertificates"}},
		{"tlsAllowInvalidHostnames", []string{"--tlsAllowInvalidHostnames"}},
	} {
		t.Run(c.flag, func(t *testing.T) {
			mongod := harness.StartTLSMongod(t, f, harness.TLSOptions{ExtraArgs: c.args})
			t.Logf("mongod with --%s: startFailed=%v", c.flag, mongod.StartFailed)
			if mongod.StartFailed {
				t.Logf("mongod said: %s", firstLine(mongod.FailureOutput))
			}
		})
	}
}
