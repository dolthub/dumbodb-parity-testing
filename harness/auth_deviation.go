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

package harness

import (
	"context"
	"testing"
	"time"
)

// AuthDeviationCase pins an intentional behavioral difference between MongoDB and
// DumboDB. Run performs the operation against one server; Mongo and Dumbo assert
// each server's own expected outcome, so the case documents both behaviors
// rather than comparing the two for equality.
type AuthDeviationCase struct {
	Name  string
	Run   func(ctx context.Context, target AuthTarget) (interface{}, error)
	Mongo func(t *testing.T, res interface{}, err error)
	Dumbo func(t *testing.T, res interface{}, err error)
}

// AuthDeviationTest runs tc against both servers and applies the per-server
// assertions. It skips when the auth suite is disabled (PARITY_AUTH unset).
func AuthDeviationTest(t *testing.T, tc AuthDeviationCase) TestResult {
	t.Helper()
	RequireAuth(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ac, err := GetAuthClients(ctx)
	if err != nil {
		t.Fatalf("AuthDeviationTest %s: get auth clients: %v", tc.Name, err)
	}

	ns := authNS(tc.Name)

	mRes, mErr := tc.Run(ctx, AuthTarget{Admin: ac.MongoAdmin, BaseURI: AuthMongoBaseURI(), NS: ns})
	tc.Mongo(t, mRes, mErr)

	dRes, dErr := tc.Run(ctx, AuthTarget{Admin: ac.DumboDBAdmin, BaseURI: AuthDumboDBBaseURI(), NS: ns})
	tc.Dumbo(t, dRes, dErr)

	if t.Failed() {
		return TestResult{Name: tc.Name, Status: StatusFail}
	}

	t.Logf("DEVIATE %s: MongoDB and DumboDB behave as intended (see per-server assertions)", tc.Name)
	return TestResult{Name: tc.Name, Status: StatusDeviate}
}
