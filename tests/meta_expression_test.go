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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $meta as an aggregation expression. Metadata values such as text scores
// and record ids are implementation details, so cases compare their types
// and presence.

func insertMetaDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "x", Value: int32(3)}, {Key: "t", Value: "red fish"}, {Key: "loc", Value: bson.A{0.0, 0.0}}},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}, {Key: "t", Value: "blue fish"}, {Key: "loc", Value: bson.A{1.0, 1.0}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(2)}, {Key: "t", Value: "no match"}, {Key: "loc", Value: bson.A{2.0, 2.0}}},
	})
	if err != nil {
		return err
	}
	_, err = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "t", Value: "text"}}},
		{Keys: bson.D{{Key: "x", Value: 1}}},
		{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}},
	})
	return err
}

var metaNames = []string{
	"textScore", "randVal", "searchScore", "searchHighlights", "geoNearDistance", "geoNearPoint",
	"recordId", "indexKey", "sortKey", "searchScoreDetails", "searchSequenceToken",
	"timeseriesBucketMinTime", "timeseriesBucketMaxTime", "vectorSearchScore",
}

// metaPending need document metadata DumboDB does not carry (workspace-gns).
var metaPending = map[string]bool{
	"Unavailable_indexKey":      true,
	"UnavailableValue_indexKey": true,
	"Unavailable_sortKey":       true,
	"UnavailableValue_sortKey":  true,
	"TextScoreAfterText":        true,
	"TextScorePositive":         true,
	"SortKeyAfterSort":          true,
	"SortKeyCompound":           true,
	"SortKeyAfterProject":       true,
	"RandValAfterSample":        true,
	"GeoNearDistance":           true,
	"GeoNearPoint":              true,
	"FindSortKey":               true,
	"FindRecordId":              true,
}

