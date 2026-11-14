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
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A hinted partial index takes the query's bounds only when the query implies
// the partialFilterExpression (mongod's isSubsetOf); otherwise mongod scans
// the whole index, [MinKey, MaxKey], and filters the documents it fetches.
// On a multikey field the two scans reach documents in different orders.

const partialHintOrderIndexName = "partial_hint_order_idx"

func partialHintOrderSetup(keys, pfe bson.D) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{time.UnixMilli(1000).UTC(), nil}}, {Key: "b", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{nil, bson.A{nil}, math.NaN()}}, {Key: "b", Value: true}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "b", Value: false}},
			bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: int32(2)}, {Key: "b", Value: "x"}},
			bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.A{int32(5), int32(-1)}}, {Key: "b", Value: nil}},
			bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: "s"}, {Key: "b", Value: int32(2)}},
			bson.D{{Key: "_id", Value: int32(8)}, {Key: "a", Value: bson.A{int32(0), "t", int32(7)}}, {Key: "b", Value: int32(3)}},
			bson.D{{Key: "_id", Value: int32(9)}, {Key: "a", Value: bson.A{int32(3), int32(-2)}}, {Key: "b", Value: int32(4)}},
			bson.D{{Key: "_id", Value: int32(10)}, {Key: "a", Value: bson.A{int32(4), "u", nil}}, {Key: "b", Value: nil}},
			bson.D{{Key: "_id", Value: int32(11)}, {Key: "a", Value: bson.A{math.NaN(), "N", int32(1)}}, {Key: "b", Value: nil}},
			bson.D{{Key: "_id", Value: int32(12)}, {Key: "a", Value: math.NaN()}, {Key: "b", Value: nil}},
		})
		if err != nil {
			return err
		}
		_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    keys,
			Options: options.Index().SetPartialFilterExpression(pfe).SetName(partialHintOrderIndexName),
		})
		return err
	}
}

func partialHintOrderRun(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		ids, err := cluFindIDs(filter, options.Find().SetHint(partialHintOrderIndexName))(ctx, col)
		if err != nil {
			return nil, err
		}
		var updated bson.D
		err = col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "m", Value: int32(1)}}}},
			options.FindOneAndUpdate().SetHint(partialHintOrderIndexName).SetProjection(bson.D{{Key: "_id", Value: 1}})).
			Decode(&updated)
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
		return bson.D{{Key: "find", Value: ids}, {Key: "findAndModify", Value: updated}}, nil
	}
}

type partialHintOrderCase struct {
	name    string
	filter  bson.D
	support harness.DumboDBSupport
}

func runPartialHintOrderCases(t *testing.T, prefix string, keys, pfe bson.D, cases []partialHintOrderCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + tc.name,
				Support: tc.support,
				Setup:   partialHintOrderSetup(keys, pfe),
				Run:     partialHintOrderRun(tc.filter),
			})
		})
	}
}

func cond(op string, v interface{}) bson.D { return bson.D{{Key: op, Value: v}} }

