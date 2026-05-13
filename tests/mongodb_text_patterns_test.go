// mongodb_text_patterns_test.go covers MongoDB $text search tutorials.
// Source: https://www.mongodb.com/docs/manual/tutorial/text-search-in-aggregation/
//         https://www.mongodb.com/docs/manual/reference/operator/query/text/
// Each test mirrors the data and operations shown on the corresponding tutorial page.
package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// https://www.mongodb.com/docs/manual/reference/operator/query/text/
//
// Collection: stores
// Text index on { name: "text", description: "text" }

func textPatternsStoresSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "Java Hut"}, {Key: "description", Value: "Coffee and cakes"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "name", Value: "Burger Buns"}, {Key: "description", Value: "Gourmet burgers"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "name", Value: "Coffee Shop"}, {Key: "description", Value: "Just coffee"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "name", Value: "Clothes Clothes Clothes"}, {Key: "description", Value: "Discount clothing"}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "name", Value: "Java Shopping Center"}, {Key: "description", Value: "A variety of goods"}},
	})
	return err
}

func textPatternsCreateStoresIndex(ctx context.Context, col *mongo.Collection) error {
	if err := textPatternsStoresSeed(ctx, col); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: "text"},
			{Key: "description", Value: "text"},
		},
	})
	return err
}

func TestTextPatterns_BasicSearch_SingleWord(t *testing.T) {
	// "Performs a text search on documents with a text index."
	// db.stores.find({ $text: { $search: "coffee" } })
	// Expected: documents 1 (Java Hut) and 3 (Coffee Shop) — both contain "coffee".
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_BasicSearch_SingleWord",
		Support: harness.DumboDBFull,
		Setup:   textPatternsCreateStoresIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(
				ctx,
				bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "coffee"}}}},
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
			// Documents 1 and 3 match "coffee".
			tutorialCheck(t, "BasicSearch_SingleWord_count", count, int32(2))
			return count, nil
		},
	})
}

func TestTextPatterns_BasicSearch_ExcludeTerm(t *testing.T) {
	// "To exclude a word, prepend it with a minus sign."
	// db.stores.find({ $text: { $search: "java -coffee" } })
	// Expected: document 5 only (Java Shopping Center) — "java" matches but "coffee" excludes doc 1.
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_BasicSearch_ExcludeTerm",
		Support: harness.DumboDBFull,
		Setup:   textPatternsCreateStoresIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(
				ctx,
				bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "java -coffee"}}}},
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
			// Only document 5 (Java Shopping Center) — "coffee" exclusion removes doc 1.
			tutorialCheck(t, "BasicSearch_ExcludeTerm_count", count, int32(1))
			if count == 1 {
				// Verify it is document 5.
				var id interface{}
				for _, e := range results[0] {
					if e.Key == "_id" {
						id = e.Value
					}
				}
				tutorialCheck(t, "BasicSearch_ExcludeTerm_id", id, int32(5))
			}
			return count, nil
		},
	})
}

func TestTextPatterns_BasicSearch_ExactPhrase(t *testing.T) {
	// "Phrase search — wrap the phrase in escaped quotes."
	// db.stores.find({ $text: { $search: "\"coffee shop\"" } })
	// Expected: document 3 only (Coffee Shop exact name match).
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_BasicSearch_ExactPhrase",
		Support: harness.DumboDBFull,
		Setup:   textPatternsCreateStoresIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(
				ctx,
				bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: `"coffee shop"`}}}},
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
			// Only document 3 matches the exact phrase "coffee shop".
			tutorialCheck(t, "BasicSearch_ExactPhrase_count", count, int32(1))
			return count, nil
		},
	})
}

// https://www.mongodb.com/docs/manual/tutorial/text-search-in-aggregation/
//
// Collection: articles
// Text index on { subject: "text" }

func textPatternsArticlesSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "subject", Value: "coffee"}, {Key: "author", Value: "xyz"}, {Key: "views", Value: int32(50)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "subject", Value: "Coffee Shopping"}, {Key: "author", Value: "efg"}, {Key: "views", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "subject", Value: "Baking a cake"}, {Key: "author", Value: "abc"}, {Key: "views", Value: int32(90)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "subject", Value: "baking"}, {Key: "author", Value: "xyz"}, {Key: "views", Value: int32(100)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "subject", Value: "Café Con Leche"}, {Key: "author", Value: "abc"}, {Key: "views", Value: int32(200)}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "subject", Value: "Сырники"}, {Key: "author", Value: "jkl"}, {Key: "views", Value: int32(80)}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "subject", Value: "coffee and cream"}, {Key: "author", Value: "efg"}, {Key: "views", Value: int32(10)}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "subject", Value: "Cafe Latte"}, {Key: "author", Value: "xyz"}, {Key: "views", Value: int32(30)}},
	})
	return err
}

func textPatternsCreateArticlesIndex(ctx context.Context, col *mongo.Collection) error {
	if err := textPatternsArticlesSeed(ctx, col); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "subject", Value: "text"}},
	})
	return err
}

func TestTextPatterns_Aggregation_MatchText(t *testing.T) {
	// "$match stage with $text finds documents containing 'coffee'."
	// db.articles.aggregate([ { $match: { $text: { $search: "coffee" } } } ])
	// Expected: documents 1, 2, 7 (subjects with "coffee"; "Cafe Latte" does NOT match).
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_Aggregation_MatchText",
		Support: harness.DumboDBFull,
		Setup:   textPatternsCreateArticlesIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "$text", Value: bson.D{{Key: "$search", Value: "coffee"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Extract IDs for comparison.
			ids := make([]interface{}, len(results))
			for i, r := range results {
				for _, e := range r {
					if e.Key == "_id" {
						ids[i] = e.Value
					}
				}
			}
			expected := []interface{}{int32(1), int32(2), int32(7)}
			tutorialCheck(t, "Aggregation_MatchText_ids", ids, expected)
			return int32(len(results)), nil
		},
	})
}

func TestTextPatterns_Aggregation_GroupTotalViews(t *testing.T) {
	// "$match + $group to sum views of coffee-related articles."
	// db.articles.aggregate([
	//   { $match: { $text: { $search: "coffee" } } },
	//   { $group: { _id: null, views: { $sum: "$views" } } }
	// ])
	// Expected: { _id: null, views: 65 }  (50 + 5 + 10; "Cafe Latte" does not match)
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_Aggregation_GroupTotalViews",
		Support: harness.DumboDBFull,
		Setup:   textPatternsCreateArticlesIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "$text", Value: bson.D{{Key: "$search", Value: "coffee"}}},
				}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "views", Value: bson.D{{Key: "$sum", Value: "$views"}}},
				}}},
			}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) != 1 {
				return int32(0), nil
			}
			var views interface{}
			for _, e := range results[0] {
				if e.Key == "views" {
					views = e.Value
				}
			}
			// Total views: 50 + 5 + 10 = 65 (docs 1, 2, 7)
			tutorialCheck(t, "Aggregation_GroupTotalViews", views, int32(65))
			return views, nil
		},
	})
}

func TestTextPatterns_Aggregation_SortByScore(t *testing.T) {
	// "$sort by textScore metadata returns most-relevant documents first."
	// db.articles.aggregate([
	//   { $match: { $text: { $search: "coffee" } } },
	//   { $sort: { score: { $meta: "textScore" } } },
	//   { $project: { subject: 1, score: { $meta: "textScore" } } }
	// ])
	// Expected: documents in descending relevance order; subject "coffee" ranks highest.
	harness.PairTest(t, harness.TestCase{
		Name:    "TextPatterns_Aggregation_SortByScore",
		Support: harness.DumboDBXFail,
		Setup:   textPatternsCreateArticlesIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "$text", Value: bson.D{{Key: "$search", Value: "coffee"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{
					{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "subject", Value: 1},
					{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}},
				}}},
			}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// 3 matching documents sorted by relevance (docs 1, 2, 7).
			count := int32(len(results))
			tutorialCheck(t, "Aggregation_SortByScore_count", count, int32(3))
			return count, nil
		},
	})
}
