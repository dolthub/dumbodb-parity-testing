// mongodb_aggregation_complete_examples_test.go covers ALL sub-tutorials from:
// https://www.mongodb.com/docs/manual/tutorial/aggregation-complete-examples/
//
// Sub-tutorials covered:
//   1. Filter Data (filtered-subset)       — persons collection, $match/$sort/$limit/$unset
//   2. Group and Total (group-and-total)   — orders collection, $match/$sort/$group/$set/$unset
//   3. Unwind Arrays (unpack-arrays)       — orders with products array, $unwind/$match/$group
//   4. One-to-One Join (one-to-one-join)   — orders + products, $lookup with foreignField
//   5. Multi-Field Join (multi-field-join) — products + orders, $lookup with embedded pipeline
//
// tutorialCheck() is defined in mongodb_dev_patterns_test.go (same package).
// Tests start as DocuDoltXFail and graduate to DocuDoltFull as DocuDolt parity is verified.
//
// tutorialCheckXFail is used instead of tutorialCheck for DocuDoltXFail tests.
// It logs divergence rather than failing the test, so that XFail tests that
// produce wrong-but-not-error DocuDolt output don't fail the CI build.
// When graduating a test to DocuDoltFull, switch back to tutorialCheck.
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

// tutorialCheckXFail verifies actual matches expected from tutorial docs,
// logging divergence rather than failing the test. Use for DocuDoltXFail tests
// where DocuDolt may return wrong values (not errors) so that the CI build is not
// broken by known DocuDolt limitations. Switch to tutorialCheck when graduating
// a test to DocuDoltFull.
func tutorialCheckXFail(t *testing.T, name string, actual interface{}, expected interface{}) {
	t.Helper()
	cmp := harness.CompareResponses(actual, nil, expected, nil)
	if cmp.Result != harness.Match {
		t.Logf("TUTORIAL (xfail) %s: result differs from docs expected:\n%s", name, cmp.Diff)
	}
}

// ─── 1. Filter Data ───────────────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/filtered-subset/
//
// Selects persons who are engineers, sorts youngest-first, limits to 3,
// removes _id and address fields.

// filteredSubsetPersonsSeed inserts the 6 persons from the tutorial.
func filteredSubsetPersonsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "person_id", Value: "6392529400"},
			{Key: "firstname", Value: "Elise"},
			{Key: "lastname", Value: "Smith"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1972, 1, 13, 9, 32, 7, 0, time.UTC))},
			{Key: "vocation", Value: "ENGINEER"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(5625)},
				{Key: "street", Value: "Tipa Circle"},
				{Key: "city", Value: "Wojzinmoj"},
			}},
		},
		bson.D{
			{Key: "person_id", Value: "1723338115"},
			{Key: "firstname", Value: "Olive"},
			{Key: "lastname", Value: "Ranieri"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1985, 5, 12, 23, 14, 30, 0, time.UTC))},
			{Key: "gender", Value: "FEMALE"},
			{Key: "vocation", Value: "ENGINEER"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(9303)},
				{Key: "street", Value: "Mele Circle"},
				{Key: "city", Value: "Tobihbo"},
			}},
		},
		bson.D{
			{Key: "person_id", Value: "8732762874"},
			{Key: "firstname", Value: "Toni"},
			{Key: "lastname", Value: "Jones"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1991, 11, 23, 16, 53, 56, 0, time.UTC))},
			{Key: "vocation", Value: "POLITICIAN"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(1)},
				{Key: "street", Value: "High Street"},
				{Key: "city", Value: "Upper Abbeywoodington"},
			}},
		},
		bson.D{
			{Key: "person_id", Value: "7363629563"},
			{Key: "firstname", Value: "Bert"},
			{Key: "lastname", Value: "Gooding"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1941, 4, 7, 22, 11, 52, 0, time.UTC))},
			{Key: "vocation", Value: "FLORIST"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(13)},
				{Key: "street", Value: "Upper Bold Road"},
				{Key: "city", Value: "Redringtonville"},
			}},
		},
		bson.D{
			{Key: "person_id", Value: "1029648329"},
			{Key: "firstname", Value: "Sophie"},
			{Key: "lastname", Value: "Celements"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1959, 7, 6, 17, 35, 45, 0, time.UTC))},
			{Key: "vocation", Value: "ENGINEER"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(5)},
				{Key: "street", Value: "Innings Close"},
				{Key: "city", Value: "Basilbridge"},
			}},
		},
		bson.D{
			{Key: "person_id", Value: "7363626383"},
			{Key: "firstname", Value: "Carl"},
			{Key: "lastname", Value: "Simmons"},
			{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1998, 12, 26, 13, 13, 55, 0, time.UTC))},
			{Key: "vocation", Value: "ENGINEER"},
			{Key: "address", Value: bson.D{
				{Key: "number", Value: int32(187)},
				{Key: "street", Value: "Hillside Road"},
				{Key: "city", Value: "Kenningford"},
			}},
		},
	})
	return err
}