func TestMetaExpression(t *testing.T) {
	meta := func(arg interface{}) bson.D { return bson.D{{Key: "$meta", Value: arg}} }
	typeOf := func(arg interface{}) bson.D { return bson.D{{Key: "$type", Value: arg}} }
	sorted := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	project := func(expr interface{}) bson.D {
		return bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: expr}}}}
	}

	type pipelineCase struct {
		name     string
		pipeline bson.A
	}
	var cases []pipelineCase
	for _, name := range metaNames {
		cases = append(cases,
			pipelineCase{"Unavailable_" + name, bson.A{sorted, project(typeOf(meta(name)))}},
			pipelineCase{"UnavailableValue_" + name, bson.A{sorted, project(meta(name))}},
		)
	}
	cases = append(cases,
		pipelineCase{"NumberArgument", bson.A{project(meta(int32(1)))}},
		pipelineCase{"ArrayArgument", bson.A{project(meta(bson.A{"textScore"}))}},
		pipelineCase{"DocumentArgument", bson.A{project(meta(bson.D{{Key: "a", Value: int32(1)}}))}},
		pipelineCase{"NullArgument", bson.A{project(meta(nil))}},
		pipelineCase{"UnknownName", bson.A{project(meta("foo"))}},
		pipelineCase{"EmptyName", bson.A{project(meta(""))}},
		pipelineCase{"WrongCase", bson.A{project(meta("TextScore"))}},
		pipelineCase{"Score", bson.A{project(meta("score"))}},
		pipelineCase{"ScoreDetails", bson.A{project(meta("scoreDetails"))}},
		pipelineCase{"Nested", bson.A{sorted, project(bson.D{{Key: "$ifNull", Value: bson.A{meta("searchScore"), "none"}}})}},
		pipelineCase{"InAddFields", bson.A{sorted, bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: typeOf(meta("searchScore"))}}}}, project("$r")}},
		pipelineCase{"InGroup", bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: typeOf(meta("searchScore"))}}}}}}}},
		pipelineCase{"InMatchExpr", bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{typeOf(meta("searchScore")), "missing"}}}}}}}, bson.D{{Key: "$count", Value: "n"}}}},

		pipelineCase{"TextScoreAfterText", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "fish"}}}}}},
			sorted, project(typeOf(meta("textScore"))),
		}},
		pipelineCase{"TextScorePositive", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "fish"}}}}}},
			sorted, project(bson.D{{Key: "$gt", Value: bson.A{meta("textScore"), int32(0)}}}),
		}},
		pipelineCase{"TextScoreAfterGroup", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "fish"}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"}}}},
			sorted, project(typeOf(meta("textScore"))),
		}},
		pipelineCase{"SortKeyAfterSort", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "x", Value: 1}}}},
			project(meta("sortKey")),
		}},
		pipelineCase{"SortKeyCompound", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "x", Value: -1}, {Key: "_id", Value: 1}}}},
			project(meta("sortKey")),
		}},
		pipelineCase{"SortKeyAfterProject", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "x", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: 1}}}},
			project(meta("sortKey")),
		}},
		pipelineCase{"RecordIdType", bson.A{sorted, project(typeOf(meta("recordId")))}},
		pipelineCase{"RandValAfterSample", bson.A{
			bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: int32(2)}}}},
			project(typeOf(meta("randVal"))),
		}},
		pipelineCase{"GeoNearDistance", bson.A{
			bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0.0, 0.0}}}},
				{Key: "key", Value: "loc"},
				{Key: "distanceField", Value: "d"},
			}}},
			project(typeOf(meta("geoNearDistance"))),
		}},
		pipelineCase{"GeoNearPoint", bson.A{
			bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0.0, 0.0}}}},
				{Key: "key", Value: "loc"},
				{Key: "distanceField", Value: "d"},
			}}},
			project(meta("geoNearPoint")),
		}},
	)

	for _, tc := range cases {
		support := harness.DumboDBFull
		if metaPending[tc.name] {
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Meta_" + tc.name,
				Support: support,
				Setup:   insertMetaDocs,
				Run:     pipelineRunExtJSON(tc.pipeline),
			})
		})
	}

	findCases := []struct {
		name       string
		filter     bson.D
		opts       *options.FindOptions
		projection bson.D
	}{
		{"FindTextScore", bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "fish"}}}}, nil, bson.D{{Key: "r", Value: meta("textScore")}}},
		{"FindTextScoreWithoutText", bson.D{}, nil, bson.D{{Key: "r", Value: meta("textScore")}}},
		{"FindSortKey", bson.D{}, options.Find().SetSort(bson.D{{Key: "x", Value: 1}}), bson.D{{Key: "r", Value: meta("sortKey")}}},
		{"FindSortKeyWithoutSort", bson.D{}, nil, bson.D{{Key: "r", Value: meta("sortKey")}}},
		{"FindRecordId", bson.D{}, nil, bson.D{{Key: "r", Value: meta("recordId")}}},
		{"FindIndexKey", bson.D{{Key: "x", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}, options.Find().SetHint(bson.D{{Key: "x", Value: 1}}), bson.D{{Key: "r", Value: meta("indexKey")}}},
		{"FindUnknown", bson.D{}, nil, bson.D{{Key: "r", Value: meta("foo")}}},
		{"FindNumber", bson.D{}, nil, bson.D{{Key: "r", Value: meta(int32(1))}}},
		{"FindSearchScore", bson.D{}, nil, bson.D{{Key: "r", Value: meta("searchScore")}}},
	}
	for _, fc := range findCases {
		support := harness.DumboDBFull
		if metaPending[fc.name] {
			support = harness.DumboDBXFail
		}
		t.Run(fc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Meta_" + fc.name,
				Support: support,
				Setup:   insertMetaDocs,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := fc.opts
					if opts == nil {
						opts = options.Find()
					}
					opts.SetProjection(fc.projection)
					cursor, err := col.Find(ctx, fc.filter, opts)
					if err != nil {
						return errOutcome(err), nil
					}
					var docs []bson.M
					if err = cursor.All(ctx, &docs); err != nil {
						return errOutcome(err), nil
					}
					types := map[string]int{}
					for _, d := range docs {
						r, ok := d["r"]
						switch {
						case !ok:
							types["missing"]++
						default:
							types[fmt.Sprintf("%T", r)]++
						}
					}
					return types, nil
				},
			})
		})
	}
}
