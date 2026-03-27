package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// seedUpdate inserts a standard document for update tests.
func seedUpdate(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: "u1"},
			{Key: "name", Value: "Alice"},
			{Key: "score", Value: int32(10)},
			{Key: "level", Value: int32(3)},
			{Key: "tags", Value: bson.A{"go", "db"}},
			{Key: "address", Value: bson.D{
				{Key: "city", Value: "Portland"},
				{Key: "zip", Value: "97201"},
			}},
		},
		bson.D{
			{Key: "_id", Value: "u2"},
			{Key: "name", Value: "Bob"},
			{Key: "score", Value: int32(20)},
			{Key: "level", Value: int32(5)},
			{Key: "tags", Value: bson.A{"go"}},
		},
		bson.D{
			{Key: "_id", Value: "u3"},
			{Key: "name", Value: "Carol"},
			{Key: "score", Value: int32(30)},
			{Key: "level", Value: int32(7)},
			{Key: "tags", Value: bson.A{"db", "sql"}},
		},
	})
	return err
}

// readOne is a helper to read back a document by _id after an update.
func readOne(ctx context.Context, col *mongo.Collection, id string) (interface{}, error) {
	var result bson.D
	err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&result)
	return result, err
}

// ─── $set ──────────────────────────────────────────────────────────────────────

func TestSet_single_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_single_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(99)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_multiple_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_multiple_fields",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{
					{Key: "score", Value: int32(50)},
					{Key: "level", Value: int32(10)},
				}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_new_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_new_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "newField", Value: "hello"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_nested_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_nested_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "address.city", Value: "Seattle"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_deep_path(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_deep_path",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "dp1"},
				{Key: "a", Value: bson.D{
					{Key: "b", Value: bson.D{
						{Key: "c", Value: "old"},
					}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "dp1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "a.b.c", Value: "new"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "dp1")
		},
	})
}

func TestSet_array_element_by_index(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_array_element_by_index",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "tags.0", Value: "rust"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_no_match",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "missing"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(1)}}}},
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

// ─── $unset ────────────────────────────────────────────────────────────────────

func TestUnset_single_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Unset_single_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "level", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestUnset_multiple_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Unset_multiple_fields",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{
					{Key: "level", Value: ""},
					{Key: "tags", Value: ""},
				}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestUnset_nonexistent_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Unset_nonexistent_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "ghost", Value: ""}}}},
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

func TestUnset_nested_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Unset_nested_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "address.zip", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── $inc ──────────────────────────────────────────────────────────────────────

func TestInc_positive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_positive",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestInc_negative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_negative",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(-3)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestInc_missing_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_missing_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $inc on a field that doesn't exist creates it with the increment value.
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "counter", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestInc_multiple_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_multiple_fields",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{
					{Key: "score", Value: int32(10)},
					{Key: "level", Value: int32(1)},
				}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestInc_on_string_field_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_on_string_field_error",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "name", Value: int32(1)}}}},
			)
			// Expect an error: cannot increment a string field.
			return nil, err
		},
	})
}

// ─── $mul ──────────────────────────────────────────────────────────────────────

func TestMul_int(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Mul_int",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: int32(3)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMul_float(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Mul_float",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: 1.5}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMul_missing_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Mul_missing_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		// $mul on missing field sets it to 0.
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "absent", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMul_zero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Mul_zero",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: int32(0)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── $rename ───────────────────────────────────────────────────────────────────

func TestRename_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Rename_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "level", Value: "rank"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestRename_nested_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Rename_nested_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "address.zip", Value: "address.postalCode"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestRename_nonexistent_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Rename_nonexistent_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		// Renaming a non-existent field is a no-op in MongoDB.
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "ghost", Value: "specter"}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── $min / $max ───────────────────────────────────────────────────────────────

