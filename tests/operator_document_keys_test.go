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

// TestOperatorDocumentNonOperatorKeys: mongod refuses a document of operators
// that also holds a key that is not an operator (2 "unknown operator"), in a
// query and in a partial filter.
func TestOperatorDocumentNonOperatorKeys(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	for _, c := range []struct {
		name   string
		filter bson.D
	}{
		{"key_before_dollar", d("a", d("$eq", 1, "#y", 1))},
		{"space_key", d("a", d("$gt", 1, " ", 1))},
		{"letter_key", d("a", d("$eq", 1, "y", 1))},
		{"embedded_document_operand", d("a", d("$eq", d("x", 1), "y", 1))},
		{"unknown_operator", d("a", d("$x", 1, "!b", 1))},
	} {
		t.Run(c.name+"_find", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "OperatorDocumentNonOperatorKeys_" + c.name + "_find",
				Support: harness.DumboDBFull,
				Setup:   insertDocs(d("_id", 1, "a", 1)),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					_, err := findIDs(ctx, col, c.filter)
					return errCode(err), nil
				},
			})
		})
		t.Run(c.name+"_partial_index", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "OperatorDocumentNonOperatorKeys_" + c.name + "_partial_index",
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
						Keys:    d("k", 1),
						Options: options.Index().SetPartialFilterExpression(c.filter),
					})
					return errCode(err), nil
				},
			})
		})
	}
}

// TestOperatorDocumentPlainKeyFirst: a document whose first key is not an
// operator is a value to equal, even when a later key starts with '$'.
func TestOperatorDocumentPlainKeyFirst(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	seed := insertDocs(d("_id", 1, "a", 1), d("_id", 2, "a", d("y", 1, "$eq", 1)))
	for _, c := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"eq", d("a", d("y", 1, "$eq", 1)), harness.DumboDBXFail},
		{"gt", d("a", d("y", 1, "$gt", 0)), harness.DumboDBXFail},
		{"under_and", d("$and", bson.A{d("a", d("y", 1, "$eq", 1))}), harness.DumboDBXFail},
		{"under_elem_match", d("a", d("$elemMatch", d("y", 1, "$eq", 1))), harness.DumboDBFull},
	} {
		t.Run(c.name+"_find", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "OperatorDocumentPlainKeyFirst_" + c.name + "_find",
				Support: c.support,
				Setup:   seed,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					ids, err := findIDs(ctx, col, c.filter)
					return bson.A{ids, errCode(err)}, nil
				},
			})
		})
		t.Run(c.name+"_delete", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "OperatorDocumentPlainKeyFirst_" + c.name + "_delete",
				Support: c.support,
				Setup:   seed,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					res, err := col.DeleteMany(ctx, c.filter)
					var n int64
					if res != nil {
						n = res.DeletedCount
					}
					return bson.A{n, errCode(err)}, nil
				},
			})
		})
	}
	t.Run("partial_index", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "OperatorDocumentPlainKeyFirst_partial_index",
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
					Keys:    d("k", 1),
					Options: options.Index().SetPartialFilterExpression(d("a", d("y", 1, "$eq", 1))),
				})
				return errCode(err), nil
			},
		})
	})
}
