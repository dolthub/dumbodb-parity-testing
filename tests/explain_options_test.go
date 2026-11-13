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
	"fmt"
	"math"
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

// TestMaxTimeMS_CommandValues: count, distinct and explain read maxTimeMS by
// mongod's generic rules; find and aggregate check its type and sign first;
// and an explained find, aggregate or count checks only its type and sign.
func TestMaxTimeMS_CommandValues(t *testing.T) {
	values := []struct {
		name  string
		value interface{}
	}{
		{"negative_half", -0.5}, {"negative_one_and_half", -1.5}, {"one_and_half", 1.5},
		{"decimal_negative_half", mustDecimal("-0.5")}, {"nan", math.NaN()}, {"decimal_nan", mustDecimal("NaN")},
		{"inf", math.Inf(1)}, {"string", "x"}, {"bool", true}, {"null", nil}, {"array", bson.A{}},
		{"negative_one", -1}, {"two_to_31", 2147483648.0}, {"huge", 1e300}, {"long", int64(10000)},
		{"decimal_negative_one", mustDecimal("-1")}, {"decimal_negative_one_and_half", mustDecimal("-1.5")},
		{"decimal_fraction", mustDecimal("1000.4")}, {"decimal_inf", mustDecimal("Infinity")},
		{"negative_inf", math.Inf(-1)}, {"long_min", int64(math.MinInt64)},
	}
	cmds := []struct {
		name string
		cmd  func(col *mongo.Collection, v interface{}) bson.D
	}{
		{"count", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: bson.D{}}, {Key: "maxTimeMS", Value: v}}
		}},
		{"distinct", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}, {Key: "maxTimeMS", Value: v}}
		}},
		{"explain", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "count", Value: col.Name()}}}, {Key: "maxTimeMS", Value: v}}
		}},
		{"explain_inner_find", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: col.Name()}, {Key: "maxTimeMS", Value: v}}}}
		}},
		{"find", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "maxTimeMS", Value: v}}
		}},
		{"aggregate", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}, {Key: "maxTimeMS", Value: v}}
		}},
		{"explain_inner_count", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "count", Value: col.Name()}, {Key: "maxTimeMS", Value: v}}}}
		}},
		{"explain_inner_distinct", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}, {Key: "maxTimeMS", Value: v}}}}
		}},
		{"explain_inner_aggregate", func(col *mongo.Collection, v interface{}) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}, {Key: "maxTimeMS", Value: v}}}}
		}},
	}
	for _, v := range values {
		for _, c := range cmds {
			t.Run(v.name+"/"+c.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "MaxTimeMS_CommandValues_" + v.name + "_" + c.name,
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						_, err := col.InsertOne(ctx, bson.D{{Key: "a", Value: 1}})
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						err := col.Database().RunCommand(ctx, c.cmd(col, v.value)).Err()
						var ce mongo.CommandError
						if errors.As(err, &ce) {
							return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: sortExpectedTypes(ce.Message)}}, nil
						}
						return fmt.Sprint(err), nil
					},
				})
			})
		}
	}
}
