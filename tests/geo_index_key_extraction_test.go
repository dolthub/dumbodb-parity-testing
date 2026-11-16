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

// Geo index builds and writes refuse values mongod cannot index: a 2dsphere
// value that is not valid GeoJSON or an in-range legacy point, 16755, and a
// 2d value that is not an in-range legacy point, 16804, 13068 or 13027
// (dolthub/dumbodb#242).

import (
	"context"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestGeoIndexKeyExtraction(t *testing.T) {
	point := func(x, y float64) bson.D {
		return bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{x, y}}}
	}
	values := []struct {
		name  string
		value interface{}
	}{
		{"linestring_huge", bson.D{{Key: "type", Value: "LineString"}, {Key: "coordinates", Value: bson.A{bson.A{-1e308, 0}, bson.A{1e308, 0}}}}},
		{"linestring_ok", bson.D{{Key: "type", Value: "LineString"}, {Key: "coordinates", Value: bson.A{bson.A{0, 0}, bson.A{1, 1}}}}},
		{"linestring_one_point", bson.D{{Key: "type", Value: "LineString"}, {Key: "coordinates", Value: bson.A{bson.A{0, 0}}}}},
		{"point_lng_out", point(500, 0)},
		{"point_lat_out", point(0, 95)},
		{"point_nan", point(math.NaN(), 0)},
		{"point_ok", point(1, 2)},
		{"polygon_ok", bson.D{{Key: "type", Value: "Polygon"}, {Key: "coordinates", Value: bson.A{bson.A{bson.A{0, 0}, bson.A{1, 0}, bson.A{1, 1}, bson.A{0, 0}}}}}},
		{"polygon_two_vertices", bson.D{{Key: "type", Value: "Polygon"}, {Key: "coordinates", Value: bson.A{bson.A{bson.A{0, 0}, bson.A{1, 1}, bson.A{0, 0}}}}}},
		{"polygon_open", bson.D{{Key: "type", Value: "Polygon"}, {Key: "coordinates", Value: bson.A{bson.A{bson.A{0, 0}, bson.A{1, 0}, bson.A{1, 1}, bson.A{0, 1}}}}}},
		{"multilinestring_out", bson.D{{Key: "type", Value: "MultiLineString"}, {Key: "coordinates", Value: bson.A{bson.A{bson.A{0, 0}, bson.A{200, 1}}}}}},
		{"collection_out", bson.D{{Key: "type", Value: "GeometryCollection"}, {Key: "geometries", Value: bson.A{point(1, 100)}}}},
		{"unknown_type", bson.D{{Key: "type", Value: "Foo"}, {Key: "coordinates", Value: bson.A{1, 2}}}},
		{"legacy_ok", bson.A{1, 2}},
		{"legacy_lng_out", bson.A{500, 0}},
		{"legacy_lat_out", bson.A{0, 95}},
		{"legacy_three", bson.A{1, 2, 3}},
		{"legacy_one", bson.A{1}},
		{"legacy_strings", bson.A{"a", "b"}},
		{"legacy_pairs", bson.A{bson.A{1, 2}, bson.A{3, 4}}},
		{"legacy_document", bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 2}}},
		{"string", "abc"},
		{"number", 5},
		{"null", nil},
		{"empty_document", bson.D{}},
		{"empty_array", bson.A{}},
	}
	var cases []indexWriteCase
	for _, v := range values {
		doc := bson.D{{Key: "loc", Value: v.value}}
		cases = append(cases,
			indexWriteCase{"2dsphere_" + v.name, bson.D{{Key: "loc", Value: "2dsphere"}}, nil, doc, doc},
			indexWriteCase{"2d_" + v.name, bson.D{{Key: "loc", Value: "2d"}}, nil, doc, doc},
		)
	}
	runIndexWriteCases(t, "GeoIndexKeyExtraction", cases)
}

