package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestIndex_CreateOne_SingleAscending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_SingleAscending",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_CreateOne_SingleDescending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_SingleDescending",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "score", Value: -1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_CreateOne_SingleField_UsedByFind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_SingleField_UsedByFind",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "Alice"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "name", Value: "Bob"}, {Key: "score", Value: int32(20)}},
				bson.D{{Key: "name", Value: "Carol"}, {Key: "score", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			cur, err := col.Find(ctx, bson.D{{Key: "name", Value: "Bob"}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_DropOne_SingleField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropOne_SingleField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
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

func TestIndex_DropAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropAll",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "name", Value: 1}}},
				{Keys: bson.D{{Key: "score", Value: -1}}},
			}
			if _, err := col.Indexes().CreateMany(ctx, models); err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropAll(ctx); err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_CreateOne_IdempotentSameSpec(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_IdempotentSameSpec",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}}
			name1, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			name2, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "name1", Value: name1},
				{Key: "name2", Value: name2},
				{Key: "same", Value: name1 == name2},
			}, nil
		},
	})
}

func TestIndex_CreateOne_Compound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_Compound",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "name", Value: 1},
				{Key: "score", Value: -1},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Compound_UsedByFind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_UsedByFind",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "Alice"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "name", Value: "Alice"}, {Key: "score", Value: int32(20)}},
				bson.D{{Key: "name", Value: "Bob"}, {Key: "score", Value: int32(5)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "name", Value: 1},
				{Key: "score", Value: -1},
			}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			cur, err := col.Find(ctx, bson.D{{Key: "name", Value: "Alice"}},
				options.Find().SetSort(bson.D{{Key: "score", Value: -1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_Compound_ThreeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_ThreeFields",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "a", Value: 1},
				{Key: "b", Value: 1},
				{Key: "c", Value: -1},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Compound_Drop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_Drop",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "name", Value: 1},
				{Key: "score", Value: -1},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
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

func TestIndex_Unique_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Unique_DuplicateKeyError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_DuplicateKeyError",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "a@b.com"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "a@b.com"}})
			if err != nil {
				return bson.D{{Key: "duplicate_key_error", Value: true}}, nil
			}
			return bson.D{{Key: "duplicate_key_error", Value: false}}, nil
		},
	})
}

func TestIndex_Unique_AllowDistinctValues(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_AllowDistinctValues",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			_, err := col.Indexes().CreateOne(ctx, model)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "email", Value: "a@x.com"}},
				bson.D{{Key: "email", Value: "b@x.com"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Unique_Compound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_Compound",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "first", Value: 1}, {Key: "last", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Sparse_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sparse := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "optional_field", Value: 1}},
				Options: &options.IndexOptions{Sparse: &sparse},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Sparse_OmitsMissingField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_OmitsMissingField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "opt", Value: "yes"}},
				bson.D{{Key: "_id", Value: "s2"}}, // no "opt" field
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sparse := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "opt", Value: 1}},
				Options: &options.IndexOptions{Sparse: &sparse},
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			// sparse index only covers s1; s2 is excluded
			count, err := col.CountDocuments(ctx, bson.D{{Key: "opt", Value: bson.D{{Key: "$exists", Value: true}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count_with_field", Value: count}}, nil
		},
	})
}

func TestIndex_Sparse_UniqueWithMissingField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_UniqueWithMissingField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sparse := true
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: &options.IndexOptions{Sparse: &sparse, Unique: &unique},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			// multiple docs without email field should be allowed under sparse+unique
			_, err = col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "A"}},
				bson.D{{Key: "name", Value: "B"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_TTL_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_TTL_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			expireAfter := int32(3600)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "createdAt", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_TTL_ZeroSeconds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_TTL_ZeroSeconds",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			expireAfter := int32(0)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "expireAt", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_TTL_InsertAndVerifyNotExpiredYet(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_TTL_InsertAndVerifyNotExpiredYet",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			expireAfter := int32(3600)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "ts", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "ts", Value: time.Now()},
				{Key: "data", Value: "fresh"},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Partial_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(10)}}}}
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetPartialFilterExpression(filter),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Partial_OnlyIndexesMatchingDocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_OnlyIndexesMatchingDocs",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "score", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "score", Value: int32(50)}},
				bson.D{{Key: "_id", Value: "p3"}, {Key: "score", Value: int32(100)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(10)}}}}
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetPartialFilterExpression(filter),
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(10)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "high_score_count", Value: count}}, nil
		},
	})
}

