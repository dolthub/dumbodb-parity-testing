// w3schools_test.go covers the w3schools MongoDB tutorial examples.
// Each test mirrors the data and operation shown on the corresponding tutorial page.
package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// w3sPostDate is a fixed date used in place of the tutorial's Date() call.
var w3sPostDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// w3sInsertPostsSeed inserts the 4 posts shown across the w3schools CRUD tutorials.
func w3sInsertPostsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "title", Value: "Post Title 1"},
			{Key: "body", Value: "Body of post."},
			{Key: "category", Value: "News"},
			{Key: "likes", Value: int32(1)},
			{Key: "tags", Value: bson.A{"news", "events"}},
			{Key: "date", Value: w3sPostDate},
		},
		bson.D{
			{Key: "title", Value: "Post Title 2"},
			{Key: "body", Value: "Body of post."},
			{Key: "category", Value: "Event"},
			{Key: "likes", Value: int32(2)},
			{Key: "tags", Value: bson.A{"news", "events"}},
			{Key: "date", Value: w3sPostDate},
		},
		bson.D{
			{Key: "title", Value: "Post Title 3"},
			{Key: "body", Value: "Body of post."},
			{Key: "category", Value: "Technology"},
			{Key: "likes", Value: int32(3)},
			{Key: "tags", Value: bson.A{"news", "events"}},
			{Key: "date", Value: w3sPostDate},
		},
		bson.D{
			{Key: "title", Value: "Post Title 4"},
			{Key: "body", Value: "Body of post."},
			{Key: "category", Value: "Event"},
			{Key: "likes", Value: int32(4)},
			{Key: "tags", Value: bson.A{"news", "events"}},
			{Key: "date", Value: w3sPostDate},
		},
	})
	return err
}

// ─── Insert ───────────────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_mongosh_insert.php

func TestW3S_Insert_InsertOne(t *testing.T) {
	// "To insert a single document, use the insertOne() method."
	// db.posts.insertOne({ title: "Post Title 1", body: "Body of post.",
	//   category: "News", likes: 1, tags: ["news","events"], date: Date() })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Insert_InsertOne",
		Support: harness.DocuDoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "title", Value: "Post Title 1"},
				{Key: "body", Value: "Body of post."},
				{Key: "category", Value: "News"},
				{Key: "likes", Value: int32(1)},
				{Key: "tags", Value: bson.A{"news", "events"}},
				{Key: "date", Value: w3sPostDate},
			})
			if err != nil {
				return nil, err
			}
			// Verify the document was inserted.
			count, err := col.CountDocuments(ctx, bson.D{{Key: "title", Value: "Post Title 1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "inserted", Value: count}}, nil
		},
	})
}

func TestW3S_Insert_InsertMany(t *testing.T) {
	// "To insert multiple documents, use the insertMany() method."
	// db.posts.insertMany([{...}, {...}, {...}])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Insert_InsertMany",
		Support: harness.DocuDoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "title", Value: "Post Title 2"},
					{Key: "body", Value: "Body of post."},
					{Key: "category", Value: "Event"},
					{Key: "likes", Value: int32(2)},
					{Key: "tags", Value: bson.A{"news", "events"}},
					{Key: "date", Value: w3sPostDate},
				},
				bson.D{
					{Key: "title", Value: "Post Title 3"},
					{Key: "body", Value: "Body of post."},
					{Key: "category", Value: "Technology"},
					{Key: "likes", Value: int32(3)},
					{Key: "tags", Value: bson.A{"news", "events"}},
					{Key: "date", Value: w3sPostDate},
				},
				bson.D{
					{Key: "title", Value: "Post Title 4"},
					{Key: "body", Value: "Body of post."},
					{Key: "category", Value: "Event"},
					{Key: "likes", Value: int32(4)},
					{Key: "tags", Value: bson.A{"news", "events"}},
					{Key: "date", Value: w3sPostDate},
				},
			})
			if err != nil {
				return nil, err
			}
			return int32(len(res.InsertedIDs)), nil
		},
	})
}

// ─── Find ─────────────────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_mongosh_find.php

func TestW3S_Find_FindAll(t *testing.T) {
	// "To select data from a collection, we can use the find() method."
	// db.posts.find()
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_FindAll",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{})
			return count, err
		},
	})
}

func TestW3S_Find_FindOne(t *testing.T) {
	// "To select only one document, we can use the findOne() method."
	// db.posts.findOne()
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_FindOne",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "title", Value: 1}})).Decode(&result)
			if err != nil {
				return nil, err
			}
			// Return just the title to avoid non-deterministic _id.
			for _, e := range result {
				if e.Key == "title" {
					return e.Value, nil
				}
			}
			return nil, nil
		},
	})
}

