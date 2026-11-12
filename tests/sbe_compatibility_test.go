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
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod 8.0 runs a $group in SBE only when every expression and accumulator
// in it, and in the stages pushed down with it, can run there. A division by
// zero next to each operator shows which engine ran: SBE reports 4848401, the
// classic engine 2.

func sbeCompatSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "v", Value: int32(5)}, {Key: "z", Value: int32(0)}, {Key: "a", Value: bson.A{int32(1), int32(2)}},
		{Key: "s", Value: "ab"}, {Key: "d", Value: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Key: "o", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "ts", Value: primitive.Timestamp{T: 1, I: 1}},
	})
	return err
}

func TestSBECompatibility_Operators(t *testing.T) {
	groups := map[string]string{
		"$abs":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$abs\": \"$v\"}}}",
		"$acos":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$acos\": 0.5}}}",
		"$acosh":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$acosh\": 2}}}",
		"$add":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$add\": [\"$v\", \"$v\"]}}}",
		"$allElementsTrue":  "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$allElementsTrue\": [[\"$v\"]]}}}",
		"$and":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$and\": [\"$v\", \"$v\"]}}}",
		"$arrayToObject":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$arrayToObject\": [[[\"k\", \"$v\"]]]}}}",
		"$asin":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$asin\": 0.5}}}",
		"$asinh":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$asinh\": \"$v\"}}}",
		"$atan":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$atan\": \"$v\"}}}",
		"$atan2":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$atan2\": [\"$v\", \"$v\"]}}}",
		"$atanh":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$atanh\": 0.5}}}",
		"$avg":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$avg\": \"$a\"}}}",
		"$anyElementTrue":   "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$anyElementTrue\": [[\"$v\"]]}}}",
		"$arrayElemAt":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$arrayElemAt\": [\"$a\", 0]}}}",
		"$binarySize":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$binarySize\": \"$s\"}}}",
		"$bsonSize":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$bsonSize\": \"$o\"}}}",
		"$ceil":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$ceil\": \"$v\"}}}",
		"$cmp":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$cmp\": [\"$v\", \"$v\"]}}}",
		"$concat":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$concat\": [\"$s\", \"$s\"]}}}",
		"$concatArrays":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$concatArrays\": [\"$a\", \"$a\"]}}}",
		"$cond":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$cond\": [true, \"$v\", 1]}}}",
		"$convert":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$convert\": {\"input\": \"$v\", \"to\": \"string\"}}}}",
		"$cos":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$cos\": \"$v\"}}}",
		"$cosh":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$cosh\": \"$v\"}}}",
		"$dateAdd":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateAdd\": {\"startDate\": \"$d\", \"unit\": \"day\", \"amount\": 1}}}}",
		"$dateDiff":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateDiff\": {\"startDate\": \"$d\", \"endDate\": \"$d\", \"unit\": \"day\"}}}}",
		"$dateFromParts":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateFromParts\": {\"year\": 2020}}}}",
		"$dateFromString":   "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateFromString\": {\"dateString\": \"2020-01-01\"}}}}",
		"$dateSubtract":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateSubtract\": {\"startDate\": \"$d\", \"unit\": \"day\", \"amount\": 1}}}}",
		"$dateToParts":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateToParts\": {\"date\": \"$d\"}}}}",
		"$dateTrunc":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateTrunc\": {\"date\": \"$d\", \"unit\": \"day\"}}}}",
		"$dateToString":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dateToString\": {\"date\": \"$d\", \"format\": \"%Y\"}}}}",
		"$degreesToRadians": "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$degreesToRadians\": \"$v\"}}}",
		"$dayOfMonth":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dayOfMonth\": \"$d\"}}}",
		"$dayOfWeek":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dayOfWeek\": \"$d\"}}}",
		"$dayOfYear":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$dayOfYear\": \"$d\"}}}",
		"$eq":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$eq\": [\"$v\", \"$v\"]}}}",
		"$exp":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$exp\": \"$v\"}}}",
		"$filter":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$filter\": {\"input\": \"$a\", \"cond\": true}}}}",
		"$first":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$first\": \"$a\"}}}",
		"$firstN":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$firstN\": {\"input\": \"$a\", \"n\": 1}}}}",
		"$floor":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$floor\": \"$v\"}}}",
		"$getField":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$getField\": \"v\"}}}",
		"$gt":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$gt\": [\"$v\", \"$v\"]}}}",
		"$gte":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$gte\": [\"$v\", \"$v\"]}}}",
		"$hour":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$hour\": \"$d\"}}}",
		"$ifNull":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$ifNull\": [\"$v\", 1]}}}",
		"$in":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$in\": [\"$v\", \"$a\"]}}}",
		"$indexOfArray":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$indexOfArray\": [\"$a\", 1]}}}",
		"$indexOfBytes":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$indexOfBytes\": [\"$s\", \"a\"]}}}",
		"$indexOfCP":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$indexOfCP\": [\"$s\", \"a\"]}}}",
		"$isArray":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$isArray\": [\"$a\"]}}}",
		"$isNumber":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$isNumber\": \"$v\"}}}",
		"$isoDayOfWeek":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$isoDayOfWeek\": \"$d\"}}}",
		"$isoWeek":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$isoWeek\": \"$d\"}}}",
		"$isoWeekYear":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$isoWeekYear\": \"$d\"}}}",
		"$last":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$last\": \"$a\"}}}",
		"$lastN":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$lastN\": {\"input\": \"$a\", \"n\": 1}}}}",
		"$let":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$let\": {\"vars\": {\"x\": 1}, \"in\": \"$$x\"}}}}",
		"$literal":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$literal\": 1}}}",
		"$ln":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$ln\": \"$v\"}}}",
		"$log":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$log\": [\"$v\", 2]}}}",
		"$log10":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$log10\": \"$v\"}}}",
		"$lt":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$lt\": [\"$v\", \"$v\"]}}}",
		"$lte":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$lte\": [\"$v\", \"$v\"]}}}",
		"$ltrim":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$ltrim\": {\"input\": \"$s\"}}}}",
		"$map":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$map\": {\"input\": \"$a\", \"in\": \"$$this\"}}}}",
		"$max":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$max\": \"$a\"}}}",
		"$maxN":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$maxN\": {\"input\": \"$a\", \"n\": 1}}}}",
		"$median":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$median\": {\"input\": \"$a\", \"method\": \"approximate\"}}}}",
		"$mergeObjects":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$mergeObjects\": [\"$o\", \"$o\"]}}}",
		"$millisecond":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$millisecond\": \"$d\"}}}",
		"$min":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$min\": \"$a\"}}}",
		"$minN":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$minN\": {\"input\": \"$a\", \"n\": 1}}}}",
		"$minute":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$minute\": \"$d\"}}}",
		"$mod":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$mod\": [\"$v\", 2]}}}",
		"$month":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$month\": \"$d\"}}}",
		"$multiply":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$multiply\": [\"$v\", \"$v\"]}}}",
		"$ne":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$ne\": [\"$v\", \"$v\"]}}}",
		"$not":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$not\": [\"$v\"]}}}",
		"$objectToArray":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$objectToArray\": \"$o\"}}}",
		"$or":               "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$or\": [\"$v\", \"$v\"]}}}",
		"$percentile":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$percentile\": {\"input\": \"$a\", \"p\": [0.5], \"method\": \"approximate\"}}}}",
		"$pow":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$pow\": [\"$v\", \"$v\"]}}}",
		"$radiansToDegrees": "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$radiansToDegrees\": \"$v\"}}}",
		"$rand":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$rand\": {}}}}",
		"$range":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$range\": [0, \"$v\"]}}}",
		"$reduce":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$reduce\": {\"input\": \"$a\", \"initialValue\": 0, \"in\": \"$$value\"}}}}",
		"$regexFind":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$regexFind\": {\"input\": \"$s\", \"regex\": \"a\"}}}}",
		"$regexFindAll":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$regexFindAll\": {\"input\": \"$s\", \"regex\": \"a\"}}}}",
		"$regexMatch":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$regexMatch\": {\"input\": \"$s\", \"regex\": \"a\"}}}}",
		"$replaceAll":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$replaceAll\": {\"input\": \"$s\", \"find\": \"a\", \"replacement\": \"b\"}}}}",
		"$replaceOne":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$replaceOne\": {\"input\": \"$s\", \"find\": \"a\", \"replacement\": \"b\"}}}}",
		"$reverseArray":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$reverseArray\": \"$a\"}}}",
		"$round":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$round\": \"$v\"}}}",
		"$rtrim":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$rtrim\": {\"input\": \"$s\"}}}}",
		"$second":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$second\": \"$d\"}}}",
		"$setField":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setField\": {\"field\": \"x\", \"input\": \"$o\", \"value\": 1}}}}",
		"$setDifference":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setDifference\": [\"$a\", \"$a\"]}}}",
		"$setEquals":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setEquals\": [\"$a\", \"$a\"]}}}",
		"$setIntersection":  "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setIntersection\": [\"$a\", \"$a\"]}}}",
		"$setIsSubset":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setIsSubset\": [\"$a\", \"$a\"]}}}",
		"$setUnion":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$setUnion\": [\"$a\", \"$a\"]}}}",
		"$sin":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$sin\": \"$v\"}}}",
		"$sinh":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$sinh\": \"$v\"}}}",
		"$size":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$size\": \"$a\"}}}",
		"$slice":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$slice\": [\"$a\", 1]}}}",
		"$sortArray":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$sortArray\": {\"input\": \"$a\", \"sortBy\": 1}}}}",
		"$split":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$split\": [\"$s\", \"a\"]}}}",
		"$sqrt":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$sqrt\": \"$v\"}}}",
		"$stdDevPop":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$stdDevPop\": \"$a\"}}}",
		"$stdDevSamp":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$stdDevSamp\": \"$a\"}}}",
		"$strcasecmp":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$strcasecmp\": [\"$s\", \"$s\"]}}}",
		"$strLenBytes":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$strLenBytes\": \"$s\"}}}",
		"$strLenCP":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$strLenCP\": \"$s\"}}}",
		"$substr":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$substr\": [\"$s\", 0, 1]}}}",
		"$substrBytes":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$substrBytes\": [\"$s\", 0, 1]}}}",
		"$substrCP":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$substrCP\": [\"$s\", 0, 1]}}}",
		"$subtract":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$subtract\": [\"$v\", \"$v\"]}}}",
		"$sum":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$sum\": \"$a\"}}}",
		"$switch":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$switch\": {\"branches\": [{\"case\": true, \"then\": 1}]}}}}",
		"$tan":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$tan\": \"$v\"}}}",
		"$tanh":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$tanh\": \"$v\"}}}",
		"$toBool":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toBool\": \"$v\"}}}",
		"$toDate":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toDate\": \"$v\"}}}",
		"$toDecimal":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toDecimal\": \"$v\"}}}",
		"$toDouble":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toDouble\": \"$v\"}}}",
		"$toInt":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toInt\": \"$v\"}}}",
		"$toLong":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toLong\": \"$v\"}}}",
		"$toLower":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toLower\": \"$s\"}}}",
		"$toObjectId":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toObjectId\": \"$oid\"}}}",
		"$toString":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toString\": \"$v\"}}}",
		"$toUUID":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toUUID\": \"$s\"}}}",
		"$toUpper":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$toUpper\": \"$s\"}}}",
		"$unsetField":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$unsetField\": {\"field\": \"x\", \"input\": \"$o\"}}}}",
		"$trim":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$trim\": {\"input\": \"$s\"}}}}",
		"$trunc":            "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$trunc\": \"$v\"}}}",
		"$tsIncrement":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$tsIncrement\": \"$ts\"}}}",
		"$tsSecond":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$tsSecond\": \"$ts\"}}}",
		"$type":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$type\": \"$v\"}}}",
		"$week":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$week\": \"$d\"}}}",
		"$year":             "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$year\": \"$d\"}}}",
		"$zip":              "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": {\"$zip\": {\"inputs\": [\"$a\", \"$a\"]}}}}",
		"acc$addToSet":      "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$addToSet\": \"$v\"}}",
		"acc$avg":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$avg\": \"$v\"}}",
		"acc$bottom":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$bottom\": {\"output\": \"$v\", \"sortBy\": {\"v\": 1}}}}",
		"acc$bottomN":       "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$bottomN\": {\"output\": \"$v\", \"sortBy\": {\"v\": 1}, \"n\": 1}}}",
		"acc$count":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$count\": {}}}",
		"acc$first":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$first\": \"$v\"}}",
		"acc$firstN":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$firstN\": {\"input\": \"$v\", \"n\": 1}}}",
		"acc$last":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$last\": \"$v\"}}",
		"acc$lastN":         "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$lastN\": {\"input\": \"$v\", \"n\": 1}}}",
		"acc$max":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$max\": \"$v\"}}",
		"acc$maxN":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$maxN\": {\"input\": \"$v\", \"n\": 1}}}",
		"acc$median":        "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$median\": {\"input\": \"$v\", \"method\": \"approximate\"}}}",
		"acc$mergeObjects":  "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$mergeObjects\": \"$o\"}}",
		"acc$min":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$min\": \"$v\"}}",
		"acc$minN":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$minN\": {\"input\": \"$v\", \"n\": 1}}}",
		"acc$percentile":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$percentile\": {\"input\": \"$v\", \"p\": [0.5], \"method\": \"approximate\"}}}",
		"acc$push":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$push\": \"$v\"}}",
		"acc$stdDevPop":     "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$stdDevPop\": \"$v\"}}",
		"acc$stdDevSamp":    "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$stdDevSamp\": \"$v\"}}",
		"acc$sum":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$sum\": \"$v\"}}",
		"acc$top":           "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$top\": {\"output\": \"$v\", \"sortBy\": {\"v\": 1}}}}",
		"acc$topN":          "{\"_id\": null, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}, \"m\": {\"$topN\": {\"output\": \"$v\", \"sortBy\": {\"v\": 1}, \"n\": 1}}}",
		"id$getField":       "{\"_id\": {\"$getField\": \"v\"}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
		"id$setField":       "{\"_id\": {\"$setField\": {\"field\": \"x\", \"input\": \"$o\", \"value\": 1}}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
		"id$rand":           "{\"_id\": {\"$rand\": {}}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
		"id$size":           "{\"_id\": {\"$size\": \"$a\"}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
		"id$add":            "{\"_id\": {\"$add\": [\"$v\", \"$v\"]}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
		"id$toString":       "{\"_id\": {\"$toString\": \"$v\"}, \"r\": {\"$push\": {\"$divide\": [\"$v\", \"$z\"]}}}",
	}
	for name, group := range groups {
		var spec bson.D
		if err := bson.UnmarshalExtJSON([]byte(group), false, &spec); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "SBECompatibility_" + name,
			Support: harness.DumboDBFull,
			Setup:   sbeCompatSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, bson.D{{Key: "$group", Value: spec}})
			},
		})
	}
}