func TestIndex_Partial_WithExistsFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_WithExistsFilter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "email", Value: bson.D{{Key: "$exists", Value: true}}}}
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: options.Index().SetPartialFilterExpression(filter),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Wildcard_AllFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Wildcard_AllFields",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "$**", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Wildcard_SpecificSubPath(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Wildcard_SpecificSubPath",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "metadata.$**", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Wildcard_QueryUnindexedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Wildcard_QueryUnindexedField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "$**", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "x", Value: 1}, {Key: "y", Value: "hello"}},
				bson.D{{Key: "x", Value: 2}, {Key: "y", Value: "world"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "y", Value: "hello"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Text_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "content", Value: "text"}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Text_MultipleFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_MultipleFields",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "title", Value: "text"},
				{Key: "body", Value: "text"},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Text_SearchQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_SearchQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "content", Value: "text"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "content", Value: "the quick brown fox"}},
				bson.D{{Key: "content", Value: "lazy dog sleeping"}},
				bson.D{{Key: "content", Value: "quick rabbit running"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick"}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Text_WithLanguage(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_WithLanguage",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "content", Value: "text"}},
				Options: options.Index().SetDefaultLanguage("english"),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_2dsphere_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_2dsphere_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "location", Value: "2dsphere"}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_2dsphere_NearQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_2dsphere_NearQuery",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "location", Value: "2dsphere"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nyc"}, {Key: "location", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-74.0060, 40.7128}},
				}}},
				bson.D{{Key: "_id", Value: "la"}, {Key: "location", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-118.2437, 34.0522}},
				}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "location", Value: bson.D{
					{Key: "$near", Value: bson.D{
						{Key: "$geometry", Value: bson.D{
							{Key: "type", Value: "Point"},
							{Key: "coordinates", Value: bson.A{-74.0, 40.7}},
						}},
						{Key: "$maxDistance", Value: 5000},
					}},
				}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_2dsphere_GeoWithinQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_2dsphere_GeoWithinQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "in"}, {Key: "loc", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-74.0, 40.7}},
				}}},
				bson.D{{Key: "_id", Value: "out"}, {Key: "loc", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{10.0, 50.0}},
				}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "loc", Value: bson.D{
					{Key: "$geoWithin", Value: bson.D{
						{Key: "$centerSphere", Value: bson.A{
							bson.A{-74.0, 40.7}, 0.01,
						}},
					}},
				}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Hashed_CreateOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hashed_CreateOne",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: "hashed"}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Hashed_EqualityQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hashed_EqualityQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "uid", Value: "hashed"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "uid", Value: "u1"}, {Key: "val", Value: 1}},
				bson.D{{Key: "uid", Value: "u2"}, {Key: "val", Value: 2}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "uid", Value: "u1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_CreateMany_TwoIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_TwoIndexes",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "name", Value: 1}}},
				{Keys: bson.D{{Key: "score", Value: -1}}},
			}
			names, err := col.Indexes().CreateMany(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(names))}}, nil
		},
	})
}

func TestIndex_CreateMany_WithUnique(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_WithUnique",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "name", Value: 1}}},
				{
					Keys:    bson.D{{Key: "email", Value: 1}},
					Options: &options.IndexOptions{Unique: &unique},
				},
			}
			names, err := col.Indexes().CreateMany(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(names))}}, nil
		},
	})
}

func TestIndex_CreateMany_ThreeCompound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_ThreeCompound",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}},
				{Keys: bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 1}}},
				{Keys: bson.D{{Key: "c", Value: 1}, {Key: "d", Value: 1}}},
			}
			names, err := col.Indexes().CreateMany(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(names))}}, nil
		},
	})
}

func TestIndex_ListIndexes_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_EmptyCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			// Only the _id index should exist
			return bson.D{{Key: "index_count", Value: int32(len(indexes))}}, nil
		},
	})
}

func TestIndex_ListIndexes_AfterCreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_AfterCreate",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "name", Value: 1}}},
				{Keys: bson.D{{Key: "score", Value: -1}}},
			}
			if _, err := col.Indexes().CreateMany(ctx, models); err != nil {
				return nil, err
			}
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_count", Value: int32(len(indexes))}}, nil
		},
	})
}

func TestIndex_ListIndexes_AfterDrop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_AfterDrop",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "x", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropOne(ctx, name); err != nil {
				return nil, err
			}
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_count", Value: int32(len(indexes))}}, nil
		},
	})
}

