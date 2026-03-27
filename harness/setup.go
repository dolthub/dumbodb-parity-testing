package harness

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	globalClients *Clients
	clientsOnce   sync.Once
	clientsErr    error
)

// Clients holds connections to MongoDB and Dongo.
type Clients struct {
	Mongo *mongo.Client
	Dongo *mongo.Client
}

func mongoURI() string {
	if v := os.Getenv("MONGO_URI"); v != "" {
		return v
	}
	return "mongodb://localhost:27017"
}

func dongoURI() string {
	if v := os.Getenv("DONGO_URI"); v != "" {
		return v
	}
	return "mongodb://localhost:27018"
}

// GetClients returns the shared Mongo+Dongo client pair, connecting on first call.
func GetClients(ctx context.Context) (*Clients, error) {
	clientsOnce.Do(func() {
		mc, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI()))
		if err != nil {
			clientsErr = fmt.Errorf("connect mongo: %w", err)
			return
		}
		if err := mc.Ping(ctx, nil); err != nil {
			clientsErr = fmt.Errorf("ping mongo: %w", err)
			return
		}

		dc, err := mongo.Connect(ctx, options.Client().ApplyURI(dongoURI()))
		if err != nil {
			_ = mc.Disconnect(ctx)
			clientsErr = fmt.Errorf("connect dongo: %w", err)
			return
		}
		if err := dc.Ping(ctx, nil); err != nil {
			clientsErr = fmt.Errorf("ping dongo: %w", err)
			return
		}

		globalClients = &Clients{Mongo: mc, Dongo: dc}
	})
	return globalClients, clientsErr
}

// TestDB creates a uniquely-named database for a single test on both servers.
// The returned cleanup function drops both databases; callers should defer it.
func (c *Clients) TestDB(ctx context.Context, testName string) (mongoCol, dongoCol *mongo.Collection, cleanup func(), err error) {
	dbName := fmt.Sprintf("parity_%s_%d", sanitizeName(testName), time.Now().UnixNano())
	const colName = "col"

	mongoCol = c.Mongo.Database(dbName).Collection(colName)
	dongoCol = c.Dongo.Database(dbName).Collection(colName)

	cleanup = func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Mongo.Database(dbName).Drop(dropCtx)
		_ = c.Dongo.Database(dbName).Drop(dropCtx)
	}
	return mongoCol, dongoCol, cleanup, nil
}

// sanitizeName converts a test name to a safe database name component.
// Budget: "parity_" (7) + name + "_" (1) + UnixNano (19) must be ≤ 63, so name ≤ 36.
func sanitizeName(s string) string {
	const maxLen = 36
	out := make([]byte, 0, maxLen)
	for i := 0; i < len(s) && len(out) < maxLen; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
