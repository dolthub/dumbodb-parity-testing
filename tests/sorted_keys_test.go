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
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// sortKeys returns v with the keys of every embedded document in byte order.
// DumboDB stores documents with sorted keys (workspace-7ob), so cases that are
// not about field order compare this form.
func sortKeys(v interface{}) interface{} {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, len(x))
		for i, e := range x {
			out[i] = bson.E{Key: e.Key, Value: sortKeys(e.Value)}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = sortKeys(e)
		}
		return out
	default:
		return v
	}
}

// pipelineRunSortedKeys is pipelineRunExtJSON with every document's keys
// sorted, so values and types are compared exactly but field order is not.
func pipelineRunSortedKeys(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return errOutcome(err), nil
		}
		var docs []bson.D
		if err = cursor.All(ctx, &docs); err != nil {
			return errOutcome(err), nil
		}
		out := make([]string, len(docs))
		for i, doc := range docs {
			raw, err := bson.Marshal(sortKeys(doc))
			if err != nil {
				return nil, err
			}
			out[i] = bson.Raw(raw).String()
		}
		return out, nil
	}
}
