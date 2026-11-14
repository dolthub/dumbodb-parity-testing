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
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestDensifyFillSpecOrder: mongod's IDL parser checks each $densify and
// $fill field's type as it reads it, parses the range object whole, and only
// then reports missing top-level fields.
func TestDensifyFillSpecOrder(t *testing.T) {
	cases := map[string]string{
		"densify_step_type":            `{"$densify":{"range":{"step":"a"}}}`,
		"densify_bounds_bad_no_field":  `{"$densify":{"range":{"step":1,"bounds":5}}}`,
		"densify_partitions_type":      `{"$densify":{"partitionByFields":5}}`,
		"densify_partition_elem_type":  `{"$densify":{"partitionByFields":[5]}}`,
		"densify_range_unknown":        `{"$densify":{"range":{"zz":1}}}`,
		"densify_range_missing_bounds": `{"$densify":{"range":{"step":1}}}`,
		"densify_unit_type":            `{"$densify":{"range":{"step":1,"unit":5}}}`,
		"densify_range_type":           `{"$densify":{"range":5}}`,
		"densify_step_negative":        `{"$densify":{"range":{"step":-1,"bounds":"full"}}}`,
		"fill_method_type_first":       `{"$fill":{"output":{"y":{"method":1,"zz":1}}}}`,
		"fill_unknown_first":           `{"$fill":{"output":{"y":{"zz":1,"method":1}}}}`,
		"fill_method_null":             `{"$fill":{"output":{"y":{"method":null,"zz":1}}}}`,
	}
	for name, stage := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(stage), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DensifyFillSpecOrder_" + name,
			Support: harness.DumboDBFull,
			Run:     pipelineRun(bson.A{s}),
		})
	}
}
