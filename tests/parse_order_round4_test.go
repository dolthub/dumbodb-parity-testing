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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestParseOrderRound4: mongod parses the whole pipeline first, so a
// malformed $match fails before a later SBE $group's error; $let and $filter
// report a missing or unknown parameter before the others; operators that
// bind a variable are not folded, so a $filter limit never reached does not
// fail; $$REMOVE is a constant missing value that operators read as null;
// $bucketAuto checks its fields in order, buckets' range last.
func TestParseOrderRound4(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}})
		return err
	}
	firstN0 := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: int32(0)}}}}}}}}
	project := func(expr interface{}) bson.A {
		return bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}}}
	}
	shift := func(def interface{}) bson.A {
		return bson.A{bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$shift", Value: bson.D{{Key: "output", Value: "$v"}, {Key: "by", Value: int32(5)}, {Key: "default", Value: def}}}}}}}}}}}
	}
	pipelines := map[string]bson.A{
		"match_undefined_before_group": {bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: primitive.Undefined{}}}}}, firstN0},
		"match_unknown_before_group":   {bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$bogus", Value: int32(1)}}}}}}, firstN0},
		"let_vars_not_object":          project(bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: int32(1)}}}}),
		"let_bad_name_no_in":           project(bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "A", Value: int32(1)}}}}}}),
		"let_no_vars":                  project(bson.D{{Key: "$let", Value: bson.D{{Key: "in", Value: int32(1)}}}}),
		"filter_no_cond":               project(bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: bson.A{int32(1)}}, {Key: "as", Value: "x"}}}}),
		"filter_no_input":              project(bson.D{{Key: "$filter", Value: bson.D{{Key: "cond", Value: true}}}}),
		"filter_limit_never_reached": append(bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: int32(5)}}}}},
			project(bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: bson.A{int32(1)}}, {Key: "cond", Value: true}, {Key: "limit", Value: int32(0)}}}})...),
		"remove_in_operators": {bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0},
			{Key: "a", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), "$$REMOVE"}}}},
			{Key: "u", Value: bson.D{{Key: "$toUpper", Value: "$$REMOVE"}}},
			{Key: "i", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$$REMOVE", int32(1)}}}},
			{Key: "c", Value: bson.D{{Key: "$cond", Value: bson.A{true, "$$REMOVE", int32(1)}}}},
			{Key: "t", Value: bson.D{{Key: "$type", Value: "$$REMOVE"}}},
			{Key: "l", Value: bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "x", Value: "$$REMOVE"}}}, {Key: "in", Value: bson.D{{Key: "$type", Value: "$$x"}}}}}}},
			{Key: "l2", Value: bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "x", Value: "$$REMOVE"}}}, {Key: "in", Value: "$$x"}}}}},
			{Key: "arr", Value: bson.A{"$$REMOVE"}},
		}}}},
		"shift_default_ifnull_remove": shift(bson.D{{Key: "$ifNull", Value: bson.A{"$$REMOVE", int32(1)}}}),
		"shift_default_add_remove":    shift(bson.D{{Key: "$add", Value: bson.A{int32(1), "$$REMOVE"}}}),
		"shift_default_upper_remove":  shift(bson.D{{Key: "$toUpper", Value: "$$REMOVE"}}),
		"bucket_auto_group_by_first":  {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "x"}, {Key: "buckets", Value: int32(0)}}}}},
		"bucket_auto_unknown":         {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "buckets", Value: int32(0)}, {Key: "zz", Value: int32(1)}}}}},
		"bucket_auto_granularity":     {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "buckets", Value: int32(0)}, {Key: "granularity", Value: int32(5)}}}}},
		"bucket_auto_output":          {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "buckets", Value: int32(0)}, {Key: "output", Value: int32(5)}}}}},
		"bucket_auto_buckets_last":    {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "buckets", Value: int32(0)}, {Key: "groupBy", Value: "$v"}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "ParseOrderRound4_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
