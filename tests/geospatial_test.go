package tests

// Most tests in this file are now DumboDBFull — DumboDB implements geospatial operators.
// A few edge-case tests remain DumboDBXFail.

import (
	"context"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// cityGeoJSON contains world cities as GeoJSON Points for 2dsphere tests.
// Coordinates are [longitude, latitude] per GeoJSON spec.
var cityGeoJSON = []interface{}{
	bson.D{{Key: "_id", Value: "nyc"}, {Key: "name", Value: "New York"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-74.006, 40.7128}}}}},
	bson.D{{Key: "_id", Value: "lax"}, {Key: "name", Value: "Los Angeles"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-118.2437, 34.0522}}}}},
	bson.D{{Key: "_id", Value: "chi"}, {Key: "name", Value: "Chicago"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-87.6298, 41.8781}}}}},
	bson.D{{Key: "_id", Value: "hou"}, {Key: "name", Value: "Houston"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-95.3698, 29.7604}}}}},
	bson.D{{Key: "_id", Value: "phx"}, {Key: "name", Value: "Phoenix"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-112.074, 33.4484}}}}},
	bson.D{{Key: "_id", Value: "phl"}, {Key: "name", Value: "Philadelphia"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-75.1652, 39.9526}}}}},
	bson.D{{Key: "_id", Value: "sat"}, {Key: "name", Value: "San Antonio"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-98.4936, 29.4241}}}}},
	bson.D{{Key: "_id", Value: "sdg"}, {Key: "name", Value: "San Diego"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-117.1611, 32.7157}}}}},
	bson.D{{Key: "_id", Value: "dal"}, {Key: "name", Value: "Dallas"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-96.797, 32.7767}}}}},
	bson.D{{Key: "_id", Value: "sjc"}, {Key: "name", Value: "San Jose"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-121.8863, 37.3382}}}}},
	bson.D{{Key: "_id", Value: "lon"}, {Key: "name", Value: "London"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-0.1276, 51.5074}}}}},
	bson.D{{Key: "_id", Value: "par"}, {Key: "name", Value: "Paris"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{2.3522, 48.8566}}}}},
	bson.D{{Key: "_id", Value: "tok"}, {Key: "name", Value: "Tokyo"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{139.6917, 35.6895}}}}},
	bson.D{{Key: "_id", Value: "syd"}, {Key: "name", Value: "Sydney"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{151.2093, -33.8688}}}}},
	bson.D{{Key: "_id", Value: "sao"}, {Key: "name", Value: "Sao Paulo"},
		{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-46.6333, -23.5505}}}}},
}

// insertCities inserts cityGeoJSON with a 2dsphere index on "loc".
func insertCities(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
	}); err != nil {
		return err
	}
	_, err := col.InsertMany(ctx, cityGeoJSON)
	return err
}

// legacyGeoDocs contains points as legacy [lng, lat] arrays for 2d index tests.
var legacyGeoDocs = []interface{}{
	bson.D{{Key: "_id", Value: "l1"}, {Key: "name", Value: "New York"}, {Key: "pos", Value: bson.A{-74.006, 40.7128}}},
	bson.D{{Key: "_id", Value: "l2"}, {Key: "name", Value: "Los Angeles"}, {Key: "pos", Value: bson.A{-118.2437, 34.0522}}},
	bson.D{{Key: "_id", Value: "l3"}, {Key: "name", Value: "Chicago"}, {Key: "pos", Value: bson.A{-87.6298, 41.8781}}},
	bson.D{{Key: "_id", Value: "l4"}, {Key: "name", Value: "London"}, {Key: "pos", Value: bson.A{-0.1276, 51.5074}}},
	bson.D{{Key: "_id", Value: "l5"}, {Key: "name", Value: "Paris"}, {Key: "pos", Value: bson.A{2.3522, 48.8566}}},
}

// insertLegacy inserts legacyGeoDocs with a 2d index on "pos".
func insertLegacy(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "pos", Value: "2d"}},
	}); err != nil {
		return err
	}
	_, err := col.InsertMany(ctx, legacyGeoDocs)
	return err
}

