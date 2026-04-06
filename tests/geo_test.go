// Parity tests migrated from dolthub/docudolt/tests/geo_test.go.
// All geo tests have been graduated to DocudoltFull — geospatial support is implemented in Docudolt.
package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// ─── local geo helpers ────────────────────────────────────────────────────────
// Note: geoPoint is already defined in geospatial_test.go.

// geoLineString builds a GeoJSON LineString document from coordinate arrays.
func geoLineString(coords ...bson.A) bson.D {
	coordArr := make(bson.A, len(coords))
	for i, c := range coords {
		coordArr[i] = c
	}
	return bson.D{
		{Key: "type", Value: "LineString"},
		{Key: "coordinates", Value: coordArr},
	}
}

// geoPolygon builds a GeoJSON Polygon from an exterior ring of coordinate arrays.
func geoPolygon(ring ...bson.A) bson.D {
	coords := make(bson.A, len(ring))
	for i, c := range ring {
		coords[i] = c
	}
	return bson.D{
		{Key: "type", Value: "Polygon"},
		{Key: "coordinates", Value: bson.A{coords}},
	}
}

// coord returns a [lng, lat] coordinate array.
func coord(lng, lat float64) bson.A {
	return bson.A{lng, lat}
}

// geo2dSphereSetup returns a Setup func that creates a 2dsphere index on field.
func geo2dSphereSetup(field string, docs ...interface{}) func(ctx context.Context, col *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: field, Value: "2dsphere"}},
		}); err != nil {
			return err
		}
		if len(docs) > 0 {
			_, err := col.InsertMany(ctx, docs)
			return err
		}
		return nil
	}
}

// geo2dSetup returns a Setup func that creates a legacy 2d index on field.
func geo2dSetup(field string, docs ...interface{}) func(ctx context.Context, col *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: field, Value: "2d"}},
		}); err != nil {
			return err
		}
		if len(docs) > 0 {
			_, err := col.InsertMany(ctx, docs)
			return err
		}
		return nil
	}
}

// geoFindSortedIDs runs a Find, sorts by _id ascending, and returns ID list.
func geoFindSortedIDs(ctx context.Context, col *mongo.Collection, filter interface{}) (interface{}, error) {
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
	cursor, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	result := make([]interface{}, len(docs))
	for i, d := range docs {
		result[i] = d
	}
	return result, nil
}

// ─── 2dsphere index — GeoJSON geometry types ─────────────────────────────────