func TestMin_updates_when_smaller(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Min_updates_when_smaller",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "score", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMin_no_update_when_larger(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Min_no_update_when_larger",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "score", Value: int32(999)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMin_missing_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Min_missing_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		// $min on missing field sets the field.
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "minNew", Value: int32(42)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMax_updates_when_larger(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Max_updates_when_larger",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "score", Value: int32(999)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMax_no_update_when_smaller(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Max_no_update_when_smaller",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "score", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMax_missing_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Max_missing_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "maxNew", Value: int32(7)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── $setOnInsert ──────────────────────────────────────────────────────────────

func TestSetOnInsert_fires_on_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetOnInsert_fires_on_upsert",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "soi-new"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}},
					{Key: "$setOnInsert", Value: bson.D{{Key: "created", Value: "yes"}}},
				},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "soi-new")
		},
	})
}

func TestSetOnInsert_noop_on_update(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetOnInsert_noop_on_update",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "soi-exist"}, {Key: "x", Value: int32(0)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "soi-exist"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}},
					{Key: "$setOnInsert", Value: bson.D{{Key: "created", Value: "yes"}}},
				},
				opts,
			)
			if err != nil {
				return nil, err
			}
			// created should NOT appear — $setOnInsert is a no-op on real updates.
			return readOne(ctx, col, "soi-exist")
		},
	})
}

// ─── $currentDate ──────────────────────────────────────────────────────────────

func TestCurrentDate_as_date(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CurrentDate_as_date",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			before := time.Now().Add(-time.Second)
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$currentDate", Value: bson.D{{Key: "lastModified", Value: true}}}},
			)
			if err != nil {
				return nil, err
			}
			var doc bson.D
			if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "u1"}}).Decode(&doc); err != nil {
				return nil, err
			}
			// Verify lastModified is a date after 'before'.
			m := doc.Map()
			ts, ok := m["lastModified"].(primitive.DateTime)
			if !ok {
				return bson.D{{Key: "hasDate", Value: false}}, nil
			}
			return bson.D{{Key: "hasDate", Value: ts.Time().After(before)}}, nil
		},
	})
}

func TestCurrentDate_as_timestamp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CurrentDate_as_timestamp",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$currentDate", Value: bson.D{{Key: "ts", Value: bson.D{{Key: "$type", Value: "timestamp"}}}}}},
			)
			if err != nil {
				return nil, err
			}
			// Signal success by presence of no error.
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

// ─── $push ─────────────────────────────────────────────────────────────────────

func TestPush_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_basic",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: "rust"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPush_each(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_each",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: bson.D{
					{Key: "$each", Value: bson.A{"c", "d"}},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPush_each_slice(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_each_slice",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: bson.D{
					{Key: "$each", Value: bson.A{"x", "y", "z"}},
					{Key: "$slice", Value: int32(3)},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPush_each_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_each_sort",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ps1"},
				{Key: "scores", Value: bson.A{int32(3), int32(1), int32(4)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "ps1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "scores", Value: bson.D{
					{Key: "$each", Value: bson.A{int32(2), int32(5)}},
					{Key: "$sort", Value: int32(1)},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "ps1")
		},
	})
}

func TestPush_each_position(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_each_position",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: bson.D{
					{Key: "$each", Value: bson.A{"first"}},
					{Key: "$position", Value: int32(0)},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPush_on_non_array_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_on_non_array_error",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "name", Value: "x"}}}},
			)
			return nil, err
		},
	})
}

// ─── $pop ──────────────────────────────────────────────────────────────────────

func TestPop_last(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pop_last",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPop_first(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pop_first",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: int32(-1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPop_empty_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pop_empty_array",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pop-empty"},
				{Key: "arr", Value: bson.A{}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "pop-empty"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "arr", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── $pull ─────────────────────────────────────────────────────────────────────

func TestPull_by_equality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pull_by_equality",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: "go"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPull_by_condition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pull_by_condition",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pull-cond"},
				{Key: "scores", Value: bson.A{int32(1), int32(5), int32(3), int32(8)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "pull-cond"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "scores", Value: bson.D{{Key: "$gt", Value: int32(4)}}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "pull-cond")
		},
	})
}

func TestPull_by_in_condition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pull_by_in_condition",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u3"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: bson.D{{Key: "$in", Value: bson.A{"db", "sql"}}}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u3")
		},
	})
}

