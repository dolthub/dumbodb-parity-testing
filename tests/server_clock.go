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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// serverNowMarker replaces a server clock reading, which cannot be equal
// across two servers, once it is known to be a current date.
const serverNowMarker = "<now>"

// maskServerNow replaces the named top-level fields of doc with
// serverNowMarker when they hold a date within a minute of the local clock.
// Any other value (wrong type, stale date) is left in place to diverge.
func maskServerNow(doc bson.D, keys ...string) bson.D {
	mask := make(map[string]bool, len(keys))
	for _, k := range keys {
		mask[k] = true
	}
	result := make(bson.D, len(doc))
	for i, elem := range doc {
		if mask[elem.Key] && isCurrentDate(elem.Value) {
			elem.Value = serverNowMarker
		}
		result[i] = elem
	}
	return result
}

func isCurrentDate(v interface{}) bool {
	d, ok := v.(primitive.DateTime)
	if !ok {
		return false
	}
	skew := time.Since(d.Time())
	return skew > -time.Minute && skew < time.Minute
}
