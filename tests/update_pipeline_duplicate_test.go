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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestUpdatePipelineDuplicate: a field repeated in an update pipeline is
// the statement's write error, so other statements of an unordered batch
// still run; findAndModify reports it as the command's error.
func TestUpdatePipelineDuplicate(t *testing.T) {
	cases := map[string]struct {
		updates string
		ordered bool
	}{
		"set":              {`[{"q":{},"u":[{"$set":{"y":1,"y":2}}]}]`, true},
		"set_second":       {`[{"q":{"_id":1},"u":{"$set":{"z":1}}},{"q":{},"u":[{"$set":{"y":1,"y":2}}]}]`, true},
		"set_nested":       {`[{"q":{},"u":[{"$set":{"y":{"q":1,"q":2}}}]}]`, true},
		"project":          {`[{"q":{},"u":[{"$project":{"y":1,"y":0}}]}]`, true},
		"add_fields_later": {`[{"q":{},"u":[{"$unset":["y"]},{"$addFields":{"y":1,"y":2}}]}]`, true},
		"ordered_stops":    {`[{"q":{},"u":[{"$set":{"y":1,"y":2}}]},{"q":{"_id":2},"u":{"$set":{"z":1}}}]`, true},
		"unordered_goes":   {`[{"q":{},"u":[{"$set":{"y":1,"y":2}}]},{"q":{"_id":2},"u":{"$set":{"z":1}}}]`, false},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 1}}, bson.D{{Key: "_id", Value: 2}}})
		return err
	}
	for name, tc := range cases {
		var updates bson.A
		if err := bson.UnmarshalExtJSON([]byte(tc.updates), false, &updates); err != nil {
			t.Fatal(err)
		}
		ordered := tc.ordered
		harness.PairTest(t, harness.TestCase{
			Name:    "UpdatePipelineDuplicate_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cmd := bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: updates}, {Key: "ordered", Value: ordered}}
				raw, err := col.Database().RunCommand(ctx, cmd).Raw()
				if err != nil && raw == nil {
					return nil, err
				}
				var res bson.M
				if err := bson.Unmarshal(raw, &res); err != nil {
					return nil, err
				}
				if ok, _ := res["ok"].(float64); ok != 1 {
					return nil, err
				}
				codes := bson.A{}
				if we, ok := res["writeErrors"].(bson.A); ok {
					for _, e := range we {
						m := e.(bson.M)
						codes = append(codes, bson.D{{Key: "index", Value: m["index"]}, {Key: "code", Value: m["code"]}})
					}
				}
				cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var docs []bson.M
				if err := cur.All(ctx, &docs); err != nil {
					return nil, err
				}
				return bson.D{{Key: "n", Value: res["n"]}, {Key: "writeErrors", Value: codes}, {Key: "docs", Value: docs}}, nil
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdatePipelineDuplicate_find_and_modify",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var update bson.A
			if err := bson.UnmarshalExtJSON([]byte(`[{"$set":{"y":1,"y":2}}]`), false, &update); err != nil {
				return nil, err
			}
			return runCommandDoc(ctx, col, bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: update}})
		},
	})
}