// geoFindIDs runs a geo query, returns IDs sorted alphabetically for stable comparison.
func geoFindIDs(ctx context.Context, col *mongo.Collection, filter interface{}) (interface{}, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(bson.D{{Key: "_id", Value: 1}})
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

// geoFindNear runs a $near/$nearSphere query and returns results in distance order (closest first).
func geoFindNear(ctx context.Context, col *mongo.Collection, filter interface{}) (interface{}, error) {
	opts := options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "name", Value: 1}})
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

// geoPoint returns a GeoJSON Point document for use in queries.
func geoPoint(lng, lat float64) bson.D {
	return bson.D{
		{Key: "type", Value: "Point"},
		{Key: "coordinates", Value: bson.A{lng, lat}},
	}
}

func TestGeo_Index_2dsphere_Create(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Index_2dsphere_Create",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "p1"},
				{Key: "loc", Value: geoPoint(-74.006, 40.7128)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
			})
			if err != nil {
				return nil, err
			}
			return name, nil
		},
	})
}

func TestGeo_Index_2d_Create(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Index_2d_Create",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "p1"},
				{Key: "pos", Value: bson.A{-74.006, 40.7128}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "pos", Value: "2d"}},
			})
			if err != nil {
				return nil, err
			}
			return name, nil
		},
	})
}

