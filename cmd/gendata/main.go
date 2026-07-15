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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	firstNames = []string{"Ava", "Liam", "Mia", "Noah", "Zoe", "Kai", "Ivy", "Leo", "Ada", "Max", "Nora", "Eli"}
	lastNames  = []string{"Stone", "Reyes", "Chen", "Okafor", "Meyer", "Patel", "Novak", "Ali", "Frost", "Lund"}
	categories = []string{"electronics", "home", "garden", "toys", "apparel", "grocery", "sports", "books"}
	adjectives = []string{"premium", "compact", "wireless", "organic", "stainless", "ergonomic", "recycled",
		"heavy-duty", "portable", "artisan", "limited", "refurbished", "eco", "deluxe"}
	streets   = []string{"Main St", "Oak Ave", "Pine Rd", "Elm Blvd", "Cedar Ln", "Maple Way"}
	tiers     = []string{"bronze", "silver", "gold", "platinum"}
	statuses  = []string{"pending", "paid", "shipped", "delivered", "cancelled", "refunded"}
	payMethod = []string{"card", "paypal", "applepay"}
	cities    = [][3]string{
		{"Seattle", "WA", "98101"}, {"Austin", "TX", "73301"}, {"Denver", "CO", "80201"},
		{"Miami", "FL", "33101"}, {"Boise", "ID", "83701"},
	}
)

func main() {
	uri := flag.String("uri", "mongodb://127.0.0.1:27017", "MongoDB connection URI of the server to populate")
	dbName := flag.String("db", "shop", "database to populate")
	nCustomers := flag.Int("customers", 3000, "number of customer documents")
	nProducts := flag.Int("products", 800, "number of product documents")
	nOrders := flag.Int("orders", 10000, "number of order documents")
	batch := flag.Int("batch", 1000, "insert batch size")
	seed := flag.Int64("seed", 1, "PRNG seed for reproducible data")
	drop := flag.Bool("drop", true, "drop the target collections before generating")
	flag.Parse()

	r := rand.New(rand.NewSource(*seed))
	ctx := context.Background()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*uri))
	if err != nil {
		log.Fatalf("connect %s: %v", *uri, err)
	}
	defer client.Disconnect(ctx)
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("ping %s: %v", *uri, err)
	}

	db := client.Database(*dbName)
	customers := db.Collection("customers")
	products := db.Collection("products")
	orders := db.Collection("orders")

	if *drop {
		for _, c := range []*mongo.Collection{customers, products, orders} {
			if err := c.Drop(ctx); err != nil {
				log.Fatalf("drop %s: %v", c.Name(), err)
			}
		}
	}

	customerIDs := genCustomers(ctx, r, customers, *nCustomers, *batch)
	skus := genProducts(ctx, r, products, *nProducts, *batch)
	genOrders(ctx, r, orders, *nOrders, *batch, customerIDs, skus)
	createIndexes(ctx, customers, products, orders)

	for _, c := range []*mongo.Collection{customers, products, orders} {
		report(ctx, db, c.Name())
	}
}

func genCustomers(ctx context.Context, r *rand.Rand, coll *mongo.Collection, n, batch int) []primitive.Binary {
	ids := make([]primitive.Binary, 0, n)
	docs := make([]any, 0, batch)
	flush := func() {
		if len(docs) == 0 {
			return
		}
		if _, err := coll.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			log.Fatalf("insert customers: %v", err)
		}
		docs = docs[:0]
	}
	for i := 0; i < n; i++ {
		id := uuidV4(r)
		ids = append(ids, id)
		city := cities[r.Intn(len(cities))]
		docs = append(docs, bson.D{
			{Key: "_id", Value: id},
			{Key: "name", Value: pick(r, firstNames) + " " + pick(r, lastNames)},
			{Key: "email", Value: fmt.Sprintf("user%d@example.com", i)},
			{Key: "tier", Value: pick(r, tiers)},
			{Key: "address", Value: address(r, city)},
			{Key: "loyaltyPoints", Value: int32(ri(r, 0, 50000))},
			{Key: "createdAt", Value: daysAgo(r, 1200)},
			{Key: "active", Value: r.Float64() > 0.1},
		})
		if len(docs) == batch {
			flush()
		}
	}
	flush()
	return ids
}

func genProducts(ctx context.Context, r *rand.Rand, coll *mongo.Collection, n, batch int) []string {
	skus := make([]string, 0, n)
	docs := make([]any, 0, batch)
	flush := func() {
		if len(docs) == 0 {
			return
		}
		if _, err := coll.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			log.Fatalf("insert products: %v", err)
		}
		docs = docs[:0]
	}
	for i := 0; i < n; i++ {
		sku := fmt.Sprintf("SKU-%05d", i)
		skus = append(skus, sku)
		docs = append(docs, bson.D{
			{Key: "_id", Value: primitive.NewObjectID()},
			{Key: "sku", Value: sku},
			{Key: "name", Value: sentence(r, 2) + " item"},
			{Key: "description", Value: sentence(r, ri(r, 20, 40))},
			{Key: "category", Value: pick(r, categories)},
			{Key: "price", Value: money(r, 3, 900)},
			{Key: "tags", Value: tags(r)},
			{Key: "inStock", Value: int32(ri(r, 0, 500))},
			{Key: "rating", Value: decimal(fmt.Sprintf("%.1f", r.Float64()*5))},
		})
		if len(docs) == batch {
			flush()
		}
	}
	flush()
	return skus
}

