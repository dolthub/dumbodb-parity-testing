package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The CRUD benchmarks below intentionally target a single dataset size (1K small
// docs) for the initial cut. Scaling dimensions are added by parameterizing
// datasetSize / docSize once the baseline stabilizes — see
// bd pa-xp1 "Scaling dimensions" in the design.
const (
	benchDatasetSize = 1000
	benchDocSize     = sizeSmall
)

// withSeededCollection handles the connect + seed + cleanup lifecycle for
// benchmarks that operate on a pre-populated collection. The returned context
// is bound to the collection's lifetime.
func withSeededCollection(b *testing.B, label string, n int, size docSize) (*mongo.Collection, context.Context) {
	b.Helper()
	ctx := context.Background()
	client, err := connect(ctx)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	col, cleanup := freshCollection(ctx, client, label)
	b.Cleanup(func() {
		cleanup()
		_ = client.Disconnect(context.Background())
	})
	if err := seedCollection(ctx, col, n, size); err != nil {
		b.Fatalf("seed: %v", err)
	}
	return col, ctx
}

// withEmptyCollection is the same as withSeededCollection but skips seeding —
// used for Insert benchmarks where the measured op is the insertion itself.
func withEmptyCollection(b *testing.B, label string) (*mongo.Collection, context.Context) {
	b.Helper()
	ctx := context.Background()
	client, err := connect(ctx)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	col, cleanup := freshCollection(ctx, client, label)
	b.Cleanup(func() {
		cleanup()
		_ = client.Disconnect(context.Background())
	})
	return col, ctx
}

func BenchmarkInsertOne(b *testing.B) {
	col, ctx := withEmptyCollection(b, "insertone")
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := makeDoc(r, i, benchDocSize)
		// Use a fresh _id per iteration so inserts never collide.
		doc[0].Value = fmt.Sprintf("iter-%d", i)
		if _, err := col.InsertOne(ctx, doc); err != nil {
			b.Fatalf("InsertOne: %v", err)
		}
	}
}

func BenchmarkInsertMany_100(b *testing.B)  { benchmarkInsertMany(b, 100) }
func BenchmarkInsertMany_1000(b *testing.B) { benchmarkInsertMany(b, 1000) }

func benchmarkInsertMany(b *testing.B, batch int) {
	col, ctx := withEmptyCollection(b, fmt.Sprintf("insertmany%d", batch))
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		docs := make([]interface{}, batch)
		for j := 0; j < batch; j++ {
			d := makeDoc(r, i*batch+j, benchDocSize)
			d[0].Value = fmt.Sprintf("iter-%d-%d", i, j)
			docs[j] = d
		}
		b.StartTimer()
		if _, err := col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("InsertMany: %v", err)
		}
	}
}

