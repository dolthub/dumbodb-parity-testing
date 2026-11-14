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
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestFacetQueryStatsLaterBranch: off admin, a $queryStats in a $facet
// branch fails only after the stages ahead of it, in earlier branches too,
// have parsed.
func TestFacetQueryStatsLaterBranch(t *testing.T) {
	cases := map[string]string{
		"limit_earlier_branch":   `{"$facet":{"a":[{"$limit":-1}],"b":[{"$queryStats":{}}]}}`,
		"limit_same_branch":      `{"$facet":{"b":[{"$limit":-1},{"$queryStats":{}}]}}`,
		"limit_after":            `{"$facet":{"b":[{"$queryStats":{}},{"$limit":-1}]}}`,
		"lookup_earlier_branch":  `{"$facet":{"a":[{"$lookup":{"from":"x"}}],"b":[{"$queryStats":{}}]}}`,
		"match_earlier_branch":   `{"$facet":{"a":[{"$match":{"$bad":1}}],"b":[{"$queryStats":{}}]}}`,
		"valid_earlier_branch":   `{"$facet":{"a":[{"$limit":1}],"b":[{"$queryStats":{}}]}}`,
		"project_earlier_branch": `{"$facet":{"a":[{"$project":{}}],"b":[{"$queryStats":{}}]}}`,
	}
	for name, stage := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(stage), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "FacetQueryStatsLaterBranch_" + name,
			Support: harness.DumboDBFull,
			Run:     pipelineRun(bson.A{s}),
		})
	}
}