func genOrders(ctx context.Context, r *rand.Rand, coll *mongo.Collection, n, batch int, customerIDs []primitive.Binary, skus []string) {
	docs := make([]any, 0, batch)
	flush := func() {
		if len(docs) == 0 {
			return
		}
		if _, err := coll.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			log.Fatalf("insert orders: %v", err)
		}
		docs = docs[:0]
	}
	for i := 0; i < n; i++ {
		nItems := ri(r, 1, 4)
		items := make(bson.A, 0, nItems)
		subtotal := 0.0
		for j := 0; j < nItems; j++ {
			qty := ri(r, 1, 6)
			unit := 3 + r.Float64()*300
			subtotal += float64(qty) * unit
			items = append(items, bson.D{
				{Key: "sku", Value: skus[r.Intn(len(skus))]},
				{Key: "name", Value: sentence(r, 2)},
				{Key: "qty", Value: int32(qty)},
				{Key: "unitPrice", Value: decimal(fmt.Sprintf("%.2f", unit))},
				{Key: "lineTotal", Value: decimal(fmt.Sprintf("%.2f", float64(qty)*unit))},
			})
		}
		tax := subtotal * 0.08
		shipping := pickFloat(r, []float64{0, 4.99, 9.99})
		city := cities[r.Intn(len(cities))]
		docs = append(docs, bson.D{
			{Key: "_id", Value: primitive.NewObjectID()},
			{Key: "orderNumber", Value: fmt.Sprintf("ORD-%07d", i)},
			{Key: "orderDate", Value: daysAgo(r, 365)},
			{Key: "customerId", Value: customerIDs[r.Intn(len(customerIDs))]},
			{Key: "status", Value: pick(r, statuses)},
			{Key: "items", Value: items},
			{Key: "subtotal", Value: decimal(fmt.Sprintf("%.2f", subtotal))},
			{Key: "tax", Value: decimal(fmt.Sprintf("%.2f", tax))},
			{Key: "shipping", Value: decimal(fmt.Sprintf("%.2f", shipping))},
			{Key: "total", Value: decimal(fmt.Sprintf("%.2f", subtotal+tax+shipping))},
			{Key: "shippingAddress", Value: address(r, city)},
			{Key: "payment", Value: bson.D{{Key: "method", Value: pick(r, payMethod)}, {Key: "last4", Value: fmt.Sprintf("%04d", ri(r, 0, 9999))}}},
			{Key: "notes", Value: sentence(r, ri(r, 15, 30))},
		})
		if len(docs) == batch {
			flush()
		}
	}
	flush()
}

func createIndexes(ctx context.Context, customers, products, orders *mongo.Collection) {
	mk := func(coll *mongo.Collection, keys bson.D, unique bool) {
		model := mongo.IndexModel{Keys: keys}
		if unique {
			model.Options = options.Index().SetUnique(true)
		}
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Fatalf("index on %s %v: %v", coll.Name(), keys, err)
		}
	}
	mk(customers, bson.D{{Key: "email", Value: 1}}, true)
	mk(customers, bson.D{{Key: "tier", Value: 1}}, false)
	mk(products, bson.D{{Key: "sku", Value: 1}}, true)
	mk(products, bson.D{{Key: "category", Value: 1}}, false)
	mk(products, bson.D{{Key: "tags", Value: 1}}, false)
	mk(orders, bson.D{{Key: "customerId", Value: 1}}, false)
	mk(orders, bson.D{{Key: "orderDate", Value: -1}}, false)
	mk(orders, bson.D{{Key: "status", Value: 1}, {Key: "orderDate", Value: -1}}, false)
}

func report(ctx context.Context, db *mongo.Database, coll string) {
	var stats bson.M
	err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: coll}}).Decode(&stats)
	if err != nil {
		log.Printf("%s: (collStats unavailable: %v)", coll, err)
		return
	}
	count, _ := toFloat(stats["count"])
	size, _ := toFloat(stats["size"])
	log.Printf("%s: %d docs, %.1f MB", coll, int(count), size/(1024*1024))
}

func address(r *rand.Rand, city [3]string) bson.D {
	return bson.D{
		{Key: "street", Value: fmt.Sprintf("%d %s", ri(r, 1, 9999), pick(r, streets))},
		{Key: "city", Value: city[0]},
		{Key: "state", Value: city[1]},
		{Key: "zip", Value: city[2]},
		{Key: "country", Value: "US"},
	}
}

func tags(r *rand.Rand) bson.A {
	n := ri(r, 2, 5)
	out := make(bson.A, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, pick(r, adjectives))
	}
	return out
}

func sentence(r *rand.Rand, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += " "
		}
		out += pick(r, adjectives)
	}
	return out
}

func uuidV4(r *rand.Rand) primitive.Binary {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return primitive.Binary{Subtype: 0x04, Data: b}
}

func money(r *rand.Rand, lo, hi float64) primitive.Decimal128 {
	return decimal(fmt.Sprintf("%.2f", lo+r.Float64()*(hi-lo)))
}

func decimal(s string) primitive.Decimal128 {
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		log.Fatalf("decimal %q: %v", s, err)
	}
	return d
}

func pick(r *rand.Rand, a []string) string { return a[r.Intn(len(a))] }

func pickFloat(r *rand.Rand, a []float64) float64 { return a[r.Intn(len(a))] }

func ri(r *rand.Rand, lo, hi int) int { return lo + r.Intn(hi-lo+1) }

func daysAgo(r *rand.Rand, maxDays int) time.Time {
	return time.Now().Add(-time.Duration(r.Intn(maxDays+1)) * 24 * time.Hour).UTC()
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