func TestGeo_Near_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_Basic",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find cities within 300km of New York.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
				{Key: "$maxDistance", Value: 300000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_SmallRadius(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_SmallRadius",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 50km radius around NYC — should return just NYC and Philadelphia.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
				{Key: "$maxDistance", Value: 150000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_LargeRadius(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_LargeRadius",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 3000km radius around Chicago — covers most of the continental US.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-87.6298, 41.8781)},
				{Key: "$maxDistance", Value: 3000000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_MinDistance",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Exclude cities closer than 200km to NYC (skips NYC itself and Philadelphia).
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
				{Key: "$minDistance", Value: 200000},
				{Key: "$maxDistance", Value: 2000000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_MinMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_MinMax",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Annular region: 500km–1500km from Los Angeles.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-118.2437, 34.0522)},
				{Key: "$minDistance", Value: 500000},
				{Key: "$maxDistance", Value: 1500000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_NoConstraints(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_NoConstraints",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// No distance constraints — returns all cities ordered by distance from Chicago.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-87.6298, 41.8781)},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Near_WithQueryFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Near_WithQueryFilter",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nyc"}, {Key: "name", Value: "New York"}, {Key: "region", Value: "east"},
					{Key: "loc", Value: geoPoint(-74.006, 40.7128)}},
				bson.D{{Key: "_id", Value: "phl"}, {Key: "name", Value: "Philadelphia"}, {Key: "region", Value: "east"},
					{Key: "loc", Value: geoPoint(-75.1652, 39.9526)}},
				bson.D{{Key: "_id", Value: "chi"}, {Key: "name", Value: "Chicago"}, {Key: "region", Value: "midwest"},
					{Key: "loc", Value: geoPoint(-87.6298, 41.8781)}},
				bson.D{{Key: "_id", Value: "bos"}, {Key: "name", Value: "Boston"}, {Key: "region", Value: "east"},
					{Key: "loc", Value: geoPoint(-71.0589, 42.3601)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $near combined with a regular field filter — only east-region cities.
			filter := bson.D{
				{Key: "region", Value: "east"},
				{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
					{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
					{Key: "$maxDistance", Value: 500000},
				}}}},
			}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_NearSphere_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_NearSphere_Basic",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Cities within 500km of London.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-0.1276, 51.5074)},
				{Key: "$maxDistance", Value: 500000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_NearSphere_MaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_NearSphere_MaxDistance",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Cities within 350km of Paris (London + Paris itself).
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(2.3522, 48.8566)},
				{Key: "$maxDistance", Value: 350000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_NearSphere_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_NearSphere_MinDistance",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Cities more than 300km from Paris but within 2000km.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(2.3522, 48.8566)},
				{Key: "$minDistance", Value: 300000},
				{Key: "$maxDistance", Value: 2000000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_NearSphere_MinMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_NearSphere_MinMax",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Annular region: 500km–3000km from London.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-0.1276, 51.5074)},
				{Key: "$minDistance", Value: 500000},
				{Key: "$maxDistance", Value: 3000000},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_NearSphere_NoConstraints(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_NearSphere_NoConstraints",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// No max/min — all cities sorted by spherical distance from Tokyo.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(139.6917, 35.6895)},
			}}}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_CenterSphere_Tiny(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_CenterSphere_Tiny",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ~55km radius (55/6371 radians) around NYC — should return only NYC.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.006, 40.7128}, 55.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_CenterSphere_Medium(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_CenterSphere_Medium",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ~1200km radius around NYC — northeast US cities.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.006, 40.7128}, 1200.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_CenterSphere_Large(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_CenterSphere_Large",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ~4000km radius around Dallas — covers most of continental US.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-96.797, 32.7767}, 4000.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_CenterSphere_Europe(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_CenterSphere_Europe",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ~600km radius around Paris — covers London and Paris.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{2.3522, 48.8566}, 600.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_Polygon_EastUS(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_Polygon_EastUS",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Polygon covering eastern US. Should match NYC, Philadelphia.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-82.0, 35.0},
						bson.A{-65.0, 35.0},
						bson.A{-65.0, 45.0},
						bson.A{-82.0, 45.0},
						bson.A{-82.0, 35.0},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_Polygon_California(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_Polygon_California",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Rough bounding box for California. Should match LA, San Diego, San Jose.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-124.5, 32.5},
						bson.A{-114.0, 32.5},
						bson.A{-114.0, 42.0},
						bson.A{-124.5, 42.0},
						bson.A{-124.5, 32.5},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_Polygon_NoMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_Polygon_NoMatch",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Polygon in the middle of the Pacific — no cities match.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-180.0, -10.0},
						bson.A{-140.0, -10.0},
						bson.A{-140.0, 10.0},
						bson.A{-180.0, 10.0},
						bson.A{-180.0, -10.0},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_Polygon_Europe(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_Polygon_Europe",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Rough Western Europe polygon. Should match London and Paris.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-10.0, 43.0},
						bson.A{10.0, 43.0},
						bson.A{10.0, 55.0},
						bson.A{-10.0, 55.0},
						bson.A{-10.0, 43.0},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoWithin_MultiPolygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoWithin_MultiPolygon",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// MultiPolygon covering East US and Western Europe simultaneously.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "MultiPolygon"},
					{Key: "coordinates", Value: bson.A{
						// East US polygon
						bson.A{bson.A{
							bson.A{-82.0, 35.0}, bson.A{-65.0, 35.0},
							bson.A{-65.0, 45.0}, bson.A{-82.0, 45.0},
							bson.A{-82.0, 35.0},
						}},
						// Western Europe polygon
						bson.A{bson.A{
							bson.A{-10.0, 43.0}, bson.A{10.0, 43.0},
							bson.A{10.0, 55.0}, bson.A{-10.0, 55.0},
							bson.A{-10.0, 43.0},
						}},
					}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoIntersects_Point_Exact(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoIntersects_Point_Exact",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoIntersects with a Point that matches exactly one document.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoIntersects_Point_NoMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoIntersects_Point_NoMatch",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Point in the ocean — no city documents intersect this point.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-30.0, 10.0)},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoIntersects_Polygon_Contains(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoIntersects_Polygon_Contains",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Polygon containing Chicago and Dallas. Point docs that fall inside it intersect.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-100.0, 28.0},
						bson.A{-85.0, 28.0},
						bson.A{-85.0, 43.0},
						bson.A{-100.0, 43.0},
						bson.A{-100.0, 28.0},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_GeoIntersects_MultiPolygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoIntersects_MultiPolygon",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// MultiPolygon covering western US and Japan.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "MultiPolygon"},
					{Key: "coordinates", Value: bson.A{
						// Western US
						bson.A{bson.A{
							bson.A{-125.0, 30.0}, bson.A{-110.0, 30.0},
							bson.A{-110.0, 42.0}, bson.A{-125.0, 42.0},
							bson.A{-125.0, 30.0},
						}},
						// Japan area
						bson.A{bson.A{
							bson.A{130.0, 30.0}, bson.A{145.0, 30.0},
							bson.A{145.0, 40.0}, bson.A{130.0, 40.0},
							bson.A{130.0, 30.0},
						}},
					}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_DocType_PolygonDocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_DocType_PolygonDocs",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "area", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "r1"}, {Key: "name", Value: "East US region"},
					{Key: "area", Value: bson.D{
						{Key: "type", Value: "Polygon"},
						{Key: "coordinates", Value: bson.A{bson.A{
							bson.A{-82.0, 35.0}, bson.A{-65.0, 35.0},
							bson.A{-65.0, 45.0}, bson.A{-82.0, 45.0},
							bson.A{-82.0, 35.0},
						}}},
					}}},
				bson.D{{Key: "_id", Value: "r2"}, {Key: "name", Value: "West US region"},
					{Key: "area", Value: bson.D{
						{Key: "type", Value: "Polygon"},
						{Key: "coordinates", Value: bson.A{bson.A{
							bson.A{-125.0, 32.0}, bson.A{-110.0, 32.0},
							bson.A{-110.0, 42.0}, bson.A{-125.0, 42.0},
							bson.A{-125.0, 32.0},
						}}},
					}}},
				bson.D{{Key: "_id", Value: "r3"}, {Key: "name", Value: "Europe region"},
					{Key: "area", Value: bson.D{
						{Key: "type", Value: "Polygon"},
						{Key: "coordinates", Value: bson.A{bson.A{
							bson.A{-10.0, 43.0}, bson.A{10.0, 43.0},
							bson.A{10.0, 55.0}, bson.A{-10.0, 55.0},
							bson.A{-10.0, 43.0},
						}}},
					}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoIntersects with a point inside the East US polygon.
			filter := bson.D{{Key: "area", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_DocType_LineStringDocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_DocType_LineStringDocs",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "route", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "rt1"}, {Key: "name", Value: "NYC to Chicago"},
					{Key: "route", Value: bson.D{
						{Key: "type", Value: "LineString"},
						{Key: "coordinates", Value: bson.A{
							bson.A{-74.006, 40.7128}, bson.A{-80.0, 41.0}, bson.A{-87.6298, 41.8781},
						}},
					}}},
				bson.D{{Key: "_id", Value: "rt2"}, {Key: "name", Value: "LA to Phoenix"},
					{Key: "route", Value: bson.D{
						{Key: "type", Value: "LineString"},
						{Key: "coordinates", Value: bson.A{
							bson.A{-118.2437, 34.0522}, bson.A{-112.074, 33.4484},
						}},
					}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find routes that intersect a polygon around the Midwest.
			filter := bson.D{{Key: "route", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-92.0, 38.0}, bson.A{-82.0, 38.0},
						bson.A{-82.0, 45.0}, bson.A{-92.0, 45.0},
						bson.A{-92.0, 38.0},
					}}},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_DocType_MultiPolygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_DocType_MultiPolygon",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "territory", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "t1"}, {Key: "name", Value: "US + Europe territory"},
					{Key: "territory", Value: bson.D{
						{Key: "type", Value: "MultiPolygon"},
						{Key: "coordinates", Value: bson.A{
							bson.A{bson.A{
								bson.A{-130.0, 25.0}, bson.A{-60.0, 25.0},
								bson.A{-60.0, 50.0}, bson.A{-130.0, 50.0},
								bson.A{-130.0, 25.0},
							}},
							bson.A{bson.A{
								bson.A{-10.0, 43.0}, bson.A{20.0, 43.0},
								bson.A{20.0, 55.0}, bson.A{-10.0, 55.0},
								bson.A{-10.0, 43.0},
							}},
						}},
					}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Check that NYC point intersects the MultiPolygon territory.
			filter := bson.D{{Key: "territory", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.006, 40.7128)},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_DocType_GeometryCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_DocType_GeometryCollection",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "geo", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "gc1"}, {Key: "name", Value: "Point + Polygon"},
					{Key: "geo", Value: bson.D{
						{Key: "type", Value: "GeometryCollection"},
						{Key: "geometries", Value: bson.A{
							bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{-74.006, 40.7128}}},
							bson.D{
								{Key: "type", Value: "Polygon"},
								{Key: "coordinates", Value: bson.A{bson.A{
									bson.A{-75.0, 40.0}, bson.A{-73.0, 40.0},
									bson.A{-73.0, 41.5}, bson.A{-75.0, 41.5},
									bson.A{-75.0, 40.0},
								}}},
							},
						}},
					}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoIntersects a point inside the GeometryCollection polygon.
			filter := bson.D{{Key: "geo", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: geoPoint(-74.0, 40.5)},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_Box(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_Box",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $box on 2d index: covers eastern US. Matches New York.
			filter := bson.D{{Key: "pos", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$box", Value: bson.A{
					bson.A{-80.0, 35.0}, // lower-left
					bson.A{-65.0, 45.0}, // upper-right
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_Polygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_Polygon",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $polygon on 2d index: covers Western Europe.
			filter := bson.D{{Key: "pos", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$polygon", Value: bson.A{
					bson.A{-10.0, 43.0},
					bson.A{10.0, 43.0},
					bson.A{10.0, 55.0},
					bson.A{-10.0, 55.0},
				}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_Center(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_Center",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $center on 2d index: 3-degree radius around NYC (covers NYC only in practice).
			filter := bson.D{{Key: "pos", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$center", Value: bson.A{bson.A{-74.006, 40.7128}, 3.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_CenterSphere_On2d(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_CenterSphere_On2d",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $centerSphere works on 2d indexes too; radius in radians.
			filter := bson.D{{Key: "pos", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{2.3522, 48.8566}, 600.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_Near2d(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_Near2d",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $near on 2d index uses flat-earth distance, radius in degrees.
			// ~5 degrees from NYC covers East Coast in flat projection.
			filter := bson.D{{Key: "pos", Value: bson.D{
				{Key: "$near", Value: bson.A{-74.006, 40.7128}},
				{Key: "$maxDistance", Value: 5.0},
			}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_Legacy_NearSphere_2d(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Legacy_NearSphere_2d",
		Support: harness.DumboDBFull,
		Setup:   insertLegacy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $nearSphere on 2d index uses spherical distance, radius in radians.
			// ~15 degrees radius from London covers Western Europe.
			filter := bson.D{{Key: "pos", Value: bson.D{
				{Key: "$nearSphere", Value: bson.A{-0.1276, 51.5074}},
				{Key: "$maxDistance", Value: 0.26}, // ~15 degrees in radians (~1670km)
			}}}
			return geoFindNear(ctx, col, filter)
		},
	})
}

func TestGeo_GeoNear_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoNear_Basic",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoNear: cities nearest to Chicago.
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-87.6298, 41.8781)},
				{Key: "distanceField", Value: "dist"},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline,
				options.Aggregate().SetAllowDiskUse(true))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			// Return just IDs in order (distance order from geoNear).
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				for _, e := range d {
					if e.Key == "_id" {
						result[i] = bson.D{{Key: "_id", Value: e.Value}}
					}
				}
			}
			return result, nil
		},
	})
}

func TestGeo_GeoNear_MaxDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoNear_MaxDistance",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoNear with maxDistance: only cities within 1000km of NYC.
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.006, 40.7128)},
				{Key: "distanceField", Value: "dist"},
				{Key: "maxDistance", Value: 1000000.0},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				for _, e := range d {
					if e.Key == "_id" {
						result[i] = bson.D{{Key: "_id", Value: e.Value}}
					}
				}
			}
			return result, nil
		},
	})
}

func TestGeo_GeoNear_MinDistance(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoNear_MinDistance",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoNear with minDistance: exclude cities too close to NYC.
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.006, 40.7128)},
				{Key: "distanceField", Value: "dist"},
				{Key: "minDistance", Value: 200000.0},
				{Key: "maxDistance", Value: 2000000.0},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				for _, e := range d {
					if e.Key == "_id" {
						result[i] = bson.D{{Key: "_id", Value: e.Value}}
					}
				}
			}
			return result, nil
		},
	})
}

func TestGeo_GeoNear_Query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoNear_Query",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nyc"}, {Key: "name", Value: "New York"}, {Key: "country", Value: "US"},
					{Key: "loc", Value: geoPoint(-74.006, 40.7128)}},
				bson.D{{Key: "_id", Value: "phl"}, {Key: "name", Value: "Philadelphia"}, {Key: "country", Value: "US"},
					{Key: "loc", Value: geoPoint(-75.1652, 39.9526)}},
				bson.D{{Key: "_id", Value: "lon"}, {Key: "name", Value: "London"}, {Key: "country", Value: "UK"},
					{Key: "loc", Value: geoPoint(-0.1276, 51.5074)}},
				bson.D{{Key: "_id", Value: "par"}, {Key: "name", Value: "Paris"}, {Key: "country", Value: "FR"},
					{Key: "loc", Value: geoPoint(2.3522, 48.8566)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoNear with query filter: only US cities near NYC.
			pipeline := bson.A{bson.D{{Key: "$geoNear", Value: bson.D{
				{Key: "near", Value: geoPoint(-74.006, 40.7128)},
				{Key: "distanceField", Value: "dist"},
				{Key: "query", Value: bson.D{{Key: "country", Value: "US"}}},
				{Key: "spherical", Value: true},
			}}}}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				for _, e := range d {
					if e.Key == "_id" {
						result[i] = bson.D{{Key: "_id", Value: e.Value}}
					}
				}
			}
			return result, nil
		},
	})
}