func TestIndex_PartialHintOrder_ExistsDescending(t *testing.T) {
	bExists := bson.D{{Key: "b", Value: cond("$exists", true)}}
	runPartialHintOrderCases(t, "Index_PartialHintOrder_ExistsDescending",
		bson.D{{Key: "a", Value: -1}}, bExists, []partialHintOrderCase{
			{"EqNull", bson.D{{Key: "a", Value: nil}}, harness.DumboDBFull},
			{"Lte", bson.D{{Key: "a", Value: cond("$lte", int32(2))}}, harness.DumboDBFull},
			{"Gte", bson.D{{Key: "a", Value: cond("$gte", int32(0))}}, harness.DumboDBFull},
			{"TypeNumber", bson.D{{Key: "a", Value: cond("$type", "number")}}, harness.DumboDBFull},
			{"BNull", bson.D{{Key: "a", Value: cond("$lte", int32(5))}, {Key: "b", Value: nil}}, harness.DumboDBFull},
			{"BNotNe1", bson.D{{Key: "a", Value: cond("$lte", int32(5))}, {Key: "b", Value: cond("$ne", int32(1))}}, harness.DumboDBFull},
			{"OrBranchWithoutB", bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "a", Value: cond("$gte", int32(3))}, {Key: "b", Value: cond("$gt", int32(0))}},
				bson.D{{Key: "a", Value: cond("$lte", int32(0))}},
			}}}, harness.DumboDBFull},
			// mongod reads $all as equalities, so $all: [null] can match a missing b.
			{"BAllNull", bson.D{{Key: "a", Value: cond("$lte", int32(9))}, {Key: "b", Value: cond("$all", bson.A{nil})}}, harness.DumboDBFull},
			{"ImpliedByAllNullAndOne", bson.D{{Key: "a", Value: cond("$lte", int32(9))}, {Key: "b", Value: cond("$all", bson.A{nil, int32(1)})}}, harness.DumboDBFull},
			{"ImpliedByExists", bson.D{{Key: "a", Value: cond("$lte", int32(5))}, {Key: "b", Value: cond("$exists", true)}}, harness.DumboDBFull},
			{"ImpliedByNeNull", bson.D{{Key: "a", Value: cond("$lte", int32(5))}, {Key: "b", Value: cond("$ne", nil)}}, harness.DumboDBFull},
			{"ImpliedByType", bson.D{{Key: "a", Value: cond("$lte", int32(5))}, {Key: "b", Value: cond("$type", "number")}}, harness.DumboDBFull},
			{"ImpliedByRange", bson.D{{Key: "a", Value: cond("$gte", int32(0))}, {Key: "b", Value: cond("$gt", int32(0))}}, harness.DumboDBFull},
			{"ImpliedUnderAnd", bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "a", Value: cond("$gte", int32(0))}},
				bson.D{{Key: "b", Value: cond("$in", bson.A{int32(1), int32(3), int32(4)})}},
			}}}, harness.DumboDBFull},
		})
}

func TestIndex_PartialHintOrder_RangeAscending(t *testing.T) {
	aNonNegative := bson.D{{Key: "a", Value: cond("$gte", int32(0))}}
	runPartialHintOrderCases(t, "Index_PartialHintOrder_RangeAscending",
		bson.D{{Key: "a", Value: 1}}, aNonNegative, []partialHintOrderCase{
			{"GteBelow", bson.D{{Key: "a", Value: cond("$gte", int32(-1))}}, harness.DumboDBFull},
			{"Lt", bson.D{{Key: "a", Value: cond("$lt", int32(5))}}, harness.DumboDBFull},
			{"InWithNegative", bson.D{{Key: "a", Value: cond("$in", bson.A{int32(2), int32(-2)})}}, harness.DumboDBFull},
			{"ImpliedGte", bson.D{{Key: "a", Value: cond("$gte", int32(1))}}, harness.DumboDBFull},
			{"ImpliedRange", bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(0)}, {Key: "$lt", Value: int32(6)}}}}, harness.DumboDBFull},
			{"ImpliedIn", bson.D{{Key: "a", Value: cond("$in", bson.A{int32(0), int32(3), int32(5)})}}, harness.DumboDBFull},
		})
}

