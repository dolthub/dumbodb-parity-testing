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

// TestBucketSpecOrder: mongod checks each $bucket field in the order given:
// an unknown field (40197), an output that is not an object (40196), a
// default that is not constant (40195), a boundary that is not constant
// (40191); constant boundaries and defaults are folded, and one that folds to
// the missing value is no default.
func TestBucketSpecOrder(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}, {Key: "x", Value: int32(3)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(12)}, {Key: "x", Value: int32(4)}},
		})
		return err
	}
	specs := map[string]bson.D{
		"default_field":          {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "default", Value: "$x"}},
		"default_before_types":   {{Key: "default", Value: "$x"}, {Key: "boundaries", Value: bson.A{int32(0), "a"}}, {Key: "groupBy", Value: "$v"}},
		"output_first":           {{Key: "output", Value: int32(5)}, {Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), "a"}}},
		"output_last":            {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "output", Value: int32(5)}},
		"unknown_first":          {{Key: "zz", Value: int32(1)}, {Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), "a"}}},
		"unknown_last":           {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "zz", Value: int32(2)}},
		"boundary_field":         {{Key: "boundaries", Value: bson.A{"$x", int32(5)}}, {Key: "groupBy", Value: "$v"}},
		"boundary_folded":        {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}, int32(20)}}},
		"default_folded":         {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "default", Value: bson.D{{Key: "$concat", Value: bson.A{"o", "x"}}}}},
		"default_document_field": {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "default", Value: bson.D{{Key: "a", Value: "$x"}}}},
		"default_remove":         {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "default", Value: "$$REMOVE"}},
		"boundary_remove":        {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), "$$REMOVE"}}},
		"boundary_divide_zero":   {{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}}}},
	}
	for name, spec := range specs {
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketSpecOrder_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(bson.A{bson.D{{Key: "$bucket", Value: spec}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}),
		})
	}
}
