// mongodb_data_modelling_test.go covers MongoDB Data Modelling Patterns tutorials.
// Source: https://www.mongodb.com/docs/manual/tutorial/
// Each test mirrors the data and operations shown on the corresponding tutorial page.
package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// ─── One-to-Many with Embedded Documents ──────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/model-embedded-one-to-many-relationships-between-documents/
//
// Pattern: embed the "many" side inside the "one" document when always accessed together.

func TestDataModelling_EmbeddedOneToMany_PatronAddresses(t *testing.T) {
	// "Model a patron with multiple addresses as embedded documents."
	// db.patrons.insertOne({
	//   _id: "joe", name: "Joe Bookreader",
	//   addresses: [
	//     { street: "123 Fake Street", city: "Faketon", state: "MA", zip: "12345" },
	//     { street: "1 Some Other Street", city: "Boston", state: "MA", zip: "12345" }
	//   ]
	// })
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_EmbeddedOneToMany_PatronAddresses",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			patron := bson.D{
				{Key: "_id", Value: "joe"},
				{Key: "name", Value: "Joe Bookreader"},
				{Key: "addresses", Value: bson.A{
					bson.D{
						{Key: "street", Value: "123 Fake Street"},
						{Key: "city", Value: "Faketon"},
						{Key: "state", Value: "MA"},
						{Key: "zip", Value: "12345"},
					},
					bson.D{
						{Key: "street", Value: "1 Some Other Street"},
						{Key: "city", Value: "Boston"},
						{Key: "state", Value: "MA"},
						{Key: "zip", Value: "12345"},
					},
				}},
			}
			_, err := col.InsertOne(ctx, patron)
			if err != nil {
				return nil, err
			}

			// Retrieve and verify the embedded addresses.
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "joe"}}).Decode(&result)
			if err != nil {
				return nil, err
			}

			// Check that addresses array has 2 entries.
			var addrCount int32
			for _, e := range result {
				if e.Key == "addresses" {
					if arr, ok := e.Value.(bson.A); ok {
						addrCount = int32(len(arr))
					}
				}
			}
			expected := bson.D{
				{Key: "name", Value: "Joe Bookreader"},
				{Key: "address_count", Value: int32(2)},
			}
			actual := bson.D{
				{Key: "name", Value: "Joe Bookreader"},
				{Key: "address_count", Value: addrCount},
			}
			tutorialCheck(t, "EmbeddedOneToMany_PatronAddresses", actual, expected)
			return actual, nil
		},
	})
}

func TestDataModelling_EmbeddedOneToMany_QueryEmbedded(t *testing.T) {
	// "Query by an embedded sub-document field."
	// db.patrons.find({ "addresses.city": "Boston" })
	// Expected: the joe patron document.
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_EmbeddedOneToMany_QueryEmbedded",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "joe"},
				{Key: "name", Value: "Joe Bookreader"},
				{Key: "addresses", Value: bson.A{
					bson.D{
						{Key: "street", Value: "123 Fake Street"},
						{Key: "city", Value: "Faketon"},
						{Key: "state", Value: "MA"},
						{Key: "zip", Value: "12345"},
					},
					bson.D{
						{Key: "street", Value: "1 Some Other Street"},
						{Key: "city", Value: "Boston"},
						{Key: "state", Value: "MA"},
						{Key: "zip", Value: "12345"},
					},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "addresses.city", Value: "Boston"}})
			if err != nil {
				return nil, err
			}
			tutorialCheck(t, "EmbeddedOneToMany_QueryEmbedded", count, int64(1))
			return count, nil
		},
	})
}

// ─── One-to-Many with Document References ─────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/model-referenced-one-to-many-relationships-between-documents/
//
// Pattern: store a reference (foreign key) in child documents pointing to parent.

var dataModelPublishDate1, _ = time.Parse(time.RFC3339, "2010-09-24T00:00:00Z")
var dataModelPublishDate2, _ = time.Parse(time.RFC3339, "2011-05-06T00:00:00Z")

