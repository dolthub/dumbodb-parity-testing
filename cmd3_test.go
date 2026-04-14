package main

import (
	"context"
	"fmt"
	"testing"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestServerStatusCompare(t *testing.T) {
	ctx := context.Background()
	
	mongoClient, _ := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	defer mongoClient.Disconnect(ctx)
	
	dumboDBClient, _ := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27026"))
	defer dumboDBClient.Disconnect(ctx)
	
	testdb1 := mongoClient.Database("test_srv")
	testdb2 := dumboDBClient.Database("test_srv")
	
	cmd := bson.D{
		{Key: "serverStatus", Value: int32(1)},
		{Key: "repl", Value: int32(1)},
		{Key: "metrics", Value: int32(0)},
		{Key: "locks", Value: int32(0)},
	}
	
	var mongo_res bson.D
	_ = testdb1.RunCommand(ctx, cmd).Decode(&mongo_res)
	fmt.Printf("MongoDB serverStatus repl keys: ")
	for _, e := range mongo_res {
		fmt.Printf("%s ", e.Key)
	}
	fmt.Println()
	
	var dumbodb_res bson.D
	_ = testdb2.RunCommand(ctx, cmd).Decode(&dumbodb_res)
	fmt.Printf("DumboDB serverStatus repl keys: ")
	for _, e := range dumbodb_res {
		fmt.Printf("%s ", e.Key)
	}
	fmt.Println()
	
	// Get repl field from mongo
	for _, e := range mongo_res {
		if e.Key == "repl" {
			fmt.Printf("MongoDB repl: %v\n", e.Value)
		}
		if e.Key == "metrics" {
			fmt.Printf("MongoDB has metrics: true\n")
		}
		if e.Key == "locks" {
			fmt.Printf("MongoDB has locks: true\n")
		}
	}
	for _, e := range dumbodb_res {
		if e.Key == "repl" {
			fmt.Printf("DumboDB repl: %v\n", e.Value)
		}
	}
	
	// Test compact empty vs non-existent
	dumboDBCol := dumboDBClient.Database("test_compact_x").Collection("col")
	var r1 bson.D
	e1 := dumboDBCol.Database().RunCommand(ctx, bson.D{{Key: "compact", Value: "col"}}).Decode(&r1)
	fmt.Printf("DumboDB compact col (never created): err=%v\n", e1)
	
	var r2 bson.D
	e2 := dumboDBCol.Database().RunCommand(ctx, bson.D{{Key: "compact", Value: "no_such_xyz"}}).Decode(&r2)
	fmt.Printf("DumboDB compact no_such_xyz: err=%v\n", e2)
	
	mongoClient.Database("test_srv").Drop(ctx)
	dumboDBClient.Database("test_srv").Drop(ctx)
	dumboDBClient.Database("test_compact_x").Drop(ctx)
}