func TestPull_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pull_no_match",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: "missing"}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── $pullAll ──────────────────────────────────────────────────────────────────

func TestPullAll_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PullAll_basic",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pullAll", Value: bson.D{{Key: "tags", Value: bson.A{"go", "db"}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPullAll_partial_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PullAll_partial_match",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$pullAll", Value: bson.D{{Key: "tags", Value: bson.A{"go", "notexist"}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── $addToSet ─────────────────────────────────────────────────────────────────

func TestAddToSet_new_element(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddToSet_new_element",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: "new"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestAddToSet_duplicate_no_change(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddToSet_duplicate_no_change",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: "go"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestAddToSet_each(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddToSet_each",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: bson.D{
					{Key: "$each", Value: bson.A{"go", "rust", "zig"}},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestAddToSet_missing_array_creates(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddToSet_missing_array_creates",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "newSet", Value: "item"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── Positional operators ───────────────────────────────────────────────────────

func TestPositional_dollar_update_first_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Positional_dollar_update_first_match",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pos1"},
				{Key: "items", Value: bson.A{
					bson.D{{Key: "x", Value: int32(1)}},
					bson.D{{Key: "x", Value: int32(2)}},
					bson.D{{Key: "x", Value: int32(3)}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "items.x", Value: int32(2)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "items.$.x", Value: int32(99)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "pos1")
		},
	})
}

func TestPositional_all_dollar_bracket(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Positional_all_dollar_bracket",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "allpos1"},
				{Key: "scores", Value: bson.A{int32(1), int32(2), int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "allpos1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "scores.$[]", Value: int32(10)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "allpos1")
		},
	})
}

func TestArrayFilters_filtered_positional(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayFilters_filtered_positional",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "af1"},
				{Key: "grades", Value: bson.A{
					bson.D{{Key: "score", Value: int32(80)}, {Key: "passing", Value: false}},
					bson.D{{Key: "score", Value: int32(90)}, {Key: "passing", Value: false}},
					bson.D{{Key: "score", Value: int32(40)}, {Key: "passing", Value: false}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetArrayFilters(options.ArrayFilters{
				Filters: []interface{}{
					bson.D{{Key: "elem.score", Value: bson.D{{Key: "$gte", Value: int32(70)}}}},
				},
			})
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "af1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "grades.$[elem].passing", Value: true}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "af1")
		},
	})
}

func TestArrayFilters_no_matching_elements(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayFilters_no_matching_elements",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "af-nomatch"},
				{Key: "vals", Value: bson.A{int32(1), int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetArrayFilters(options.ArrayFilters{
				Filters: []interface{}{
					bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: int32(100)}}}},
				},
			})
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "af-nomatch"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "vals.$[v]", Value: int32(0)}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── Pipeline-style updates ────────────────────────────────────────────────────

func TestPipelineUpdate_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PipelineUpdate_set",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$set", Value: bson.D{
					{Key: "doubled", Value: bson.D{{Key: "$multiply", Value: bson.A{"$score", 2}}}},
				}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPipelineUpdate_unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PipelineUpdate_unset",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$unset", Value: bson.A{"level", "tags"}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPipelineUpdate_addFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PipelineUpdate_addFields",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "scorePlusTen", Value: bson.D{{Key: "$add", Value: bson.A{"$score", 10}}}},
				}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPipelineUpdate_replaceWith(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PipelineUpdate_replaceWith",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$replaceWith", Value: bson.D{
					{Key: "_id", Value: "$_id"},
					{Key: "name", Value: "$name"},
					{Key: "newField", Value: "replaced"},
				}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestPipelineUpdate_many(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PipelineUpdate_many",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$set", Value: bson.D{
					{Key: "processed", Value: true},
				}}},
			}
			res, err := col.UpdateMany(ctx,
				bson.D{},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── $bit ──────────────────────────────────────────────────────────────────────

func TestBit_and_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Bit_and_int32",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit1"},
				{Key: "flags", Value: int32(15)}, // 0b1111
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit1"}},
				bson.D{{Key: "$bit", Value: bson.D{{Key: "flags", Value: bson.D{{Key: "and", Value: int32(10)}}}}}}, // 0b1010
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "bit1")
		},
	})
}

