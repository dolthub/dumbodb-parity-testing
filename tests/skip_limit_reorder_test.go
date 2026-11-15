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

// TestSkipLimitReorder: mongod moves a $skip ahead of the stages that
// transform each document, a $sort takes the $limit stages after it past the
// stages that keep the document count, and the query layer takes the leading
// $skip and $limit stages from past those stages, so the documents they drop
// are never evaluated. Each pipeline divides by zero for every document it
// evaluates. C names the collection, V a view that divides by zero and VG a
// view that groups.
func TestSkipLimitReorder(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(2)}},
		}); err != nil {
			return err
		}
		db := col.Database()
		divide := mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$divide", Value: bson.A{"$a", 0}}}}}}}}
		if err := db.CreateView(ctx, col.Name()+"_v", col.Name(), divide); err != nil {
			return err
		}
		group := mongo.Pipeline{{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"}, {Key: "a", Value: bson.D{{Key: "$first", Value: "$a"}}}}}}}
		return db.CreateView(ctx, col.Name()+"_vg", col.Name(), group)
	}

	const (
		div      = `{"$divide":["$a",0]}`
		set      = `{"$set":{"x":` + div + `}}`
		group    = `{"$group":{"_id":"$_id","a":{"$first":"$a"}}}`
		lookup   = `{"$lookup":{"from":"C","as":"j","pipeline":[]}}`
		maxInt64 = `{"$numberLong":"9223372036854775807"}`
	)
	shapes := map[string]string{
		"replace_root":        `{"$replaceRoot":{"newRoot":{"x":` + div + `}}},{"$skip":2}`,
		"replace_with":        `{"$replaceWith":{"x":` + div + `}},{"$skip":2}`,
		"project_empty_match": `{"$project":{"x":` + div + `}},{"$match":{}},{"$skip":2}`,
		"set_limit":           set + `,{"$limit":5},{"$skip":2}`,
		"set_limit_short":     set + `,{"$limit":1},{"$skip":2}`,
		"set_skip_limit_skip": set + `,{"$skip":1},{"$limit":5},{"$skip":1}`,
		"set_lookup":          set + `,` + lookup + `,{"$skip":2}`,
		"set_skip":            set + `,{"$skip":2}`,
		"sort_set_limit":      `{"$sort":{"a":1}},` + set + `,{"$limit":5},{"$skip":2}`,
		"sort_set_lookup":     `{"$sort":{"a":1}},` + set + `,` + lookup + `,{"$limit":5},{"$skip":2}`,
		"sort_lookup_set":     `{"$sort":{"a":1}},` + lookup + `,` + set + `,{"$limit":5},{"$skip":2}`,
		"set_sample":          set + `,{"$sample":{"size":5}},{"$skip":2}`,
		"lookup_set_sample":   lookup + `,` + set + `,{"$sample":{"size":5}},{"$skip":2}`,
		"sort_two_limits":     `{"$sort":{"a":1}},{"$set":{"y":1}},{"$skip":1},{"$limit":5},` + lookup + `,{"$limit":9},{"$skip":1},` + set + `,{"$skip":1}`,
	}
	cases := map[string]string{
		"chain":                `[` + set + `,{"$unset":"q"},{"$project":{"q":0}},{"$addFields":{"y":1}},{"$replaceRoot":{"newRoot":"$$ROOT"}},{"$skip":2}]`,
		"match_on_id":          `[{"$project":{"x":` + div + `}},{"$match":{"_id":{"$gt":0}}},{"$skip":2}]`,
		"match_on_dropped":     `[{"$project":{"x":` + div + `}},{"$match":{"a":{"$exists":false}}},{"$skip":2}]`,
		"match_on_computed":    `[` + set + `,{"$match":{"x":{"$exists":true}}},{"$skip":2}]`,
		"graph_lookup":         `[` + set + `,{"$graphLookup":{"from":"C","as":"j","startWith":"$a","connectFromField":"a","connectToField":"b"}},{"$skip":2}]`,
		"lookup_unwind":        `[` + set + `,` + lookup + `,{"$unwind":"$j"},{"$skip":4}]`,
		"lookup_skip_unwind":   `[` + set + `,` + lookup + `,{"$skip":2},{"$unwind":"$j"}]`,
		"set_sort_skip":        `[` + set + `,{"$sort":{"a":1}},{"$skip":2}]`,
		"set_unwind_skip":      `[` + set + `,{"$unwind":"$a"},{"$skip":2}]`,
		"set_redact_skip":      `[` + set + `,{"$redact":"$$KEEP"},{"$skip":2}]`,
		"set_window_skip":      `[` + set + `,{"$setWindowFields":{"output":{"n":{"$sum":1}}}},{"$skip":2}]`,
		"set_set_limit_skip":   `[` + set + `,{"$set":{"y":2}},{"$limit":3},{"$set":{"z":2}},{"$skip":2}]`,
		"limit_limit_skip":     `[` + set + `,{"$limit":1},{"$limit":5},{"$skip":2}]`,
		"project_sort":         `[{"$project":{"a":1}},{"$sort":{"a":1}},` + set + `,{"$limit":5},{"$skip":2}]`,
		"match_sort":           `[{"$sort":{"a":1}},{"$match":{"a":{"$gt":0}}},{"$sort":{"a":-1}},` + set + `,{"$limit":5},{"$skip":2}]`,
		"unwind_first":         `[{"$unwind":"$a"},` + set + `,{"$limit":5},{"$skip":2}]`,
		"graph_lookup_sample":  `[{"$graphLookup":{"from":"C","as":"j","startWith":"$a","connectFromField":"a","connectToField":"b"}},` + set + `,{"$sample":{"size":5}},{"$skip":2}]`,
		"set_sort_sample":      `[` + set + `,{"$sort":{"a":1}},{"$sample":{"size":5}},{"$skip":2}]`,
		"set_limit_sample":     `[{"$group":{"_id":"$_id","a":{"$first":"$a"}}},` + set + `,{"$limit":5},{"$sample":{"size":5}},{"$skip":2}]`,
		"skip_overflow":        `[` + set + `,{"$skip":1},{"$skip":` + maxInt64 + `}]`,
		"limit_skip_overflow":  `[` + set + `,{"$limit":5},{"$skip":1},{"$skip":` + maxInt64 + `}]`,
		"sort_skip_overflow":   `[{"$sort":{"a":1}},` + set + `,{"$skip":` + maxInt64 + `},{"$limit":5}]`,
		"skip_invalid":         `[` + set + `,{"$skip":"bad"}]`,
		"limit_invalid":        `[` + set + `,{"$limit":5},{"$skip":-1}]`,
		"set_invalid":          `[{"$set":5},{"$skip":2}]`,
		"set_invalid_limit":    `[{"$set":5},{"$limit":5},{"$skip":2}]`,
		"facet_match":          `[{"$facet":{"f":[` + set + `,{"$match":{}},{"$skip":2}]}}]`,
		"lookup_local_field":   `[{"$limit":1},{"$lookup":{"from":"C","as":"j","localField":"q","foreignField":"q","pipeline":[` + set + `,{"$limit":5},{"$skip":2}]}}]`,
		"lookup_documents":     `[{"$limit":1},{"$lookup":{"as":"j","pipeline":[{"$documents":[{"a":1},{"a":2}]},` + set + `,{"$limit":5},{"$skip":2}]}}]`,
		"lookup_documents_set": `[{"$limit":1},{"$lookup":{"as":"j","pipeline":[{"$documents":[{"a":1},{"a":2}]},` + set + `,{"$skip":2}]}}]`,
		"lookup_facet":         `[{"$limit":1},{"$lookup":{"from":"C","as":"j","pipeline":[{"$facet":{"f":[` + set + `,{"$limit":5},{"$skip":2}]}}]}}]`,
		"facet_lookup":         `[{"$facet":{"f":[{"$lookup":{"from":"C","as":"j","pipeline":[` + set + `,{"$limit":5},{"$skip":2}]}}]}}]`,
		"view":                 `V:[{"$skip":2}]`,
		"view_limit":           `V:[{"$limit":5},{"$skip":2}]`,
		"view_project_match":   `V:[{"$project":{"x":1}},{"$match":{}},{"$skip":2}]`,
		"view_group_limit":     `VG:[` + set + `,{"$limit":5},{"$skip":2}]`,
		"view_group_skip":      `VG:[` + set + `,{"$skip":2}]`,
		"view_group_sort":      `VG:[{"$sort":{"a":1}},` + set + `,{"$limit":5},{"$skip":2}]`,
	}
	for name, shape := range shapes {
		cases["top_"+name] = `[` + shape + `]`
		cases["facet_"+name] = `[{"$facet":{"f":[` + shape + `]}}]`
		cases["lookup_"+name] = `[{"$limit":1},{"$lookup":{"from":"C","as":"j","pipeline":[` + shape + `]}}]`
		cases["union_"+name] = `[{"$limit":1},{"$unionWith":{"coll":"C","pipeline":[` + shape + `]}}]`
		cases["group_"+name] = `[` + group + `,` + shape + `]`
		cases["documents_"+name] = `DB:[{"$documents":[{"a":1},{"a":2}]},` + shape + `]`
	}
	for name, pipeline := range cases {
		target := ""
		if before, after, ok := strings.Cut(pipeline, ":["); ok && !strings.Contains(before, "{") {
			target, pipeline = before, "["+after
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "SkipLimitReorder_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var p bson.A
				if err := bson.UnmarshalExtJSON([]byte(strings.ReplaceAll(pipeline, `"C"`, `"`+col.Name()+`"`)), false, &p); err != nil {
					return nil, err
				}
				var cursor *mongo.Cursor
				var err error
				switch target {
				case "DB":
					cursor, err = col.Database().Aggregate(ctx, p)
				case "V":
					cursor, err = col.Database().Collection(col.Name()+"_v").Aggregate(ctx, p)
				case "VG":
					cursor, err = col.Database().Collection(col.Name()+"_vg").Aggregate(ctx, p)
				default:
					cursor, err = col.Aggregate(ctx, p)
				}
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
