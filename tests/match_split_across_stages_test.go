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

// TestMatchSplitAcrossUnwind: mongod moves the conditions of a $match that
// do not read an $unwind's path or index field ahead of the $unwind, so they
// run first on every document, and an error there is not masked by a
// condition on the unwound field.
func TestMatchSplitAcrossUnwind(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{int32(1), int32(2)}}, {Key: "v", Value: int32(9)}},
		})
		return err
	}
	sizeIs2 := func(path string) bson.D {
		return bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$size", Value: path}}, int32(2)}}}}}
	}
	vNot9 := bson.D{{Key: "v", Value: bson.D{{Key: "$ne", Value: int32(9)}}}}
	unwindV := bson.D{{Key: "$unwind", Value: "$v"}}
	match := func(conds ...bson.D) bson.D {
		filter := bson.D{}
		for _, c := range conds {
			filter = append(filter, c...)
		}
		return bson.D{{Key: "$match", Value: filter}}
	}
	cases := map[string]bson.A{
		"other_field":      {unwindV, match(vNot9, sizeIs2("$a"))},
		"unwound_field":    {unwindV, match(vNot9, sizeIs2("$v"))},
		"root_path":        {unwindV, match(vNot9, sizeIs2("$$ROOT.a"))},
		"current_path":     {unwindV, match(vNot9, sizeIs2("$$CURRENT.a"))},
		"explicit_and":     {unwindV, bson.D{{Key: "$match", Value: bson.D{{Key: "$and", Value: bson.A{vNot9, sizeIs2("$a")}}}}}},
		"or_reads_unwound": {unwindV, bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{vNot9, sizeIs2("$a")}}}}}},
		"index_field": {bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$v"}, {Key: "includeArrayIndex", Value: "a"}}}},
			match(vNot9, sizeIs2("$a"))},
		"dotted_path":  {bson.D{{Key: "$unwind", Value: "$v.x"}}, match(vNot9, sizeIs2("$a"))},
		"sibling_path": {unwindV, match(bson.D{{Key: "v.y", Value: bson.D{{Key: "$ne", Value: int32(9)}}}}, sizeIs2("$a"))},
		"after_project": {bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}, {Key: "v", Value: 1}}}}, unwindV,
			match(vNot9, sizeIs2("$a"))},
		"keeps_results": {unwindV, match(bson.D{{Key: "v", Value: int32(9)}}, bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}),
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}},
	}
	for name, pipeline := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MatchSplitAcrossUnwind_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