func BenchmarkFindOne_ById(b *testing.B) {
	col, ctx := withSeededCollection(b, "findone", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

// seedWithIds seeds n docs using makeDoc, but replaces the generated integer
// _id with idFn(i). Used by the _id-type FindOne benchmarks.
func seedWithIds(ctx context.Context, col *mongo.Collection, n int, size docSize, idFn func(i int) interface{}) error {
	r := rand.New(rand.NewSource(*dataSeed))
	const batch = 500
	buf := make([]interface{}, 0, batch)
	for i := 0; i < n; i++ {
		d := makeDoc(r, i, size)
		d[0].Value = idFn(i)
		buf = append(buf, d)
		if len(buf) == batch || i == n-1 {
			if _, err := col.InsertMany(ctx, buf, options.InsertMany().SetOrdered(false)); err != nil {
				return fmt.Errorf("seed insert: %w", err)
			}
			buf = buf[:0]
		}
	}
	return nil
}

func BenchmarkFindOne_ById_String(b *testing.B) {
	col, ctx := withEmptyCollection(b, "findone_string")
	idFn := func(i int) interface{} { return fmt.Sprintf("doc-%d", i) }
	if err := seedWithIds(ctx, col, benchDatasetSize, benchDocSize, idFn); err != nil {
		b.Fatalf("seed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("doc-%d", i%benchDatasetSize)
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFindOne_ById_ObjectId(b *testing.B) {
	col, ctx := withEmptyCollection(b, "findone_oid")
	ids := make([]primitive.ObjectID, benchDatasetSize)
	for i := range ids {
		ids[i] = primitive.NewObjectID()
	}
	idFn := func(i int) interface{} { return ids[i] }
	if err := seedWithIds(ctx, col, benchDatasetSize, benchDocSize, idFn); err != nil {
		b.Fatalf("seed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%benchDatasetSize]
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFindOne_ById_Document(b *testing.B) {
	col, ctx := withEmptyCollection(b, "findone_doc")
	makeID := func(i int) bson.D {
		return bson.D{{Key: "a", Value: i}, {Key: "b", Value: "foo"}}
	}
	idFn := func(i int) interface{} { return makeID(i) }
	if err := seedWithIds(ctx, col, benchDatasetSize, benchDocSize, idFn); err != nil {
		b.Fatalf("seed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := makeID(i % benchDatasetSize)
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFind_Scan(b *testing.B) {
	col, ctx := withSeededCollection(b, "findscan", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkFind_FilterEq(b *testing.B) {
	col, ctx := withSeededCollection(b, "findeq", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// grp has 10 values; expect ~benchDatasetSize/10 results.
		cur, err := col.Find(ctx, bson.D{{Key: "grp", Value: i % 10}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkFind_FilterRange(b *testing.B) {
	col, ctx := withSeededCollection(b, "findrange", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "i", Value: bson.D{
			{Key: "$gte", Value: 100},
			{Key: "$lt", Value: 200},
		}}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkUpdateOne(b *testing.B) {
	col, ctx := withSeededCollection(b, "updateone", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateOne: %v", err)
		}
	}
}

func BenchmarkUpdateMany(b *testing.B) {
	col, ctx := withSeededCollection(b, "updatemany", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Touch one of 10 groups per iteration (~100 docs each).
		_, err := col.UpdateMany(ctx,
			bson.D{{Key: "grp", Value: i % 10}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateMany: %v", err)
		}
	}
}

func BenchmarkReplaceOne(b *testing.B) {
	col, ctx := withSeededCollection(b, "replaceone", benchDatasetSize, benchDocSize)
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		replacement := makeDoc(r, id, benchDocSize)
		_, err := col.ReplaceOne(ctx, bson.D{{Key: "_id", Value: id}}, replacement)
		if err != nil {
			b.Fatalf("ReplaceOne: %v", err)
		}
	}
}

// BenchmarkDeleteOne and BenchmarkDeleteMany reseed every N iterations so the
// collection never runs dry. The reseed happens outside the timed section.
func BenchmarkDeleteOne(b *testing.B) {
	col, ctx := withSeededCollection(b, "deleteone", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		if i > 0 && i%benchDatasetSize == 0 {
			b.StopTimer()
			_, _ = col.DeleteMany(ctx, bson.D{})
			if err := seedCollection(ctx, col, benchDatasetSize, benchDocSize); err != nil {
				b.Fatalf("reseed: %v", err)
			}
			b.StartTimer()
		}
		if _, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
			b.Fatalf("DeleteOne: %v", err)
		}
	}
}

func BenchmarkDeleteMany(b *testing.B) {
	col, ctx := withSeededCollection(b, "deletemany", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Delete one group (~100 docs), then reseed that group off the clock.
		grp := i % 10
		if _, err := col.DeleteMany(ctx, bson.D{{Key: "grp", Value: grp}}); err != nil {
			b.Fatalf("DeleteMany: %v", err)
		}
		b.StopTimer()
		r := rand.New(rand.NewSource(*dataSeed + int64(i)))
		var refill []interface{}
		for j := 0; j < benchDatasetSize/10; j++ {
			d := makeDoc(r, i*benchDatasetSize+j, benchDocSize)
			d[0].Value = fmt.Sprintf("refill-%d-%d", i, j)
			d[2].Value = grp
			refill = append(refill, d)
		}
		if _, err := col.InsertMany(ctx, refill, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("refill: %v", err)
		}
		b.StartTimer()
	}
}

func BenchmarkCountDocuments(b *testing.B) {
	col, ctx := withSeededCollection(b, "count", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.CountDocuments(ctx, bson.D{}); err != nil {
			b.Fatalf("CountDocuments: %v", err)
		}
	}
}

func BenchmarkDistinct(b *testing.B) {
	col, ctx := withSeededCollection(b, "distinct", benchDatasetSize, benchDocSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.Distinct(ctx, "grp", bson.D{}); err != nil {
			b.Fatalf("Distinct: %v", err)
		}
	}
}
