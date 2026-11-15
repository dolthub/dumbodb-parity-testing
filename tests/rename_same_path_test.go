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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
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