// Index options and value shapes the first geo cases leave out: 2d bounds
// from min and max, partial and sparse indexes, document coordinates, array
// paths, empty multi-geometries and non-finite 2d points.
func TestGeoIndexKeyExtractionShapes(t *testing.T) {
	sphere := bson.D{{Key: "loc", Value: "2dsphere"}}
	planar := bson.D{{Key: "loc", Value: "2d"}}
	loc := func(v interface{}) bson.D { return bson.D{{Key: "loc", Value: v}} }
	geo := func(typ string, coords interface{}) bson.D {
		return bson.D{{Key: "type", Value: typ}, {Key: "coordinates", Value: coords}}
	}
	nested := func(locs ...interface{}) bson.D {
		a := bson.A{}
		for _, l := range locs {
			a = append(a, bson.D{{Key: "loc", Value: l}})
		}
		return bson.D{{Key: "a", Value: a}}
	}
	bounds := func(min, max float64) *options.IndexOptions { return options.Index().SetMin(min).SetMax(max) }
	partial := options.Index().SetPartialFilterExpression(bson.D{{Key: "k", Value: 1}})
	sparse := options.Index().SetSparse(true)
	type c = indexWriteCase
	runIndexWriteCases(t, "GeoIndexKeyExtractionShapes", []indexWriteCase{
		c{"2d_bounds_inside", planar, bounds(0, 1000), loc(bson.A{500, 500}), loc(bson.A{500, 500})},
		c{"2d_bounds_below", planar, bounds(0, 1000), loc(bson.A{-5, 5}), loc(bson.A{-5, 5})},
		c{"2d_small_bounds", planar, bounds(-10, 10), loc(bson.A{50, 0}), nil},
		c{"2dsphere_partial_excluded", sphere, partial, bson.D{{Key: "k", Value: 2}, {Key: "loc", Value: "abc"}}, nil},
		c{"2dsphere_partial_included", sphere, partial, bson.D{{Key: "k", Value: 1}, {Key: "loc", Value: "abc"}}, nil},
		c{"2dsphere_sparse_string", sphere, sparse, loc("abc"), nil},
		c{"2d_partial_excluded", planar, partial, bson.D{{Key: "k", Value: 2}, {Key: "loc", Value: bson.A{500, 0}}}, nil},
		c{"2dsphere_point_document_coordinates", sphere, nil, loc(geo("Point", bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 2}})), nil},
		c{"2d_array_of_point_documents", planar, nil, loc(bson.A{bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 2}}, bson.D{{Key: "x", Value: 3}, {Key: "y", Value: 4}}}), nil},
		c{"2dsphere_legacy_document_out_of_range", sphere, nil, loc(bson.D{{Key: "x", Value: 500}, {Key: "y", Value: 0}}), nil},
		c{"2dsphere_legacy_document_string", sphere, nil, loc(bson.D{{Key: "x", Value: "a"}, {Key: "y", Value: 1}}), nil},
		c{"2dsphere_legacy_document_one_field", sphere, nil, loc(bson.D{{Key: "x", Value: 1}}), nil},
		c{"2dsphere_coordinates_without_type", sphere, nil, loc(bson.D{{Key: "coordinates", Value: bson.A{1, 2}}}), nil},
		c{"2dsphere_linestring_one_vertex", sphere, nil, loc(geo("LineString", bson.A{bson.A{0, 0}, bson.A{0, 0}})), nil},
		c{"2dsphere_polygon_empty", sphere, nil, loc(geo("Polygon", bson.A{})), nil},
		c{"2dsphere_multipoint_empty", sphere, nil, loc(geo("MultiPoint", bson.A{})), nil},
		c{"2dsphere_multilinestring_empty", sphere, nil, loc(geo("MultiLineString", bson.A{})), nil},
		c{"2dsphere_multipolygon_empty", sphere, nil, loc(geo("MultiPolygon", bson.A{})), nil},
		c{"2dsphere_collection_empty", sphere, nil, loc(bson.D{{Key: "type", Value: "GeometryCollection"}, {Key: "geometries", Value: bson.A{}}}), nil},
		c{"2dsphere_array_path", bson.D{{Key: "a.loc", Value: "2dsphere"}}, nil, nested(bson.A{0, 0}, bson.A{0, 100}), nil},
		c{"2dsphere_array_path_ok", bson.D{{Key: "a.loc", Value: "2dsphere"}}, nil, nested(bson.A{0, 0}, bson.A{1, 1}), nil},
		c{"2d_pair_and_number", planar, nil, loc(bson.A{bson.A{1, 2}, 5}), nil},
		c{"2d_nested_pair", planar, nil, loc(bson.A{bson.A{bson.A{1, 2}}}), nil},
		c{"2d_array_path", bson.D{{Key: "a.loc", Value: "2d"}}, nil, nested(bson.A{0, 0}, bson.A{300, 0}), nil},
		c{"2d_nan", planar, nil, loc(bson.A{math.NaN(), 0}), nil},
		c{"2d_infinity", planar, nil, loc(bson.A{math.Inf(1), 0}), nil},
	})
}