func TestW3S_Find_QueryByField(t *testing.T) {
	// "To query for documents with specific fields, add the field to the find() method."
	// db.posts.find( {category: "News"} )
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_QueryByField",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "category", Value: "News"}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

func TestW3S_Find_ProjectInclude(t *testing.T) {
	// "Both MongoDB and DocuDolt return only the projected fields (plus _id by default)."
	// db.posts.find({}, {title: 1, date: 1})
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_ProjectInclude",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx,
				bson.D{},
				options.Find().SetProjection(bson.D{
					{Key: "title", Value: 1},
					{Key: "date", Value: 1},
				}).SetSort(bson.D{{Key: "title", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Verify field count (title + date + _id = 3 keys per doc).
			if len(results) == 0 {
				return int32(0), nil
			}
			return int32(len(results[0])), nil
		},
	})
}

func TestW3S_Find_ProjectExcludeId(t *testing.T) {
	// "You can prevent _id from being added to the result by setting it to 0."
	// db.posts.find({}, {_id: 0, title: 1, date: 1})
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_ProjectExcludeId",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx,
				bson.D{},
				options.Find().SetProjection(bson.D{
					{Key: "_id", Value: 0},
					{Key: "title", Value: 1},
					{Key: "category", Value: 1},
				}).SetSort(bson.D{{Key: "title", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_Find_ProjectExcludeField(t *testing.T) {
	// "We can exclude a field by setting it to 0 in the projection."
	// db.posts.find({}, {category: 0})
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Find_ProjectExcludeField",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx,
				bson.D{},
				options.Find().SetProjection(bson.D{
					{Key: "category", Value: 0},
				}).SetSort(bson.D{{Key: "title", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Verify category is absent from results.
			for _, doc := range results {
				for _, e := range doc {
					if e.Key == "category" {
						return "category unexpectedly present", nil
					}
				}
			}
			return int32(len(results)), nil
		},
	})
}

// ─── Query Operators ──────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_query_operators.php
// The tutorial page is a reference index; the tests below demonstrate each operator.

func TestW3S_QueryOperators_Gt(t *testing.T) {
	// "$gt — greater than"
	// db.posts.find({ likes: { $gt: 2 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Gt",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$gt", Value: int32(2)}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_In(t *testing.T) {
	// "$in — matches any value in the specified array"
	// db.posts.find({ category: { $in: ["News", "Technology"] } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_In",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: bson.D{{Key: "$in", Value: bson.A{"News", "Technology"}}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_And(t *testing.T) {
	// "$and — joins clauses with a logical AND"
	// db.posts.find({ $and: [ {category: "Event"}, {likes: {$gte: 3}} ] })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_And",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "category", Value: "Event"}},
				bson.D{{Key: "likes", Value: bson.D{{Key: "$gte", Value: int32(3)}}}},
			}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Or(t *testing.T) {
	// "$or — joins clauses with a logical OR"
	// db.posts.find({ $or: [ {category: "News"}, {category: "Technology"} ] })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Or",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "category", Value: "News"}},
				bson.D{{Key: "category", Value: "Technology"}},
			}}})
			return count, err
		},
	})
}

// ─── Update ───────────────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_mongosh_update.php

func TestW3S_Update_UpdateOne(t *testing.T) {
	// "The updateOne() method updates the first document that matches the filter."
	// db.posts.updateOne( { title: "Post Title 1" }, { $set: { likes: 2 } } )
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Update_UpdateOne",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "likes", Value: int32(2)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matched", Value: res.MatchedCount},
				{Key: "modified", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestW3S_Update_Upsert(t *testing.T) {
	// "If no documents match the filter, insertOne is performed when upsert is true."
	// db.posts.updateOne( { title: "Post Title 5" }, { $set: {...} }, { upsert: true } )
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Update_Upsert",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 5"}},
				bson.D{{Key: "$set", Value: bson.D{
					{Key: "title", Value: "Post Title 5"},
					{Key: "body", Value: "Body of post."},
					{Key: "category", Value: "Event"},
					{Key: "likes", Value: int32(5)},
					{Key: "tags", Value: bson.A{"news", "events"}},
					{Key: "date", Value: w3sPostDate},
				}}},
				options.Update().SetUpsert(true),
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matched", Value: res.MatchedCount},
				{Key: "upserted", Value: res.UpsertedCount},
			}, nil
		},
	})
}

func TestW3S_Update_UpdateMany(t *testing.T) {
	// "The updateMany() method updates all documents that match the filter."
	// db.posts.updateMany({}, { $inc: { likes: 1 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Update_UpdateMany",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "likes", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return res.ModifiedCount, err
		},
	})
}

// ─── Update Operators ─────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_update_operators.php
// The tutorial page is a reference index; the tests below demonstrate each operator.

func TestW3S_UpdateOperators_Set(t *testing.T) {
	// "$set — sets the value of a field"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Set",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "category", Value: "Updated"}}}},
			)
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: "Updated"}})
			return count, err
		},
	})
}

func TestW3S_UpdateOperators_Inc(t *testing.T) {
	// "$inc — increments the value of a field"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Inc",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "likes", Value: int32(10)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "likes" {
					return e.Value, nil
				}
			}
			return nil, nil
		},
	})
}

