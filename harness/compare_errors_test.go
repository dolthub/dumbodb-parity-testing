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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func cmdErr(code int32, name, msg string) mongo.CommandError {
	return mongo.CommandError{Code: code, Name: name, Message: msg}
}

// TestCompareResponses_ErrorIdentity verifies the comparator keys on error
// code and codeName (not just ok:0 or message text): same code+name matches,
// differing code or codeName diverges.
func TestCompareResponses_ErrorIdentity(t *testing.T) {
	cases := []struct {
		name       string
		mongo      error
		dumbo      error
		wantResult CompareResult
		diffSubstr string
	}{
		{
			name:       "same_code_name_message_matches",
			mongo:      cmdErr(13, "Unauthorized", "not authorized"),
			dumbo:      cmdErr(13, "Unauthorized", "not authorized"),
			wantResult: Match,
		},
		{
			name:       "different_code_diverges",
			mongo:      cmdErr(13, "Unauthorized", "not authorized"),
			dumbo:      cmdErr(18, "AuthenticationFailed", "auth failed"),
			wantResult: Diverge,
			diffSubstr: "code",
		},
		{
			name:       "same_code_different_codeName_diverges",
			mongo:      cmdErr(13, "Unauthorized", "x"),
			dumbo:      cmdErr(13, "SomeOtherName", "x"),
			wantResult: Diverge,
			diffSubstr: "codeName",
		},
		{
			name:       "error_on_one_side_only_diverges",
			mongo:      cmdErr(11, "UserNotFound", "no user"),
			dumbo:      nil,
			wantResult: Diverge,
		},
		{
			name:       "both_nil_matches",
			mongo:      nil,
			dumbo:      nil,
			wantResult: Match,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareResponses(nil, tc.mongo, nil, tc.dumbo)
			if got.Result != tc.wantResult {
				t.Fatalf("Result = %v, want %v (diff: %s)", got.Result, tc.wantResult, got.Diff)
			}
			if tc.diffSubstr != "" && !strings.Contains(got.Diff, tc.diffSubstr) {
				t.Fatalf("Diff = %q, want it to contain %q", got.Diff, tc.diffSubstr)
			}
		})
	}
}