// A 2d index's min, max and bits options must describe a valid region.
// geo2DIndexOptionsXFail lists the cases DumboDB fails until it validates 2d
// bits and ranges as mongod does.
var geo2DIndexOptionsXFail = map[string]bool{
	"TestGeo2DIndexOptions/bits_decimal":                 true,
	"TestGeo2DIndexOptions/bits_decimal_too_large":       true,
	"TestGeo2DIndexOptions/bits_fraction_above_max":      true,
	"TestGeo2DIndexOptions/bits_fraction_below_one":      true,
	"TestGeo2DIndexOptions/range_overflows":              true,
	"TestGeo2DIndexOptions/range_too_large_for_one_bit":  true,
	"TestGeo2DIndexOptions/range_too_large_to_hash":      true,
	"TestGeo2DIndexOptions/range_too_large_to_hash_wide": true,
	"TestGeo2DIndexOptions/range_too_small":              true,
}

func TestGeo2DIndexOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  bson.D
		opts bson.D
	}{
		{"min_nan", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: math.NaN()}}},
		{"min_above_max", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: 10}, {Key: "max", Value: -10}}},
		{"min_equals_max", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: 5}, {Key: "max", Value: 5}}},
		{"min_string", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: "x"}}},
		{"max_infinity", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "max", Value: math.Inf(1)}}},
		{"bits_too_large", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 1000000}}},
		{"bits_zero", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 0}}},
		{"bits_max", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 32}}},
		{"bits_string", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: "x"}}},
		{"valid_bounds", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: -10}, {Key: "max", Value: 10}}},
		{"2dsphere_bounds_ignored", bson.D{{Key: "loc", Value: "2dsphere"}}, bson.D{{Key: "min", Value: 10}, {Key: "max", Value: -10}}},
		{"plain_bounds_ignored", bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "min", Value: 10}, {Key: "max", Value: -10}}},
		{"bits_fraction_below_one", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 0.5}}},
		{"bits_fraction_above_max", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 32.5}}},
		{"bits_decimal", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: decimal("5")}}},
		{"bits_decimal_too_large", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: decimal("33")}}},
		{"bits_null", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: nil}}},
		{"range_too_small", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: 0}, {Key: "max", Value: 1e-300}}},
		{"range_overflows", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: -1e308}, {Key: "max", Value: 1e308}}},
		{"min_negative_infinity", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: math.Inf(-1)}}},
		{"range_largest_hashable", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: 0}, {Key: "max", Value: 6.3e161}}},
		{"range_too_large_to_hash", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: 0}, {Key: "max", Value: 6.4e161}}},
		{"range_too_large_to_hash_wide", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "min", Value: -1e200}, {Key: "max", Value: 1e200}}},
		{"range_too_large_for_one_bit", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 1}, {Key: "min", Value: 0}, {Key: "max", Value: 1.9e154}}},
		{"range_hashable_with_one_bit", bson.D{{Key: "loc", Value: "2d"}}, bson.D{{Key: "bits", Value: 1}, {Key: "min", Value: 0}, {Key: "max", Value: 1.8e154}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Geo2DIndexOptions_" + tc.name,
				Support: supportFor(t, geo2DIndexOptionsXFail),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					spec := append(bson.D{{Key: "key", Value: tc.key}, {Key: "name", Value: "ix"}}, tc.opts...)
					err := col.Database().RunCommand(ctx, bson.D{
						{Key: "createIndexes", Value: col.Name()},
						{Key: "indexes", Value: bson.A{spec}},
					}).Err()
					return errCode(err), nil
				},
			})
		})
	}
}

func decimal(s string) primitive.Decimal128 {
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		panic(err)
	}
	return d
}