func TestW3S_UpdateOperators_Unset(t *testing.T) {
	// "$unset — removes a field from a document"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Unset",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "category", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: bson.D{{Key: "$exists", Value: false}}}})
			return count, err
		},
	})
}

func TestW3S_UpdateOperators_Rename(t *testing.T) {
	// "$rename — renames a field"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Rename",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "category", Value: "type"}}}},
			)
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "type", Value: bson.D{{Key: "$exists", Value: true}}}})
			return count, err
		},
	})
}

func TestW3S_UpdateOperators_Push(t *testing.T) {
	// "$push — adds an item to an array field"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Push",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: "featured"}}}},
			)
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "tags", Value: "featured"}})
			return count, err
		},
	})
}

func TestW3S_UpdateOperators_AddToSet(t *testing.T) {
	// "$addToSet — adds an item to an array only if it does not already exist"
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_AddToSet",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Add "events" (already present) — should be a no-op.
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: "events"}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matched", Value: res.MatchedCount},
				{Key: "modified", Value: res.ModifiedCount},
			}, nil
		},
	})
}

// ─── Delete ───────────────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_mongosh_delete.php

func TestW3S_Delete_DeleteOne(t *testing.T) {
	// "The deleteOne() method deletes the first document that matches the filter."
	// db.posts.deleteOne({ title: "Post Title 5" })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Delete_DeleteOne",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}})
			if err != nil {
				return nil, err
			}
			remaining, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "deleted", Value: res.DeletedCount},
				{Key: "remaining", Value: remaining},
			}, nil
		},
	})
}

func TestW3S_Delete_DeleteMany(t *testing.T) {
	// "The deleteMany() method deletes all documents that match the filter."
	// db.posts.deleteMany({ category: "Technology" })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Delete_DeleteMany",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{{Key: "category", Value: "Technology"}})
			if err != nil {
				return nil, err
			}
			return res.DeletedCount, err
		},
	})
}

// ─── Aggregation $group ───────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_group.php

// w3sListingsSeed inserts listing documents resembling the sample_airbnb dataset.
func w3sListingsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "l1"}, {Key: "property_type", Value: "House"}, {Key: "name", Value: "Cozy House"}, {Key: "accommodates", Value: int32(4)}, {Key: "price", Value: int32(120)}},
		bson.D{{Key: "_id", Value: "l2"}, {Key: "property_type", Value: "Apartment"}, {Key: "name", Value: "Downtown Apt"}, {Key: "accommodates", Value: int32(2)}, {Key: "price", Value: int32(80)}},
		bson.D{{Key: "_id", Value: "l3"}, {Key: "property_type", Value: "House"}, {Key: "name", Value: "Big House"}, {Key: "accommodates", Value: int32(8)}, {Key: "price", Value: int32(200)}},
		bson.D{{Key: "_id", Value: "l4"}, {Key: "property_type", Value: "Apartment"}, {Key: "name", Value: "Studio Apt"}, {Key: "accommodates", Value: int32(1)}, {Key: "price", Value: int32(60)}},
		bson.D{{Key: "_id", Value: "l5"}, {Key: "property_type", Value: "Villa"}, {Key: "name", Value: "Sunny Villa"}, {Key: "accommodates", Value: int32(6)}, {Key: "price", Value: int32(350)}},
	})
	return err
}

func TestW3S_AggGroup_DistinctPropertyType(t *testing.T) {
	// "Use $group to group documents by a field — here to get distinct property types."
	// db.listingsAndReviews.aggregate([ { $group: { _id: "$property_type" } } ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_DistinctPropertyType",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$property_type"}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggGroup_CountPerGroup(t *testing.T) {
	// "Use $group with $sum to count documents per property type."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_CountPerGroup",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $limit ───────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_limit.php

func TestW3S_AggLimit_LimitOne(t *testing.T) {
	// "The $limit stage limits the number of documents passed to the next stage."
	// db.movies.aggregate([ { $limit: 1 } ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggLimit_LimitOne",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$limit", Value: int64(1)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

func TestW3S_AggLimit_LimitThree(t *testing.T) {
	// "Limit to 3 results from a larger collection."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggLimit_LimitThree",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$limit", Value: int64(3)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

// ─── Aggregation $project ─────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_project.php

// w3sRestaurantsSeed inserts restaurant documents resembling the sample_restaurants dataset.
func w3sRestaurantsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: "r1"},
			{Key: "name", Value: "The Golden Fork"},
			{Key: "cuisine", Value: "Italian"},
			{Key: "address", Value: bson.D{{Key: "street", Value: "123 Main St"}}},
			{Key: "grades", Value: bson.A{
				bson.D{{Key: "score", Value: int32(85)}},
				bson.D{{Key: "score", Value: int32(90)}},
			}},
		},
		bson.D{
			{Key: "_id", Value: "r2"},
			{Key: "name", Value: "Dragon Palace"},
			{Key: "cuisine", Value: "Chinese"},
			{Key: "address", Value: bson.D{{Key: "street", Value: "456 Oak Ave"}}},
			{Key: "grades", Value: bson.A{
				bson.D{{Key: "score", Value: int32(70)}},
				bson.D{{Key: "score", Value: int32(75)}},
				bson.D{{Key: "score", Value: int32(80)}},
			}},
		},
		bson.D{
			{Key: "_id", Value: "r3"},
			{Key: "name", Value: "Spice Garden"},
			{Key: "cuisine", Value: "Indian"},
			{Key: "address", Value: bson.D{{Key: "street", Value: "789 Elm Rd"}}},
			{Key: "grades", Value: bson.A{
				bson.D{{Key: "score", Value: int32(95)}},
			}},
		},
		bson.D{
			{Key: "_id", Value: "r4"},
			{Key: "name", Value: "Burger Barn"},
			{Key: "cuisine", Value: "Chinese"},
			{Key: "address", Value: bson.D{{Key: "street", Value: "321 Pine St"}}},
			{Key: "grades", Value: bson.A{
				bson.D{{Key: "score", Value: int32(60)}},
				bson.D{{Key: "score", Value: int32(65)}},
			}},
		},
		bson.D{
			{Key: "_id", Value: "r5"},
			{Key: "name", Value: "Sushi House"},
			{Key: "cuisine", Value: "Japanese"},
			{Key: "address", Value: bson.D{{Key: "street", Value: "654 Maple Dr"}}},
			{Key: "grades", Value: bson.A{
				bson.D{{Key: "score", Value: int32(88)}},
				bson.D{{Key: "score", Value: int32(92)}},
			}},
		},
	})
	return err
}

