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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MongoDB's canonical form replaces an $or of one branch with that branch, so
// a hinted scan of a two-sided range on a multikey field keeps one bound as it
// would without the $or. Here it keeps $gte, which reaches _id 29 at its 0
// key, after the -1 keys, rather than at its null key.

const orSingleBranchIndexName = "or_single_branch_idx"

var orSingleBranchRange = bson.D{{Key: "a", Value: bson.D{
	{Key: "$gte", Value: int32(-1)},
	{Key: "$lt", Value: primitive.MaxKey{}},
}}}

func orSingleBranchSetup(sparse bool) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(20)}, {Key: "a", Value: int32(-1)}},
			bson.D{{Key: "_id", Value: int32(26)}, {Key: "a", Value: int32(-1)}},
			bson.D{{Key: "_id", Value: int32(29)}, {Key: "a", Value: bson.A{nil, true, int32(0)}}},
		})
		if err != nil {
			return err
		}
		_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetSparse(sparse).SetName(orSingleBranchIndexName),
		})
		return err
	}
}

func orSingleBranchRun(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		ids, err := cluFindIDs(filter, options.Find().SetHint(orSingleBranchIndexName))(ctx, col)
		if err != nil {
			return nil, err
		}
		var updated bson.D
		err = col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "m", Value: int32(1)}}}},
			options.FindOneAndUpdate().SetHint(orSingleBranchIndexName).SetProjection(bson.D{{Key: "_id", Value: 1}})).
			Decode(&updated)
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "find", Value: ids}, {Key: "findAndModify", Value: updated}}, nil
	}
}

func TestIndex_OrSingleBranch_HintedRange(t *testing.T) {
	for _, fc := range []struct {
		name   string
		filter bson.D
	}{
		{"Or", bson.D{{Key: "$or", Value: bson.A{orSingleBranchRange}}}},
		{"OrOfAnd", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "$and", Value: bson.A{orSingleBranchRange}}}}}}},
		{"OrTwoFields", bson.D{{Key: "$or", Value: bson.A{append(bson.D{}, orSingleBranchRange[0],
			bson.E{Key: "b", Value: bson.D{{Key: "$exists", Value: false}}})}}}},
		{"AndOfOr", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "$or", Value: bson.A{orSingleBranchRange}}}}}}},
	} {
		for _, sparse := range []bool{false, true} {
			name := fc.name
			if sparse {
				name += "_Sparse"
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Index_OrSingleBranch_HintedRange_" + name,
					Support: harness.DumboDBFull,
					Setup:   orSingleBranchSetup(sparse),
					Run:     orSingleBranchRun(fc.filter),
				})
			})
		}
	}
}