func TestIndex_ListIndexes_VerifyNames(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_VerifyNames",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "field", Value: 1}}}
			indexName, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			found := false
			for _, idx := range indexes {
				for _, elem := range idx {
					if elem.Key == "name" && elem.Value == indexName {
						found = true
					}
				}
			}
			return bson.D{{Key: "created_index_found", Value: found}}, nil
		},
	})
}

func TestIndex_DropOne_NonExistent(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropOne_NonExistent",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Indexes().DropOne(ctx, "nonexistent_index_1")
			if err != nil {
				return bson.D{{Key: "error", Value: true}}, nil
			}
			return bson.D{{Key: "error", Value: false}}, nil
		},
	})
}

func TestIndex_DropOne_IdIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropOne_IdIndex",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Indexes().DropOne(ctx, "_id_")
			if err != nil {
				return bson.D{{Key: "error_dropping_id", Value: true}}, nil
			}
			return bson.D{{Key: "error_dropping_id", Value: false}}, nil
		},
	})
}

func TestIndex_DropAll_AfterCreateMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropAll_AfterCreateMany",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "a", Value: 1}}},
				{Keys: bson.D{{Key: "b", Value: 1}}},
				{Keys: bson.D{{Key: "c", Value: 1}}},
			}
			if _, err := col.Indexes().CreateMany(ctx, models); err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropAll(ctx); err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_Hint_Find_ByName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_Find_ByName",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "score", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "score", Value: int32(1)}},
				bson.D{{Key: "score", Value: int32(2)}},
				bson.D{{Key: "score", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetHint("score_1"))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_Hint_Find_BySpec(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_Find_BySpec",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "Alice"}},
				bson.D{{Key: "name", Value: "Bob"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetHint(bson.D{{Key: "name", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_Hint_IdIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_IdIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "h1"}, {Key: "x", Value: 1}},
				bson.D{{Key: "_id", Value: "h2"}, {Key: "x", Value: 2}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetHint(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_Hint_NonExistentIndexError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_NonExistentIndexError",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetHint("nonexistent_idx"))
			if err != nil {
				return bson.D{{Key: "error", Value: true}}, nil
			}
			var results []bson.D
			if err2 := cur.All(ctx, &results); err2 != nil {
				return bson.D{{Key: "error", Value: true}}, nil
			}
			return bson.D{{Key: "error", Value: false}}, nil
		},
	})
}

func TestIndex_Explain_WithHint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Explain_WithHint",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "score", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "score", Value: int32(10)}},
				bson.D{{Key: "score", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			result := col.Database().RunCommand(ctx, bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{}},
					{Key: "hint", Value: bson.D{{Key: "score", Value: 1}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			})
			if result.Err() != nil {
				return nil, result.Err()
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_IndexStats_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_IndexStats_Basic",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "score", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "score", Value: int32(1)}},
				bson.D{{Key: "score", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$indexStats", Value: bson.D{}}},
			}
			cur, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var stats []bson.D
			if err := cur.All(ctx, &stats); err != nil {
				return nil, err
			}
			return bson.D{{Key: "stat_count", Value: int32(len(stats))}}, nil
		},
	})
}

func TestIndex_IndexStats_NoIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_IndexStats_NoIndexes",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$indexStats", Value: bson.D{}}},
			}
			cur, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var stats []bson.D
			if err := cur.All(ctx, &stats); err != nil {
				return nil, err
			}
			// Only _id index stats should appear
			return bson.D{{Key: "stat_count", Value: int32(len(stats))}}, nil
		},
	})
}

func TestIndex_CreateOne_CustomName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_CustomName",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("my_score_idx"),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_DropOne_CustomName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DropOne_CustomName",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("my_score_idx"),
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropOne(ctx, "my_score_idx"); err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_CreateOne_WithBackground(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_WithBackground",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "val", Value: 1}},
				Options: options.Index().SetBackground(true),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_CreateOne_OnNestedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_OnNestedField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "address.city", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_CreateOne_OnArrayField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_OnArrayField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "tags", Value: bson.A{"go", "web"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "tags", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_CreateOne_MultiKey_Query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_MultiKey_Query",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "tags", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "tags", Value: bson.A{"js", "web"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "tags", Value: "go"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_CreateMany_Empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_Empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			names, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(names))}}, nil
		},
	})
}