func TestGeo_GeoNear_DistanceMultiplier(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_GeoNear_DistanceMultiplier",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// distanceMultiplier converts meters to km (multiply by 0.001).
			pipeline := bson.A{
				bson.D{{Key: "$geoNear", Value: bson.D{
					{Key: "near", Value: geoPoint(-87.6298, 41.8781)},
					{Key: "distanceField", Value: "distKm"},
					{Key: "distanceMultiplier", Value: 0.001},
					{Key: "maxDistance", Value: 1500000.0},
					{Key: "spherical", Value: true},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "name", Value: 1},
				}}},
			}
			cursor, err := col.Aggregate(ctx, pipeline)
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
		},
	})
}

func TestGeo_Edge_NullLocation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Edge_NullLocation",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "name", Value: "Valid"},
					{Key: "loc", Value: geoPoint(-74.006, 40.7128)}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "name", Value: "Null loc"},
					{Key: "loc", Value: nil}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Null location docs should be excluded from geo query results.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.006, 40.7128}, 100.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Edge_MissingLocation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Edge_MissingLocation",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "name", Value: "With loc"},
					{Key: "loc", Value: geoPoint(-74.006, 40.7128)}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "name", Value: "No loc field"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Documents without the geo field should be excluded from geo queries.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.006, 40.7128}, 100.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Edge_EmptyResult(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Edge_EmptyResult",
		Support: harness.DumboDBFull,
		Setup:   insertCities,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Very small radius in the middle of the ocean — zero results.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{0.0, 0.0}, 1.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

func TestGeo_Edge_MultipleGeoFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Geo_Edge_MultipleGeoFields",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// Index on one geo field; query the other.
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "home", Value: "2dsphere"}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: "u1"},
					{Key: "home", Value: geoPoint(-74.006, 40.7128)},
					{Key: "office", Value: geoPoint(-74.0, 40.72)},
				},
				bson.D{
					{Key: "_id", Value: "u2"},
					{Key: "home", Value: geoPoint(-87.6298, 41.8781)},
					{Key: "office", Value: geoPoint(-87.6, 41.9)},
				},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Query on the indexed field (home).
			filter := bson.D{{Key: "home", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-74.006, 40.7128}, 50.0 / 6371.0}},
			}}}}}
			return geoFindIDs(ctx, col, filter)
		},
	})
}