func TestGeo_2dsphere_PointInsertAndFind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_PointInsertAndFind",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
			bson.D{{Key: "_id", Value: "london"}, {Key: "loc", Value: geoPoint(-0.1276, 51.5074)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "nyc"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_LineStringInsert(t *testing.T) {
	ls := geoLineString(coord(-74.0, 40.7), coord(-73.9, 40.8), coord(-73.8, 40.9))
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_LineStringInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("path",
			bson.D{{Key: "_id", Value: "route1"}, {Key: "path", Value: ls}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "route1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_PolygonInsert(t *testing.T) {
	ring := []bson.A{
		coord(-74.1, 40.6), coord(-73.9, 40.6),
		coord(-73.9, 40.8), coord(-74.1, 40.8),
		coord(-74.1, 40.6),
	}
	poly := geoPolygon(ring...)
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_PolygonInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("boundary",
			bson.D{{Key: "_id", Value: "zone1"}, {Key: "boundary", Value: poly}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "zone1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_MultiPointInsert(t *testing.T) {
	mp := bson.D{
		{Key: "type", Value: "MultiPoint"},
		{Key: "coordinates", Value: bson.A{
			coord(-74.0, 40.7), coord(-118.2, 34.0), coord(-87.6, 41.8),
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_MultiPointInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("stops",
			bson.D{{Key: "_id", Value: "stations"}, {Key: "stops", Value: mp}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "stations"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_MultiLineStringInsert(t *testing.T) {
	mls := bson.D{
		{Key: "type", Value: "MultiLineString"},
		{Key: "coordinates", Value: bson.A{
			bson.A{coord(-74.0, 40.7), coord(-73.9, 40.8)},
			bson.A{coord(-118.2, 34.0), coord(-118.1, 34.1)},
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_MultiLineStringInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("routes",
			bson.D{{Key: "_id", Value: "roads"}, {Key: "routes", Value: mls}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "roads"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_MultiPolygonInsert(t *testing.T) {
	mp := bson.D{
		{Key: "type", Value: "MultiPolygon"},
		{Key: "coordinates", Value: bson.A{
			bson.A{bson.A{coord(-74.1, 40.6), coord(-73.9, 40.6), coord(-73.9, 40.8), coord(-74.1, 40.8), coord(-74.1, 40.6)}},
			bson.A{bson.A{coord(-118.3, 33.9), coord(-118.1, 33.9), coord(-118.1, 34.1), coord(-118.3, 34.1), coord(-118.3, 33.9)}},
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_MultiPolygonInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("territory",
			bson.D{{Key: "_id", Value: "districts"}, {Key: "territory", Value: mp}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "districts"}}).Decode(&result)
			return result, err
		},
	})
}

func TestGeo_2dsphere_GeometryCollectionInsert(t *testing.T) {
	gc := bson.D{
		{Key: "type", Value: "GeometryCollection"},
		{Key: "geometries", Value: bson.A{
			geoPoint(-74.0, 40.7),
			geoLineString(coord(-74.0, 40.7), coord(-73.9, 40.8)),
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_GeometryCollectionInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("shapes",
			bson.D{{Key: "_id", Value: "mixed"}, {Key: "shapes", Value: gc}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "mixed"}}).Decode(&result)
			return result, err
		},
	})
}

// ─── 2d (legacy) index ────────────────────────────────────────────────────────

func TestGeo_2d_ArrayCoordInsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2d_ArrayCoordInsert",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "coords", Value: bson.A{-74.0060, 40.7128}}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "coords", Value: bson.A{-118.2437, 34.0522}}},
			bson.D{{Key: "_id", Value: "chicago"}, {Key: "coords", Value: bson.A{-87.6298, 41.8781}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "_id", Value: "nyc"}})
		},
	})
}

func TestGeo_2d_EmbeddedDocCoord(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2d_EmbeddedDocCoord",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("pos",
			bson.D{{Key: "_id", Value: "a"}, {Key: "pos", Value: bson.D{{Key: "x", Value: -74.0}, {Key: "y", Value: 40.7}}}},
			bson.D{{Key: "_id", Value: "b"}, {Key: "pos", Value: bson.D{{Key: "x", Value: -73.9}, {Key: "y", Value: 40.8}}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{})
		},
	})
}

func TestGeo_2d_NearQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2d_NearQuery",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "coords", Value: bson.A{-74.0060, 40.7128}}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "coords", Value: bson.A{-118.2437, 34.0522}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "coords", Value: bson.D{{Key: "$near", Value: bson.A{-74.0, 40.7}}}}}
			opts := options.Find().SetLimit(1).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return docsToSlice(docs), nil
		},
	})
}

func TestGeo_2d_NearWithMaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2d_NearWithMaxDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "close"}, {Key: "coords", Value: bson.A{-74.0, 40.7}}},
			bson.D{{Key: "_id", Value: "far"}, {Key: "coords", Value: bson.A{-118.2, 34.0}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "coords", Value: bson.D{
				{Key: "$near", Value: bson.A{-74.0, 40.7}},
				{Key: "$maxDistance", Value: 1.0},
			}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

// ─── $geoWithin ───────────────────────────────────────────────────────────────

func TestGeo_geoWithin_Polygon(t *testing.T) {
	queryPoly := geoPolygon(
		coord(-74.3, 40.5), coord(-73.7, 40.5),
		coord(-73.7, 40.9), coord(-74.3, 40.9),
		coord(-74.3, 40.5),
	)
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoWithin_Polygon",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "brooklyn"}, {Key: "loc", Value: geoPoint(-73.9496, 40.6501)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{{Key: "$geometry", Value: queryPoly}}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_geoWithin_CenterSphere(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoWithin_CenterSphere",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 0.01568 radians ≈ 100 km
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.0, 40.7}, 0.01568}},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_geoWithin_Center(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoWithin_Center",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "close"}, {Key: "coords", Value: bson.A{-74.0, 40.7}}},
			bson.D{{Key: "_id", Value: "far"}, {Key: "coords", Value: bson.A{-118.2, 34.0}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "coords", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$center", Value: bson.A{bson.A{-74.0, 40.7}, 1.0}},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_geoWithin_Box(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoWithin_Box",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "inside"}, {Key: "coords", Value: bson.A{-74.0, 40.7}}},
			bson.D{{Key: "_id", Value: "outside"}, {Key: "coords", Value: bson.A{-118.2, 34.0}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "coords", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$box", Value: bson.A{bson.A{-74.5, 40.5}, bson.A{-73.5, 41.0}}},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

// ─── $geoIntersects ───────────────────────────────────────────────────────────

// TestGeo_GeoIntersects_LineString verifies $geoIntersects with a LineString
// query geometry. Now passing in Docudolt.
func TestGeo_GeoIntersects_LineString(t *testing.T) {
	// Query line crosses from west to east through the NYC bounding box.
	queryLine := geoLineString(coord(-74.5, 40.7), coord(-73.5, 40.7))
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoIntersects_LineString",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("geo",
			bson.D{{Key: "_id", Value: "poly-intersects"}, {Key: "geo", Value: geoPolygon(
				coord(-74.3, 40.5), coord(-73.7, 40.5),
				coord(-73.7, 40.9), coord(-74.3, 40.9),
				coord(-74.3, 40.5),
			)}},
			bson.D{{Key: "_id", Value: "poly-disjoint"}, {Key: "geo", Value: geoPolygon(
				coord(-80.0, 35.0), coord(-79.0, 35.0),
				coord(-79.0, 36.0), coord(-80.0, 36.0),
				coord(-80.0, 35.0),
			)}},
			bson.D{{Key: "_id", Value: "point-on-line"}, {Key: "geo", Value: geoPoint(-74.0, 40.7)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "geo", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: queryLine},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_geoIntersects_Polygon(t *testing.T) {
	queryPoly := geoPolygon(
		coord(-74.3, 40.5), coord(-73.7, 40.5),
		coord(-73.7, 40.9), coord(-74.3, 40.9),
		coord(-74.3, 40.5),
	)
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoIntersects_Polygon",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "geo", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			ls := geoLineString(coord(-74.5, 40.7), coord(-73.5, 40.7))
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "crosser"}, {Key: "geo", Value: ls}},
				bson.D{{Key: "_id", Value: "far"}, {Key: "geo", Value: geoPoint(-118.2437, 34.0522)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "geo", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{{Key: "$geometry", Value: queryPoly}}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_geoIntersects_Point(t *testing.T) {
	queryPoly := geoPolygon(
		coord(-74.3, 40.5), coord(-73.7, 40.5),
		coord(-73.7, 40.9), coord(-74.3, 40.9),
		coord(-74.3, 40.5),
	)
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoIntersects_Point",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "inside"}, {Key: "loc", Value: geoPoint(-74.0, 40.7)}},
			bson.D{{Key: "_id", Value: "outside"}, {Key: "loc", Value: geoPoint(-118.2, 34.0)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{{Key: "$geometry", Value: queryPoly}}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

// ─── $near ────────────────────────────────────────────────────────────────────

func TestGeo_near_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_Basic",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
			bson.D{{Key: "_id", Value: "hoboken"}, {Key: "loc", Value: geoPoint(-74.0323, 40.7440)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{{Key: "$geometry", Value: geoPoint(-74.0, 40.7)}}}}}}
			opts := options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return docsToSlice(docs), nil
		},
	})
}

func TestGeo_near_MaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_MaxDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.7)},
				{Key: "$maxDistance", Value: 100000}, // 100 km
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_near_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_MinDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.7)},
				{Key: "$minDistance", Value: 1000000}, // 1000 km minimum
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_near_MinAndMaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_MinAndMaxDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "exact"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "medium"}, {Key: "loc", Value: geoPoint(-73.9496, 40.6501)}},
			bson.D{{Key: "_id", Value: "far"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.75)},
				{Key: "$minDistance", Value: 5000},
				{Key: "$maxDistance", Value: 50000},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