// Stages pushed down with the $group, views and nesting decide the engine too.
func TestSBECompatibility_Pipelines(t *testing.T) {
	const div = `{"$divide":["$v","$z"]}`
	pipelines := map[string]string{
		"ProjectWithClassicOperator": `[{"$project":{"s":{"$size":"$a"},"v":1,"z":1}},{"$group":{"_id":null,"r":{"$push":` + div + `}}}]`,
		"MatchExprClassicOperator":   `[{"$match":{"$expr":{"$in":[1,"$a"]}}},{"$group":{"_id":null,"r":{"$push":` + div + `}}}]`,
		"MatchQueryIn":               `[{"$match":{"v":{"$in":[5]}}},{"$group":{"_id":null,"r":{"$push":` + div + `}}}]`,
		"LimitThenComputedProject":   `[{"$limit":4},{"$project":{"r":` + div + `}},{"$group":{"_id":"$r"}}]`,
		"LimitThenInclusionProject":  `[{"$limit":4},{"$project":{"v":1,"z":1}},{"$group":{"_id":` + div + `}}]`,
		"AddTwoDates":                `[{"$group":{"_id":null,"r":{"$push":{"$add":["$d","$d"]}}}}]`,
		"UnionWithInLookup":          `[{"$lookup":{"from":"COLL","pipeline":[{"$limit":1},{"$unionWith":{"coll":"COLL","pipeline":[{"$group":{"_id":null,"r":{"$push":` + div + `}}}]}}],"as":"j"}}]`,
		"UnionWithInFacet":           `[{"$facet":{"f":[{"$unionWith":{"coll":"COLL","pipeline":[{"$group":{"_id":null,"r":{"$push":` + div + `}}}]}}]}}]`,
		"UnionWithOverSetView":       `[{"$unionWith":{"coll":"COLL_vs","pipeline":[{"$group":{"_id":null,"r":{"$push":` + div + `}}}]}}]`,
		"UnionWithOverGroupView":     `[{"$unionWith":{"coll":"COLL_vg"}}]`,
		"LookupFromViewThenGroup":    `[{"$lookup":{"from":"COLL_vs","localField":"v","foreignField":"v","as":"j"}},{"$group":{"_id":null,"r":{"$push":` + div + `}}}]`,
	}
	for name, pipeline := range pipelines {
		pipeline := pipeline
		harness.PairTest(t, harness.TestCase{
			Name:    "SBECompatibility_" + name,
			Support: harness.DumboDBFull,
			Setup:   sbeCompatSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				views := map[string]string{
					col.Name() + "_vs": `[{"$set":{"q":1}}]`,
					col.Name() + "_vg": `[{"$group":{"_id":null,"r":{"$push":` + div + `}}}]`,
				}
				for view, def := range views {
					var stages bson.A
					if err := bson.UnmarshalExtJSON([]byte(def), false, &stages); err != nil {
						return nil, err
					}
					if err := db.CreateView(ctx, view, col.Name(), stages); err != nil {
						return nil, err
					}
					defer db.Collection(view).Drop(ctx)
				}
				var stages []bson.D
				text := strings.ReplaceAll(pipeline, "COLL", col.Name())
				if err := bson.UnmarshalExtJSON([]byte(`{"p":`+text+`}`), false, &struct {
					P *[]bson.D `bson:"p"`
				}{&stages}); err != nil {
					return nil, err
				}
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}