func TestDataModelling_ReferencedOneToMany_FindBooksByPublisher(t *testing.T) {
	// Publisher document: { _id: "oreilly", name: "O'Reilly Media", founded: 1980 }
	// Book documents reference publisher via publisher_id field.
	// db.books.find({ publisher_id: "oreilly" }) → 2 books
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_ReferencedOneToMany_FindBooksByPublisher",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			db := col.Database()
			publishers := db.Collection(col.Name() + "_publishers")
			_, err := publishers.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "oreilly"},
				{Key: "name", Value: "O'Reilly Media"},
				{Key: "founded", Value: int32(1980)},
				{Key: "location", Value: "CA"},
			})
			if err != nil {
				return err
			}
			_, err = col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: int32(123456789)},
					{Key: "title", Value: "MongoDB: The Definitive Guide"},
					{Key: "author", Value: bson.A{"Kristina Chodorow", "Mike Dirolf"}},
					{Key: "published_date", Value: dataModelPublishDate1},
					{Key: "pages", Value: int32(216)},
					{Key: "language", Value: "English"},
					{Key: "publisher_id", Value: "oreilly"},
				},
				bson.D{
					{Key: "_id", Value: int32(234567890)},
					{Key: "title", Value: "50 Tips and Tricks for MongoDB Developer"},
					{Key: "author", Value: "Kristina Chodorow"},
					{Key: "published_date", Value: dataModelPublishDate2},
					{Key: "pages", Value: int32(68)},
					{Key: "language", Value: "English"},
					{Key: "publisher_id", Value: "oreilly"},
				},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "publisher_id", Value: "oreilly"}})
			if err != nil {
				return nil, err
			}
			// Both books reference oreilly as their publisher.
			tutorialCheck(t, "ReferencedOneToMany_BookCount", count, int64(2))
			return count, nil
		},
	})
}

func TestDataModelling_ReferencedOneToMany_LookupPublisher(t *testing.T) {
	// "$lookup to join books with publisher in a single aggregation."
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_ReferencedOneToMany_LookupPublisher",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			db := col.Database()
			publishers := db.Collection(col.Name() + "_pub2")
			_, err := publishers.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "oreilly"},
				{Key: "name", Value: "O'Reilly Media"},
			})
			if err != nil {
				return err
			}
			_, err = col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "title", Value: "MongoDB: The Definitive Guide"}, {Key: "publisher_id", Value: "oreilly"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "title", Value: "50 Tips and Tricks"}, {Key: "publisher_id", Value: "oreilly"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			pipeline := bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: col.Name() + "_pub2"},
					{Key: "localField", Value: "publisher_id"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "publisher"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
			cursor, err := db.Collection(col.Name()).Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			count := int32(len(results))
			tutorialCheck(t, "ReferencedOneToMany_LookupCount", count, int32(2))
			return count, nil
		},
	})
}

// ─── Model Tree Structures with Child References ───────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/model-tree-structures-with-child-references/

func devPatternsChildRefsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "MongoDB"}, {Key: "name", Value: "MongoDB"}, {Key: "children", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "dbm"}, {Key: "name", Value: "dbm"}, {Key: "children", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "Databases"}, {Key: "name", Value: "Databases"}, {Key: "children", Value: bson.A{"MongoDB", "dbm"}}},
		bson.D{{Key: "_id", Value: "Languages"}, {Key: "name", Value: "Languages"}, {Key: "children", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "Programming"}, {Key: "name", Value: "Programming"}, {Key: "children", Value: bson.A{"Databases", "Languages"}}},
		bson.D{{Key: "_id", Value: "Books"}, {Key: "name", Value: "Books"}, {Key: "children", Value: bson.A{"Programming"}}},
	})
	return err
}

func TestDataModelling_ChildRefs_FindImmediateChildren(t *testing.T) {
	// "Find the immediate children of 'Databases' by reading the children array."
	// db.categories.findOne({ _id: "Databases" })
	// Expected children: ["MongoDB", "dbm"]
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_ChildRefs_FindImmediateChildren",
		Support: harness.DongoFull,
		Setup:   devPatternsChildRefsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "Databases"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			var children bson.A
			for _, e := range result {
				if e.Key == "children" {
					if a, ok := e.Value.(bson.A); ok {
						children = a
					}
				}
			}
			count := int32(len(children))
			tutorialCheck(t, "ChildRefs_FindImmediateChildren_count", count, int32(2))
			return bson.D{
				{Key: "children_count", Value: count},
			}, nil
		},
	})
}

