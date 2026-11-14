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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestSBEOperatorOutsideAccumulator: an operator used as a $group _id, as a
// $project field before an SBE $group, or as a $sortByCount expression
// fails with its SBE code, while operands folded to constants at parse time
// keep the classic code.
func TestSBEOperatorOutsideAccumulator(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{
			{Key: "_id", Value: int32(1)},
			{Key: "a", Value: bson.A{int32(1), int32(2)}},
			{Key: "v", Value: "abc"},
			{Key: "s", Value: "x"},
			{Key: "n", Value: int32(-1)},
			{Key: "z", Value: int32(0)},
			{Key: "d", Value: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Key: "tz", Value: "bogus"},
			{Key: "w", Value: "xx"},
			{Key: "o", Value: int32(1)},
			{Key: "r", Value: int32(1)},
		})
		return err
	}
	groupID := func(expr string) string {
		return `[{"$group":{"_id":` + expr + `}}]`
	}
	cases := map[string]string{
		"year":                     groupID(`{"$year":"$v"}`),
		"to_upper":                 groupID(`{"$toUpper":"$a"}`),
		"date_to_parts":            groupID(`{"$dateToParts":{"date":"$v"}}`),
		"project_then_group":       `[{"$project":{"y":{"$year":"$v"}}},{"$group":{"_id":"$y"}}]`,
		"sort_by_count":            `[{"$sortByCount":{"$year":"$v"}}]`,
		"index_of_bytes_negative":  groupID(`{"$indexOfBytes":["$v","b","$n"]}`),
		"index_of_cp_negative_end": groupID(`{"$indexOfCP":["$v","b",0,"$n"]}`),
		"index_of_bytes_string":    groupID(`{"$indexOfBytes":["$v","b","$s"]}`),
		"index_of_cp_string_end":   groupID(`{"$indexOfCP":["$v","b",0,"$s"]}`),
		"replace_one_replacement":  groupID(`{"$replaceOne":{"input":"$v","find":"b","replacement":"$r"}}`),
		"array_to_object_element":  groupID(`{"$arrayToObject":[["$s"]]}`),
		"date_diff_timezone":       groupID(`{"$dateDiff":{"startDate":"$d","endDate":"$d","unit":"day","timezone":"$tz"}}`),
		"date_diff_start_of_week":  groupID(`{"$dateDiff":{"startDate":"$d","endDate":"$d","unit":"week","startOfWeek":"$w"}}`),
		"date_trunc_bin_size":      groupID(`{"$dateTrunc":{"date":"$d","unit":"day","binSize":"$z"}}`),
		"date_trunc_start_of_week": groupID(`{"$dateTrunc":{"date":"$d","unit":"week","startOfWeek":"$w"}}`),
		"regex_match_options":      groupID(`{"$regexMatch":{"input":"$v","regex":"b","options":"$o"}}`),
		"const_date_diff_timezone": groupID(`{"$dateDiff":{"startDate":"$d","endDate":"$d","unit":"day","timezone":"x"}}`),
		"const_date_trunc_bin":     groupID(`{"$dateTrunc":{"date":"$d","unit":"day","binSize":0}}`),
		"const_regex_options":      groupID(`{"$regexMatch":{"input":"$v","regex":"b","options":1}}`),
		"const_array_to_object":    groupID(`{"$arrayToObject":[["x"]]}`),
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "SBEOperatorOutsideAccumulator_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(p),
		})
	}
}
