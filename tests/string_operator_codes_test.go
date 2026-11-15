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

// The string operators' input errors carry the classic engine's codes in
// $project and SBE's in a $group mongod runs in SBE.
func TestStringOperatorCodes(t *testing.T) {
	exprs := map[string]bson.D{
		"ConcatNumber":       {{Key: "$concat", Value: bson.A{"$s", "$v"}}},
		"StrLenBytesNumber":  {{Key: "$strLenBytes", Value: "$v"}},
		"StrLenBytesNull":    {{Key: "$strLenBytes", Value: "$n"}},
		"StrLenBytesMissing": {{Key: "$strLenBytes", Value: "$missing"}},
		"TrimInputNumber":    {{Key: "$trim", Value: bson.D{{Key: "input", Value: "$v"}}}},
		"TrimCharsNumber":    {{Key: "$trim", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "chars", Value: "$v"}}}},
		"LtrimCharsNumber":   {{Key: "$ltrim", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "chars", Value: "$v"}}}},
		"TrimCharsNull":      {{Key: "$trim", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "chars", Value: "$n"}}}},
	}
	for name, expr := range exprs {
		for _, form := range []string{"Project", "Group"} {
			stage := bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: expr}}}}
			if form == "Group" {
				stage = bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: expr}}}}}}
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "StringOperatorCodes_" + form + "_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "ab"}, {Key: "v", Value: int32(5)}, {Key: "n", Value: nil}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col, stage)
				},
			})
		}
	}
}
