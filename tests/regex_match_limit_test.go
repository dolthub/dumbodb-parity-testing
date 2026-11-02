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
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A filter regex that hits mongod's PCRE2 match limit counts as no match,
// so $not, $nin and $nor match the document even though the pattern really
// matches it. DumboDB bounds matches with a wall-clock timeout instead, so the
// same answer would depend on load; it fails the operation.

const catastrophicPattern = `(a+)+b|c`

func insertMatchLimitDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: strings.Repeat("a", 30) + "c"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "aac"}},
	})
	return err
}

// codeOutcome is errOutcome without codeName, which write errors lack.
func codeOutcome(err error) bson.M {
	out := errOutcome(err)
	delete(out, "codeName")
	return out
}

func matchLimitFindIDs(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return codeOutcome(err), nil
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return codeOutcome(err), nil
		}
		ids := make([]interface{}, len(docs))
		for i, d := range docs {
			ids[i] = d["_id"]
		}
		return ids, nil
	}
}

// regexMatchLimitRun runs run on a fresh collection holding the match-limit
// documents.
func regexMatchLimitRun(run func(context.Context, *mongo.Collection) (interface{}, error)) func(context.Context, harness.AuthTarget) (interface{}, error) {
	return func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := tgt.Admin.Database(tgt.NS)
		defer func() { _ = db.Drop(ctx) }()
		col := db.Collection("c")
		if err := insertMatchLimitDocs(ctx, col); err != nil {
			return nil, err
		}
		return run(ctx, col)
	}
}

func expectOutcome(server string, want interface{}) func(*testing.T, interface{}, error) {
	return func(t *testing.T, res interface{}, err error) {
		if err != nil || !reflect.DeepEqual(res, want) {
			t.Errorf("%s: want %v, got %v (err %v)", server, want, res, err)
		}
	}
}

func TestRegexMatchLimit(t *testing.T) {
	re := primitive.Regex{Pattern: catastrophicPattern}
	timedOut := bson.M{"ok": false, "code": int32(51156)}
	filters := map[string]struct {
		filter bson.D
		mongo  []interface{}
	}{
		"Plain": {bson.D{{Key: "s", Value: re}}, []interface{}{int32(2)}},
		"In":    {bson.D{{Key: "s", Value: bson.D{{Key: "$in", Value: bson.A{re}}}}}, []interface{}{int32(2)}},
		"Not":   {bson.D{{Key: "s", Value: bson.D{{Key: "$not", Value: re}}}}, []interface{}{int32(1)}},
		"Nin":   {bson.D{{Key: "s", Value: bson.D{{Key: "$nin", Value: bson.A{re}}}}}, []interface{}{int32(1)}},
		"Nor":   {bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "s", Value: re}}}}}, []interface{}{int32(1)}},
	}
	for name, f := range filters {
		harness.AuthPairTest(t, harness.AuthCase{
			Name:        "RegexMatchLimit_Find" + name,
			Support:     harness.DumboDBDeviates,
			Run:         regexMatchLimitRun(matchLimitFindIDs(f.filter)),
			MongoExpect: expectOutcome("mongo", f.mongo),
			DumboExpect: expectOutcome("dumbodb", timedOut),
		})
	}

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "RegexMatchLimit_DeleteManyNot",
		Support: harness.DumboDBDeviates,
		Run: regexMatchLimitRun(func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.M{}
			if res, err := col.DeleteMany(ctx, filters["Not"].filter); err != nil {
				out["delete"] = codeOutcome(err)
			} else {
				out["deleted"] = res.DeletedCount
			}
			left, err := matchLimitFindIDs(bson.D{})(ctx, col)
			out["left"] = left
			return out, err
		}),
		MongoExpect: expectOutcome("mongo", bson.M{"deleted": int64(1), "left": []interface{}{int32(2)}}),
		DumboExpect: expectOutcome("dumbodb", bson.M{"delete": timedOut, "left": []interface{}{int32(1), int32(2)}}),
	})

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "RegexMatchLimit_InsertValidatorNot",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			validator := bson.D{{Key: "s", Value: bson.D{{Key: "$not", Value: re}}}}
			if err := db.CreateCollection(ctx, "c", options.CreateCollection().SetValidator(validator)); err != nil {
				return nil, err
			}
			col := db.Collection("c")
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: strings.Repeat("a", 30) + "c"}})
			left, ferr := matchLimitFindIDs(bson.D{})(ctx, col)
			return bson.M{"insert": codeOutcome(err), "left": left}, ferr
		},
		MongoExpect: expectOutcome("mongo", bson.M{"insert": bson.M{"ok": true}, "left": []interface{}{int32(1)}}),
		DumboExpect: expectOutcome("dumbodb", bson.M{"insert": timedOut, "left": []interface{}{}}),
	})

	harness.PairTest(t, harness.TestCase{
		Name:    "RegexMatchLimit_ShortInputMatches",
		Support: harness.DumboDBFull,
		Setup:   insertMatchLimitDocs,
		Run:     matchLimitFindIDs(bson.D{{Key: "s", Value: re}, {Key: "_id", Value: int32(2)}}),
	})

	regexMatch := bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "regex", Value: catastrophicPattern}}}}
	harness.PairTest(t, harness.TestCase{
		Name:    "RegexMatchLimit_AggregateRegexMatch",
		Support: harness.DumboDBFull,
		Setup:   insertMatchLimitDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "m", Value: regexMatch}}}},
			})
			if err != nil {
				return errOutcome(err), nil
			}
			var docs []bson.M
			if err := cur.All(ctx, &docs); err != nil {
				return errOutcome(err), nil
			}
			return docs, nil
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "RegexMatchLimit_FindExprRegexMatch",
		Support: harness.DumboDBFull,
		Setup:   insertMatchLimitDocs,
		Run:     matchLimitFindIDs(bson.D{{Key: "$expr", Value: regexMatch}}),
	})
}
