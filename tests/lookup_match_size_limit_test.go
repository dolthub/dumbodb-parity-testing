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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod fails a $lookup whose matches for one input document total more than
// 100MB (internalLookupStageIntermediateDocumentMaxSizeBytes), unless an
// $unwind of the "as" field directly follows and is absorbed into the join.

const lookupLimitForeignDocs = 26

func lookupLimitSeed(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: int32(2)}},
	}); err != nil {
		return err
	}
	foreign := col.Database().Collection(col.Name() + "_f")
	pad := strings.Repeat("x", 4_100_000)
	for i := 0; i < lookupLimitForeignDocs; i++ {
		doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: int32(1)}, {Key: "pad", Value: pad}}
		if _, err := foreign.InsertOne(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}

func lookupLimitStage(col *mongo.Collection, pipeline bson.A) bson.D {
	spec := bson.D{
		{Key: "from", Value: col.Name() + "_f"},
		{Key: "localField", Value: "k"},
		{Key: "foreignField", Value: "k"},
	}
	if pipeline != nil {
		spec = append(spec, bson.E{Key: "pipeline", Value: pipeline})
	}
	return bson.D{{Key: "$lookup", Value: append(spec, bson.E{Key: "as", Value: "j"})}}
}

func TestLookupMatchSizeLimit(t *testing.T) {
	sizeOfJ := bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}, {Key: "j", Value: 1}}}}
	cases := []struct {
		name  string
		build func(col *mongo.Collection) []bson.D
	}{
		{"LookupLimit_Equality", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil), sizeOfJ}
		}},
		{"LookupLimit_EqualityRaw", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, nil)}
		}},
		{"LookupLimit_SubPipeline", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, bson.A{}), sizeOfJ, byID}
		}},
		{"LookupLimit_SubPipelineUnderLimit", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupLimitStage(col, bson.A{bson.D{{Key: "$limit", Value: int32(3)}}}), sizeOfJ, byID}
		}},
		{"LookupLimit_EqualityUnwound", func(col *mongo.Collection) []bson.D {
			return []bson.D{
				lookupLimitStage(col, nil),
				{{Key: "$unwind", Value: "$j"}},
				{{Key: "$project", Value: bson.D{{Key: "j", Value: "$j._id"}}}},
				byID,
			}
		}},
		{"LookupLimit_UnionWithUnlimited", func(col *mongo.Collection) []bson.D {
			return []bson.D{
				{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name() + "_f"}, {Key: "pipeline", Value: bson.A{}}}}},
				{{Key: "$project", Value: bson.D{{Key: "k", Value: 1}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
		}},
		{"LookupLimit_SubPipelineUnwoundDocForm", func(col *mongo.Collection) []bson.D {
			return []bson.D{
				lookupLimitStage(col, bson.A{}),
				{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$j"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}},
				{{Key: "$project", Value: bson.D{{Key: "j", Value: "$j._id"}}}},
				byID,
			}
		}},
	}
	for _, c := range cases {
		build := c.build
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   lookupLimitSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, build(col)...)
			},
		})
	}
}

// The sub-pipeline form refuses a further match only once the matches so far
// pass the limit, so 105 one-megabyte matches pass and 106 do not.
func TestLookupMatchSizeLimit_SubPipelineBoundary(t *testing.T) {
	for _, n := range []int32{105, 106} {
		n := n
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLimit_SubPipelineBoundary_" + map[int32]string{105: "Under", 106: "Over"}[n],
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
					return err
				}
				pad := strings.Repeat("x", 1_000_000)
				foreign := col.Database().Collection(col.Name() + "_f")
				for i := int32(0); i < 106; i++ {
					if _, err := foreign.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "pad", Value: pad}}); err != nil {
						return err
					}
				}
				return nil
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col,
					bson.D{{Key: "$lookup", Value: bson.D{
						{Key: "from", Value: col.Name() + "_f"},
						{Key: "pipeline", Value: bson.A{bson.D{{Key: "$limit", Value: n}}}},
						{Key: "as", Value: "j"},
					}}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}},
				)
			},
		})
	}
}
