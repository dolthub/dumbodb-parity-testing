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

// updateWriteErrorCodes runs an update command and returns its write error
// codes; a top-level error is returned as the error.
func updateWriteErrorCodes(ctx context.Context, col *mongo.Collection, updates bson.A) (interface{}, error) {
	cmd := bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: updates}}
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
	return bson.D{{Key: "n", Value: res["n"]}, {Key: "writeErrors", Value: codes}}, nil
}

// TestLiteParseGraphLookupAndSessions: mongod's lite parse checks the
// $graphLookup spec and 'from' field, the $listSessions spec and refuses
// $listCachedAndActiveUsers, before a later unknown stage and before an
// earlier stage's own errors; in sub-pipelines, views and updates too.
func TestLiteParseGraphLookupAndSessions(t *testing.T) {
	const bogus = `,{"$bogus":1}`
	stages := map[string]string{
		"graph_not_object":     `{"$graphLookup":5}`,
		"graph_from_int":       `{"$graphLookup":{"from":5}}`,
		"graph_no_from":        `{"$graphLookup":{"startWith":1}}`,
		"graph_from_db_coll":   `{"$graphLookup":{"from":{"db":"x","coll":"y"}}}`,
		"graph_from_db_int":    `{"$graphLookup":{"from":{"db":5,"coll":"y"}}}`,
		"graph_from_unknown":   `{"$graphLookup":{"from":{"db":"x","z":1}}}`,
		"graph_from_empty":     `{"$graphLookup":{"from":""}}`,
		"graph_from_dot":       `{"$graphLookup":{"from":".a"}}`,
		"graph_from_nul":       `{"$graphLookup":{"from":"a\u0000b"}}`,
		"graph_unknown_later":  `{"$graphLookup":{"zz":1,"from":5}}`,
		"graph_from_ok":        `{"$graphLookup":{"from":"x","zz":1}}`,
		"lookup_from_empty":    `{"$lookup":{"from":"","as":"r"}}`,
		"cached_users":         `{"$listCachedAndActiveUsers":{}}`,
		"sessions_not_object":  `{"$listSessions":5}`,
		"sessions_unknown":     `{"$listSessions":{"zz":1}}`,
		"sessions_bool":        `{"$listSessions":{"allUsers":5}}`,
		"sessions_users_type":  `{"$listSessions":{"users":5}}`,
		"sessions_missing_db":  `{"$listSessions":{"users":[{"user":"a"}]}}`,
		"sessions_predicate":   `{"$listSessions":{"$_internalPredicate":5}}`,
		"sessions_ok":          `{"$listSessions":{"allUsers":true,"users":[]}}`,
		"local_predicate":      `{"$listLocalSessions":{"$_internalPredicate":{}}}`,
		"local_predicate_type": `{"$listLocalSessions":{"$_internalPredicate":1}}`,
	}
	shapes := map[string]string{
		"before_bogus": `[S` + bogus + `]`,
		"after_limit":  `[{"$limit":-1},S]`,
		"in_lookup":    `[{"$lookup":{"from":"c","as":"r","pipeline":[S` + bogus + `]}}]`,
		"in_facet":     `[{"$facet":{"a":[S` + bogus + `]}}]`,
		"view":         `V[S]`,
		"update":       `U[S]`,
	}
	for stageName, stage := range stages {
		for shapeName, shape := range shapes {
			kind := shape[0]
			if kind == 'V' || kind == 'U' {
				shape = shape[1:]
			}
			text := ""
			for _, r := range shape {
				if r == 'S' {
					text += stage
				} else {
					text += string(r)
				}
			}
			var p bson.A
			if err := bson.UnmarshalExtJSON([]byte(text), false, &p); err != nil {
				t.Fatal(err)
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "LiteParseGraphLookupAndSessions_" + stageName + "_" + shapeName,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					switch kind {
					case 'V':
						err := col.Database().CreateView(ctx, "v_"+col.Name(), col.Name(), p)
						return errOutcome(err), nil
					case 'U':
						return updateWriteErrorCodes(ctx, col, bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: p}}})
					}
					cursor, err := col.Aggregate(ctx, p)
					return drainCursor(ctx, cursor, err)
				},
			})
		}
	}
}