func TestBit_or_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Bit_or_int32",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit2"},
				{Key: "flags", Value: int32(5)}, // 0b0101
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit2"}},
				bson.D{{Key: "$bit", Value: bson.D{{Key: "flags", Value: bson.D{{Key: "or", Value: int32(10)}}}}}}, // 0b1010
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "bit2")
		},
	})
}

func TestBit_xor_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Bit_xor_int32",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit3"},
				{Key: "flags", Value: int32(15)}, // 0b1111
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit3"}},
				bson.D{{Key: "$bit", Value: bson.D{{Key: "flags", Value: bson.D{{Key: "xor", Value: int32(5)}}}}}}, // 0b0101
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "bit3")
		},
	})
}

func TestBit_and_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Bit_and_int64",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit4"},
				{Key: "flags", Value: int64(255)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit4"}},
				bson.D{{Key: "$bit", Value: bson.D{{Key: "flags", Value: bson.D{{Key: "and", Value: int64(170)}}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "bit4")
		},
	})
}

// ─── UpdateMany ────────────────────────────────────────────────────────────────

func TestUpdateMany_set_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_set_all",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{},
				bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "active"}}}},
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

func TestUpdateMany_inc_filtered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_inc_filtered",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(100)}}}},
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

func TestUpdateMany_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_no_match",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(9999)}}}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "matched", Value: res.MatchedCount}}, nil
		},
	})
}

func TestUpdateMany_unset_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_unset_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "level", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

// ─── Upsert ────────────────────────────────────────────────────────────────────

func TestUpsert_creates_doc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Upsert_creates_doc",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "upsert-new"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "created", Value: true}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "upserted", Value: res.UpsertedCount},
				{Key: "matched", Value: res.MatchedCount},
			}, nil
		},
	})
}

func TestUpsert_updates_existing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Upsert_updates_existing",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "upsert-exist"}, {Key: "v", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "upsert-exist"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(2)}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "upserted", Value: res.UpsertedCount},
				{Key: "modified", Value: res.ModifiedCount},
			}, nil
		},
	})
}

// ─── $set _id immutable ─────────────────────────────────────────────────────────

func TestSet_id_immutable_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_id_immutable_error",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: "changed"}}}},
			)
			return nil, err
		},
	})
}

// ─── Empty update document ─────────────────────────────────────────────────────

func TestUpdate_empty_document_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Update_empty_document_error",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{},
			)
			return nil, err
		},
	})
}

// ─── FindOneAndUpdate ──────────────────────────────────────────────────────────

func TestFindOneAndUpdate_default_returns_before(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_default_returns_before",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var before bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(99)}}}},
			).Decode(&before)
			return before, err
		},
	})
}

func TestFindOneAndUpdate_return_after(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_return_after",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
			var after bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(77)}}}},
				opts,
			).Decode(&after)
			return after, err
		},
	})
}

func TestFindOneAndUpdate_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_upsert",
		Support: harness.DongoXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().
				SetUpsert(true).
				SetReturnDocument(options.After)
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "foau-new"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndUpdate_projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_projection",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().
				SetReturnDocument(options.After).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(11)}}}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndUpdate_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_sort",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().
				SetSort(bson.D{{Key: "score", Value: -1}}).
				SetReturnDocument(options.After)
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{},
				bson.D{{Key: "$set", Value: bson.D{{Key: "top", Value: true}}}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndUpdate_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_no_match",
		Support: harness.DongoXFail,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "notexist"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}},
			).Decode(&result)
			// Expect mongo.ErrNoDocuments.
			return nil, err
		},
	})
}

// ─── Combine multiple operators ─────────────────────────────────────────────────