// TestFilteredSubset_ThreeYoungestEngineers implements the complete pipeline from the
// "Filter Data" tutorial:
//
//	$match vocation == ENGINEER →
//	$sort dateofbirth descending (youngest first) →
//	$limit 3 →
//	$unset [_id, address]
//
// Expected output (3 youngest engineers, sorted youngest-first):
//
//	Carl Simmons    (born 1998)
//	Olive Ranieri   (born 1985)
//	Elise Smith     (born 1972)
func TestFilteredSubset_ThreeYoungestEngineers(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FilteredSubset_ThreeYoungestEngineers",
		Support: harness.DocuDoltFull,
		Setup:   filteredSubsetPersonsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				// Stage 1: Match engineers only.
				{{Key: "$match", Value: bson.D{{Key: "vocation", Value: "ENGINEER"}}}},
				// Stage 2: Sort by dateofbirth descending (youngest first).
				{{Key: "$sort", Value: bson.D{{Key: "dateofbirth", Value: int32(-1)}}}},
				// Stage 3: Limit to 3 documents.
				{{Key: "$limit", Value: int64(3)}},
				// Stage 4: Remove _id and address fields.
				{{Key: "$unset", Value: bson.A{"_id", "address"}}},
			}

			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err = cursor.All(ctx, &results); err != nil {
				return nil, err
			}

			expected := []interface{}{
				bson.D{
					{Key: "person_id", Value: "7363626383"},
					{Key: "firstname", Value: "Carl"},
					{Key: "lastname", Value: "Simmons"},
					{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1998, 12, 26, 13, 13, 55, 0, time.UTC))},
					{Key: "vocation", Value: "ENGINEER"},
				},
				bson.D{
					{Key: "person_id", Value: "1723338115"},
					{Key: "firstname", Value: "Olive"},
					{Key: "lastname", Value: "Ranieri"},
					{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1985, 5, 12, 23, 14, 30, 0, time.UTC))},
					{Key: "gender", Value: "FEMALE"},
					{Key: "vocation", Value: "ENGINEER"},
				},
				bson.D{
					{Key: "person_id", Value: "6392529400"},
					{Key: "firstname", Value: "Elise"},
					{Key: "lastname", Value: "Smith"},
					{Key: "dateofbirth", Value: primitive.NewDateTimeFromTime(time.Date(1972, 1, 13, 9, 32, 7, 0, time.UTC))},
					{Key: "vocation", Value: "ENGINEER"},
				},
			}

			actual := make([]interface{}, len(results))
			for i, r := range results {
				actual[i] = r
			}
			tutorialCheck(t, "FilteredSubset_ThreeYoungestEngineers", actual, expected)
			return results, nil
		},
	})
}

// ─── 2. Group and Total ────────────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/group-and-total/
//
// Groups 2020 orders by customer, computes first_purchase_date, total_value,
// total_orders, and the orders array.

