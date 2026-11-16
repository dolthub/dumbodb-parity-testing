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

// A $rename whose source and target lie on one path is refused before any
// document is read (dolthub/dumbodb#250).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestRenameSamePath(t *testing.T) {
	one := bson.D{{Key: "_id", Value: 1}}
	none := bson.D{{Key: "_id", Value: 9}}
	rename := func(from, to string) bson.D {
		return bson.D{{Key: "$rename", Value: bson.D{{Key: from, Value: to}}}}
	}
	runUpdateCases(t, "RenameSamePath", []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "x"}, {Key: "a", Value: bson.D{{Key: "b", Value: 1}}}},
	}, []struct {
		name   string
		filter bson.D
		update interface{}
		upsert bool
	}{
		{"onto_sub_path", one, rename("s", "s.t"), false},
		{"onto_parent", one, rename("a.b", "a"), false},
		{"onto_sub_path_no_match", none, rename("s", "s.t"), false},
		{"onto_sub_path_upsert", none, rename("s", "s.t"), true},
		{"shared_prefix_only", one, rename("a", "ab"), false},
		{"sibling", one, rename("a.b", "a.c"), false},
	})
}

// Empty field names in either $rename path fail first, and a conflict with an
// operator listed before $rename wins over the same-path check.
func TestRenamePathPrecedence(t *testing.T) {
	one := bson.D{{Key: "_id", Value: 1}}
	none := bson.D{{Key: "_id", Value: 9}}
	rename := func(from, to string) bson.E { return bson.E{Key: "$rename", Value: bson.D{{Key: from, Value: to}}} }
	set := func(field string) bson.E { return bson.E{Key: "$set", Value: bson.D{{Key: field, Value: 1}}} }
	for _, c := range []struct {
		name   string
		filter bson.D
		update bson.D
	}{
		{"target_trailing_dot", one, bson.D{rename("s", "s.")}},
		{"target_leading_dot", one, bson.D{rename("s", ".s")}},
		{"target_empty_element", one, bson.D{rename("s", "s..t")}},
		{"source_empty_element", one, bson.D{rename("a..b", "c")}},
		{"source_leading_dot_no_match", none, bson.D{rename(".s", "c")}},
		{"set_then_rename_same_path", one, bson.D{set("s"), rename("s", "s.t")}},
		{"set_then_rename_same_path_no_match", none, bson.D{set("s"), rename("s", "s.t")}},
		{"rename_same_path_then_set", one, bson.D{rename("s", "s.t"), set("s")}},
		{"set_then_rename_onto_it", one, bson.D{set("q"), rename("s", "q")}},
		{"rename_then_set_its_target", one, bson.D{rename("s", "q"), set("q")}},
	} {
		for _, w := range updateWriters(c.filter, c.update, false) {
			t.Run(c.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "RenamePathPrecedence_" + c.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "x"}}),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
					},
				})
			})
		}
	}
}

// With several errors in one update, mongod reports the first it meets while
// parsing the operators in order.
func TestUpdateErrorPrecedence(t *testing.T) {
	e := func(op string, field string, v interface{}) bson.E {
		return bson.E{Key: op, Value: bson.D{{Key: field, Value: v}}}
	}
	for _, c := range []struct {
		name   string
		update bson.D
	}{
		{"positional_first_then_unknown_modifier", bson.D{e("$set", "$", 1), e("$foo", "a", 1)}},
		{"unknown_modifier_then_positional_first", bson.D{e("$foo", "a", 1), e("$set", "$", 1)}},
		{"rename_same_path_then_positional_first", bson.D{e("$rename", "s", "s.t"), e("$set", "$", 1)}},
		{"positional_first_then_rename_same_path", bson.D{e("$set", "$", 1), e("$rename", "s", "s.t")}},
		{"set_then_rename_to_empty_name", bson.D{e("$set", "s", 1), e("$rename", "s", "s.")}},
		{"rename_to_empty_name_then_set", bson.D{e("$rename", "s", "s."), e("$set", "s", 1)}},
		{"rename_then_inc_below_source", bson.D{e("$rename", "s", "t"), e("$inc", "s.a", 1)}},
		{"set_then_rename_below_target", bson.D{e("$set", "t", 1), e("$rename", "s.a", "t.b")}},
	} {
		for _, w := range updateWriters(bson.D{{Key: "_id", Value: 1}}, c.update, false) {
			t.Run(c.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "UpdateErrorPrecedence_" + c.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "x"}}),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
					},
				})
			})
		}
	}
}
