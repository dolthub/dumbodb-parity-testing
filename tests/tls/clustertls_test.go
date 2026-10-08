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

// The cluster TLS options: the certificate a member presents to its peers,
// rather than the one it serves to clients.
//
// These five moved out of the refused-by-name list in unsupportedflags_test.go
// when the branch implemented them. Moving them there is what makes CI green;
// this file is what stops that being a loss of coverage, because "no longer
// refused" and "does something" are different claims and only the second one
// is worth having.
//
// Every expectation here was measured against both servers first.

package tls

import (
	"strings"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const (
	clusterOrg       = "DumboCluster"
	clusterExtension = "dumbo-members"
	clusterKeyPass   = "hunter2"
)

// A member certificate has to satisfy both roles at once: mongod validates the
// cluster X.509 criteria against the serving certificate AND the cluster
// certificate, so one file is used as both.
func clusterMemberArgs(t *testing.T, f *harness.TLSFixture, extra ...string) ([]string, string) {
	t.Helper()
	member := harness.ClusterMemberPEM(t, f, "cluster-member.pem", clusterOrg, clusterExtension)
	return append([]string{
		"--replSet", "rs0",
		"--tlsClusterFile", member,
		"--tlsClusterCAFile", f.CAFile,
	}, extra...), member
}

// TestClusterTLS_OptionsRequireReplSet pins the precondition that made the
// old refused-by-name test look like a failure: without --replSet the options
// are rejected, and the message names the missing prerequisite rather than the
// option, because the option is not the thing that is wrong.
func TestClusterTLS_OptionsRequireReplSet(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)

	for name, args := range map[string][]string{
		"tlsClusterFile":     {"--tlsClusterFile", f.ServerPEMFile},
		"tlsClusterCAFile":   {"--tlsClusterCAFile", f.CAFile},
		"tlsClusterPassword": {"--tlsClusterPassword", clusterKeyPass},
	} {
		t.Run(name, func(t *testing.T) {
			dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: args})
			if !dumbodb.StartFailed {
				t.Fatalf("dumbodb started with --%s and no --replSet; the option has no meaning outside a replica set", name)
			}
			if !strings.Contains(dumbodb.FailureOutput, "--replSet") {
				t.Errorf("dumbodb refused --%s without naming --replSet, so the operator is not told what is missing.\n%s",
					name, firstLine(dumbodb.FailureOutput))
			}
		})
	}
}

// TestClusterTLS_ClusterCertificateIsAccepted is the positive case the move
// to the implemented list asserts: given its prerequisite, the option starts a
// server rather than being refused.
func TestClusterTLS_ClusterCertificateIsAccepted(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	args, _ := clusterMemberArgs(t, f)

	dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: args})
	if dumbodb.StartFailed {
		t.Fatalf("dumbodb refused a replica-set TLS certificate it implements:\n%s", dumbodb.FailureOutput)
	}
}

// TestClusterTLS_PasswordUnlocksAnEncryptedClusterKey covers the one option of
// the five that nothing else reaches. The replication cases use an unencrypted
// cluster key, so without this --tlsClusterPassword would be implemented and
// never exercised.
func TestClusterTLS_PasswordUnlocksAnEncryptedClusterKey(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	encrypted := harness.EncryptedClusterPEM(t, f, "cluster-encrypted.pem", clusterOrg, clusterKeyPass)

	base := []string{"--replSet", "rs0", "--tlsClusterFile", encrypted, "--tlsClusterCAFile", f.CAFile}

	t.Run("correct password", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			ExtraArgs: append(append([]string{}, base...), "--tlsClusterPassword", clusterKeyPass),
		})
		if dumbodb.StartFailed {
			t.Errorf("dumbodb could not open an encrypted cluster key with the right password:\n%s", dumbodb.FailureOutput)
		}
	})

	// Both negative cases matter. A password that is merely ignored would let
	// the correct-password case pass while the option did nothing at all.
	t.Run("wrong password", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			ExtraArgs: append(append([]string{}, base...), "--tlsClusterPassword", "not-the-password"),
		})
		if !dumbodb.StartFailed {
			t.Error("dumbodb started with the wrong cluster key password, so the key was never really decrypted")
		}
	})

	t.Run("no password", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: base})
		if !dumbodb.StartFailed {
			t.Error("dumbodb started with an encrypted cluster key and no password")
		}
	})
}

// TestClusterX509_IdentityOptionsRequireAnX509AuthMode pins the second
// precondition. These two options describe how cluster members are identified
// by certificate, which is meaningless when membership is authenticated by
// keyfile.
func TestClusterX509_IdentityOptionsRequireAnX509AuthMode(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)

	for name, args := range map[string][]string{
		"tlsClusterAuthX509Attributes":     {"--tlsClusterAuthX509Attributes", "O=" + clusterOrg},
		"tlsClusterAuthX509ExtensionValue": {"--tlsClusterAuthX509ExtensionValue", clusterExtension},
	} {
		t.Run(name, func(t *testing.T) {
			full, _ := clusterMemberArgs(t, f, args...)
			dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: full})
			if !dumbodb.StartFailed {
				t.Fatalf("dumbodb started with --%s under the default keyfile auth mode, where certificate identity means nothing", name)
			}
			if !strings.Contains(dumbodb.FailureOutput, "clusterAuthMode") {
				t.Errorf("dumbodb refused --%s without naming --clusterAuthMode.\n%s", name, firstLine(dumbodb.FailureOutput))
			}
		})
	}
}