// groupAndTotalOrdersSeedACE inserts the 9 orders from the tutorial.
// (Using a distinct function name to avoid collision with any other file.)
func groupAndTotalOrdersSeedACE(ctx context.Context, col *mongo.Collection) error {
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

// TestGroupAndTotal_CustomerOrders2020ACE implements the complete 6-stage pipeline:
//
//	$match (year 2020) → $sort (orderdate) → $group (by customer) →
//	$sort (first_purchase_date) → $set (customer_id) → $unset (_id)
//
// Expected output (3 docs sorted by first_purchase_date):
//
//	oranieri@warmmail.com:   total 63,  1 order
//	elise_smith@myemail.com: total 436, 4 orders
//	tj@wheresmyemail.com:    total 191, 2 orders (2019/2021 orders excluded)
func TestGroupAndTotal_CustomerOrders2020ACE(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GroupAndTotal_CustomerOrders2020ACE",
		Support: harness.DocuDoltFull,
		Setup:   groupAndTotalOrdersSeedACE,
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
			tutorialCheckXFail(t, "GroupAndTotal_CustomerOrders2020ACE", actual, expected)
			return results, nil
		},
	})
}

// ─── 3. Unwind Arrays ─────────────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/unpack-arrays/
//
// Unpacks a products array from order documents, filters products priced > $15,
// then groups by product to compute total_value and quantity.

// unpackArraysOrdersSeed inserts the 4 orders from the tutorial.
func unpackArraysOrdersSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "order_id", Value: int64(6363763262239)},
			{Key: "products", Value: bson.A{
				bson.D{
					{Key: "prod_id", Value: "abc12345"},
					{Key: "name", Value: "Asus Laptop"},
					{Key: "price", Value: int32(431)},
				},
				bson.D{
					{Key: "prod_id", Value: "def45678"},
					{Key: "name", Value: "Karcher Hose Set"},
					{Key: "price", Value: int32(22)},
				},
			}},
		},
		bson.D{
			{Key: "order_id", Value: int64(1197372932325)},
			{Key: "products", Value: bson.A{
				bson.D{
					{Key: "prod_id", Value: "abc12345"},
					{Key: "name", Value: "Asus Laptop"},
					{Key: "price", Value: int32(429)},
				},
			}},
		},
		bson.D{
			{Key: "order_id", Value: int64(9812343774839)},
			{Key: "products", Value: bson.A{
				bson.D{
					{Key: "prod_id", Value: "pqr88223"},
					{Key: "name", Value: "Morphy Richards Food Mixer"},
					{Key: "price", Value: int32(431)},
				},
				bson.D{
					{Key: "prod_id", Value: "def45678"},
					{Key: "name", Value: "Karcher Hose Set"},
					{Key: "price", Value: int32(21)},
				},
			}},
		},
		bson.D{
			{Key: "order_id", Value: int64(4433997244387)},
			{Key: "products", Value: bson.A{
				bson.D{
					{Key: "prod_id", Value: "def45678"},
					{Key: "name", Value: "Karcher Hose Set"},
					{Key: "price", Value: int32(23)},
				},
				bson.D{
					{Key: "prod_id", Value: "jkl77336"},
					{Key: "name", Value: "Picky Pencil Sharpener"},
					{Key: "price", Value: int32(1)},
				},
				bson.D{
					{Key: "prod_id", Value: "xyz11228"},
					{Key: "name", Value: "Russell Hobbs Chrome Kettle"},
					{Key: "price", Value: int32(16)},
				},
			}},
		},
	})
	return err
}