// TestGeo_NonFinitePoint: a NaN or infinite query point, or a NaN or
// infinite distance bound, in $near, $nearSphere and $geoNear.
func TestGeo_NonFinitePoint(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	near := func(op string, point interface{}, extra ...bson.E) bson.D {
		return bson.D{{Key: "loc", Value: append(bson.D{{Key: op, Value: point}}, extra...)}}
	}
	geometry := func(x, y float64) bson.D { return bson.D{{Key: "$geometry", Value: geoPoint(x, y)}} }
	pairOn := func(field string, op string, x, y float64) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: op, Value: bson.A{x, y}}}}}
	}
	setup2d := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "pos", Value: bson.A{1.0, 1.0}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "pos", Value: bson.A{5.0, 5.0}}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "pos", Value: "2d"}}})
		return err
	}
	find := func(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return geoFindNear(ctx, col, filter)
		}
	}
	geoNearStage := func(near interface{}, extra ...bson.E) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			stage := append(bson.D{{Key: "near", Value: near}, {Key: "distanceField", Value: "d"}, {Key: "spherical", Value: true}}, extra...)
			cur, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$geoNear", Value: stage}}})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return len(docs), err
		}
	}
	cases := []struct {
		name  string
		setup func(context.Context, *mongo.Collection) error
		run   func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"near_geometry_nan", insertCities, find(near("$near", geometry(nan, 0)))},
		{"near_geometry_inf", insertCities, find(near("$near", geometry(inf, 0)))},
		{"near_sphere_legacy_nan", insertCities, find(near("$nearSphere", bson.A{nan, 0.0}))},
		{"near_max_distance_nan", insertCities, find(near("$near", geometry(0, 0), bson.E{Key: "$maxDistance", Value: nan}))},
		{"near_max_distance_inf", insertCities, find(near("$near", geometry(0, 0), bson.E{Key: "$maxDistance", Value: inf}))},
		{"near_min_distance_nan", insertCities, find(near("$near", geometry(0, 0), bson.E{Key: "$minDistance", Value: nan}))},
		{"near_2d_nan", setup2d, find(pairOn("pos", "$near", nan, 0))},
		{"near_2d_inf", setup2d, find(pairOn("pos", "$near", inf, 0))},
		{"geonear_nan", insertCities, geoNearStage(geoPoint(nan, 0))},
		{"geonear_inf", insertCities, geoNearStage(geoPoint(inf, 0))},
		{"geonear_max_distance_nan", insertCities, geoNearStage(geoPoint(0, 0), bson.E{Key: "maxDistance", Value: nan})},
		{"near_inner_max_distance_nan", insertCities, find(near("$near", append(geometry(0, 0), bson.E{Key: "$maxDistance", Value: nan})))},
		{"near_inner_max_distance_inf", insertCities, find(near("$near", append(geometry(0, 0), bson.E{Key: "$maxDistance", Value: inf})))},
		{"near_inner_max_distance_negative", insertCities, find(near("$near", append(geometry(0, 0), bson.E{Key: "$maxDistance", Value: -1.0})))},
		{"near_inner_min_distance_nan", insertCities, find(near("$near", append(geometry(0, 0), bson.E{Key: "$minDistance", Value: nan})))},
		{"near_sphere_geometry_nan", insertCities, find(near("$nearSphere", geometry(nan, 0)))},
		{"near_2d_second_nan", setup2d, find(pairOn("pos", "$near", 0, nan))},
		{"near_2d_max_distance_nan", setup2d, find(bson.D{{Key: "pos", Value: bson.D{{Key: "$near", Value: bson.A{0.0, 0.0}}, {Key: "$maxDistance", Value: nan}}}})},
		{"near_2d_max_distance_inf", setup2d, find(bson.D{{Key: "pos", Value: bson.D{{Key: "$near", Value: bson.A{0.0, 0.0}}, {Key: "$maxDistance", Value: inf}}}})},
		{"geonear_legacy_nan", setup2d, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$geoNear", Value: bson.D{{Key: "near", Value: bson.A{nan, 0.0}}, {Key: "distanceField", Value: "d"}}}}})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return len(docs), err
		}},
		{"geonear_min_distance_nan", insertCities, geoNearStage(geoPoint(0, 0), bson.E{Key: "minDistance", Value: nan})},
		{"geonear_max_distance_inf", insertCities, geoNearStage(geoPoint(0, 0), bson.E{Key: "maxDistance", Value: inf})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Geo_NonFinitePoint_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   tc.setup,
				Run:     tc.run,
			})
		})
	}
}

