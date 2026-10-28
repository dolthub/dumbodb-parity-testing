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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An anchored regex with a literal prefix is answered from the index range
// holding that prefix; the results must match a collection scan's.

func regexPrefixSetup(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 0, 120)
	for i := 0; i < 100; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: fmt.Sprintf("s%03d", i)}})
	}
	extra := []interface{}{
		primitive.Regex{Pattern: "^s05"},
		primitive.Regex{Pattern: "^s05", Options: "i"},
		bson.A{"x", "s050x"},
		bson.A{primitive.Regex{Pattern: "^s05"}},
		"s05\x00z",
		"S050",
		"s05.",
		"a.b",
		"axb",
		"\xc3\xa9t\xc3\xa9",
		"line\ns050",
		int32(5),
		nil,
	}
	for i, v := range extra {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(100 + i)}, {Key: "s", Value: v}})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	return createIndex(ctx, col, bson.D{{Key: "s", Value: 1}})
}

func TestIndex_RegexPrefix(t *testing.T) {
	byID := bson.D{{Key: "_id", Value: 1}}
	re := func(pattern, opts string) bson.D {
		return bson.D{{Key: "s", Value: primitive.Regex{Pattern: pattern, Options: opts}}}
	}
	op := func(pattern, opts string) bson.D {
		d := bson.D{{Key: "$regex", Value: pattern}}
		if opts != "" {
			d = append(d, bson.E{Key: "$options", Value: opts})
		}
		return bson.D{{Key: "s", Value: d}}
	}
	for _, tc := range []struct {
		name   string
		filter bson.D
		sort   bson.D
		limit  int64
	}{
		{"Bare", re("^s05", ""), byID, 0},
		{"Operator", op("^s05", ""), byID, 0},
		{"DotAll", op("^s05.", "s"), byID, 0},
		{"CaseInsensitive", re("^s05", "i"), byID, 0},
		{"Multiline", re("^s05", "m"), byID, 0},
		{"EscapedDot", re(`^a\.b`, ""), byID, 0},
		{"OptionalLast", re("^s05?0", ""), byID, 0},
		{"Exact", re("^s050$", ""), byID, 0},
		{"Unicode", re("^\xc3\xa9t", ""), byID, 0},
		{"NoMatch", re("^zzz", ""), byID, 0},
		{"SortLimit", re("^s0", ""), bson.D{{Key: "s", Value: -1}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_RegexPrefix_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   regexPrefixSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetSort(tc.sort).SetProjection(bson.D{{Key: "_id", Value: 1}})
					if tc.limit > 0 {
						opts.SetLimit(tc.limit)
					}
					cursor, err := col.Find(ctx, tc.filter, opts)
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
}
