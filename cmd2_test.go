package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoErrorCodes(t *testing.T) {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)
	
	// Test autoCompact against non-admin db (should return Unauthorized)
	testdb := client.Database("test_codes_xyz")
	var r1 bson.D
	e1 := testdb.RunCommand(ctx, bson.D{{Key: "autoCompact", Value: true}}).Decode(&r1)
	var cmdErr1 mongo.CommandError
	if errors.As(e1, &cmdErr1) {
		fmt.Printf("autoCompact non-admin: code=%d, msg=%s\n", cmdErr1.Code, cmdErr1.Message)
	}
	
	// Test collMod unknown option error code
	testdb2 := client.Database("test_codes_xyz2")
	testdb2.Collection("col").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
	var r2 bson.D
	e2 := testdb2.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: "col"},
		{Key: "unknownOption", Value: true},
	}).Decode(&r2)
	var cmdErr2 mongo.CommandError
	if errors.As(e2, &cmdErr2) {
		fmt.Printf("collMod unknownOption: code=%d, msg=%s\n", cmdErr2.Code, cmdErr2.Message)
	}
	
	// Test convertToCapped non-existent
	var r3 bson.D
	e3 := testdb.RunCommand(ctx, bson.D{
		{Key: "convertToCapped", Value: "no_such_xyz"},
		{Key: "size", Value: int64(1024)},
	}).Decode(&r3)
	var cmdErr3 mongo.CommandError
	if errors.As(e3, &cmdErr3) {
		fmt.Printf("convertToCapped non-existent: code=%d, msg=%s\n", cmdErr3.Code, cmdErr3.Message)
	}
	
	// Test serverStatus with repl filter against non-admin db
	var r4 bson.D
	e4 := testdb.RunCommand(ctx, bson.D{
		{Key: "serverStatus", Value: 1},
		{Key: "repl", Value: 1},
		{Key: "metrics", Value: 0},
		{Key: "locks", Value: 0},
	}).Decode(&r4)
	if e4 != nil {
		fmt.Printf("serverStatus repl err: %v\n", e4)
	} else {
		keys := []string{}
		for _, el := range r4 {
			keys = append(keys, el.Key)
		}
		fmt.Printf("serverStatus repl keys: %v\n", keys)
	}
	
	// Test renameCollection non-existent source
	admDb := client.Database("admin")
	var r5 bson.D
	e5 := admDb.RunCommand(ctx, bson.D{
		{Key: "renameCollection", Value: "test_codes_xyz.no_such_col"},
		{Key: "to", Value: "test_codes_xyz.new_col"},
	}).Decode(&r5)
	var cmdErr5 mongo.CommandError
	if errors.As(e5, &cmdErr5) {
		fmt.Printf("renameCollection non-existent: code=%d, msg=%s\n", cmdErr5.Code, cmdErr5.Message)
	}
	
	// Cleanup
	client.Database("test_codes_xyz").Drop(ctx)
	client.Database("test_codes_xyz2").Drop(ctx)
}