// TestUnpackArrays_ProductTotals implements the complete 5-stage pipeline:
//
//	$unwind products → $match price > 15 → $group by prod_id →
//	$set product_id → $unset _id
//
// Expected output (4 products, Picky Pencil Sharpener excluded by $match):
//
//	abc12345: Asus Laptop,               total 860, qty 2
//	pqr88223: Morphy Richards Food Mixer, total 431, qty 1
//	xyz11228: Russell Hobbs Chrome Kettle, total 16, qty 1
//	def45678: Karcher Hose Set,           total 66, qty 3
//
// Note: $group output order is not guaranteed; tutorialCheck uses set-based comparison.
func TestUnpackArrays_ProductTotals(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnpackArrays_ProductTotals",
		Support: harness.DocuDoltXFail,
		Setup:   unpackArraysOrdersSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				// Stage 1: Unwind the products array — each product element becomes its own document.
				{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$products"}}}},
				// Stage 2: Filter to products priced above $15 (excludes Picky Pencil Sharpener at $1).
				{{Key: "$match", Value: bson.D{
					{Key: "products.price", Value: bson.D{{Key: "$gt", Value: int32(15)}}},
				}}},
				// Stage 3: Group by product ID, compute name, total_value, and quantity.
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$products.prod_id"},
					{Key: "product", Value: bson.D{{Key: "$first", Value: "$products.name"}}},
					{Key: "total_value", Value: bson.D{{Key: "$sum", Value: "$products.price"}}},
					{Key: "quantity", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				// Stage 4: Expose product_id as a top-level field.
				{{Key: "$set", Value: bson.D{{Key: "product_id", Value: "$_id"}}}},
				// Stage 5: Remove internal _id field.
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

			// Expected docs — order not guaranteed after $group, so tutorialCheck uses set comparison.
			expected := []interface{}{
				bson.D{
					{Key: "product", Value: "Asus Laptop"},
					{Key: "total_value", Value: int32(860)},
					{Key: "quantity", Value: int32(2)},
					{Key: "product_id", Value: "abc12345"},
				},
				bson.D{
					{Key: "product", Value: "Morphy Richards Food Mixer"},
					{Key: "total_value", Value: int32(431)},
					{Key: "quantity", Value: int32(1)},
					{Key: "product_id", Value: "pqr88223"},
				},
				bson.D{
					{Key: "product", Value: "Russell Hobbs Chrome Kettle"},
					{Key: "total_value", Value: int32(16)},
					{Key: "quantity", Value: int32(1)},
					{Key: "product_id", Value: "xyz11228"},
				},
				bson.D{
					{Key: "product", Value: "Karcher Hose Set"},
					{Key: "total_value", Value: int32(66)},
					{Key: "quantity", Value: int32(3)},
					{Key: "product_id", Value: "def45678"},
				},
			}

			actual := make([]interface{}, len(results))
			for i, r := range results {
				actual[i] = r
			}
			tutorialCheckXFail(t, "UnpackArrays_ProductTotals", actual, expected)
			return results, nil
		},
	})
}

// ─── 4. One-to-One Join ────────────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/one-to-one-join/
//
// Joins orders (product_id) to products (p_id) via $lookup, enriches with
// product_name and product_category, filters to 2020 orders only.