func TestW3S_AggProject_SelectFields(t *testing.T) {
	// "The $project stage passes only specified fields to the next pipeline stage."
	// db.restaurants.aggregate([
	//   { $project: { "name": 1, "cuisine": 1, "address": 1 } },
	//   { $limit: 5 }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggProject_SelectFields",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "name", Value: 1},
					{Key: "cuisine", Value: 1},
					{Key: "address", Value: 1},
				}}},
				bson.D{{Key: "$limit", Value: int64(5)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggProject_ExcludeId(t *testing.T) {
	// "Exclude _id from the projected output."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggProject_ExcludeId",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "cuisine", Value: 1},
				}}},
				bson.D{{Key: "$limit", Value: int64(3)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $sort ────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_sort.php

func TestW3S_AggSort_SortDescending(t *testing.T) {
	// "Use $sort: -1 for descending order."
	// db.listingsAndReviews.aggregate([
	//   { $sort: { "accommodates": -1 } },
	//   { $project: { "name": 1, "accommodates": 1 } },
	//   { $limit: 5 }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggSort_SortDescending",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "accommodates", Value: int32(-1)}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "accommodates", Value: 1},
				}}},
				bson.D{{Key: "$limit", Value: int64(5)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggSort_SortAscending(t *testing.T) {
	// "Use $sort: 1 for ascending order."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggSort_SortAscending",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "price", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "price", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $match ───────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_match.php

func TestW3S_AggMatch_FilterByPropertyType(t *testing.T) {
	// "The $match stage filters documents that match the given condition."
	// db.listingsAndReviews.aggregate([
	//   { $match: { property_type: "House" } },
	//   { $limit: 2 },
	//   { $project: { "name": 1, "bedrooms": 1, "price": 1 } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggMatch_FilterByPropertyType",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "property_type", Value: "House"}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$limit", Value: int64(2)}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "accommodates", Value: 1},
					{Key: "price", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggMatch_CountAfterFilter(t *testing.T) {
	// "Count the documents after applying $match."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggMatch_CountAfterFilter",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "property_type", Value: "Apartment"}}}},
				bson.D{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $addFields ───────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_addFields.php

func TestW3S_AggAddFields_ComputedAvg(t *testing.T) {
	// "Use $addFields with $avg to compute a new field from an array sub-field."
	// db.restaurants.aggregate([
	//   { $addFields: { avgGrade: { $avg: "$grades.score" } } },
	//   { $project: { "name": 1, "avgGrade": 1 } },
	//   { $limit: 5 }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggAddFields_ComputedAvg",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "avgGrade", Value: bson.D{{Key: "$avg", Value: "$grades.score"}}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "avgGrade", Value: 1},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
				bson.D{{Key: "$limit", Value: int64(5)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggAddFields_StaticField(t *testing.T) {
	// "Use $addFields to add a static computed field to every document."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggAddFields_StaticField",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "reviewed", Value: true}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "reviewed", Value: 1},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $count ───────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_count.php

func TestW3S_AggCount_CountFiltered(t *testing.T) {
	// "The $count stage counts the documents passing through the pipeline."
	// db.restaurants.aggregate([
	//   { $match: { "cuisine": "Chinese" } },
	//   { $count: "totalChinese" }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggCount_CountFiltered",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "cuisine", Value: "Chinese"}}}},
				bson.D{{Key: "$count", Value: "totalChinese"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggCount_CountAll(t *testing.T) {
	// "Count all documents in the collection."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggCount_CountAll",
		Support: harness.DocuDoltFull,
		Setup:   w3sRestaurantsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $lookup ──────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_lookup.php

func TestW3S_AggLookup_JoinCollections(t *testing.T) {
	// "Use $lookup to join documents from another collection."
	// db.comments.aggregate([
	//   { $lookup: { from: "movies", localField: "movie_id",
	//                foreignField: "_id", as: "movie_details" } },
	//   { $limit: 1 }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggLookup_JoinCollections",
		Support: harness.DocuDoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// col = comments collection
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "c1"}, {Key: "movie_id", Value: "m1"}, {Key: "text", Value: "Great film!"}},
				bson.D{{Key: "_id", Value: "c2"}, {Key: "movie_id", Value: "m2"}, {Key: "text", Value: "Loved it."}},
			}); err != nil {
				return err
			}
			movies := col.Database().Collection("movies")
			_, err := movies.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "title", Value: "The Grand Tour"}, {Key: "year", Value: int32(2023)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "title", Value: "Ocean Blue"}, {Key: "year", Value: int32(2022)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "movies"},
					{Key: "localField", Value: "movie_id"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "movie_details"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$limit", Value: int64(1)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggLookup_NoMatchEmptyArray(t *testing.T) {
	// "When $lookup finds no matching foreign documents, it produces an empty array."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggLookup_NoMatchEmptyArray",
		Support: harness.DocuDoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "c1"}, {Key: "movie_id", Value: "no-such-movie"}, {Key: "text", Value: "Orphan comment"}},
			}); err != nil {
				return err
			}
			movies := col.Database().Collection("movies_lookup_empty")
			_, err := movies.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "title", Value: "Some Movie"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "movies_lookup_empty"},
					{Key: "localField", Value: "movie_id"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "movie_details"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $group (additional) ─────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_group.php

func TestW3S_AggGroup_SumField(t *testing.T) {
	// "Use $group with $sum to calculate the total of a numeric field per group."
	// db.listingsAndReviews.aggregate([
	//   { $group: { _id: "$property_type", totalPrice: { $sum: "$price" } } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_SumField",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "totalPrice", Value: bson.D{{Key: "$sum", Value: "$price"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggGroup_Avg(t *testing.T) {
	// "Use $group with $avg to compute the average accommodates per property type."
	// db.listingsAndReviews.aggregate([
	//   { $group: { _id: "$property_type", avgAccommodates: { $avg: "$accommodates" } } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_Avg",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "avgAccommodates", Value: bson.D{{Key: "$avg", Value: "$accommodates"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggGroup_MinMax(t *testing.T) {
	// "Use $group with $min and $max to find the cheapest and most expensive listing per type."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_MinMax",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "minPrice", Value: bson.D{{Key: "$min", Value: "$price"}}},
					{Key: "maxPrice", Value: bson.D{{Key: "$max", Value: "$price"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggGroup_Push(t *testing.T) {
	// "Use $group with $push to collect all listing names per property type into an array."
	// db.listingsAndReviews.aggregate([
	//   { $group: { _id: "$property_type", names: { $push: "$name" } } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggGroup_Push",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "names", Value: bson.D{{Key: "$push", Value: "$name"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Return the count of groups rather than the full nested arrays.
			return int32(len(results)), nil
		},
	})
}

// ─── Aggregation $out ─────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_out.php

func TestW3S_AggOut_GroupAndWrite(t *testing.T) {
	// "The $out stage writes the result of the pipeline to a specified collection."
	// db.listingsAndReviews.aggregate([
	//   { $group: { _id: "$property_type",
	//               properties: { $push: { name: "$name", accommodates: "$accommodates", price: "$price" } } } },
	//   { $out: "properties_by_type" }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggOut_GroupAndWrite",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "properties", Value: bson.D{{Key: "$push", Value: bson.D{
						{Key: "name", Value: "$name"},
						{Key: "accommodates", Value: "$accommodates"},
						{Key: "price", Value: "$price"},
					}}}},
				}}},
				bson.D{{Key: "$out", Value: "properties_by_type"}},
			})
			if err != nil {
				return nil, err
			}
			// $out returns no documents in cursor.
			var cursorDocs []bson.D
			if err := cursor.All(ctx, &cursorDocs); err != nil {
				return nil, err
			}
			// Verify output collection has the expected number of groups.
			out := col.Database().Collection("properties_by_type")
			count, err := out.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

// ─── Query Operators (additional) ────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_query_operators.php

func TestW3S_QueryOperators_Lt(t *testing.T) {
	// "$lt — less than"
	// db.posts.find({ likes: { $lt: 3 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Lt",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$lt", Value: int32(3)}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Lte(t *testing.T) {
	// "$lte — less than or equal"
	// db.posts.find({ likes: { $lte: 2 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Lte",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$lte", Value: int32(2)}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Gte(t *testing.T) {
	// "$gte — greater than or equal"
	// db.posts.find({ likes: { $gte: 3 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Gte",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$gte", Value: int32(3)}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Ne(t *testing.T) {
	// "$ne — not equal"
	// db.posts.find({ category: { $ne: "News" } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Ne",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: bson.D{{Key: "$ne", Value: "News"}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Nin(t *testing.T) {
	// "$nin — not in: matches values not in the given array"
	// db.posts.find({ category: { $nin: ["News", "Technology"] } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Nin",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: bson.D{{Key: "$nin", Value: bson.A{"News", "Technology"}}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Nor(t *testing.T) {
	// "$nor — joins clauses with a logical NOR (neither condition true)"
	// db.posts.find({ $nor: [ {category: "News"}, {category: "Technology"} ] })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Nor",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "$nor", Value: bson.A{
				bson.D{{Key: "category", Value: "News"}},
				bson.D{{Key: "category", Value: "Technology"}},
			}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Not(t *testing.T) {
	// "$not — inverts the effect of a query expression"
	// db.posts.find({ likes: { $not: { $gt: 2 } } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Not",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Exists(t *testing.T) {
	// "$exists — matches documents that have the specified field"
	// db.posts.find({ category: { $exists: true } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Exists",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: bson.D{{Key: "$exists", Value: true}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Type(t *testing.T) {
	// "$type — selects documents where the field is of the specified BSON type"
	// db.posts.find({ likes: { $type: "int" } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Type",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "likes", Value: bson.D{{Key: "$type", Value: "int"}}}})
			return count, err
		},
	})
}

