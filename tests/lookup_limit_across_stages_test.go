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

// TestLookupLimitAcrossStages: an $unwind of a $lookup's "as" is absorbed
// across a $match whose $jsonSchema does not read "as", so the 100MB limit on
// one document's matches does not apply. A $group on _id that keeps one value
// per group, with _id written as "$$CURRENT._id", "$$ROOT._id" or an object of
// one _id path, is rewritten by mongod as for "$_id". A schema name reads the
// field before its first ".".
func TestLookupLimitAcrossStages(t *testing.T) {
	count := bson.D{{Key: "$count", Value: "n"}}
	unwind := bson.D{{Key: "$unwind", Value: "$j"}}
	schema := func(s bson.D) bson.D {
		return bson.D{{Key: "$match", Value: bson.D{{Key: "$jsonSchema", Value: s}}}}
	}
	sizeOfJ := bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}}
	groupBy := func(id any) bson.D {
		return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: id}, {Key: "k", Value: bson.D{{Key: "$first", Value: "$k"}}}}}}
	}
	cases := map[string]func(col *mongo.Collection) []bson.D{
		"schema_required_other": func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), schema(bson.D{{Key: "required", Value: bson.A{"k"}}}), unwind, count}
		},
		"schema_properties_other": func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), schema(bson.D{{Key: "properties", Value: bson.D{{Key: "k", Value: bson.D{{Key: "bsonType", Value: "int"}}}}}}), unwind, count}
		},
		"schema_required_as": func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), schema(bson.D{{Key: "required", Value: bson.A{"j"}}}), unwind, count}
		},
		"schema_required_as_path": func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), schema(bson.D{{Key: "required", Value: bson.A{"j.x"}}}), unwind, count}
		},
		"schema_properties_as_path": func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), schema(bson.D{{Key: "properties", Value: bson.D{{Key: "j.x", Value: bson.D{{Key: "bsonType", Value: "int"}}}}}}), unwind, count}
		},
		"group_object_id_two_paths": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.D{{Key: "i", Value: "$_id"}, {Key: "j", Value: "$$ROOT._id"}}), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_object_id_same_path_twice": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.D{{Key: "a", Value: "$_id"}, {Key: "b", Value: "$_id"}}), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_current_id": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy("$$CURRENT._id"), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_root_id": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy("$$ROOT._id"), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_object_id": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.D{{Key: "i", Value: "$_id"}}), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_add_id": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.D{{Key: "$add", Value: bson.A{"$_id", 0}}}), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_array_id": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.A{"$_id"}), lookupLimitStage(col, nil), sizeOfJ}
		},
		"group_object_id_and_k": func(col *mongo.Collection) []bson.D {
			return []bson.D{groupBy(bson.D{{Key: "i", Value: "$_id"}, {Key: "k", Value: "$k"}}), lookupLimitStage(col, nil), sizeOfJ}
		},
	}
	for name, build := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLimitAcrossStages_" + name,
			Support: harness.DumboDBFull,
			Setup:   lookupLimitSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, append(build(col), bson.D{{Key: "$sort", Value: bson.D{{Key: "s", Value: 1}}}})...)
			},
		})
	}
}

// TestLookupMatchSplit: mongod moves the conditions of a $match after a
// $lookup that do not read "as" ahead of it. A $jsonSchema is split by
// field; a moved property constraint makes the $lookup run in the classic
// engine, without SBE's limit on one document's matches, while a moved
// required field does not. Whole $match stages that move keep their form.
func TestLookupMatchSplit(t *testing.T) {
	unwind := bson.D{{Key: "$unwind", Value: "$j"}}
	count := bson.D{{Key: "$count", Value: "n"}}
	sizeOfJ := bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}}
	match := func(filter bson.D) bson.D { return bson.D{{Key: "$match", Value: filter}} }
	schema := func(s bson.D) bson.E { return bson.E{Key: "$jsonSchema", Value: s} }
	propK := func(s bson.D) bson.E { return bson.E{Key: "properties", Value: bson.D{{Key: "k", Value: s}}} }
	noOthers := bson.E{Key: "additionalProperties", Value: false}
	jSize := bson.E{Key: "j", Value: bson.D{{Key: "$size", Value: int32(26)}}}
	cases := []struct {
		name  string
		build func(col *mongo.Collection) []bson.D
	}{
		{"match_other", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{{Key: "k", Value: int32(2)}}), sizeOfJ}
		}},
		{"match_other_and_as", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{{Key: "k", Value: int32(2)}, {Key: "j", Value: bson.D{{Key: "$size", Value: int32(0)}}}}), sizeOfJ}
		}},
		{"schema_property_and_no_others", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{propK(bson.D{{Key: "bsonType", Value: "int"}}), noOthers})}), unwind, count}
		}},
		{"schema_unconstrained_property_and_no_others", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{propK(bson.D{}), noOthers})}), unwind, count}
		}},
		{"schema_required_and_no_others", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{{Key: "required", Value: bson.A{"k"}}, noOthers})}), unwind, count}
		}},
		{"schema_required_as_and_property", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{{Key: "required", Value: bson.A{"j"}}, propK(bson.D{{Key: "bsonType", Value: "int"}})})}), unwind, count}
		}},
		{"schema_property_and_as", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{propK(bson.D{{Key: "enum", Value: bson.A{1, 2}}})}), jSize}), sizeOfJ}
		}},
		{"schema_required_and_as", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{{Key: "required", Value: bson.A{"k"}}}), jSize}), sizeOfJ}
		}},
		{"schema_required_whole", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), match(bson.D{schema(bson.D{{Key: "required", Value: bson.A{"k"}}})}), sizeOfJ}
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "LookupMatchSplit",
		Support: harness.DumboDBFull,
		Setup:   lookupLimitSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.M{}
			for _, tc := range cases {
				res, err := fieldOpsAgg(ctx, col, append(tc.build(col), bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})...)
				if err != nil {
					res = errOutcome(err)
				}
				out[tc.name] = res
			}
			return out, nil
		},
	})
}