func TestDataModelling_ChildRefs_FindParentByChildrenArray(t *testing.T) {
	// "Find the parent of a node by searching for it in children arrays."
	// db.categories.find({ children: "MongoDB" })
	// Expected: Databases document (contains "MongoDB" in children).
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_ChildRefs_FindParentByChildrenArray",
		Support: harness.DongoFull,
		Setup:   devPatternsChildRefsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "children", Value: "MongoDB"}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			count := int32(len(results))
			tutorialCheck(t, "ChildRefs_FindParentByChildrenArray_count", count, int32(1))
			if count == 1 {
				var id interface{}
				for _, e := range results[0] {
					if e.Key == "_id" {
						id = e.Value
					}
				}
				tutorialCheck(t, "ChildRefs_FindParentByChildrenArray_parent_id", id, "Databases")
			}
			return count, nil
		},
	})
}

// ─── Model Tree Structures with Materialized Paths ────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/model-tree-structures-with-materialized-paths/
//
// Pattern: store full ancestor path as a comma-delimited string in each document.

func devPatternsMaterializedPathsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "Books"}, {Key: "path", Value: nil}},
		bson.D{{Key: "_id", Value: "Programming"}, {Key: "path", Value: ",Books,"}},
		bson.D{{Key: "_id", Value: "Databases"}, {Key: "path", Value: ",Books,Programming,"}},
		bson.D{{Key: "_id", Value: "Languages"}, {Key: "path", Value: ",Books,Programming,"}},
		bson.D{{Key: "_id", Value: "MongoDB"}, {Key: "path", Value: ",Books,Programming,Databases,"}},
		bson.D{{Key: "_id", Value: "dbm"}, {Key: "path", Value: ",Books,Programming,Databases,"}},
	})
	return err
}

func TestDataModelling_MaterializedPaths_FindAllDescendants(t *testing.T) {
	// "Find all descendants of 'Books' using a regex match on path."
	// db.categories.find({ path: /,Books,/ })
	// Expected: 5 documents (all nodes except Books itself).
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_MaterializedPaths_FindAllDescendants",
		Support: harness.DongoFull,
		Setup:   devPatternsMaterializedPathsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "path", Value: bson.D{{Key: "$regex", Value: ",Books,"}}}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			count := int32(len(results))
			tutorialCheck(t, "MaterializedPaths_FindAllDescendants_count", count, int32(5))
			return count, nil
		},
	})
}

func TestDataModelling_MaterializedPaths_FindSubtreeDescendants(t *testing.T) {
	// "Find all descendants of 'Programming'."
	// db.categories.find({ path: /,Books,Programming,/ })
	// Expected: Databases, Languages, MongoDB, dbm (4 documents).
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_MaterializedPaths_FindSubtreeDescendants",
		Support: harness.DongoFull,
		Setup:   devPatternsMaterializedPathsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(
				ctx,
				bson.D{{Key: "path", Value: bson.D{{Key: "$regex", Value: ",Books,Programming,"}}}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			count := int32(len(results))
			tutorialCheck(t, "MaterializedPaths_FindSubtreeDescendants_count", count, int32(4))
			return count, nil
		},
	})
}

func TestDataModelling_MaterializedPaths_SortByPath(t *testing.T) {
	// "Retrieve tree sorted by path gives breadth-first ordering."
	// db.categories.find().sort({ path: 1 })
	// Expected: Books first (null path), then nodes in path-alphabetical order.
	harness.PairTest(t, harness.TestCase{
		Name:    "DataModelling_MaterializedPaths_SortByPath",
		Support: harness.DongoFull,
		Setup:   devPatternsMaterializedPathsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "path", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			count := int32(len(results))
			// All 6 documents in the collection.
			tutorialCheck(t, "MaterializedPaths_SortByPath_count", count, int32(6))
			// First document (null path) should be Books.
			if len(results) > 0 {
				var firstID interface{}
				for _, e := range results[0] {
					if e.Key == "_id" {
						firstID = e.Value
					}
				}
				tutorialCheck(t, "MaterializedPaths_SortByPath_firstNode", firstID, "Books")
			}
			return count, nil
		},
	})
}
