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
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// explainOutcome runs cmd and returns its ok and whether it carries a query
// plan, or its error code and message.
func explainOutcome(ctx context.Context, db *mongo.Database, cmd bson.D) (interface{}, error) {
	var res bson.M
	err := db.RunCommand(ctx, cmd).Decode(&res)
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return bson.D{{Key: "code", Value: ce.Code}, {Key: "codeName", Value: ce.Name}, {Key: "message", Value: ce.Message}}, nil
	}
	if err != nil {
		return nil, err
	}
	_, planned := res["queryPlanner"]
	return bson.D{{Key: "ok", Value: res["ok"]}, {Key: "planned", Value: planned}}, nil
}

func insertExplainOptionDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 200)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 7)}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

// TestExplain_Options: explain accepts maxTimeMS and readConcern beside the
// command or inside it, and validates maxTimeMS.
func TestExplain_Options(t *testing.T) {
	find := func(col *mongo.Collection, extra ...bson.E) bson.D {
		return append(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: int32(1)}}}}, extra...)
	}
	cases := []struct {
		name string
		cmd  func(col *mongo.Collection) bson.D
	}{
		{"max_time_outer", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: int32(10000)}}
		}},
		{"max_time_inner", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col, bson.E{Key: "maxTimeMS", Value: int32(10000)})}, {Key: "verbosity", Value: "executionStats"}}
		}},
		{"max_time_negative", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: int32(-1)}}
		}},
		{"max_time_string", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: "x"}}
		}},
		{"max_time_fraction", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: 1.5}}
		}},
		{"max_time_negative_fraction", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: -1.5}}
		}},
		{"max_time_too_large", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: int64(2147483648)}}
		}},
		{"max_time_null", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: nil}}
		}},
		{"max_time_zero", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "executionStats"}, {Key: "maxTimeMS", Value: int32(0)}}
		}},
		{"read_concern_outer", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col)}, {Key: "verbosity", Value: "queryPlanner"}, {Key: "readConcern", Value: bson.D{{Key: "level", Value: "local"}}}}
		}},
		{"read_concern_inner", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: find(col, bson.E{Key: "readConcern", Value: bson.D{{Key: "level", Value: "local"}}})}, {Key: "verbosity", Value: "queryPlanner"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Explain_Options_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertExplainOptionDocs,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainOutcome(ctx, col.Database(), tc.cmd(col))
				},
			})
		})
	}
}

// TestExplain_InTransaction: explain is not allowed in a multi-document
// transaction, and the transaction stays usable.
func TestExplain_InTransaction(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "Explain_InTransaction",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup:    insertExplainOptionDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			session, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer session.EndSession(ctx)
			if err := session.StartTransaction(); err != nil {
				return nil, err
			}
			txCtx := mongo.NewSessionContext(ctx, session)
			if _, err := col.InsertOne(txCtx, bson.D{{Key: "_id", Value: "tx"}}); err != nil {
				return nil, err
			}
			cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{}}}}, {Key: "verbosity", Value: "queryPlanner"}}
			explained, err := explainOutcome(txCtx, col.Database(), cmd)
			if err != nil {
				return nil, err
			}
			commitErr := session.CommitTransaction(ctx)
			n, err := col.CountDocuments(ctx, bson.D{{Key: "_id", Value: "tx"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "explain", Value: explained}, {Key: "committed", Value: commitErr == nil}, {Key: "inserted", Value: n}}, nil
		},
	})
}
