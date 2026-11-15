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

// TestUnwindNulPath: a $unwind path with a null byte anywhere is refused
// before its field names are checked.
func TestUnwindNulPath(t *testing.T) {
	cases := map[string]string{
		"string":        `{"$unwind":"$p\u0000q"}`,
		"document":      `{"$unwind":{"path":"$p\u0000q"}}`,
		"only_nul":      `{"$unwind":"$\u0000"}`,
		"empty_part":    `{"$unwind":"$a..\u0000"}`,
		"dollar_part":   `{"$unwind":"$a\u0000.$b"}`,
		"variable":      `{"$unwind":"$$a\u0000"}`,
		"no_prefix":     `{"$unwind":"p\u0000q"}`,
		"index_nul":     `{"$unwind":{"path":"$p","includeArrayIndex":"a\u0000b"}}`,
		"plain_ok_path": `{"$unwind":"$p"}`,
	}
	for name, stage := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(stage), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "UnwindNulPath_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "p", Value: bson.A{1, 2}}})
				return err
			},
			Run: pipelineRun(bson.A{s, bson.D{{Key: "$sort", Value: bson.D{{Key: "p", Value: 1}}}}}),
		})
	}
}
