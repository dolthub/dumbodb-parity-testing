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
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
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
	runIndexWriteCases(t, "GeoIndexKeyExtraction", nil, cases)
}