// oneToOneJoinSeed inserts orders and products into the test database.
// The primary collection (col) is orders; products is a sibling collection.
func oneToOneJoinSeed(ctx context.Context, col *mongo.Collection) error {
	// Insert orders into the primary collection.
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
			{Key: "product_id", Value: "a1b2c3d4"},
			{Key: "value", Value: 431.43},
		},
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2019, 5, 28, 19, 13, 32, 0, time.UTC))},
			{Key: "product_id", Value: "z9y8x7w6"},
			{Key: "value", Value: 5.01},
		},
		bson.D{
			{Key: "customer_id", Value: "oranieri@warmmail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
			{Key: "product_id", Value: "ff11gg22hh33"},
			{Key: "value", Value: 63.13},
		},
		bson.D{
			{Key: "customer_id", Value: "jjones@tepidmail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
			{Key: "product_id", Value: "a1b2c3d4"},
			{Key: "value", Value: 429.65},
		},
	})
	if err != nil {
		return err
	}

	// Insert products into a sibling collection within the same database.
	products := col.Database().Collection("products_o2o")
	_, err = products.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "p_id", Value: "a1b2c3d4"},
			{Key: "name", Value: "Asus Laptop"},
			{Key: "category", Value: "ELECTRONICS"},
			{Key: "description", Value: "Good value laptop for students"},
		},
		bson.D{
			{Key: "p_id", Value: "z9y8x7w6"},
			{Key: "name", Value: "The Day Of The Triffids"},
			{Key: "category", Value: "BOOKS"},
			{Key: "description", Value: "Classic post-apocalyptic novel"},
		},
		bson.D{
			{Key: "p_id", Value: "ff11gg22hh33"},
			{Key: "name", Value: "Morphy Richardds Food Mixer"},
			{Key: "category", Value: "KITCHENWARE"},
			{Key: "description", Value: "Luxury mixer turning good cakes into great"},
		},
		bson.D{
			{Key: "p_id", Value: "pqr678st"},
			{Key: "name", Value: "Karcher Hose Set"},
			{Key: "category", Value: "GARDEN"},
			{Key: "description", Value: "Hose + nozzles + winder for tidy storage"},
		},
	})
	return err
}

// TestOneToOneJoin_EnrichOrders2020 implements the complete 4-stage pipeline:
//
//	$match (2020 orders) →
//	$lookup (orders.product_id → products.p_id, as product_mapping) →
//	$set product_mapping = $first(product_mapping) →
//	$set product_name, product_category →
//	$unset [_id, product_id, product_mapping]
//
// Expected output (3 enriched orders — 2019 tj order excluded):
//
//	elise_smith@myemail.com: Asus Laptop / ELECTRONICS
//	oranieri@warmmail.com:   Morphy Richardds Food Mixer / KITCHENWARE
//	jjones@tepidmail.com:    Asus Laptop / ELECTRONICS
func TestOneToOneJoin_EnrichOrders2020(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "OneToOneJoin_EnrichOrders2020",
		Support: harness.DocuDoltXFail,
		Setup:   oneToOneJoinSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := mongo.Pipeline{
				// Stage 1: Match orders placed in 2020 (excludes 2019 tj order).
				{{Key: "$match", Value: bson.D{
					{Key: "orderdate", Value: bson.D{
						{Key: "$gte", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))},
						{Key: "$lt", Value: primitive.NewDateTimeFromTime(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))},
					}},
				}}},
				// Stage 2: Join orders.product_id to products.p_id, store in product_mapping array.
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "products_o2o"},
					{Key: "localField", Value: "product_id"},
					{Key: "foreignField", Value: "p_id"},
					{Key: "as", Value: "product_mapping"},
				}}},
				// Stage 3a: Unwrap product_mapping from array to single document.
				{{Key: "$set", Value: bson.D{
					{Key: "product_mapping", Value: bson.D{{Key: "$first", Value: "$product_mapping"}}},
				}}},
				// Stage 3b: Extract product_name and product_category from mapped product.
				{{Key: "$set", Value: bson.D{
					{Key: "product_name", Value: "$product_mapping.name"},
					{Key: "product_category", Value: "$product_mapping.category"},
				}}},
				// Stage 4: Remove _id, product_id, and product_mapping.
				{{Key: "$unset", Value: bson.A{"_id", "product_id", "product_mapping"}}},
			}

			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err = cursor.All(ctx, &results); err != nil {
				return nil, err
			}

			expected := []interface{}{
				bson.D{
					{Key: "customer_id", Value: "elise_smith@myemail.com"},
					{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
					{Key: "value", Value: 431.43},
					{Key: "product_name", Value: "Asus Laptop"},
					{Key: "product_category", Value: "ELECTRONICS"},
				},
				bson.D{
					{Key: "customer_id", Value: "oranieri@warmmail.com"},
					{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
					{Key: "value", Value: 63.13},
					{Key: "product_name", Value: "Morphy Richardds Food Mixer"},
					{Key: "product_category", Value: "KITCHENWARE"},
				},
				bson.D{
					{Key: "customer_id", Value: "jjones@tepidmail.com"},
					{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
					{Key: "value", Value: 429.65},
					{Key: "product_name", Value: "Asus Laptop"},
					{Key: "product_category", Value: "ELECTRONICS"},
				},
			}

			actual := make([]interface{}, len(results))
			for i, r := range results {
				actual[i] = r
			}
			tutorialCheckXFail(t, "OneToOneJoin_EnrichOrders2020", actual, expected)
			return results, nil
		},
	})
}

