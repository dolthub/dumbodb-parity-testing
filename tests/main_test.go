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

package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMain provisions every server the parity suites need (a plain pair and an
// access-control-enabled pair), then runs the tests. If a required server binary
// cannot be found or a server fails to start, it EXITS NON-ZERO rather than
// skipping: a run that cannot compare against real servers must fail loudly, not
// report a green suite that verified nothing.
//
// Binaries are located via MONGOD_BIN / DUMBODB_BIN (or PATH). Any of the four
// endpoints may instead be supplied directly with MONGO_URI, DUMBODB_URI,
// MONGO_AUTH_URI, DUMBODB_AUTH_URI, in which case that endpoint is used as-is and
// must be reachable.
func TestMain(m *testing.M) {
	if err := harness.ProvisionServers(); err != nil {
		fmt.Fprintf(os.Stderr,
			"\nFATAL: parity tests could not start their servers: %v\n\n"+
				"  The suite compares MongoDB against DumboDB and cannot run without both.\n"+
				"  Provide the binaries (MONGOD_BIN, DUMBODB_BIN, or on PATH), or point at\n"+
				"  already-running servers via MONGO_URI / DUMBODB_URI / MONGO_AUTH_URI /\n"+
				"  DUMBODB_AUTH_URI.\n\n",
			err,
		)
		harness.TeardownServers()
		os.Exit(1)
	}

	code := m.Run()
	harness.TeardownServers()
	os.Exit(code)
}
