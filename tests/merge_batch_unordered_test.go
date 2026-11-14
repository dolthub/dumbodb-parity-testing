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
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMergeBatchUnordered: $merge writes a batch unordered, so a document
// that fails whenMatched: fail, whenNotMatched: fail or an _id change is
// skipped while the batch's other documents are written, and the first
// failure is reported.
func TestMergeBatchUnordered(t *testing.T) {
	cases := map[string]struct {
		spec   string
		target []string
	}{
		"NotMatchedFailMerge":    {`{"whenNotMatched":"fail"}`, []string{`{"_id":1}`, `{"_id":3}`}},
		"NotMatchedFailReplace":  {`{"whenMatched":"replace","whenNotMatched":"fail"}`, []string{`{"_id":1}`, `{"_id":3}`}},
		"NotMatchedFailPipeline": {`{"whenMatched":[{"$set":{"a":1}}],"whenNotMatched":"fail"}`, []string{`{"_id":1,"x":1}`, `{"_id":2,"x":2}`}},
		"MatchedFail":            {`{"whenMatched":"fail"}`, []string{`{"_id":2}`}},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeBatchUnordered_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(10)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(20)}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: int32(30)}},
				})
				return err
			},
			Run: mergePipelineRun(tc.spec, tc.target),
		})
	}
}