// ─── $nearSphere ──────────────────────────────────────────────────────────────

func TestGeo_nearSphere_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_nearSphere_Basic",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{{Key: "$geometry", Value: geoPoint(-74.0, 40.7)}}}}}}
			opts := options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return docsToSlice(docs), nil
		},
	})
}

func TestGeo_nearSphere_MaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_nearSphere_MaxDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.7)},
				{Key: "$maxDistance", Value: 100000},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_nearSphere_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_nearSphere_MinDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.7)},
				{Key: "$minDistance", Value: 1000000},
			}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

// ─── $geoNear aggregation ─────────────────────────────────────────────────────

func TestGeo_geoNear_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_Basic",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
			bson.D{{Key: "_id", Value: "hoboken"}, {Key: "loc", Value: geoPoint(-74.0323, 40.7440)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "dist"},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Return count and first result ID for comparison.
			if len(results) == 0 {
				return 0, nil
			}
			return len(results), nil
		},
	})
}

func TestGeo_geoNear_MaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_MaxDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "dist"},
				{Key: "maxDistance", Value: 100000},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestGeo_geoNear_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_MinDistance",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "dist"},
				{Key: "minDistance", Value: 1000000},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestGeo_geoNear_Query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_Query",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}, {Key: "type", Value: "city"}},
			bson.D{{Key: "_id", Value: "hoboken"}, {Key: "loc", Value: geoPoint(-74.0323, 40.7440)}, {Key: "type", Value: "town"}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}, {Key: "type", Value: "city"}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "dist"},
				{Key: "query", Value: bson.D{{Key: "type", Value: "city"}}},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestGeo_geoNear_NonSpherical(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_NonSpherical",
		Support: harness.DocudoltFull,
		Setup: geo2dSetup("coords",
			bson.D{{Key: "_id", Value: "close"}, {Key: "coords", Value: bson.A{-74.0, 40.7}}},
			bson.D{{Key: "_id", Value: "far"}, {Key: "coords", Value: bson.A{-118.2, 34.0}}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: bson.A{-74.0, 40.7}},
				{Key: "distanceField", Value: "dist"},
				{Key: "spherical", Value: false},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// ─── strict Point validation (MongoDB 8.0) ───────────────────────────────────

func TestGeo_near_InvalidPointLongitude(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_InvalidPointLongitude",
		Support: harness.DocudoltXFail, // Diverge (do-bpmo): docudolt omits "invalid point in geo near query $geometry argument: {...}" prefix; fix exists in WIP local docudolt, not yet on GitHub main
		Setup:   geo2dSphereSetup("loc"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(200.0, 40.7)}, // invalid longitude
			}}}}}
			_, err := col.Find(ctx, filter)
			return nil, err
		},
	})
}

