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

// TestDatabaseSourceStageLaterFirstOnly: after the source stage of an
// {aggregate: 1} pipeline, $collStats and $listLocalSessions are refused as
// stages that must be first, once their specifications parse.
func TestDatabaseSourceStageLaterFirstOnly(t *testing.T) {
	cases := map[string]string{
		"DocumentsCollStats":           `[{"$documents":[{"a":1}]},{"$collStats":{}}]`,
		"DocumentsCollStatsBadSpec":    `[{"$documents":[{"a":1}]},{"$collStats":{"bogus":1}}]`,
		"DocumentsLocalSessions":       `[{"$documents":[{"a":1}]},{"$listLocalSessions":{}}]`,
		"DocumentsLocalSessionsBad":    `[{"$documents":[{"a":1}]},{"$listLocalSessions":1}]`,
		"CatalogCollStats":             `[{"$listClusterCatalog":{}},{"$collStats":{}}]`,
		"CatalogLocalSessions":         `[{"$listClusterCatalog":{}},{"$listLocalSessions":{}}]`,
		"DocumentsCollStatsThenDocs":   `[{"$documents":[]},{"$collStats":{}},{"$documents":[]}]`,
		"DocumentsCollStatsAfterMatch": `[{"$documents":[{"a":1}]},{"$match":{}},{"$collStats":{}}]`,
	}
	for name, js := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DatabaseSourceStageLaterFirstOnly_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Aggregate(ctx, pipeline)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}

// TestDatabaseSourceLaterStages: the stages after $listLocalSessions and
// $currentOp run over the source's documents, and a stage that must be first
// is refused with 40602 in any later position.
func TestDatabaseSourceLaterStages(t *testing.T) {
	type tc struct {
		admin    bool
		pipeline string
	}
	cases := map[string]tc{
		"SessionsMatchNone":          {pipeline: `[{"$listLocalSessions":{}},{"$match":{"nope":1}}]`},
		"SessionsLimitZero":          {pipeline: `[{"$listLocalSessions":{}},{"$limit":0}]`},
		"SessionsSessions":           {pipeline: `[{"$listLocalSessions":{}},{"$listLocalSessions":{}}]`},
		"SessionsCollStats":          {pipeline: `[{"$listLocalSessions":{}},{"$collStats":{}}]`},
		"SessionsDocuments":          {pipeline: `[{"$listLocalSessions":{}},{"$documents":[]}]`},
		"SessionsDocumentsNotArray":  {pipeline: `[{"$listLocalSessions":{}},{"$documents":5}]`},
		"SessionsIndexStats":         {pipeline: `[{"$listLocalSessions":{}},{"$indexStats":{}}]`},
		"SessionsOutNotLast":         {pipeline: `[{"$listLocalSessions":{}},{"$out":"o"},{"$match":{}}]`},
		"SessionsDocumentsBeforeOut": {pipeline: `[{"$listLocalSessions":{}},{"$documents":[]},{"$out":"o"},{"$match":{}}]`},
		"SessionsMergeSystem":        {pipeline: `[{"$listLocalSessions":{}},{"$merge":{"into":"system.foo"}}]`},

		"AdminCurrentOpSessions":       {admin: true, pipeline: `[{"$currentOp":{}},{"$listLocalSessions":{}}]`},
		"AdminCurrentOpDocuments":      {admin: true, pipeline: `[{"$currentOp":{}},{"$documents":[]}]`},
		"AdminCurrentOpCurrentOp":      {admin: true, pipeline: `[{"$currentOp":{}},{"$currentOp":{}}]`},
		"AdminCurrentOpOut":            {admin: true, pipeline: `[{"$currentOp":{}},{"$out":"o"}]`},
		"AdminSessionsCurrentOp":       {admin: true, pipeline: `[{"$listLocalSessions":{}},{"$currentOp":{}}]`},
		"AdminDocumentsCurrentOp":      {admin: true, pipeline: `[{"$documents":[]},{"$currentOp":{}}]`},
		"AdminDocumentsThenQueryStats": {admin: true, pipeline: `[{"$listLocalSessions":{}},{"$documents":[]},{"$queryStats":{}}]`},
	}
	for name, c := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+c.pipeline+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DatabaseSourceLaterStages_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				if c.admin {
					db = db.Client().Database("admin")
				}
				cursor, err := db.Aggregate(ctx, pipeline)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}