func TestW3S_QueryOperators_Regex(t *testing.T) {
	// "$regex — selects documents where values match a specified regular expression"
	// db.posts.find({ title: { $regex: /Post/ } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_QueryOperators_Regex",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "Post"}}}})
			return count, err
		},
	})
}

// ─── Update Operators (additional) ───────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_update_operators.php

func TestW3S_UpdateOperators_Mul(t *testing.T) {
	// "$mul — multiplies the value of a field by a number"
	// db.posts.updateOne({ title: "Post Title 1" }, { $mul: { likes: 3 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Mul",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "likes", Value: int32(3)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "likes" {
					return e.Value, nil
				}
			}
			return nil, nil
		},
	})
}

func TestW3S_UpdateOperators_Min(t *testing.T) {
	// "$min — only updates the field if the specified value is less than the existing value"
	// db.posts.updateOne({ title: "Post Title 4" }, { $min: { likes: 2 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Min",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Post Title 4 has likes=4; $min with 2 should update to 2.
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 4"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "likes", Value: int32(2)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 4"}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "likes" {
					return e.Value, nil
				}
			}
			return nil, nil
		},
	})
}

func TestW3S_UpdateOperators_Max(t *testing.T) {
	// "$max — only updates the field if the specified value is greater than the existing value"
	// db.posts.updateOne({ title: "Post Title 1" }, { $max: { likes: 10 } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Max",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Post Title 1 has likes=1; $max with 10 should update to 10.
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "likes", Value: int32(10)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "likes" {
					return e.Value, nil
				}
			}
			return nil, nil
		},
	})
}

