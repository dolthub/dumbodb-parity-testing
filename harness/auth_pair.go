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
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// AuthCase is the auth-parity analogue of TestCase. Its Run is invoked once per
// server with that server's admin client and credential-free base URI, plus a
// unique namespace token the test uses to name any databases/users/roles it
// creates (and should clean up). The result/error from the two runs is compared
// exactly like PairTest, honoring the Support level.
type AuthCase struct {
	Name    string
	Support DumboDBSupport
	Run     func(ctx context.Context, admin AuthTarget) (interface{}, error)

	MongoExpect func(t *testing.T, res interface{}, err error)
	DumboExpect func(t *testing.T, res interface{}, err error)
}

// AuthTarget bundles what an AuthCase.Run needs to exercise one server.
type AuthTarget struct {
	// Admin is an admin-authenticated client (MongoDB) or an unauthenticated
	// full-access client (DumboDB, until it implements user management).
	Admin *mongo.Client
	// BaseURI is the credential-free URI for dialing as a specific user via
	// ConnectAs.
	BaseURI string
	// NS is a unique-per-test token for naming databases/users/roles.
	NS string
}

// AuthPairTest runs tc against both servers as admin, comparing per Support.
// It skips when the auth suite is disabled (PARITY_AUTH unset).
func AuthPairTest(t *testing.T, tc AuthCase) TestResult {
	t.Helper()
	RequireAuth(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ac, err := GetAuthClients(ctx)
	if err != nil {
		t.Fatalf("AuthPairTest %s: get auth clients: %v", tc.Name, err)
	}

	ns := authNS(tc.Name)
	mongoTarget := AuthTarget{Admin: ac.MongoAdmin, BaseURI: AuthMongoBaseURI(), NS: ns}
	dumboTarget := AuthTarget{Admin: ac.DumboDBAdmin, BaseURI: AuthDumboDBBaseURI(), NS: ns}

	switch tc.Support {
	case DumboDBMongoOnly:
		_, mErr := tc.Run(ctx, mongoTarget)
		if mErr != nil {
			t.Logf("MONGO_ONLY %s: mongo error: %v", tc.Name, mErr)
			return TestResult{Name: tc.Name, Status: StatusFail, Diff: mErr.Error()}
		}
		t.Logf("MONGO_ONLY %s: OK (DumboDB skipped)", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusSkip}

	case DumboDBFull:
		mRes, mErr := tc.Run(ctx, mongoTarget)
		dRes, dErr := tc.Run(ctx, dumboTarget)
		cmp := CompareResponses(mRes, mErr, dRes, dErr)
		if cmp.Result == Match {
			t.Logf("FULL %s: PASS", tc.Name)
			return TestResult{Name: tc.Name, Status: StatusPass}
		}
		t.Errorf("FULL %s: DIVERGE\n%s", tc.Name, cmp.Diff)
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: cmp.Diff}

	case DumboDBXFail:
		mRes, mErr := tc.Run(ctx, mongoTarget)
		dRes, dErr := tc.Run(ctx, dumboTarget)
		cmp := CompareResponses(mRes, mErr, dRes, dErr)
		if cmp.Result == Match {
			t.Logf("XFAIL %s: PASS (DumboDB matched) -- consider promoting to DumboDBFull", tc.Name)
			return TestResult{Name: tc.Name, Status: StatusPass}
		}
		t.Logf("XFAIL %s: diverged as expected\n%s", tc.Name, cmp.Diff)
		return TestResult{Name: tc.Name, Status: StatusXFail, Diff: cmp.Diff}

	case DumboDBDeviates:
		mRes, mErr := tc.Run(ctx, mongoTarget)
		if tc.MongoExpect != nil {
			tc.MongoExpect(t, mRes, mErr)
		}
		dRes, dErr := tc.Run(ctx, dumboTarget)
		if tc.DumboExpect != nil {
			tc.DumboExpect(t, dRes, dErr)
		}
		if t.Failed() {
			return TestResult{Name: tc.Name, Status: StatusFail}
		}
		t.Logf("DEVIATE %s: MongoDB and DumboDB behave as intended", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusDeviate}

	default:
		t.Fatalf("AuthPairTest %s: unknown DumboDBSupport level %d", tc.Name, tc.Support)
		return TestResult{Name: tc.Name, Status: StatusFail}
	}
}

// authNS derives a filesystem/db-safe unique token from a test name.
func authNS(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