func TestCombined_set_and_inc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_set_and_inc",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "name", Value: "Alicia"}}},
					{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}},
				},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestCombined_set_and_unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_set_and_unset",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "newField", Value: "added"}}},
					{Key: "$unset", Value: bson.D{{Key: "level", Value: ""}}},
				},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestCombined_inc_and_push(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_inc_and_push",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{
					{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(1)}}},
					{Key: "$push", Value: bson.D{{Key: "tags", Value: "new"}}},
				},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

// ─── ReplaceOne ────────────────────────────────────────────────────────────────

func TestReplaceOne_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplaceOne_basic",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{
					{Key: "_id", Value: "u1"},
					{Key: "replaced", Value: true},
					{Key: "score", Value: int32(0)},
				},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestReplaceOne_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplaceOne_upsert",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Replace().SetUpsert(true)
			res, err := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "replace-new"}},
				bson.D{{Key: "_id", Value: "replace-new"}, {Key: "v", Value: int32(42)}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "upserted", Value: res.UpsertedCount}}, nil
		},
	})
}

func TestReplaceOne_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplaceOne_no_match",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "ghost"}},
				bson.D{{Key: "_id", Value: "ghost"}, {Key: "x", Value: int32(1)}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "matched", Value: res.MatchedCount}}, nil
		},
	})
}

// ─── DeleteOne / DeleteMany (completion) ───────────────────────────────────────

func TestDeleteOne_by_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteOne_by_filter",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: "u1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deleted", Value: res.DeletedCount}}, nil
		},
	})
}

func TestDeleteMany_filtered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteMany_filtered",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deleted", Value: res.DeletedCount}}, nil
		},
	})
}

// ─── $min/$max with dates ──────────────────────────────────────────────────────

func TestMin_with_date(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Min_with_date",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "date-min"},
				{Key: "ts", Value: primitive.NewDateTimeFromTime(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			earlier := primitive.NewDateTimeFromTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "date-min"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "ts", Value: earlier}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "date-min")
		},
	})
}

func TestMax_with_date(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Max_with_date",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "date-max"},
				{Key: "ts", Value: primitive.NewDateTimeFromTime(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			later := primitive.NewDateTimeFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "date-max"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "ts", Value: later}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "date-max")
		},
	})
}

// ─── Edge cases ────────────────────────────────────────────────────────────────

func TestSet_null_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_null_value",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "nullable", Value: nil}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_boolean_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_boolean_value",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: true}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestInc_large_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Inc_large_value",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "big"},
				{Key: "n", Value: int64(1000000000)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "big"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int64(999999999)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "big")
		},
	})
}

func TestUpdateOne_matched_not_modified(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_matched_not_modified",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Setting the same value: matched=1 but modified=0.
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(10)}}}}, // same as seed
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

func TestPush_creates_array_if_missing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Push_creates_array_if_missing",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "newArr", Value: "first"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestMul_on_non_numeric_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Mul_on_non_numeric_error",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "name", Value: int32(2)}}}},
			)
			return nil, err
		},
	})
}

func TestSet_object_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_object_value",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "meta", Value: bson.D{
					{Key: "source", Value: "test"},
					{Key: "version", Value: int32(1)},
				}}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestUnset_array_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Unset_array_field",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "tags", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u1")
		},
	})
}

func TestSet_create_nested_path(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_create_nested_path",
		Support: harness.DongoFull,
		Setup:   seedUpdate,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Create a nested path that doesn't exist yet.
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u2"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "profile.bio", Value: "Hello"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "u2")
		},
	})
}

func TestAddToSet_int_values(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddToSet_int_values",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ats-int"},
				{Key: "nums", Value: bson.A{int32(1), int32(2), int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Add an existing value (no-op) and a new value.
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "ats-int"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "nums", Value: int32(2)}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "ats-int")
		},
	})
}

func TestPull_from_nested_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Pull_from_nested_array",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pull-nested"},
				{Key: "data", Value: bson.D{
					{Key: "items", Value: bson.A{"a", "b", "c"}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "pull-nested"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "data.items", Value: "b"}}}},
			)
			if err != nil {
				return nil, err
			}
			return readOne(ctx, col, "pull-nested")
		},
	})
}
