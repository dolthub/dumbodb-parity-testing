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

// TestStagePathArguments: $densify paths and $merge 'on' fields are field
// paths, refused with mongod's FieldPath codes (a NUL byte gives 16411), and
// a $out or $merge target with a NUL byte in its name gives InvalidNamespace.
func TestStagePathArguments(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}, {Key: "k", Value: "a"}})
		return err
	}
	densify := func(field string, partitions ...any) bson.D {
		spec := bson.D{{Key: "field", Value: field}}
		if partitions != nil {
			spec = append(spec, bson.E{Key: "partitionByFields", Value: bson.A(partitions)})
		}
		spec = append(spec, bson.E{Key: "range", Value: bson.D{{Key: "step", Value: 1}, {Key: "bounds", Value: "full"}}})
		return bson.D{{Key: "$densify", Value: spec}}
	}
	merge := func(on any) bson.D {
		return bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: "m"}, {Key: "on", Value: on}}}}
	}
	cases := map[string]bson.D{
		"densify_field_nul":       densify("p\x00q"),
		"densify_partition_nul":   densify("x", "p\x00q"),
		"densify_field_empty":     densify(""),
		"densify_field_dots":      densify("a..b"),
		"densify_field_dollar":    densify("$x"),
		"merge_on_dollar":         merge("$a"),
		"merge_on_dots":           merge("a..b"),
		"merge_on_trailing_dot":   merge("a."),
		"merge_on_empty":          merge(""),
		"merge_on_array_nul":      merge(bson.A{"k", "p\x00q"}),
		"out_coll_nul":            {{Key: "$out", Value: "o\x00p"}},
		"out_db_nul":              {{Key: "$out", Value: bson.D{{Key: "db", Value: "pr\x00obe"}, {Key: "coll", Value: "op"}}}},
		"merge_into_nul":          {{Key: "$merge", Value: bson.D{{Key: "into", Value: "o\x00p"}}}},
		"merge_into_doc_coll_nul": {{Key: "$merge", Value: bson.D{{Key: "into", Value: bson.D{{Key: "db", Value: "probe"}, {Key: "coll", Value: "o\x00p"}}}}}},
		"merge_into_doc_db_nul":   {{Key: "$merge", Value: bson.D{{Key: "into", Value: bson.D{{Key: "db", Value: "pr\x00obe"}, {Key: "coll", Value: "op"}}}}}},
	}
	for name, stage := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "StagePathArguments_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(bson.A{stage}),
		})
	}
}
