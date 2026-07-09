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
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Non-numeric _id equality edge cases. _id equality is by exact type and value:
// no cross-type coercion outside the numeric family.

func TestBSON_id_binary_subtype_distinct(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_binary_subtype_distinct",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.Binary{Subtype: 0x00, Data: data},
			primitive.Binary{Subtype: 0x04, Data: data},
		),
	})
}

func TestBSON_id_binary_same_subtype_match(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_binary_same_subtype_match",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.Binary{Subtype: 0x00, Data: data},
			primitive.Binary{Subtype: 0x00, Data: data},
		),
	})
}

func TestBSON_id_date_vs_timestamp_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_date_vs_timestamp_distinct",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.NewDateTimeFromTime(time.Unix(1, 0).UTC()),
			primitive.Timestamp{T: 1, I: 0},
		),
	})
}

func TestBSON_id_bool_vs_int_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_bool_vs_int_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(true, int32(1)),
	})
}

func TestBSON_id_minkey_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_minkey_match",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.MinKey{}, primitive.MinKey{}),
	})
}

func TestBSON_id_minkey_vs_maxkey_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_minkey_vs_maxkey_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.MinKey{}, primitive.MaxKey{}),
	})
}

func TestBSON_id_timestamp_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_timestamp_match",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.Timestamp{T: 5, I: 1}, primitive.Timestamp{T: 5, I: 1}),
	})
}

func TestBSON_id_timestamp_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_timestamp_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.Timestamp{T: 5, I: 1}, primitive.Timestamp{T: 5, I: 2}),
	})
}