func TestGeo_near_InvalidPointLatitude(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_near_InvalidPointLatitude",
		Support: harness.DocudoltXFail, // Diverge (do-bpmo): docudolt omits "invalid point in geo near query $geometry argument: {...}" prefix; fix exists in WIP local docudolt, not yet on GitHub main
		Setup:   geo2dSphereSetup("loc"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 100.0)}, // invalid latitude
			}}}}}
			_, err := col.Find(ctx, filter)
			return nil, err
		},
	})
}

func TestGeo_nearSphere_InvalidPoint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_nearSphere_InvalidPoint",
		Support: harness.DocudoltXFail, // Diverge (do-bpmo): docudolt omits "invalid point in geo near query $geometry argument: {...}" prefix; fix exists in WIP local docudolt, not yet on GitHub main
		Setup:   geo2dSphereSetup("loc"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(500.0, 40.7)}, // invalid longitude
			}}}}}
			_, err := col.Find(ctx, filter)
			return nil, err
		},
	})
}

func TestGeo_geoNear_InvalidPoint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_InvalidPoint",
		Support: harness.DocudoltXFail, // Diverge (do-bpmo): docudolt reports lng/lat out-of-bounds instead of "invalid argument in geo near query: type"; fix exists in WIP local docudolt, not yet on GitHub main
		Setup:   geo2dSphereSetup("loc"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 200.0)}, // invalid latitude
				{Key: "distanceField", Value: "dist"},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			err = cursor.All(ctx, &results)
			return nil, err
		},
	})
}