func TestIndex_CreateOne_DuplicateConflictingSpec(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_DuplicateConflictingSpec",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Create index with name A
			model1 := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_asc"),
			}
			if _, err := col.Indexes().CreateOne(ctx, model1); err != nil {
				return nil, err
			}
			// Same keys, different name — should error in MongoDB
			model2 := mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_different_name"),
			}
			_, err := col.Indexes().CreateOne(ctx, model2)
			if err != nil {
				return bson.D{{Key: "conflict_error", Value: true}}, nil
			}
			return bson.D{{Key: "conflict_error", Value: false}}, nil
		},
	})
}

func TestIndex_Hint_Aggregate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_Aggregate",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "score", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "score", Value: int32(1)}},
				bson.D{{Key: "score", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}}},
			}
			cur, err := col.Aggregate(ctx, pipeline,
				options.Aggregate().SetHint(bson.D{{Key: "score", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestIndex_Unique_NullValues(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_NullValues",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			// Two docs with null email — MongoDB allows only one null under unique
			_, err1 := col.InsertOne(ctx, bson.D{{Key: "email", Value: nil}})
			_, err2 := col.InsertOne(ctx, bson.D{{Key: "email", Value: nil}})
			secondFailed := err2 != nil
			_ = err1
			return bson.D{{Key: "second_null_rejected", Value: secondFailed}}, nil
		},
	})
}

func TestIndex_Unique_DropAndRecreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_DropAndRecreate",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "uid", Value: 1}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropOne(ctx, name); err != nil {
				return nil, err
			}
			name2, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "name", Value: name2}}, nil
		},
	})
}

func TestIndex_Compound_SortOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_SortOrder",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(3)}},
				bson.D{{Key: "a", Value: int32(2)}, {Key: "b", Value: int32(1)}},
				bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "a", Value: 1},
				{Key: "b", Value: -1},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Compound_UniqueMultiField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_UniqueMultiField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys: bson.D{
					{Key: "region", Value: 1},
					{Key: "product_id", Value: 1},
				},
				Options: &options.IndexOptions{Unique: &unique},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			// Insert two docs with same region+product_id → should error
			_, err1 := col.InsertOne(ctx, bson.D{{Key: "region", Value: "us"}, {Key: "product_id", Value: int32(1)}})
			_, err2 := col.InsertOne(ctx, bson.D{{Key: "region", Value: "us"}, {Key: "product_id", Value: int32(1)}})
			_ = err1
			return bson.D{
				{Key: "index_name", Value: name},
				{Key: "duplicate_rejected", Value: err2 != nil},
			}, nil
		},
	})
}

func TestIndex_Compound_PrefixQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Compound_PrefixQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "category", Value: 1},
				{Key: "price", Value: 1},
			}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "category", Value: "books"}, {Key: "price", Value: int32(10)}},
				bson.D{{Key: "category", Value: "books"}, {Key: "price", Value: int32(20)}},
				bson.D{{Key: "category", Value: "electronics"}, {Key: "price", Value: int32(100)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Query using only prefix field — index should still be used
			count, err := col.CountDocuments(ctx, bson.D{{Key: "category", Value: "books"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Sort_WithIndexedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sort_WithIndexedField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "rank", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "rank", Value: int32(3)}},
				bson.D{{Key: "rank", Value: int32(1)}},
				bson.D{{Key: "rank", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetSort(bson.D{{Key: "rank", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "first_rank", Value: nil}}, nil
			}
			firstRank := results[0].Map()["rank"]
			return bson.D{{Key: "first_rank", Value: firstRank}}, nil
		},
	})
}

func TestIndex_Sort_DescendingWithIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sort_DescendingWithIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "ts", Value: -1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "ts", Value: int32(10)}},
				bson.D{{Key: "ts", Value: int32(30)}},
				bson.D{{Key: "ts", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "first_ts", Value: nil}}, nil
			}
			firstTs := results[0].Map()["ts"]
			return bson.D{{Key: "first_ts", Value: firstTs}}, nil
		},
	})
}

