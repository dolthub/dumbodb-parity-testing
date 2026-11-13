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

package fuzz

import (
	"math"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Signature groups findings that are likely one bug: the class, the command
// kind, the hinted index, and the filter's shape with each value replaced by
// its type.
func Signature(class Class, step Step) string {
	return string(class) + " " + step.Kind + " hint=" + step.Hint + " " + FilterShape(step.Filter)
}

// FilterShape keeps field names and operators. Values become a type name;
// MinKey, MaxKey, null and NaN keep their own names because index bounds
// treat them specially. An array becomes its sorted distinct element shapes.
func FilterShape(v any) string {
	switch t := v.(type) {
	case bson.D:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, e.Key+":"+FilterShape(e.Value))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case bson.A:
		seen := map[string]bool{}
		var parts []string
		for _, e := range t {
			s := FilterShape(e)
			if !seen[s] {
				seen[s] = true
				parts = append(parts, s)
			}
		}
		sort.Strings(parts)
		return "[" + strings.Join(parts, ",") + "]"
	case nil:
		return "null"
	case primitive.MinKey:
		return "minKey"
	case primitive.MaxKey:
		return "maxKey"
	case float64:
		if math.IsNaN(t) {
			return "nan"
		}
		return "num"
	case int32, int64, int, primitive.Decimal128:
		return "num"
	case string:
		return "str"
	case bool:
		return "bool"
	case primitive.Regex:
		return "regex"
	case primitive.DateTime:
		return "date"
	case primitive.ObjectID:
		return "oid"
	}
	return "other"
}