func TestW3S_UpdateOperators_Pop(t *testing.T) {
	// "$pop — removes the first or last element of an array"
	// db.posts.updateOne({ title: "Post Title 1" }, { $pop: { tags: 1 } })  // 1=last, -1=first
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Pop",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "tags" {
					arr, _ := e.Value.(bson.A)
					return int32(len(arr)), nil
				}
			}
			return int32(0), nil
		},
	})
}

func TestW3S_UpdateOperators_Pull(t *testing.T) {
	// "$pull — removes all array elements that match a specified condition"
	// db.posts.updateOne({ title: "Post Title 1" }, { $pull: { tags: "news" } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_UpdateOperators_Pull",
		Support: harness.DocuDoltFull,
		Setup:   w3sInsertPostsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "title", Value: "Post Title 1"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: "news"}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "title", Value: "Post Title 1"}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "tags" {
					arr, _ := e.Value.(bson.A)
					return int32(len(arr)), nil
				}
			}
			return int32(0), nil
		},
	})
}

// ─── Aggregation $project (additional) ───────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_project.php

func TestW3S_AggProject_ComputedField(t *testing.T) {
	// "Use $project with an expression to compute a new field."
	// db.listingsAndReviews.aggregate([
	//   { $project: { name: 1, doublePrce: { $multiply: ["$price", 2] } } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggProject_ComputedField",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "doublePrice", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", int32(2)}}}},
				}}},
				bson.D{{Key: "$limit", Value: int64(3)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $sort (additional) ──────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_sort.php

func TestW3S_AggSort_MultiField(t *testing.T) {
	// "Sort by multiple fields: property_type ascending, then price descending."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggSort_MultiField",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{
					{Key: "property_type", Value: int32(1)},
					{Key: "price", Value: int32(-1)},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "property_type", Value: 1},
					{Key: "price", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $match (additional) ─────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_match.php

func TestW3S_AggMatch_ComparisonFilter(t *testing.T) {
	// "Use $match with a comparison operator to filter documents."
	// db.listingsAndReviews.aggregate([
	//   { $match: { price: { $gte: 100 } } },
	//   { $project: { name: 1, price: 1, _id: 0 } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggMatch_ComparisonFilter",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "price", Value: bson.D{{Key: "$gte", Value: int32(100)}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "price", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestW3S_AggMatch_MatchThenGroup(t *testing.T) {
	// "Combine $match and $group to count documents after filtering."
	// db.listingsAndReviews.aggregate([
	//   { $match: { accommodates: { $gte: 4 } } },
	//   { $group: { _id: "$property_type", count: { $sum: 1 } } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggMatch_MatchThenGroup",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "accommodates", Value: bson.D{{Key: "$gte", Value: int32(4)}}}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$property_type"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $addFields (additional) ─────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_addFields.php

func TestW3S_AggAddFields_ArithmeticExpr(t *testing.T) {
	// "Use $addFields with $add to compute a derived field."
	// db.listingsAndReviews.aggregate([
	//   { $addFields: { priceWithFee: { $add: ["$price", 10] } } },
	//   { $project: { name: 1, price: 1, priceWithFee: 1, _id: 0 } }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggAddFields_ArithmeticExpr",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "priceWithFee", Value: bson.D{{Key: "$add", Value: bson.A{"$price", int32(10)}}}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "price", Value: 1},
					{Key: "priceWithFee", Value: 1},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $count (additional) ─────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_count.php

func TestW3S_AggCount_AfterMultipleStages(t *testing.T) {
	// "Count documents after applying both $match and $sort stages."
	// db.listingsAndReviews.aggregate([
	//   { $match: { accommodates: { $gt: 1 } } },
	//   { $sort: { price: 1 } },
	//   { $count: "result" }
	// ])
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggCount_AfterMultipleStages",
		Support: harness.DocuDoltFull,
		Setup:   w3sListingsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "accommodates", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "price", Value: 1}}}},
				bson.D{{Key: "$count", Value: "result"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Aggregation $lookup (additional) ────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_aggregations_lookup.php

func TestW3S_AggLookup_AllRows(t *testing.T) {
	// "Use $lookup to join all comments with movie details."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_AggLookup_AllRows",
		Support: harness.DocuDoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "c1"}, {Key: "movie_id", Value: "m1"}, {Key: "text", Value: "Excellent!"}},
				bson.D{{Key: "_id", Value: "c2"}, {Key: "movie_id", Value: "m2"}, {Key: "text", Value: "Brilliant."}},
			}); err != nil {
				return err
			}
			movies := col.Database().Collection("movies_all")
			_, err := movies.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "title", Value: "Space Odyssey"}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "title", Value: "Cosmos"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "movies_all"},
					{Key: "localField", Value: "movie_id"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "movie_details"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

// ─── Indexing / Search ────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_indexing_search.php

// w3sMoviesSeed inserts movie documents for indexing and search tests.
func w3sMoviesSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "mv1"}, {Key: "title", Value: "Star Wars"}, {Key: "year", Value: int32(1977)}},
		bson.D{{Key: "_id", Value: "mv2"}, {Key: "title", Value: "Star Trek"}, {Key: "year", Value: int32(1979)}},
		bson.D{{Key: "_id", Value: "mv3"}, {Key: "title", Value: "The Godfather"}, {Key: "year", Value: int32(1972)}},
		bson.D{{Key: "_id", Value: "mv4"}, {Key: "title", Value: "Interstellar"}, {Key: "year", Value: int32(2014)}},
		bson.D{{Key: "_id", Value: "mv5"}, {Key: "title", Value: "Star Wars: Empire"}, {Key: "year", Value: int32(1980)}},
	})
	return err
}