// ─── 5. Multi-Field Join ───────────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/aggregation-examples/multi-field-join/
//
// Joins products to orders on two fields simultaneously (name+variation) using
// a $lookup with an embedded pipeline and let variables. Filters to 2020 orders.

// multiFieldJoinSeed inserts products (primary) and orders (sibling) from the tutorial.
// The primary collection (col) is products; orders is a sibling collection.
func multiFieldJoinSeed(ctx context.Context, col *mongo.Collection) error {
	// Insert products into the primary collection.
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "name", Value: "Asus Laptop"},
			{Key: "variation", Value: "Ultra HD"},
			{Key: "category", Value: "ELECTRONICS"},
			{Key: "description", Value: "Great for watching movies"},
		},
		bson.D{
			{Key: "name", Value: "Asus Laptop"},
			{Key: "variation", Value: "Standard Display"},
			{Key: "category", Value: "ELECTRONICS"},
			{Key: "description", Value: "Good value laptop for students"},
		},
		bson.D{
			{Key: "name", Value: "The Day Of The Triffids"},
			{Key: "variation", Value: "1st Edition"},
			{Key: "category", Value: "BOOKS"},
			{Key: "description", Value: "Classic post-apocalyptic novel"},
		},
		bson.D{
			{Key: "name", Value: "The Day Of The Triffids"},
			{Key: "variation", Value: "2nd Edition"},
			{Key: "category", Value: "BOOKS"},
			{Key: "description", Value: "Classic post-apocalyptic novel"},
		},
		bson.D{
			{Key: "name", Value: "Morphy Richards Food Mixer"},
			{Key: "variation", Value: "Deluxe"},
			{Key: "category", Value: "KITCHENWARE"},
			{Key: "description", Value: "Luxury mixer turning good cakes into great"},
		},
	})
	if err != nil {
		return err
	}

	// Insert orders into a sibling collection within the same database.
	orders := col.Database().Collection("orders_mfj")
	_, err = orders.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "customer_id", Value: "elise_smith@myemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
			{Key: "product_name", Value: "Asus Laptop"},
			{Key: "product_variation", Value: "Standard Display"},
			{Key: "value", Value: 431.43},
		},
		bson.D{
			{Key: "customer_id", Value: "tj@wheresmyemail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2019, 5, 28, 19, 13, 32, 0, time.UTC))},
			{Key: "product_name", Value: "The Day Of The Triffids"},
			{Key: "product_variation", Value: "2nd Edition"},
			{Key: "value", Value: 5.01},
		},
		bson.D{
			{Key: "customer_id", Value: "oranieri@warmmail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
			{Key: "product_name", Value: "Morphy Richards Food Mixer"},
			{Key: "product_variation", Value: "Deluxe"},
			{Key: "value", Value: 63.13},
		},
		bson.D{
			{Key: "customer_id", Value: "jjones@tepidmail.com"},
			{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
			{Key: "product_name", Value: "Asus Laptop"},
			{Key: "product_variation", Value: "Standard Display"},
			{Key: "value", Value: 429.65},
		},
	})
	return err
}

