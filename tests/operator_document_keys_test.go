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

// mongod refuses a document of operators that also holds a key that is not an
// operator (2 "unknown operator"), in a query and in a partial filter.
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
				Support: supportFor(t, operatorDocumentNonOperatorKeysXFail),
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
				Support: supportFor(t, operatorDocumentNonOperatorKeysXFail),
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

// operatorDocumentNonOperatorKeysXFail lists the cases DumboDB fails until it
// refuses non-operator keys in a document of operators.
var operatorDocumentNonOperatorKeysXFail = map[string]bool{
	"TestOperatorDocumentNonOperatorKeys/embedded_document_operand_find":          true,
	"TestOperatorDocumentNonOperatorKeys/embedded_document_operand_partial_index": true,
	"TestOperatorDocumentNonOperatorKeys/key_before_dollar_find":                  true,
	"TestOperatorDocumentNonOperatorKeys/key_before_dollar_partial_index":         true,
	"TestOperatorDocumentNonOperatorKeys/letter_key_find":                         true,
	"TestOperatorDocumentNonOperatorKeys/letter_key_partial_index":                true,
	"TestOperatorDocumentNonOperatorKeys/space_key_find":                          true,
	"TestOperatorDocumentNonOperatorKeys/space_key_partial_index":                 true,
	"TestOperatorDocumentNonOperatorKeys/unknown_operator_partial_index":          true,
}