func TestW3S_Indexing_CreateSingleFieldIndex(t *testing.T) {
	// "createIndex() creates an index on the specified field."
	// db.movies.createIndex({ title: 1 })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_CreateSingleFieldIndex",
		Support: harness.DocuDoltFull,
		Setup:   w3sMoviesSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "title", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestW3S_Indexing_CreateCompoundIndex(t *testing.T) {
	// "A compound index spans multiple fields."
	// db.movies.createIndex({ title: 1, year: -1 })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_CreateCompoundIndex",
		Support: harness.DocuDoltFull,
		Setup:   w3sMoviesSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{
					{Key: "title", Value: int32(1)},
					{Key: "year", Value: int32(-1)},
				},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestW3S_Indexing_CreateTextIndex(t *testing.T) {
	// "A text index enables text search on string fields."
	// db.movies.createIndex({ title: "text" })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_CreateTextIndex",
		Support: harness.DocuDoltFull,
		Setup:   w3sMoviesSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "title", Value: "text"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestW3S_Indexing_TextSearch(t *testing.T) {
	// "Use $text with a text index to perform a full-text search."
	// db.movies.find({ $text: { $search: "star" } })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_TextSearch",
		Support: harness.DocuDoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := w3sMoviesSeed(ctx, col); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "title", Value: "text"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx,
				bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "star"}}}},
				options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