// TestDatabaseSourceSessionsOut: $out after $listLocalSessions writes the
// documents the stages before it produce.
func TestDatabaseSourceSessionsOut(t *testing.T) {
	pipeline := bson.A{
		bson.D{{Key: "$listLocalSessions", Value: bson.D{}}},
		bson.D{{Key: "$limit", Value: int32(1)}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "x", Value: bson.D{{Key: "$literal", Value: int32(1)}}}}}},
		bson.D{{Key: "$out", Value: "o"}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "DatabaseSourceSessionsOut",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Database().Aggregate(ctx, pipeline)
			res, _ := drainCursor(ctx, cursor, err)
			found, err := col.Database().Collection("o").Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}}))
			docs, _ := drainCursor(ctx, found, err)
			return bson.M{"result": res, "target": docs}, nil
		},
	})
}

// TestCurrentOpParseOrder: $currentOp checks that it runs on the admin
// database (73), then its options, while it parses, so these come in stage
// order with the other stages' parse errors and before position checks.
func TestCurrentOpParseOrder(t *testing.T) {
	type tc struct {
		admin    bool
		pipeline string
	}
	cases := map[string]tc{
		"SessionsCurrentOp":             {pipeline: `[{"$listLocalSessions":{}},{"$currentOp":{}}]`},
		"SessionsCurrentOpThenStats":    {pipeline: `[{"$listLocalSessions":{}},{"$currentOp":{}},{"$queryStats":{}}]`},
		"CurrentOpLimitZero":            {pipeline: `[{"$currentOp":{}},{"$limit":0}]`},
		"CurrentOpBadMatch":             {pipeline: `[{"$currentOp":{}},{"$match":{"$bogus":1}}]`},
		"CurrentOpBadOption":            {pipeline: `[{"$currentOp":{"bogus":1}},{"$limit":0}]`},
		"CurrentOpNotObject":            {pipeline: `[{"$currentOp":5},{"$limit":0}]`},
		"CurrentOpQueryStats":           {pipeline: `[{"$currentOp":{}},{"$queryStats":{}}]`},
		"MatchCurrentOp":                {pipeline: `[{"$match":{}},{"$currentOp":{}}]`},
		"BadMatchCurrentOp":             {pipeline: `[{"$match":{"$bogus":1}},{"$currentOp":{}}]`},
		"DocumentsOutCurrentOp":         {pipeline: `[{"$documents":[]},{"$out":"o"},{"$currentOp":{}}]`},
		"AdminCurrentOpLimitZero":       {admin: true, pipeline: `[{"$currentOp":{}},{"$limit":0}]`},
		"AdminCurrentOpBadOption":       {admin: true, pipeline: `[{"$currentOp":{"bogus":1}},{"$limit":0}]`},
		"AdminSessionsCurrentOpBad":     {admin: true, pipeline: `[{"$listLocalSessions":{}},{"$currentOp":{"bogus":1}}]`},
		"AdminCurrentOpOptionType":      {admin: true, pipeline: `[{"$currentOp":{"idleCursors":5,"bogus":1}}]`},
		"AdminCurrentOpUnknownFirst":    {admin: true, pipeline: `[{"$currentOp":{"bogus":1,"idleCursors":5}}]`},
		"AdminCurrentOpAllUsersType":    {admin: true, pipeline: `[{"$currentOp":{"bogus":1,"allUsers":5}}]`},
		"AdminCurrentOpLocalOpsFirst":   {admin: true, pipeline: `[{"$currentOp":{"idleCursors":5,"localOps":5,"allUsers":5}}]`},
		"AdminCurrentOpTargetAllNodes":  {admin: true, pipeline: `[{"$currentOp":{"targetAllNodes":true}}]`},
		"AdminCurrentOpTargetAllNodesB": {admin: true, pipeline: `[{"$currentOp":{"targetAllNodes":true,"bogus":1}}]`},
	}
	for name, c := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+c.pipeline+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "CurrentOpParseOrder_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				if c.admin {
					db = db.Client().Database("admin")
				}
				cursor, err := db.Aggregate(ctx, pipeline)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
