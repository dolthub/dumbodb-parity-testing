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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Expression input errors in a $group that mongod runs in SBE carry SBE's
// codes, for string, date, set and array operators as for arithmetic ones
// (dolthub/dumbodb#125); the same errors elsewhere carry the classic codes.

func TestSBEInputErrors(t *testing.T) {
	pipelines := map[string]string{
		"concat":             "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$concat\": [\"$s\", \"$v\"]}}}}]",
		"strLenBytes":        "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$strLenBytes\": \"$v\"}}}}]",
		"dateToString":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateToString\": {\"date\": \"$v\", \"format\": \"%Y\"}}}}}]",
		"rangeFrac":          "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$range\": [0, 1.5]}}}}]",
		"rangeStepZero":      "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$range\": [0, 5, 0]}}}}]",
		"rangeStr":           "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$range\": [0, \"$s\"]}}}}]",
		"toUpperObj":         "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$toUpper\": \"$o\"}}}}]",
		"toLowerObj":         "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$toLower\": \"$o\"}}}}]",
		"ltrimNum":           "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$ltrim\": {\"input\": \"$v\"}}}}}]",
		"trimNum":            "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$trim\": {\"input\": \"$v\"}}}}}]",
		"indexOfBytesNum":    "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$indexOfBytes\": [\"$v\", \"a\"]}}}}]",
		"indexOfCPNum":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$indexOfCP\": [\"$v\", \"a\"]}}}}]",
		"regexMatchNum":      "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$regexMatch\": {\"input\": \"$v\", \"regex\": \"a\"}}}}}]",
		"regexFindNum":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$regexFind\": {\"input\": \"$v\", \"regex\": \"a\"}}}}}]",
		"replaceOneNum":      "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$replaceOne\": {\"input\": \"$v\", \"find\": \"a\", \"replacement\": \"b\"}}}}}]",
		"yearStr":            "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$year\": \"$s\"}}}}]",
		"monthStr":           "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$month\": \"$s\"}}}}]",
		"hourStr":            "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$hour\": \"$s\"}}}}]",
		"dateAddStr":         "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateAdd\": {\"startDate\": \"$s\", \"unit\": \"day\", \"amount\": 1}}}}}]",
		"dateDiffStr":        "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateDiff\": {\"startDate\": \"$s\", \"endDate\": \"$s\", \"unit\": \"day\"}}}}}]",
		"dateTruncStr":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateTrunc\": {\"date\": \"$s\", \"unit\": \"day\"}}}}}]",
		"dateToPartsStr":     "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateToParts\": {\"date\": \"$s\"}}}}}]",
		"dateFromStringBad":  "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateFromString\": {\"dateString\": \"$s\"}}}}}]",
		"dateFromPartsBad":   "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateFromParts\": {\"year\": \"$s\"}}}}}]",
		"isoWeekStr":         "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$isoWeek\": \"$s\"}}}}]",
		"weekStr":            "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$week\": \"$s\"}}}}]",
		"secondStr":          "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$second\": \"$s\"}}}}]",
		"concatArraysNum":    "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$concatArrays\": [\"$a\", \"$v\"]}}}}]",
		"setUnionNum":        "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$setUnion\": [\"$a\", \"$v\"]}}}}]",
		"setDifferenceNum":   "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$setDifference\": [\"$a\", \"$v\"]}}}}]",
		"setEqualsNum":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$setEquals\": [\"$a\", \"$v\"]}}}}]",
		"setIntersectionNum": "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$setIntersection\": [\"$a\", \"$v\"]}}}}]",
		"setIsSubsetNum":     "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$setIsSubset\": [\"$a\", \"$v\"]}}}}]",
		"reverseArrayNum":    "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$reverseArray\": \"$v\"}}}}]",
		"isArrayOk":          "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$isArray\": [\"$v\"]}}}}]",
		"objectToArrayNum":   "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$objectToArray\": \"$v\"}}}}]",
		"arrayToObjectNum":   "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$arrayToObject\": \"$v\"}}}}]",
		"sortArrayNum":       "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$sortArray\": {\"input\": \"$v\", \"sortBy\": 1}}}}}]",
		"allElementsTrueNum": "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$anyElementTrue\": [\"$v\"]}}}}]",
		"roundBad":           "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$round\": [\"$v\", \"$s\"]}}}}]",
		"truncBad":           "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$trunc\": [\"$v\", 1000]}}}}]",
		"powStr":             "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$pow\": [\"$s\", 2]}}}}]",
		"expStr":             "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$exp\": \"$s\"}}}}]",
		"sqrtNeg":            "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$sqrt\": -1}}}}]",
		"tsSecondNum":        "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$tsSecond\": \"$v\"}}}}]",
		"tsIncrementNum":     "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$tsIncrement\": \"$v\"}}}}]",
		"switchNoMatch":      "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$switch\": {\"branches\": [{\"case\": false, \"then\": 1}]}}}}}]",
		"mergeObjectsNum":    "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$mergeObjects\": [\"$o\", \"$v\"]}}}}]",
		"bsonSizeNum":        "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$bsonSize\": \"$v\"}}}}]",
		"dateSubtractStr":    "[{\"$group\": {\"_id\": null, \"r\": {\"$push\": {\"$dateSubtract\": {\"startDate\": \"$s\", \"unit\": \"day\", \"amount\": 1}}}}}]",
		"bucketNoDefault":    "[{\"$bucket\": {\"groupBy\": \"$v\", \"boundaries\": [0, 1]}}]",
		"switchClassic":      "[{\"$project\": {\"r\": {\"$switch\": {\"branches\": [{\"case\": false, \"then\": 1}]}}}}]",
		"mergeClassic":       "[{\"$project\": {\"r\": {\"$mergeObjects\": [\"$o\", \"$v\"]}}}]",
		"mergeNulls":         "[{\"$project\": {\"_id\": 0, \"r\": {\"$mergeObjects\": [\"$o\", null, \"$nope\"]}}}]",
		"mergeAcc":           "[{\"$group\": {\"_id\": null, \"r\": {\"$mergeObjects\": \"$v\"}}}]",
		"mergeErr":           "[{\"$project\": {\"r\": {\"$mergeObjects\": [{\"$divide\": [1, \"$z\"]}]}}}]",
		"toUpperClassic":     "[{\"$project\": {\"r\": {\"$toUpper\": \"$o\"}}}]",
	}
	for name, pipeline := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+pipeline+`}`), false, &struct {
			P *[]bson.D `bson:"p"`
		}{&stages}); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "SBEInputErrors_" + name,
			Support: harness.DumboDBFull,
			Setup:   sbeCompatSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "SBEInputErrors_BucketNoDefaultClassic",
		Support: harness.DumboDBFull,
		Run: docsAgg(bson.A{bson.D{{Key: "v", Value: int32(5)}}},
			bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{int32(0), int32(1)}}}}}),
	})
}