func TestW3S_Indexing_DropIndex(t *testing.T) {
	// "dropIndex() removes an index by name."
	// db.movies.dropIndex("title_1")
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_DropIndex",
		Support: harness.DocuDoltFull,
		Setup:   w3sMoviesSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "title", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropOne(ctx, name); err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestW3S_Indexing_AtlasSearch(t *testing.T) {
	// "The w3schools indexing page shows Atlas Search using the $search aggregation stage."
	// db.movies.aggregate([
	//   { $search: { index: "default", text: { query: "star wars", path: "title" } } },
	//   { $project: { title: 1, year: 1 } }
	// ])
	// Atlas Search requires a running Atlas cluster — not available in standard DocuDolt.
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Indexing_AtlasSearch",
		Support: harness.DocuDoltFull,
		Setup:   w3sMoviesSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$search", Value: bson.D{
					{Key: "index", Value: "default"},
					{Key: "text", Value: bson.D{
						{Key: "query", Value: "star wars"},
						{Key: "path", Value: "title"},
					}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "title", Value: 1},
					{Key: "year", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Validation ───────────────────────────────────────────────────────────────
// https://www.w3schools.com/mongodb/mongodb_schema_validation.php

func TestW3S_Validation_CreateCollectionWithValidator(t *testing.T) {
	// "Use db.createCollection() with a validator to enforce a JSON Schema."
	// db.createCollection("posts", {
	//   validator: {
	//     $jsonSchema: {
	//       bsonType: "object",
	//       required: ["title", "body"],
	//       properties: {
	//         title:    { bsonType: "string" },
	//         body:     { bsonType: "string" },
	//         category: { bsonType: "string" },
	//         likes:    { bsonType: "int" },
	//         tags:     { bsonType: "array", items: { bsonType: "string" } },
	//         date:     { bsonType: "date" }
	//       }
	//     }
	//   }
	// })
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Validation_CreateCollectionWithValidator",
		Support: harness.DocuDoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			validator := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"title", "body"}},
				{Key: "properties", Value: bson.D{
					{Key: "title", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "body", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "category", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "likes", Value: bson.D{{Key: "bsonType", Value: "int"}}},
					{Key: "tags", Value: bson.D{
						{Key: "bsonType", Value: "array"},
						{Key: "items", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					}},
					{Key: "date", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			}}}
			opts := options.CreateCollection().SetValidator(validator)
			colName := col.Name() + "_validated"
			if err := col.Database().CreateCollection(ctx, colName, opts); err != nil {
				return nil, err
			}
			validCol := col.Database().Collection(colName)
			// Insert a valid document (has required title and body).
			_, err := validCol.InsertOne(ctx, bson.D{
				{Key: "title", Value: "Post Title 1"},
				{Key: "body", Value: "Body of post."},
				{Key: "category", Value: "News"},
				{Key: "likes", Value: int32(1)},
				{Key: "tags", Value: bson.A{"news", "events"}},
				{Key: "date", Value: w3sPostDate},
			})
			if err != nil {
				return nil, err
			}
			count, err := validCol.CountDocuments(ctx, bson.D{})
			return count, err
		},
	})
}

func TestW3S_Validation_ValidDocumentPasses(t *testing.T) {
	// "A document satisfying the schema is accepted."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Validation_ValidDocumentPasses",
		Support: harness.DocuDoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			validator := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"title", "body"}},
				{Key: "properties", Value: bson.D{
					{Key: "title", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "body", Value: bson.D{{Key: "bsonType", Value: "string"}}},
				}},
			}}}
			opts := options.CreateCollection().SetValidator(validator)
			colName := col.Name() + "_valid_pass"
			if err := col.Database().CreateCollection(ctx, colName, opts); err != nil {
				return nil, err
			}
			validCol := col.Database().Collection(colName)
			_, err := validCol.InsertOne(ctx, bson.D{
				{Key: "title", Value: "Valid Post"},
				{Key: "body", Value: "This is valid."},
			})
			if err != nil {
				return "insert_failed", nil
			}
			return "insert_ok", nil
		},
	})
}

func TestW3S_Validation_InvalidDocumentRejected(t *testing.T) {
	// "A document missing required fields is rejected by the validator."
	harness.PairTest(t, harness.TestCase{
		Name:    "W3S_Validation_InvalidDocumentRejected",
		Support: harness.DocuDoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			validator := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"title", "body"}},
				{Key: "properties", Value: bson.D{
					{Key: "title", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "body", Value: bson.D{{Key: "bsonType", Value: "string"}}},
				}},
			}}}
			opts := options.CreateCollection().SetValidator(validator)
			colName := col.Name() + "_valid_reject"
			if err := col.Database().CreateCollection(ctx, colName, opts); err != nil {
				return nil, err
			}
			validCol := col.Database().Collection(colName)
			// Missing required "body" field — should be rejected.
			_, err := validCol.InsertOne(ctx, bson.D{
				{Key: "title", Value: "Missing Body"},
			})
			if err != nil {
				return "insert_rejected", nil
			}
			return "insert_accepted", nil
		},
	})
}
