package main

import (
	"context"
	"fmt"
	"testing"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoCommands(t *testing.T) {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)
	
	db := client.Database("admin")
	
	var result bson.D
	err = db.RunCommand(ctx, bson.D{{Key: "autoCompact", Value: true}}).Decode(&result)
	fmt.Printf("autoCompact result: %v, err: %v\n", result, err)
	
	testdb := client.Database("test_server_status")
	var result2 bson.D
	err2 := testdb.RunCommand(ctx, bson.D{
		{Key: "serverStatus", Value: 1},
		{Key: "repl", Value: 1},
		{Key: "metrics", Value: 0},
		{Key: "locks", Value: 0},
	}).Decode(&result2)
	fmt.Printf("serverStatus keys: ")
	for _, e := range result2 {
		fmt.Printf("%s ", e.Key)
	}
	fmt.Printf("\nerr2: %v\n", err2)
	
	var r3 bson.D
	e3 := testdb.RunCommand(ctx, bson.D{
		{Key: "convertToCapped", Value: "no_such_xyz"},
		{Key: "size", Value: int64(1024)},
	}).Decode(&r3)
	fmt.Printf("convertToCapped non-existent err: %v\n", e3)
	
	var r4 bson.D
	e4 := testdb.RunCommand(ctx, bson.D{
		{Key: "convertToCapped", Value: "mytest"},
		{Key: "size", Value: int64(0)},
	}).Decode(&r4)
	fmt.Printf("convertToCapped zero size err: %v\n", e4)
	
	var r5 bson.D
	e5 := testdb.RunCommand(ctx, bson.D{
		{Key: "convertToCapped", Value: "mytest"},
	}).Decode(&r5)
	fmt.Printf("convertToCapped missing size err: %v\n", e5)
	
	col := client.Database("testdb_xyz").Collection("mycol")
	col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
	var r6 bson.D
	e6 := col.Database().RunCommand(ctx, bson.D{
		{Key: "collMod", Value: "mycol"},
		{Key: "unknownOption", Value: true},
	}).Decode(&r6)
	fmt.Printf("collMod unknown option err: %v\n", e6)
	
	client.Database("testdb_xyz").Drop(ctx)
	client.Database("test_server_status").Drop(ctx)
}
