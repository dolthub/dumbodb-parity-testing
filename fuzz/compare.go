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
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Class is how a DumboDB result differs from MongoDB's.
type Class string

const (
	Match Class = ""
	// Mismatch is a different result or error.
	Mismatch Class = "mismatch"
	// OrderOnly is the same documents in a different order.
	OrderOnly Class = "order-only"
	// ServerFailure is a server that stopped answering.
	ServerFailure Class = "server-failure"
)

// Outcome is one server's answer to one command, reduced to what is compared.
type Outcome struct {
	Value       any
	Err         error
	Unreachable bool
}

// ignoredReplyFields vary between servers without saying anything about the
// documents a command saw or changed.
var ignoredReplyFields = map[string]bool{
	"ok": true, "operationTime": true, "$clusterTime": true, "electionId": true, "opTime": true, "errmsg": true,
}

// Execute runs cmd and keeps a cursor's first batch, or else the reply
// without ignoredReplyFields.
func Execute(ctx context.Context, db *mongo.Database, cmd bson.D) Outcome {
	var reply bson.D
	err := db.RunCommand(ctx, cmd).Decode(&reply)
	if err != nil {
		unreachable := mongo.IsNetworkError(err) || mongo.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded)
		return Outcome{Err: err, Unreachable: unreachable}
	}
	if cursor, ok := reply.Map()["cursor"].(bson.D); ok {
		return Outcome{Value: canonical(cursor.Map()["firstBatch"])}
	}
	return Outcome{Value: canonical(reply)}
}

// canonical sorts document fields so field order never counts as a
// difference, and drops ignoredReplyFields.
func canonical(v any) any {
	switch t := v.(type) {
	case bson.D:
		out := bson.D{}
		for _, e := range t {
			if !ignoredReplyFields[e.Key] {
				out = append(out, bson.E{Key: e.Key, Value: canonical(e.Value)})
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out
	case bson.A:
		out := bson.A{}
		for _, e := range t {
			out = append(out, canonical(e))
		}
		return out
	}
	return v
}

// Render is the outcome as relaxed Extended JSON, or its error.
func (o Outcome) Render() string {
	if o.Err != nil {
		code, name, _ := harness.CommandErrorCode(o.Err)
		if name != "" {
			return fmt.Sprintf("error %d %s: %v", code, name, o.Err)
		}
		return "error: " + o.Err.Error()
	}
	return extJSON(o.Value, false)
}

// extJSON renders v; canonical keeps every BSON type, relaxed is for reading.
func extJSON(v any, canonical bool) string {
	b, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: v}}, canonical, false)
	if err != nil {
		return fmt.Sprintf("<unrenderable: %v>", err)
	}
	return string(b[len(`{"v":`) : len(b)-1])
}

// Compare classifies dumbodb's outcome against mongo's and says what differs.
func Compare(mongoOut, dumboOut Outcome) (Class, string) {
	if mongoOut.Unreachable || dumboOut.Unreachable {
		return ServerFailure, fmt.Sprintf("mongodb: %s; dumbodb: %s", mongoOut.Render(), dumboOut.Render())
	}
	if mongoOut.Err != nil || dumboOut.Err != nil {
		c := harness.CompareResponses(nil, mongoOut.Err, nil, dumboOut.Err)
		if c.Result == harness.Match {
			return Match, ""
		}
		return Mismatch, c.Diff
	}
	m, d := extJSON(mongoOut.Value, true), extJSON(dumboOut.Value, true)
	if m == d {
		return Match, ""
	}
	ma, mOK := mongoOut.Value.(bson.A)
	da, dOK := dumboOut.Value.(bson.A)
	if !mOK || !dOK {
		return Mismatch, fmt.Sprintf("mongodb %s, dumbodb %s", mongoOut.Render(), dumboOut.Render())
	}
	missing, extra := multisetDifference(ma, da)
	if len(missing) == 0 && len(extra) == 0 {
		return OrderOnly, fmt.Sprintf("same %d documents; mongodb order %s, dumbodb order %s",
			len(ma), mongoOut.Render(), dumboOut.Render())
	}
	return Mismatch, fmt.Sprintf("dumbodb lacks %s and adds %s (mongodb %d documents, dumbodb %d)",
		strings.Join(missing, ", "), strings.Join(extra, ", "), len(ma), len(da))
}

// multisetDifference lists, in relaxed Extended JSON, the elements of mongo
// that dumbo lacks and those dumbo adds.
func multisetDifference(mongoElems, dumboElems bson.A) (missing, extra []string) {
	counts := map[string]int{}
	for _, e := range mongoElems {
		counts[extJSON(e, true)]++
	}
	for _, e := range dumboElems {
		counts[extJSON(e, true)]--
	}
	for _, e := range mongoElems {
		if k := extJSON(e, true); counts[k] > 0 {
			counts[k]--
			missing = append(missing, extJSON(e, false))
		}
	}
	for _, e := range dumboElems {
		if k := extJSON(e, true); counts[k] < 0 {
			counts[k]++
			extra = append(extra, extJSON(e, false))
		}
	}
	return missing, extra
}