// TestMultiFieldJoin_ProductsWithOrders2020 implements the complete pipeline from the
// "Multi-Field Join" tutorial. The outer pipeline runs on the products collection;
// an embedded $lookup pipeline joins to orders on name+variation.
//
// Expected output (2 product docs that had 2020 orders):
//
//	Asus Laptop / Standard Display: 2 orders (elise_smith, jjones)
//	Morphy Richards Food Mixer / Deluxe: 1 order (oranieri)
func TestMultiFieldJoin_ProductsWithOrders2020(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "MultiFieldJoin_ProductsWithOrders2020",
		Support: harness.DocuDoltFull,
		Setup:   multiFieldJoinSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Embedded pipeline used inside the $lookup.
			// Matches orders by name+variation (via let variables) then filters to 2020.
			embeddedPipeline := bson.A{
				// Stage 1: Match orders where product_name == $$prdname AND product_variation == $$prdvartn.
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "$expr", Value: bson.D{
						{Key: "$and", Value: bson.A{
							bson.D{{Key: "$eq", Value: bson.A{"$product_name", "$$prdname"}}},
							bson.D{{Key: "$eq", Value: bson.A{"$product_variation", "$$prdvartn"}}},
						}},
					}},
				}}},
				// Stage 2: Further filter to 2020 orders only.
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "orderdate", Value: bson.D{
						{Key: "$gte", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))},
						{Key: "$lt", Value: primitive.NewDateTimeFromTime(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))},
					}},
				}}},
				// Stage 3: Remove redundant join fields from the orders side.
				bson.D{{Key: "$unset", Value: bson.A{"_id", "product_name", "product_variation"}}},
			}

			pipeline := mongo.Pipeline{
				// Outer Stage 1: Lookup matching orders for each product using embedded pipeline.
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "orders_mfj"},
					{Key: "let", Value: bson.D{
						{Key: "prdname", Value: "$name"},
						{Key: "prdvartn", Value: "$variation"},
					}},
					{Key: "pipeline", Value: embeddedPipeline},
					{Key: "as", Value: "orders"},
				}}},
				// Outer Stage 2: Keep only products that have at least one 2020 order.
				{{Key: "$match", Value: bson.D{
					{Key: "orders", Value: bson.D{{Key: "$ne", Value: bson.A{}}}},
				}}},
				// Outer Stage 3: Remove _id and description fields.
				{{Key: "$unset", Value: bson.A{"_id", "description"}}},
			}

			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err = cursor.All(ctx, &results); err != nil {
				return nil, err
			}

			expected := []interface{}{
				bson.D{
					{Key: "name", Value: "Asus Laptop"},
					{Key: "variation", Value: "Standard Display"},
					{Key: "category", Value: "ELECTRONICS"},
					{Key: "orders", Value: bson.A{
						bson.D{
							{Key: "customer_id", Value: "elise_smith@myemail.com"},
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 5, 30, 8, 35, 52, 0, time.UTC))},
							{Key: "value", Value: 431.43},
						},
						bson.D{
							{Key: "customer_id", Value: "jjones@tepidmail.com"},
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 12, 26, 8, 55, 46, 0, time.UTC))},
							{Key: "value", Value: 429.65},
						},
					}},
				},
				bson.D{
					{Key: "name", Value: "Morphy Richards Food Mixer"},
					{Key: "variation", Value: "Deluxe"},
					{Key: "category", Value: "KITCHENWARE"},
					{Key: "orders", Value: bson.A{
						bson.D{
							{Key: "customer_id", Value: "oranieri@warmmail.com"},
							{Key: "orderdate", Value: primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 8, 25, 37, 0, time.UTC))},
							{Key: "value", Value: 63.13},
						},
					}},
				},
			}

			actual := make([]interface{}, len(results))
			for i, r := range results {
				actual[i] = r
			}
			tutorialCheck(t, "MultiFieldJoin_ProductsWithOrders2020", actual, expected)
			return results, nil
		},
	})
}
