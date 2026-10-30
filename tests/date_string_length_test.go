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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// DumboDB rejects a $dateFromString dateString longer than 4096 bytes as a
// parse failure (workspace-8tj.11). mongod has no limit; longer valid
// strings only repeat tokens or pad, and parsing them costs CPU per byte.

const maxDateStringLength = 4096

// paddedDateString is a valid date string of exactly n bytes.
func paddedDateString(n int) string {
	const date = "2021-03-04"
	return date + strings.Repeat(" ", n-len(date))
}

func dateFromStringMillis(onError bool) func(ctx context.Context, col *mongo.Collection, s string) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection, s string) (interface{}, error) {
		spec := bson.D{{Key: "dateString", Value: s}}
		if onError {
			spec = append(spec, bson.E{Key: "onError", Value: "unparsed"})
		}
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
			return nil, err
		}
		cur, err := col.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$project", Value: bson.D{{Key: "d", Value: bson.D{{Key: "$dateFromString", Value: spec}}}}}},
			{{Key: "$project", Value: bson.D{{Key: "d", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$d"}}, "date"}}},
				bson.D{{Key: "$toLong", Value: bson.D{{Key: "$subtract", Value: bson.A{"$d", bson.D{{Key: "$toDate", Value: int64(0)}}}}}}},
				"$d",
			}}}}}}},
		})
		if err != nil {
			return nil, err
		}
		var out []bson.D
		if err := cur.All(ctx, &out); err != nil {
			return nil, err
		}
		return out[0].Map()["d"], nil
	}
}

func TestDateFromString_LengthLimit_AtLimit(t *testing.T) {
	run := dateFromStringMillis(false)
	harness.PairTest(t, harness.TestCase{
		Name:    "DateFromString_LengthLimit_AtLimit",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return run(ctx, col, paddedDateString(maxDateStringLength))
		},
	})
}

func TestDateFromString_LengthLimit_OverLimit(t *testing.T) {
	const wantMillis = int64(1614816000000) // 2021-03-04T00:00:00Z
	for _, tc := range []struct {
		name      string
		onError   bool
		dumboWant func(res interface{}, err error) bool
	}{
		{"NoOnError", false, func(_ interface{}, err error) bool {
			var cmdErr mongo.CommandError
			return errors.As(err, &cmdErr) && cmdErr.Code == 241
		}},
		{"OnError", true, func(res interface{}, err error) bool { return err == nil && res == "unparsed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := dateFromStringMillis(tc.onError)
			harness.AuthPairTest(t, harness.AuthCase{
				Name:    "DateFromString_LengthLimit_OverLimit_" + tc.name,
				Support: harness.DumboDBDeviates,
				Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
					db := tgt.Admin.Database(tgt.NS)
					defer func() { _ = db.Drop(ctx) }()
					return run(ctx, db.Collection("c"), paddedDateString(maxDateStringLength+1))
				},
				MongoExpect: func(t *testing.T, res interface{}, err error) {
					if err != nil || fmt.Sprint(res) != fmt.Sprint(wantMillis) {
						t.Errorf("mongo: want %d, got %v (err %v)", wantMillis, res, err)
					}
				},
				DumboExpect: func(t *testing.T, res interface{}, err error) {
					if !tc.dumboWant(res, err) {
						t.Errorf("dumbodb: got %v (err %v)", res, err)
					}
				},
			})
		})
	}
}

// $toDate parses with the same date parser, so the same limit applies.
func TestToDate_LengthLimit(t *testing.T) {
	toDate := func(s string) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: s}}); err != nil {
				return nil, err
			}
			cur, err := col.Aggregate(ctx, mongo.Pipeline{
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "d", Value: bson.D{{Key: "$toLong", Value: bson.D{{Key: "$toDate", Value: "$s"}}}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var out []bson.D
			if err := cur.All(ctx, &out); err != nil {
				return nil, err
			}
			return out[0].Map()["d"], nil
		}
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "ToDate_LengthLimit_AtLimit",
		Support: harness.DumboDBFull,
		Run:     toDate(paddedDateString(maxDateStringLength)),
	})

	run := toDate(paddedDateString(maxDateStringLength + 1))
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "ToDate_LengthLimit_OverLimit",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			return run(ctx, db.Collection("c"))
		},
		MongoExpect: func(t *testing.T, res interface{}, err error) {
			if err != nil || res != int64(1614816000000) {
				t.Errorf("mongo: want 1614816000000, got %v (err %v)", res, err)
			}
		},
		DumboExpect: func(t *testing.T, res interface{}, err error) {
			var cmdErr mongo.CommandError
			if !errors.As(err, &cmdErr) || cmdErr.Code != 241 {
				t.Errorf("dumbodb: want ConversionFailure, got %v (err %v)", res, err)
			}
		},
	})
}
