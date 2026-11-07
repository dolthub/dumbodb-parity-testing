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

package benchmarks

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Geo benchmarks: a 2dsphere index serves nearest-first queries by scanning
// outward from the query point, so a limit or a maximum distance reads a small
// part of the collection rather than all of it.

// geoPointDoc places document i on a 100 x 100 grid of points 0.01 degrees
// apart.
func geoPointDoc(i int) bson.D {
	return bson.D{
		{Key: "_id", Value: int32(i)},
		{Key: "loc", Value: bson.D{
			{Key: "type", Value: "Point"},
			{Key: "coordinates", Value: bson.A{float64(i%100) * 0.01, float64(i/100) * 0.01}},
		}},
	}
}

var geoKey = bson.D{{Key: "loc", Value: "2dsphere"}}

// geoPoint returns a GeoJSON point inside the grid that moves with i.
func geoPoint(i int) bson.D {
	return bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{float64(i%90)*0.01 + 0.05, float64(i%70)*0.01 + 0.15}}}
}

// geoNearLimit is a $geoNear from geoPoint(i), then a limit of 10.
func geoNearLimit(i int) bson.A {
	return bson.A{
		bson.D{{Key: "$geoNear", Value: bson.D{{Key: "near", Value: geoPoint(i)}, {Key: "distanceField", Value: "dist"}, {Key: "spherical", Value: true}}}},
		bson.D{{Key: "$limit", Value: 10}},
	}
}

func BenchmarkGeoNear_Limit_10K(b *testing.B) {
	col, ctx, dumbo := withIndexUseCollectionN(b, "geo_near_limit", indexUseDocs, options.Index(), geoKey, geoPointDoc)
	requireWork(b, ctx, col, dumbo, aggregateCmd(col, geoNearLimit(0)), docsWithin(indexUseDocs/10), mustUseIndex)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, geoNearLimit(i))
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func nearFilter(i int, maxMeters float64) bson.D {
	near := bson.D{{Key: "$geometry", Value: geoPoint(i)}}
	if maxMeters > 0 {
		near = append(near, bson.E{Key: "$maxDistance", Value: maxMeters})
	}
	return bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: near}}}}
}

func BenchmarkNear_Limit_10K(b *testing.B) {
	col, ctx, dumbo := withIndexUseCollectionN(b, "geo_near_find_limit", indexUseDocs, options.Index(), geoKey, geoPointDoc)
	requireWork(b, ctx, col, dumbo, findCmd(col, nearFilter(0, 0), nil, 10), docsWithin(indexUseDocs/10), mustUseIndex)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runFind(b, ctx, col, nearFilter(i, 0), nil, 10)
	}
}

// A $maxDistance of 2 km holds about ten grid points.
func BenchmarkNear_MaxDistance_10K(b *testing.B) {
	col, ctx, dumbo := withIndexUseCollectionN(b, "geo_near_max_distance", indexUseDocs, options.Index(), geoKey, geoPointDoc)
	requireWork(b, ctx, col, dumbo, findCmd(col, nearFilter(0, 2000), nil, 0), docsWithin(indexUseDocs/10), mustUseIndex)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runFind(b, ctx, col, nearFilter(i, 2000), nil, 0)
	}
}