func TestIndex_Collation_CaseInsensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Collation_CaseInsensitive",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			collation := options.Collation{Locale: "en", Strength: 2}
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "username", Value: 1}},
				Options: options.Index().SetCollation(&collation),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Collation_UniqueWithCollation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Collation_UniqueWithCollation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			collation := options.Collation{Locale: "en", Strength: 2}
			model := mongo.IndexModel{
				Keys: bson.D{{Key: "name", Value: 1}},
				Options: options.Index().
					SetUnique(unique).
					SetCollation(&collation),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Sparse_CreateAndQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_CreateAndQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			sparse := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "phone", Value: 1}},
				Options: &options.IndexOptions{Sparse: &sparse},
			}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "u1"}, {Key: "phone", Value: "555-0100"}},
				bson.D{{Key: "_id", Value: "u2"}},
				bson.D{{Key: "_id", Value: "u3"}, {Key: "phone", Value: "555-0200"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "phone", Value: bson.D{{Key: "$exists", Value: true}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Sparse_Drop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_Drop",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sparse := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "optField", Value: 1}},
				Options: &options.IndexOptions{Sparse: &sparse},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
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

func TestIndex_TTL_OnNestedDateField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_TTL_OnNestedDateField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			expireAfter := int32(86400)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "meta.expireAt", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Partial_UniquePartial(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_UniquePartial",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "active", Value: true}}
			unique := true
			model := mongo.IndexModel{
				Keys: bson.D{{Key: "email", Value: 1}},
				Options: options.Index().
					SetPartialFilterExpression(filter).
					SetUnique(unique),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Partial_CompoundKeys(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_CompoundKeys",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "status", Value: "active"}}
			model := mongo.IndexModel{
				Keys: bson.D{
					{Key: "user_id", Value: 1},
					{Key: "created_at", Value: -1},
				},
				Options: options.Index().SetPartialFilterExpression(filter),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Text_WeightedFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_WeightedFields",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys: bson.D{
					{Key: "title", Value: "text"},
					{Key: "body", Value: "text"},
				},
				Options: options.Index().SetWeights(bson.D{
					{Key: "title", Value: int32(10)},
					{Key: "body", Value: int32(1)},
				}),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_Text_PhrasedSearch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_PhrasedSearch",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "body", Value: "text"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "body", Value: "the quick brown fox"}},
				bson.D{{Key: "body", Value: "quick and lazy brown fox"}},
				bson.D{{Key: "body", Value: "something else entirely"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "$text", Value: bson.D{{Key: "$search", Value: "\"quick brown\""}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Text_ExcludeWord(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Text_ExcludeWord",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "content", Value: "text"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "content", Value: "coffee and tea"}},
				bson.D{{Key: "content", Value: "coffee only"}},
				bson.D{{Key: "content", Value: "tea only"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "$text", Value: bson.D{{Key: "$search", Value: "coffee -tea"}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Wildcard_WithWildcardProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Wildcard_WithWildcardProjection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{
				Keys: bson.D{{Key: "$**", Value: 1}},
				Options: options.Index().SetWildcardProjection(bson.D{
					{Key: "a", Value: int32(1)},
					{Key: "b", Value: int32(1)},
				}),
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_2dsphere_Compound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_2dsphere_Compound",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{
				{Key: "location", Value: "2dsphere"},
				{Key: "category", Value: 1},
			}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_2dsphere_GeoIntersects(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_2dsphere_GeoIntersects",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "geo", Value: "2dsphere"}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "geo", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{
						bson.A{
							bson.A{0.0, 0.0},
							bson.A{1.0, 0.0},
							bson.A{1.0, 1.0},
							bson.A{0.0, 1.0},
							bson.A{0.0, 0.0},
						},
					}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{
				{Key: "geo", Value: bson.D{
					{Key: "$geoIntersects", Value: bson.D{
						{Key: "$geometry", Value: bson.D{
							{Key: "type", Value: "Point"},
							{Key: "coordinates", Value: bson.A{0.5, 0.5}},
						}},
					}},
				}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_Hashed_CannotBeUnique(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hashed_CannotBeUnique",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			unique := true
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "shardKey", Value: "hashed"}},
				Options: &options.IndexOptions{Unique: &unique},
			}
			_, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return bson.D{{Key: "error_as_expected", Value: true}}, nil
			}
			return bson.D{{Key: "error_as_expected", Value: false}}, nil
		},
	})
}

func TestIndex_Hint_FindOne_ByName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_FindOne_ByName",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "val", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "val", Value: int32(42)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "val", Value: int32(42)}},
				options.FindOne().SetHint("val_1")).Decode(&result)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "found", Value: true}}, nil
		},
	})
}

func TestIndex_Hint_CountDocuments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Hint_CountDocuments",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "status", Value: "active"}},
				bson.D{{Key: "status", Value: "inactive"}},
				bson.D{{Key: "status", Value: "active"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx,
				bson.D{{Key: "status", Value: "active"}},
				options.Count().SetHint("status_1"),
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_ListIndexes_AfterDropAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_AfterDropAll",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "x", Value: 1}}},
				{Keys: bson.D{{Key: "y", Value: -1}}},
			}
			if _, err := col.Indexes().CreateMany(ctx, models); err != nil {
				return nil, err
			}
			if _, err := col.Indexes().DropAll(ctx); err != nil {
				return nil, err
			}
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			// After DropAll, only _id index remains
			return bson.D{{Key: "index_count", Value: int32(len(indexes))}}, nil
		},
	})
}