// mongod compares a string of the query with the partial filter only under a
// shared collation, and then under it. The strings here are lowercase, so
// membership in the partial index does not depend on the collation.
func TestIndex_PartialHintOrder_Collated(t *testing.T) {
	en2 := &options.Collation{Locale: "en", Strength: 2}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(21)}, {Key: "a", Value: bson.A{"q", int32(1)}}},
			bson.D{{Key: "_id", Value: int32(22)}, {Key: "a", Value: "n"}},
			bson.D{{Key: "_id", Value: int32(23)}, {Key: "a", Value: bson.A{"p", "b"}}},
			bson.D{{Key: "_id", Value: int32(24)}, {Key: "a", Value: bson.A{"o", nil}}},
			bson.D{{Key: "_id", Value: int32(25)}, {Key: "a", Value: "a"}},
		})
		if err != nil {
			return err
		}
		_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "a", Value: 1}},
			Options: options.Index().SetCollation(en2).SetName(partialHintOrderIndexName).
				SetPartialFilterExpression(bson.D{{Key: "a", Value: cond("$gte", "m")}}),
		})
		return err
	}
	run := func(filter bson.D, coll *options.Collation) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			findOpts := options.Find().SetHint(partialHintOrderIndexName)
			famOpts := options.FindOneAndUpdate().SetHint(partialHintOrderIndexName).SetProjection(bson.D{{Key: "_id", Value: 1}})
			if coll != nil {
				findOpts.SetCollation(coll)
				famOpts.SetCollation(coll)
			}
			ids, err := cluFindIDs(filter, findOpts)(ctx, col)
			if err != nil {
				return nil, err
			}
			var updated bson.D
			err = col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "m", Value: int32(1)}}}}, famOpts).Decode(&updated)
			if err != nil && err != mongo.ErrNoDocuments {
				return nil, err
			}
			return bson.D{{Key: "find", Value: ids}, {Key: "findAndModify", Value: updated}}, nil
		}
	}
	for _, tc := range []struct {
		name    string
		filter  bson.D
		coll    *options.Collation
		support harness.DumboDBSupport
	}{
		{"SimpleQuery", bson.D{{Key: "a", Value: cond("$gte", "n")}}, nil, harness.DumboDBFull},
		{"SharedCollation", bson.D{{Key: "a", Value: cond("$gte", "n")}}, en2, harness.DumboDBFull},
		{"SharedCollationCaseVariant", bson.D{{Key: "a", Value: cond("$gte", "N")}}, en2, harness.DumboDBFull},
		{"SimpleQueryNumber", bson.D{{Key: "a", Value: cond("$gte", int32(0))}}, nil, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_PartialHintOrder_Collated_" + tc.name,
				Support: tc.support,
				Setup:   setup,
				Run:     run(tc.filter, tc.coll),
			})
		})
	}
}

// Without a hint mongod answers from a partial index whenever the query
// implies its filter expression, by the same rule as for a hinted one.
func TestIndex_PartialUnhinted_Implied(t *testing.T) {
	bExists := bson.D{{Key: "b", Value: cond("$exists", true)}}
	for _, tc := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"ExistsUnderAnd", bson.D{{Key: "a", Value: cond("$gte", int32(-5))}, {Key: "$and", Value: bson.A{bExists}}}, harness.DumboDBFull},
		{"NorNull", bson.D{{Key: "a", Value: cond("$gte", int32(-5))}, {Key: "$nor", Value: bson.A{bson.D{{Key: "b", Value: nil}}}}}, harness.DumboDBFull},
		{"Exists", bson.D{{Key: "a", Value: cond("$gte", int32(-5))}, {Key: "b", Value: cond("$exists", true)}}, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_PartialUnhinted_Implied_" + tc.name,
				Support: tc.support,
				Setup:   partialHintOrderSetup(bson.D{{Key: "a", Value: 1}}, bExists),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					ids, err := cluFindIDs(tc.filter)(ctx, col)
					if err != nil {
						return nil, err
					}
					explain, err := explainRunExplain(ctx, col, cluFindCmd(tc.filter)(col), "queryPlanner")
					if err != nil {
						return nil, err
					}
					qp, _ := lookup(explain, "queryPlanner")
					qpDoc, _ := asDoc(qp)
					plan, _ := lookup(qpDoc, "winningPlan")
					return bson.D{{Key: "find", Value: ids}, {Key: "stages", Value: planStages(plan)}}, nil
				},
			})
		})
	}
}