// TestGeo_NearShapes: nearest-first reads over 2d locations stored as arrays
// of points or {x, y} documents, 2dsphere lines and polygons, the distances
// $geoNear reports, and the locations a 2dsphere index refuses.
func TestGeo_NearShapes(t *testing.T) {
	setup2d := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "pos", Value: bson.A{4.0, 4.0}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "pos", Value: bson.A{bson.A{9.0, 9.0}, bson.A{1.0, 1.0}}}},
			bson.D{{Key: "_id", Value: 3}, {Key: "pos", Value: bson.D{{Key: "x", Value: 2.0}, {Key: "y", Value: 2.0}}}},
			bson.D{{Key: "_id", Value: 4}, {Key: "pos", Value: bson.D{{Key: "y", Value: 3.0}, {Key: "x", Value: 0.5}}}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "pos", Value: "2d"}}})
		return err
	}
	line := func(pts ...bson.A) bson.D {
		return bson.D{{Key: "type", Value: "LineString"}, {Key: "coordinates", Value: bson.A(func() []interface{} {
			out := make([]interface{}, len(pts))
			for i, p := range pts {
				out[i] = p
			}
			return out
		}())}}
	}
	setupSphere := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "loc", Value: geoPoint(3, 0)}},
			bson.D{{Key: "_id", Value: 2}, {Key: "loc", Value: line(bson.A{1.0, -1.0}, bson.A{1.0, 1.0})}},
			bson.D{{Key: "_id", Value: 3}, {Key: "loc", Value: bson.D{{Key: "type", Value: "Polygon"}, {Key: "coordinates", Value: bson.A{bson.A{
				bson.A{2.0, -1.0}, bson.A{2.5, -1.0}, bson.A{2.5, 1.0}, bson.A{2.0, 1.0}, bson.A{2.0, -1.0}}}}}}},
			bson.D{{Key: "_id", Value: 4}, {Key: "loc", Value: geoPoint(0.5, 0.5)}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
		return err
	}
	ids := func(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return geoFindNear(ctx, col, filter)
		}
	}
	geoNear := func(stage bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$geoNear", Value: stage}}, {{Key: "$project", Value: bson.D{{Key: "d", Value: 1}}}}})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}
	}
	cases := []struct {
		name  string
		setup func(context.Context, *mongo.Collection) error
		run   func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"near_2d_point_arrays_and_docs", setup2d, ids(bson.D{{Key: "pos", Value: bson.D{{Key: "$near", Value: bson.A{0.0, 0.0}}}}})},
		{"geonear_2d_point_arrays_and_docs", setup2d, geoNear(bson.D{{Key: "near", Value: bson.A{0.0, 0.0}}, {Key: "distanceField", Value: "d"}})},
		{"near_sphere_lines_and_polygons", setupSphere, ids(bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{{Key: "$geometry", Value: geoPoint(0, 0)}}}}}})},
		{"geonear_sphere_distances", setupSphere, geoNear(bson.D{{Key: "near", Value: geoPoint(0, 0)}, {Key: "distanceField", Value: "d"}, {Key: "spherical", Value: true}})},
		{"sphere_rejects_legacy_pair_array", setupSphere, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 9}, {Key: "loc", Value: bson.A{bson.A{1.0, 1.0}, bson.A{2.0, 2.0}}}})
			return nil, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Geo_NearShapes_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     tc.setup,
				Run:       tc.run,
			})
		})
	}
}
