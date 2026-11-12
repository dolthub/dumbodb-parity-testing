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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A time-series collection has no _id index. With a metaField, mongod builds a
// {<metaField>: 1, <timeField>: 1} index, and listCollections reports neither
// a uuid nor an idIndex, with the default granularity filled in.

func tsCatalogCreate(ctx context.Context, db *mongo.Database, name string, spec bson.D, extra ...bson.E) error {
	cmd := append(bson.D{{Key: "create", Value: name}, {Key: "timeseries", Value: spec}}, extra...)
	return db.RunCommand(ctx, cmd).Err()
}

// tsCatalogListIndexes returns the collection's indexes sorted by name, since
// dumbodb and mongod order them differently.
func tsCatalogListIndexes(ctx context.Context, db *mongo.Database, name string) (interface{}, error) {
	cur, err := db.Collection(name).Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	var idx []bson.D
	if err := cur.All(ctx, &idx); err != nil {
		return nil, err
	}
	sort.Slice(idx, func(i, j int) bool {
		return idx[i].Map()["name"].(string) < idx[j].Map()["name"].(string)
	})
	return idx, nil
}

// tsCatalogListCollection returns the listCollections entry for name, with a
// collation's engine-specific ICU version checked and then normalized (see
// expectedMongoCollationVersion).
func tsCatalogListCollection(ctx context.Context, db *mongo.Database, name string) (interface{}, error) {
	cur, err := db.ListCollections(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return nil, err
	}
	var entries []bson.M
	if err := cur.All(ctx, &entries); err != nil {
		return nil, err
	}
	expected := expectedMongoCollationVersion
	if harness.ServerURI(ctx) == harness.DumboDBURI() {
		expected = expectedDumboDBCollationVersion
	}
	for _, e := range entries {
		opts, _ := e["options"].(bson.M)
		coll, ok := opts["collation"].(bson.M)
		if !ok {
			continue
		}
		version, _ := coll["version"].(string)
		coll["version"] = "<engine-specific>"
		e["collation_version_matches_expected"] = version == expected
	}
	return entries, nil
}

func TestTimeSeriesCatalog(t *testing.T) {
	cases := []struct {
		name  string
		spec  bson.D
		extra []bson.E
	}{
		{"TimeSeriesCatalog_MetaField", bson.D{{Key: "timeField", Value: "t"}, {Key: "metaField", Value: "m"}}, nil},
		{"TimeSeriesCatalog_NoMetaField", bson.D{{Key: "timeField", Value: "ts"}}, nil},
		{"TimeSeriesCatalog_Hours", bson.D{{Key: "timeField", Value: "ts"}, {Key: "metaField", Value: "tags"}, {Key: "granularity", Value: "hours"}}, nil},
		{"TimeSeriesCatalog_Minutes", bson.D{{Key: "timeField", Value: "ts"}, {Key: "granularity", Value: "minutes"}}, nil},
		{"TimeSeriesCatalog_Collation", bson.D{{Key: "timeField", Value: "ts"}, {Key: "metaField", Value: "mm"}},
			[]bson.E{{Key: "collation", Value: bson.D{{Key: "locale", Value: "fr"}}}}},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				if err := tsCatalogCreate(ctx, db, "ts", c.spec, c.extra...); err != nil {
					return nil, err
				}
				indexes, err := tsCatalogListIndexes(ctx, db, "ts")
				if err != nil {
					return nil, err
				}
				coll, err := tsCatalogListCollection(ctx, db, "ts")
				if err != nil {
					return nil, err
				}
				return bson.D{{Key: "indexes", Value: indexes}, {Key: "collection", Value: coll}}, nil
			},
		})
	}
}

func TestTimeSeriesCatalog_IndexCommands(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeriesCatalog_IndexCommands",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			if err := tsCatalogCreate(ctx, db, "tsm", bson.D{{Key: "timeField", Value: "t"}, {Key: "metaField", Value: "m"}}); err != nil {
				return nil, err
			}
			if err := tsCatalogCreate(ctx, db, "tsn", bson.D{{Key: "timeField", Value: "t"}}); err != nil {
				return nil, err
			}
			var out bson.A
			for _, cmd := range []bson.D{
				{{Key: "createIndexes", Value: "tsm"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "m", Value: 1}}}, {Key: "name", Value: "m_1"}}}}},
				{{Key: "createIndexes", Value: "tsn"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "x", Value: 1}}}, {Key: "name", Value: "x_1"}}}}},
				{{Key: "dropIndexes", Value: "tsm"}, {Key: "index", Value: "m_1_t_1"}},
				{{Key: "dropIndexes", Value: "tsn"}, {Key: "index", Value: "*"}},
			} {
				var res bson.D
				if err := db.RunCommand(ctx, cmd).Decode(&res); err != nil {
					return nil, err
				}
				out = append(out, res)
			}
			for _, name := range []string{"tsm", "tsn"} {
				idx, err := tsCatalogListIndexes(ctx, db, name)
				if err != nil {
					return nil, err
				}
				out = append(out, idx)
			}
			return out, nil
		},
	})
}