// TestClusterX509_AttributesMustMatchTheCertificate asserts the option selects
// rather than merely parses: a subject that satisfies it starts, one that does
// not is refused.
//
// mongod is checked alongside, because this is the case where DumboDB looked
// stricter than mongod on inspection and is not. Both refuse when the SERVING
// certificate lacks the attributes, even though a separate cluster certificate
// carries them.
func TestClusterX509_AttributesMustMatchTheCertificate(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	member := harness.ClusterMemberPEM(t, f, "cluster-member.pem", clusterOrg, clusterExtension)

	matching, _ := clusterMemberArgs(t, f, "--clusterAuthMode", "x509",
		"--tlsClusterAuthX509Attributes", "O="+clusterOrg)
	wrong, _ := clusterMemberArgs(t, f, "--clusterAuthMode", "x509",
		"--tlsClusterAuthX509Attributes", "O=SomeOtherOrganisation")

	t.Run("subject matches", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			CertificateKeyFile: member, ExtraArgs: matching,
		})
		if dumbodb.StartFailed {
			t.Errorf("dumbodb refused a certificate carrying O=%s:\n%s", clusterOrg, dumbodb.FailureOutput)
		}
	})

	t.Run("subject does not match", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			CertificateKeyFile: member, ExtraArgs: wrong,
		})
		if !dumbodb.StartFailed {
			t.Error("dumbodb accepted attributes its own certificate does not carry, so the option selects nothing")
		}
	})

	// The serving certificate is checked too, not only the cluster one. Stated
	// as a mongod comparison because it is surprising enough to be worth an
	// oracle rather than an assertion about us alone.
	t.Run("serving certificate is checked, same as mongod", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{ExtraArgs: matching})
		if !dumbodb.StartFailed {
			t.Error("dumbodb accepted a serving certificate without the configured attributes")
		}
		mongod := harness.StartTLSMongod(t, f, harness.TLSOptions{
			ExtraArgs: append(append([]string{}, matching...), "--clusterAuthMode", "x509"),
		})
		if !mongod.StartFailed {
			t.Error("mongod accepted it, so this is a deviation rather than the shared rule this case assumes")
		}
	})
}

// TestClusterX509_ExtensionValueMustMatchTheCertificate is the same claim for
// the extension form: MongoDB's cluster membership OID must be present and
// carry the configured value.
func TestClusterX509_ExtensionValueMustMatchTheCertificate(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	member := harness.ClusterMemberPEM(t, f, "cluster-member.pem", clusterOrg, clusterExtension)
	plain := harness.ClusterMemberPEM(t, f, "cluster-plain.pem", clusterOrg, "")

	matching, _ := clusterMemberArgs(t, f, "--clusterAuthMode", "x509",
		"--tlsClusterAuthX509ExtensionValue", clusterExtension)
	wrong, _ := clusterMemberArgs(t, f, "--clusterAuthMode", "x509",
		"--tlsClusterAuthX509ExtensionValue", "a-different-value")

	t.Run("extension matches", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			CertificateKeyFile: member, ExtraArgs: matching,
		})
		if dumbodb.StartFailed {
			t.Errorf("dumbodb refused a certificate carrying the configured extension:\n%s", dumbodb.FailureOutput)
		}
	})

	t.Run("extension value differs", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			CertificateKeyFile: member, ExtraArgs: wrong,
		})
		if !dumbodb.StartFailed {
			t.Error("dumbodb accepted an extension value its certificate does not carry")
		}
	})

	t.Run("extension absent", func(t *testing.T) {
		dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
			CertificateKeyFile: plain, ExtraArgs: matching,
		})
		if !dumbodb.StartFailed {
			t.Error("dumbodb accepted a certificate with no cluster membership extension at all")
		}
	})
}

// TestClusterX509_AttributesAndExtensionAreMutuallyExclusive: they are two
// ways to say the same thing, and accepting both would leave which one applies
// undefined.
func TestClusterX509_AttributesAndExtensionAreMutuallyExclusive(t *testing.T) {
	t.Parallel()
	f := harness.NewTLSFixture(t)
	member := harness.ClusterMemberPEM(t, f, "cluster-member.pem", clusterOrg, clusterExtension)
	both, _ := clusterMemberArgs(t, f, "--clusterAuthMode", "x509",
		"--tlsClusterAuthX509Attributes", "O="+clusterOrg,
		"--tlsClusterAuthX509ExtensionValue", clusterExtension)

	dumbodb := harness.StartTLSDumboDB(t, f, harness.TLSOptions{
		CertificateKeyFile: member, ExtraArgs: both,
	})
	if !dumbodb.StartFailed {
		t.Fatal("dumbodb accepted both identity options at once, leaving it undefined which one identifies a peer")
	}
	if !strings.Contains(dumbodb.FailureOutput, "mutually exclusive") {
		t.Errorf("dumbodb refused both options without saying they are mutually exclusive.\n%s",
			firstLine(dumbodb.FailureOutput))
	}
}