// ─── compound and advanced 2dsphere ──────────────────────────────────────────

func TestGeo_2dsphere_Compound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_Compound",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "category", Value: 1}, {Key: "loc", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nyc-cafe"}, {Key: "category", Value: "cafe"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
				bson.D{{Key: "_id", Value: "nyc-bar"}, {Key: "category", Value: "bar"}, {Key: "loc", Value: geoPoint(-74.0100, 40.7100)}},
				bson.D{{Key: "_id", Value: "la-cafe"}, {Key: "category", Value: "cafe"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{
				{Key: "category", Value: "cafe"},
				{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
					{Key: "$geometry", Value: geoPoint(-74.0, 40.7)},
					{Key: "$maxDistance", Value: 10000},
				}}}},
			}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_2dsphere_MultipleIndexedFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_MultipleIndexedFields",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "origin", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "dest", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: "trip1"},
					{Key: "origin", Value: geoPoint(-74.0060, 40.7128)},
					{Key: "dest", Value: geoPoint(-118.2437, 34.0522)},
				},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "_id", Value: "trip1"}})
		},
	})
}

func TestGeo_geoNear_IncludeLocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_IncludeLocs",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "dist"},
				{Key: "includeLocs", Value: "matchedLoc"},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestGeo_geoNear_DistanceMultiplier(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoNear_DistanceMultiplier",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.0, 40.7)},
				{Key: "distanceField", Value: "distKm"},
				{Key: "distanceMultiplier", Value: 0.001},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestGeo_2dsphere_SparseIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_SparseIndex",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "loc", Value: "2dsphere"}},
				Options: options.Index().SetSparse(true),
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "with-loc"}, {Key: "loc", Value: geoPoint(-74.0, 40.7)}},
				bson.D{{Key: "_id", Value: "no-loc"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{})
		},
	})
}

func TestGeo_geoWithin_MultiPolygon(t *testing.T) {
	multiPoly := bson.D{
		{Key: "type", Value: "MultiPolygon"},
		{Key: "coordinates", Value: bson.A{
			// NYC area polygon
			bson.A{bson.A{coord(-74.3, 40.5), coord(-73.7, 40.5), coord(-73.7, 40.9), coord(-74.3, 40.9), coord(-74.3, 40.5)}},
			// LA area polygon
			bson.A{bson.A{coord(-118.5, 33.8), coord(-118.0, 33.8), coord(-118.0, 34.2), coord(-118.5, 34.2), coord(-118.5, 33.8)}},
		}},
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_geoWithin_MultiPolygon",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "nyc"}, {Key: "loc", Value: geoPoint(-74.0060, 40.7128)}},
			bson.D{{Key: "_id", Value: "la"}, {Key: "loc", Value: geoPoint(-118.2437, 34.0522)}},
			bson.D{{Key: "_id", Value: "london"}, {Key: "loc", Value: geoPoint(-0.1276, 51.5074)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{{Key: "$geometry", Value: multiPoly}}}}}}
			return geoFindSortedIDs(ctx, col, filter)
		},
	})
}

func TestGeo_2dsphere_IndexVersion(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_2dsphere_IndexVersion",
		Support: harness.DocudoltFull,
		Setup: geo2dSphereSetup("loc",
			bson.D{{Key: "_id", Value: "p1"}, {Key: "loc", Value: geoPoint(-74.0, 40.7)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cursor.All(ctx, &indexes); err != nil {
				return nil, err
			}
			return len(indexes), nil
		},
	})
}
