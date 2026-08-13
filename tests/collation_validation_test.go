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

// Collation validation parity: MongoDB rejects an operation collation whose
// resolved caseFirst/backwards conflicts with a low strength -- these attributes
// are set by a locale's CLDR tailoring even when the caller omits them (Danish
// and Maltese resolve caseFirst=upper; French Canadian resolves backwards=on).
// The locale-sweep surfaced these as error-asymmetry cells; each case here does
// the offending find on both servers and compares (rejected, error code).

import (
	"context"

	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// runCollationValidation issues a find carrying {locale, strength} and reports
// whether the server rejected it and with what error code.
func runCollationValidation(t *testing.T, name, locale string, strength int, support harness.DumboDBSupport) {
	harness.PairTest(t, harness.TestCase{
		Name:    name,
		Support: support,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res := col.Database().RunCommand(ctx, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "collation", Value: bson.D{{Key: "locale", Value: locale}, {Key: "strength", Value: strength}}},
			})
			err := res.Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{
				{Key: "rejected", Value: err != nil},
				{Key: "code", Value: code},
			}, nil
		},
	})
}

// caseFirst-tailored locales rejected at strength <= 2 (BadValue).
func TestCollation_Validate_CaseFirst_da_s1(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_CaseFirst_da_s1", "da", 1, harness.DumboDBFull)
}

func TestCollation_Validate_CaseFirst_da_s2(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_CaseFirst_da_s2", "da", 2, harness.DumboDBFull)
}

func TestCollation_Validate_CaseFirst_mt_s1(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_CaseFirst_mt_s1", "mt", 1, harness.DumboDBFull)
}

func TestCollation_Validate_CaseFirst_mt_s2(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_CaseFirst_mt_s2", "mt", 2, harness.DumboDBFull)
}

// backwards-tailored locale rejected at strength 1 (BadValue).
func TestCollation_Validate_Backwards_frCA_s1(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_Backwards_frCA_s1", "fr_CA", 1, harness.DumboDBFull)
}

// Control: a valid combination (Danish at its default strength) is accepted by
// both servers -- guards against over-rejection.
func TestCollation_Validate_Accepts_da_s3(t *testing.T) {
	runCollationValidation(t, "Collation_Validate_Accepts_da_s3", "da", 3, harness.DumboDBFull)
}
