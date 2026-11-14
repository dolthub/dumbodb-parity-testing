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

// TestSampleGeoNearSpecErrors: $sample checks its fields in order with
// mongod's codes, and $geoNear without near or with a non-string
// distanceField gives mongod's codes.
func TestSampleGeoNearSpecErrors(t *testing.T) {
	cases := map[string]string{
		"sample_not_object":     `{"$sample":5}`,
		"sample_size_string":    `{"$sample":{"size":"a"}}`,
		"sample_size_null":      `{"$sample":{"size":null}}`,
		"sample_size_negative":  `{"$sample":{"size":-1}}`,
		"sample_unknown":        `{"$sample":{"zz":1}}`,
		"sample_unknown_first":  `{"$sample":{"zz":1,"size":"a"}}`,
		"sample_missing_size":   `{"$sample":{}}`,
		"sample_fraction":       `{"$sample":{"size":1.5}}`,
		"geo_near_no_near":      `{"$geoNear":{"zz":1}}`,
		"geo_near_distance_int": `{"$geoNear":{"near":[0,0],"distanceField":5}}`,
	}
	for name, stage := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(stage), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "SampleGeoNearSpecErrors_" + name,
			Support: harness.DumboDBFull,
			Run:     pipelineRun(bson.A{s}),
		})
	}
}