func TestIndex_ListIndexes_CheckKeys(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_ListIndexes_CheckKeys",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "username", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return nil, err
			}
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var indexes []bson.D
			if err := cur.All(ctx, &indexes); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(indexes))}}, nil
		},
	})
}

func TestIndex_CreateMany_MixedTypes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_MixedTypes",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sparse := true
			unique := true
			models := []mongo.IndexModel{
				{Keys: bson.D{{Key: "field_a", Value: 1}}},
				{
					Keys:    bson.D{{Key: "field_b", Value: 1}},
					Options: &options.IndexOptions{Sparse: &sparse},
				},
				{
					Keys:    bson.D{{Key: "field_c", Value: 1}},
					Options: &options.IndexOptions{Unique: &unique},
				},
			}
			names, err := col.Indexes().CreateMany(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(names))}}, nil
		},
	})
}

func TestIndex_CreateMany_WithCustomNames(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateMany_WithCustomNames",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "p", Value: 1}},
					Options: options.Index().SetName("p_asc"),
				},
				{
					Keys:    bson.D{{Key: "q", Value: -1}},
					Options: options.Index().SetName("q_desc"),
				},
			}
			names, err := col.Indexes().CreateMany(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "names", Value: bson.A{names[0], names[1]}}}, nil
		},
	})
}

func TestIndex_Explain_CollectionScan(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Explain_CollectionScan",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "x", Value: int32(1)}},
				bson.D{{Key: "x", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			result := col.Database().RunCommand(ctx, bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{{Key: "x", Value: int32(1)}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			})
			if result.Err() != nil {
				return nil, result.Err()
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_Explain_UpdateWithHint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Explain_UpdateWithHint",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "k", Value: int32(1)}, {Key: "v", Value: "a"}},
				bson.D{{Key: "k", Value: int32(2)}, {Key: "v", Value: "b"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			result := col.Database().RunCommand(ctx, bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "update", Value: col.Name()},
					{Key: "updates", Value: bson.A{bson.D{
						{Key: "q", Value: bson.D{{Key: "k", Value: int32(1)}}},
						{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: "updated"}}}}},
						{Key: "hint", Value: bson.D{{Key: "k", Value: 1}}},
					}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			})
			if result.Err() != nil {
				return nil, result.Err()
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestIndex_CreateOne_DeepNested(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_CreateOne_DeepNested",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "a.b.c", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

func TestIndex_NestedField_Query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_NestedField_Query",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "user.age", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "user", Value: bson.D{{Key: "name", Value: "Alice"}, {Key: "age", Value: int32(30)}}}},
				bson.D{{Key: "user", Value: bson.D{{Key: "name", Value: "Bob"}, {Key: "age", Value: int32(25)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "user.age", Value: bson.D{{Key: "$gte", Value: int32(30)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestIndex_IndexStats_AfterInsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_IndexStats_AfterInsert",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.IndexModel{Keys: bson.D{{Key: "hits", Value: 1}}}
			if _, err := col.Indexes().CreateOne(ctx, model); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "hits", Value: int32(10)}},
				bson.D{{Key: "hits", Value: int32(20)}},
				bson.D{{Key: "hits", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Run a query to generate some index usage
			if _, err := col.CountDocuments(ctx, bson.D{{Key: "hits", Value: bson.D{{Key: "$gt", Value: int32(5)}}}}); err != nil {
				return nil, err
			}
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$indexStats", Value: bson.D{}}},
			}
			cur, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var stats []bson.D
			if err := cur.All(ctx, &stats); err != nil {
				return nil, err
			}
			return bson.D{{Key: "stat_count", Value: int32(len(stats))}}, nil
		},
	})
}
