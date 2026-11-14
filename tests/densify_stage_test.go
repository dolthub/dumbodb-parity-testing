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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $densify (dolthub/dumbodb#186). mongod interleaves partitions in its own
// order under "full" bounds, so results are sorted before comparing.

func densifyNumericSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(1)}, {Key: "v", Value: "one"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(4)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "x", Value: 2.5}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "p", Value: "b"}, {Key: "x", Value: int32(7)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "p", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "p", Value: "b"}, {Key: "x", Value: nil}},
	})
	return err
}

func densifyDateSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "d", Value: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "d", Value: time.Date(2024, 4, 2, 12, 0, 0, 0, time.UTC)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "d", Value: time.Date(2024, 2, 29, 6, 30, 0, 0, time.UTC)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "p", Value: "b"}, {Key: "d", Value: nil}},
	})
	return err
}

func densifyPipeline(spec interface{}) bson.A {
	return bson.A{
		bson.D{{Key: "$densify", Value: spec}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "p", Value: 1}, {Key: "x", Value: 1}, {Key: "d", Value: 1}, {Key: "y.z", Value: 1}, {Key: "v", Value: 1}}}},
	}
}

func dz(s string) primitive.Decimal128 {
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestDensifyStage(t *testing.T) {
	rng := func(step, bounds interface{}, extra ...bson.E) bson.D {
		return append(bson.D{{Key: "step", Value: step}, {Key: "bounds", Value: bounds}}, extra...)
	}
	spec := func(field string, r bson.D, extra ...bson.E) bson.D {
		return append(bson.D{{Key: "field", Value: field}, {Key: "range", Value: r}}, extra...)
	}
	byP := bson.E{Key: "partitionByFields", Value: bson.A{"p"}}
	day := bson.E{Key: "unit", Value: "day"}
	jan := func(d int) time.Time { return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC) }
	numeric := []struct {
		name string
		spec interface{}
	}{
		{"Full", spec("x", rng(1, "full"))},
		{"FullPartitioned", spec("x", rng(1, "full"), byP)},
		{"Partition", spec("x", rng(1, "partition"), byP)},
		{"Explicit", spec("x", rng(2, bson.A{0, 6}))},
		{"ExplicitPartitioned", spec("x", rng(2, bson.A{0, 6}), byP)},
		{"ExplicitBeyondData", spec("x", rng(3, bson.A{2, 20}), byP)},
		{"ExplicitDouble", spec("x", rng(1.5, bson.A{3.0, 5.0}))},
		{"FullDecimalStep", spec("x", rng(dz("1.5"), "full"))},
		{"ExplicitDecimal", spec("x", rng(dz("2"), bson.A{dz("0"), dz("6")}))},
		{"StepLargerThanRange", spec("x", rng(100, "full"))},
		{"DottedFieldMissingEverywhere", spec("y.z", rng(2, bson.A{0, 5}))},
		{"ErrNotObject", int32(5)},
		{"ErrEmpty", bson.D{}},
		{"ErrNoRange", bson.D{{Key: "field", Value: "x"}}},
		{"ErrNoStep", spec("x", bson.D{{Key: "bounds", Value: "full"}})},
		{"ErrNoBounds", spec("x", bson.D{{Key: "step", Value: 1}})},
		{"ErrStepZero", spec("x", rng(0, "full"))},
		{"ErrStepNegative", spec("x", rng(-1, "full"))},
		{"ErrStepString", spec("x", rng("a", "full"))},
		{"ErrBoundsString", spec("x", rng(1, "other"))},
		{"ErrBoundsDescending", spec("x", rng(1, bson.A{5, 1}))},
		{"ErrBoundsOneElement", spec("x", rng(1, bson.A{1}))},
		{"ErrBoundsStrings", spec("x", rng(1, bson.A{"a", "b"}))},
		{"ErrBoundsTypeVsStep", spec("x", rng(2.0, bson.A{0, 6}))},
		{"ErrBoundsLongVsInt", spec("x", rng(int64(2), bson.A{0, 6}))},
		{"ErrBoundsMixed", spec("x", rng(2, bson.A{0, 6.0}))},
		{"ErrDateBoundsNoUnit", spec("x", rng(1, bson.A{jan(1), jan(4)}))},
		{"ErrDateAndNumberBounds", spec("x", rng(1, bson.A{jan(1), 5}, day))},
		{"ErrNumericFieldWithUnit", spec("x", rng(1, "full", day))},
		{"ErrDateBoundsNumericField", spec("x", rng(1, bson.A{jan(1), jan(4)}, day))},
		{"ErrPartitionsNotArray", spec("x", rng(1, "full"), bson.E{Key: "partitionByFields", Value: "p"})},
		{"ErrPartitionIncludesField", spec("x", rng(1, "full"), bson.E{Key: "partitionByFields", Value: bson.A{"x"}})},
		{"ErrPartitionDollar", spec("x", rng(1, "full"), bson.E{Key: "partitionByFields", Value: bson.A{"$p"}})},
		{"ErrFieldDollar", spec("$x", rng(1, "full"))},
		{"ErrUnknownField", spec("x", rng(1, "full"), bson.E{Key: "extra", Value: 1})},
		{"ErrUnknownRangeField", spec("x", rng(1, "full", bson.E{Key: "extra", Value: 1}))},
		{"ErrPartitionBoundsWithoutFields", spec("x", rng(1, "partition"))},
		{"ErrUnknownUnit", spec("x", rng(1, "full", bson.E{Key: "unit", Value: "decade"}))},
		{"ErrBoundsNumber", spec("x", rng(1, 5))},
		{"ErrBoundsObject", spec("x", rng(1, bson.D{}))},
		{"ErrBoundsNull", spec("x", rng(1, nil))},
		{"ErrFieldNull", bson.D{{Key: "field", Value: nil}, {Key: "range", Value: rng(1, "full")}}},
		{"ErrRangeNull", bson.D{{Key: "field", Value: "x"}, {Key: "range", Value: nil}}},
		{"ErrStepNull", spec("x", rng(nil, "full"))},
		{"NullPartitionByFields", spec("x", rng(1, "full"), bson.E{Key: "partitionByFields", Value: nil})},
		{"NullUnit", spec("x", rng(1, "full", bson.E{Key: "unit", Value: nil}))},
	}
	for _, c := range numeric {
		harness.PairTest(t, harness.TestCase{
			Name:    "Densify_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   densifyNumericSeed,
			Run:     pipelineRun(densifyPipeline(c.spec)),
		})
	}

	dates := []struct {
		name string
		spec interface{}
	}{
		{"DateMonth", spec("d", rng(1, "full", bson.E{Key: "unit", Value: "month"}))},
		{"DateWeekPartitioned", spec("d", rng(3, "full", bson.E{Key: "unit", Value: "week"}), byP)},
		{"DatePartitionMonth", spec("d", rng(1, "partition", bson.E{Key: "unit", Value: "month"}), byP)},
		{"DateHours", spec("d", rng(12, "full", bson.E{Key: "unit", Value: "hour"}))},
		{"DateExplicit", spec("d", rng(1, bson.A{jan(1), jan(4)}, day))},
		{"DateExplicitPartitioned", spec("d", rng(1, bson.A{jan(29), time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)}, day), byP)},
		{"DateQuarterLongStep", spec("d", rng(int64(1), "full", bson.E{Key: "unit", Value: "quarter"}))},
		{"DateWholeDoubleStep", spec("d", rng(2.0, "full", bson.E{Key: "unit", Value: "week"}))},
		{"DateMilliseconds", spec("d", rng(3600000, bson.A{jan(1), time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)}, bson.E{Key: "unit", Value: "millisecond"}))},
		{"ErrDateFractionalStep", spec("d", rng(1.5, "full", day))},
		{"ErrDateFieldWithoutUnit", spec("d", rng(1, "full"))},
	}
	for _, c := range dates {
		harness.PairTest(t, harness.TestCase{
			Name:    "Densify_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   densifyDateSeed,
			Run:     pipelineRun(densifyPipeline(c.spec)),
		})
	}
}

func TestDensifyStage_Errors(t *testing.T) {
	cases := []struct {
		name string
		docs []interface{}
		spec bson.D
	}{
		{"StringValue", []interface{}{bson.D{{Key: "x", Value: int32(1)}}, bson.D{{Key: "x", Value: "s"}}},
			bson.D{{Key: "field", Value: "x"}, {Key: "range", Value: bson.D{{Key: "step", Value: 1}, {Key: "bounds", Value: "full"}}}}},
		{"TooManyDocuments", []interface{}{bson.D{{Key: "x", Value: int32(0)}}, bson.D{{Key: "x", Value: int32(600000)}}},
			bson.D{{Key: "field", Value: "x"}, {Key: "range", Value: bson.D{{Key: "step", Value: 1}, {Key: "bounds", Value: "full"}}}}},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    "Densify_Err" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, c.docs)
				return err
			},
			Run: pipelineRun(bson.A{bson.D{{Key: "$densify", Value: c.spec}}, bson.D{{Key: "$count", Value: "n"}}}),
		})
	}
}
