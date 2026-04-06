// mongodb_group_and_total_test.go covers the MongoDB aggregation tutorial:
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/group-and-total/
//
// The tutorial shows a 6-stage pipeline against an "orders" collection:
//   $match (year 2020) → $sort (orderdate) → $group (by customer) →
//   $sort (first_purchase_date) → $set (customer_id) → $unset (_id)
//
// tutorialCheck is defined in mongodb_dev_patterns_test.go (same package).
package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// groupAndTotalOrdersSeed inserts the 9 orders from the tutorial.
// Source: https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/group-and-total/
func groupAndTotalOrdersSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
			{Key: "value", Value: int32(231)},
		},
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 13, 9, 32, 7, 0, time.UTC))},
			{Key: "value", Value: int32(99)},
		},
		bson.D{
			{Key: "customer_id", Value: "oranieri@warmmail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
			{Key: "value", Value: int32(63)},
		},
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2019, 5, 28, 19, 13, 32, 0, time.UTC))},
			{Key: "value", Value: int32(2)},
		},
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 11, 23, 22, 56, 53, 0, time.UTC))},
			{Key: "value", Value: int32(187)},
		},
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 8, 18, 23, 4, 48, 0, time.UTC))},
			{Key: "value", Value: int32(4)},
		},
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
			{Key: "value", Value: int32(4)},
		},
		// Note: tutorial shell shows 2021-02-29 which doesn't exist; Go examples use 2021-02-28.
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2021, 2, 28, 7, 49, 32, 0, time.UTC))},
			{Key: "value", Value: int32(1024)},
		},
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 10, 3, 13, 49, 44, 0, time.UTC))},
			{Key: "value", Value: int32(102)},
		},
	})
	return err
}

// TestGroupAndTotal_CustomerOrders2020 implements the complete pipeline from the
// "Group and Total" tutorial:
//
//	$match orders in 2020 →
//	$sort by orderdate →
//	$group by customer_id ($first date, $sum value, $sum count, $push orders) →
//	$sort by first_purchase_date →
//	$set customer_id = $_id →
//	$unset _id
//
// Expected output (3 docs sorted by first_purchase_date):
//
//	oranieri@warmmail.com:    total 63,  1 order
//	elise_smith@myemail.com:  total 436, 4 orders
//	tj@wheresmyemail.com:     total 191, 2 orders  (2019/2021 orders excluded by $match)
func TestGroupAndTotal_CustomerOrders2020(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GroupAndTotal_CustomerOrders2020",
		Support: harness.DocudoltXFail,
		Setup:   groupAndTotalOrdersSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				// Stage 1: Match orders placed in year 2020.
				{{Key: "$match", Value: bson.D{
					{Key: "orderdate", Value: bson.D{
						{Key: "$gte", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))},
						{Key: "$lt", Value: primitive.NewDateTimeFromTime(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))},
					}},
				}}},
				// Stage 2: Sort by orderdate ascending (makes $push order deterministic).
				{{Key: "$sort", Value: bson.D{{Key: "orderdate", Value: int32(1)}}}},
				// Stage 3: Group by customer_id, accumulate totals and order list.
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$customer_id"},
					{Key: "first_purchase_date", Value: bson.D{{Key: "$first", Value: "$orderdate"}}},
					{Key: "total_value", Value: bson.D{{Key: "$sum", Value: "$value"}}},
					{Key: "total_orders", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
					{Key: "orders", Value: bson.D{{Key: "$push", Value: bson.D{
						{Key: "orderdate", Value: "$orderdate"},
						{Key: "value", Value: "$value"},
					}}}},
				}}},
				// Stage 4: Sort results by first_purchase_date ascending.
				{{Key: "$sort", Value: bson.D{{Key: "first_purchase_date", Value: int32(1)}}}},
				// Stage 5: Expose customer_id as a top-level field.
				{{Key: "$set", Value: bson.D{{Key: "customer_id", Value: "$_id"}}}},
				// Stage 6: Remove the internal _id field.
				{{Key: "$unset", Value: bson.A{"_id"}}},
			}

			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err = cursor.All(ctx, &results); err != nil {
				return nil, err
			}

			// Validate against the exact output shown in the tutorial docs.
			// Date sentinels: harness.CompareResponses normalises primitive.DateTime
			// to "<DateTime>" so date values match structurally rather than literally.
			expected := []interface{}{
				bson.D{
					{Key: "first_purchase_date", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
					{Key: "total_value", Value: int32(63)},
					{Key: "total_orders", Value: int32(1)},
					{Key: "orders", Value: bson.A{
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
							{Key: "value", Value: int32(63)},
						},
					}},
					{Key: "customer_id", Value: "oranieri@warmmail.com"},
				},
				bson.D{
					{Key: "first_purchase_date", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 13, 9, 32, 7, 0, time.UTC))},
					{Key: "total_value", Value: int32(436)},
					{Key: "total_orders", Value: int32(4)},
					{Key: "orders", Value: bson.A{
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 13, 9, 32, 7, 0, time.UTC))},
							{Key: "value", Value: int32(99)},
						},
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
							{Key: "value", Value: int32(231)},
						},
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 10, 3, 13, 49, 44, 0, time.UTC))},
							{Key: "value", Value: int32(102)},
						},
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
							{Key: "value", Value: int32(4)},
						},
					}},
					{Key: "customer_id", Value: "elise_smith@myemail.com"},
				},
				bson.D{
					{Key: "first_purchase_date", Value: primitive.NewDateTimeFromTime(time.Date(2020, 8, 18, 23, 4, 48, 0, time.UTC))},
					{Key: "total_value", Value: int32(191)},
					{Key: "total_orders", Value: int32(2)},
					{Key: "orders", Value: bson.A{
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 8, 18, 23, 4, 48, 0, time.UTC))},
							{Key: "value", Value: int32(4)},
						},
						bson.D{
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 11, 23, 22, 56, 53, 0, time.UTC))},
							{Key: "value", Value: int32(187)},
						},
					}},
					{Key: "customer_id", Value: "tj@wheresmyemail.com"},
				},
			}

			actual := make([]interface{}, len(results))
			for i, r := range results {
				actual[i] = r
			}
			tutorialCheck(t, "GroupAndTotal_CustomerOrders2020", actual, expected)

			return results, nil
		},
	})
}
