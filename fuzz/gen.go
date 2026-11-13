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

// Package fuzz runs randomly generated workloads against MongoDB and DumboDB
// and reports every command whose results differ. MongoDB is the oracle.
package fuzz

import (
	"math"
	"math/rand/v2"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Collection is the collection every generated command targets.
const Collection = "t"

// Mode selects what a case's steps do.
type Mode string

const (
	// Reads runs hinted find, find with limit, count, aggregate and distinct
	// against one dataset.
	Reads Mode = "reads"
	// Writes resets the dataset before each hinted single-document write and
	// reads the whole collection after it.
	Writes Mode = "writes"
)

// Case is one generated workload: a dataset, its indexes, and the steps run
// against it.
type Case struct {
	Seed    uint64
	Mode    Mode
	Indexes []bson.D
	Docs    []bson.D
	Steps   []Step
}

// Step is one command under test. Check, when set, reads the collection back
// after a write.
type Step struct {
	Kind    string
	Hint    string
	Filter  bson.D
	Command bson.D
	Check   bson.D
}

type generator struct {
	rng       *rand.Rand
	orWeight  float64
	scalars   []any
	indexes   []bson.D
	hintNames []string
}

// Generate builds the case for seed. The same seed, mode and step count
// always produce the same case.
func Generate(seed uint64, mode Mode, steps int) Case {
	g := newGenerator(seed)
	c := Case{Seed: seed, Mode: mode, Indexes: g.indexes, Docs: g.docs()}
	for i := 0; i < steps; i++ {
		filter, hint := g.filter(), g.pick(g.hintNames).(string)
		if mode == Reads {
			c.Steps = append(c.Steps, readSteps(filter, hint)...)
		} else {
			c.Steps = append(c.Steps, g.writeStep(filter, hint))
		}
	}
	return c
}

func newGenerator(seed uint64) *generator {
	g := &generator{
		rng:      rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)),
		orWeight: 0.13,
		scalars: []any{
			int32(0), int32(1), int32(2), int32(3), int32(-1), 2.5, int64(2), mustDecimal("2.0"),
			"", "a", "ab", "b", "B", "x", nil, true, false,
			primitive.MinKey{}, primitive.MaxKey{}, primitive.DateTime(1000),
			mustObjectID("5f0000000000000000000001"), math.NaN(),
		},
	}
	exists := bson.D{{Key: "$exists", Value: true}}
	g.indexes = []bson.D{
		index("a1", bson.D{{Key: "a", Value: 1}}),
		index("ad", bson.D{{Key: "a", Value: -1}}),
		index("ab", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}),
		index("bda", bson.D{{Key: "b", Value: -1}, {Key: "a", Value: 1}}),
		sparse(index("bs", bson.D{{Key: "b", Value: 1}})),
		partial(index("ap", bson.D{{Key: "a", Value: 1}, {Key: "c", Value: 1}}), bson.D{{Key: "c", Value: exists}}),
		partial(index("bap", bson.D{{Key: "b", Value: 1}, {Key: "a", Value: -1}}), bson.D{{Key: "c", Value: bson.D{{Key: "$gte", Value: 1}}}}),
		partial(index("cps", bson.D{{Key: "c", Value: -1}}), bson.D{{Key: "b", Value: bson.D{{Key: "$type", Value: "number"}}}}),
		index("ef", bson.D{{Key: "e.f", Value: 1}}),
		sparse(index("efb", bson.D{{Key: "e.f", Value: -1}, {Key: "b", Value: 1}})),
		sparse(index("cs", bson.D{{Key: "c", Value: 1}})),
		sparse(index("as", bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 1}})),
		partial(index("adbp", bson.D{{Key: "a", Value: -1}, {Key: "b", Value: 1}}), bson.D{{Key: "b", Value: exists}}),
		partial(index("efap", bson.D{{Key: "e.f", Value: 1}, {Key: "c", Value: 1}}), bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 0}}}}),
		sparse(index("efs", bson.D{{Key: "e.f", Value: 1}})),
		partial(index("efdp", bson.D{{Key: "e.f", Value: -1}, {Key: "c", Value: 1}}), bson.D{{Key: "c", Value: exists}}),
		sparse(index("asp", bson.D{{Key: "a", Value: 1}})),
		partial(index("adp", bson.D{{Key: "a", Value: -1}}), bson.D{{Key: "b", Value: exists}}),
	}
	for _, ix := range g.indexes {
		g.hintNames = append(g.hintNames, ix.Map()["name"].(string))
	}
	return g
}

func index(name string, key bson.D) bson.D {
	return bson.D{{Key: "key", Value: key}, {Key: "name", Value: name}}
}

func sparse(ix bson.D) bson.D { return append(ix, bson.E{Key: "sparse", Value: true}) }

func partial(ix bson.D, filter bson.D) bson.D {
	return append(ix, bson.E{Key: "partialFilterExpression", Value: filter})
}

func mustDecimal(s string) primitive.Decimal128 {
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		panic(err)
	}
	return d
}

func mustObjectID(s string) primitive.ObjectID {
	id, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		panic(err)
	}
	return id
}

func (g *generator) chance(p float64) bool { return g.rng.Float64() < p }

func (g *generator) between(lo, hi int) int { return lo + g.rng.IntN(hi-lo+1) }

func (g *generator) pick(from any) any {
	switch s := from.(type) {
	case []any:
		return s[g.rng.IntN(len(s))]
	case []string:
		return s[g.rng.IntN(len(s))]
	}
	panic("pick: unsupported slice type")
}

func (g *generator) scalar() any { return g.pick(g.scalars) }

// sample returns k distinct scalars.
func (g *generator) sample(k int) bson.A {
	out := bson.A{}
	for _, i := range g.rng.Perm(len(g.scalars))[:k] {
		out = append(out, g.scalars[i])
	}
	return out
}

// value is a scalar, or up to two levels of arrays, or an embedded document.
func (g *generator) value(allowArray bool, depth int) any {
	r := g.rng.Float64()
	if r < 0.6 || !allowArray || depth > 1 {
		return g.scalar()
	}
	if r < 0.85 {
		arr := bson.A{}
		for n := g.between(0, 3); n > 0; n-- {
			arr = append(arr, g.value(true, depth+1))
		}
		return arr
	}
	return bson.D{{Key: "x", Value: g.scalar()}}
}

func (g *generator) docs() []bson.D {
	var docs []bson.D
	for id := int32(1); id <= 40; id++ {
		d := bson.D{{Key: "_id", Value: id}}
		if g.chance(0.9) {
			d = append(d, bson.E{Key: "a", Value: g.value(true, 0)})
		}
		if g.chance(0.85) {
			d = append(d, bson.E{Key: "b", Value: g.value(false, 0)})
		}
		if g.chance(0.5) {
			d = append(d, bson.E{Key: "c", Value: g.pick([]any{int32(0), int32(1), int32(2), int32(3), "s", nil})})
		}
		switch {
		case g.chance(0.6):
			if g.chance(0.6) {
				d = append(d, bson.E{Key: "e", Value: bson.D{{Key: "f", Value: g.value(true, 0)}}})
			} else {
				arr := bson.A{}
				for n := g.between(0, 3); n > 0; n-- {
					arr = append(arr, g.scalar())
				}
				d = append(d, bson.E{Key: "e", Value: bson.D{{Key: "f", Value: arr}}})
			}
		case g.chance(0.2):
			d = append(d, bson.E{Key: "e", Value: bson.A{
				bson.D{{Key: "f", Value: g.scalar()}},
				bson.D{{Key: "f", Value: g.scalar()}},
			}})
		}
		docs = append(docs, d)
	}
	return docs
}

var (
	filterFields   = []string{"a", "a", "b", "c", "e.f", "e.f"}
	distinctFields = []string{"a", "b", "c", "e.f"}
	conditionOps   = []string{
		"nulls", "nulls", "rng2", "rng2", "rng3", "rng3", "rng3", "eq", "gt", "gte", "lt", "lte",
		"in", "inre", "type", "regex", "exists", "ne", "nin", "elem", "not", "null", "range", "rx",
	}
)

func op(name string, v any) bson.D { return bson.D{{Key: name, Value: v}} }

func (g *generator) regex(patterns ...string) primitive.Regex {
	return primitive.Regex{Pattern: g.pick(patterns).(string)}
}

// condition is the value a filter gives one field: a literal or an operator
// document.
func (g *generator) condition() any {
	name := g.pick(conditionOps).(string)
	v := g.scalar()
	switch name {
	case "eq":
		return v
	case "gt", "gte", "lt", "lte", "ne":
		return op("$"+name, v)
	case "range":
		return bson.D{
			{Key: "$gt", Value: g.pick([]any{int32(0), int32(1), "", "a"})},
			{Key: "$lt", Value: g.pick([]any{int32(3), "b", "x", primitive.MaxKey{}})},
		}
	case "in":
		return op("$in", g.sample(g.between(1, 4)))
	case "inre":
		in := bson.A{g.regex("^a", "^$", "b", "^")}
		return op("$in", append(in, g.sample(g.between(0, 2))...))
	case "nin":
		return op("$nin", g.sample(g.between(1, 3)))
	case "exists":
		return op("$exists", g.chance(0.5))
	case "type":
		return op("$type", g.pick([]any{
			"string", "number", "array", "null", "bool", "minKey", "maxKey", "double", "object",
			bson.A{"string", "number"}, bson.A{"array", "null"},
		}))
	case "regex":
		return bson.D{
			{Key: "$regex", Value: g.pick([]string{"^a", "^$", "b", "^", "a$"})},
			{Key: "$options", Value: g.pick([]string{"", "i"})},
		}
	case "rx":
		return g.regex("^a", "^$", "b")
	case "elem":
		return op("$elemMatch", op("$"+g.pick([]string{"gt", "lte", "eq", "ne"}).(string), v))
	case "not":
		return op("$not", g.pick([]any{
			op("$type", "string"), op("$gt", int32(1)), op("$regex", "^a"), op("$in", bson.A{int32(1), "a"}),
		}))
	case "null":
		return nil
	case "nulls":
		return g.nullCondition()
	case "rng3":
		lo := g.pick([]any{int32(0), int32(1), int32(-1), 2.5, "", "a", nil, primitive.MinKey{}})
		hi := g.pick([]any{int32(2), int32(3), int32(5), "b", "x", nil, primitive.MaxKey{}})
		return g.twoSided(lo, hi)
	case "rng2":
		return g.twoSided(g.scalar(), g.scalar())
	}
	panic("condition: unknown op " + name)
}

func (g *generator) twoSided(lo, hi any) bson.D {
	return bson.D{
		{Key: g.pick([]string{"$gt", "$gte"}).(string), Value: lo},
		{Key: g.pick([]string{"$lt", "$lte"}).(string), Value: hi},
	}
}

// nullCondition covers the null and missing-field forms that index bounds
// treat specially.
func (g *generator) nullCondition() bson.D {
	switch g.rng.IntN(13) {
	case 0:
		return op("$lte", nil)
	case 1:
		return op("$gte", nil)
	case 2:
		return op("$eq", nil)
	case 3:
		return op("$in", append(bson.A{nil}, g.sample(g.between(0, 2))...))
	case 4:
		return op("$ne", nil)
	case 5:
		return op("$nin", bson.A{nil, g.scalar()})
	case 6:
		return op("$not", op("$eq", nil))
	case 7:
		return op("$lt", nil)
	case 8:
		return op("$gt", nil)
	case 9:
		return op("$exists", false)
	case 10:
		return op("$not", op("$lte", nil))
	case 11:
		return bson.D{{Key: "$gte", Value: nil}, {Key: "$lte", Value: g.scalar()}}
	default:
		return bson.D{{Key: "$lte", Value: nil}, {Key: "$gte", Value: g.scalar()}}
	}
}

func (g *generator) field() string { return g.pick(filterFields).(string) }

// twoFields returns two distinct fields.
func (g *generator) twoFields() (string, string) {
	p := g.rng.Perm(len(distinctFields))
	return distinctFields[p[0]], distinctFields[p[1]]
}

func (g *generator) filter() bson.D {
	if g.chance(g.orWeight) {
		return g.orFilter()
	}
	r := g.rng.Float64()
	if r > 0.88 && g.chance(0.5) {
		f := g.field()
		return op("$and", bson.A{op(f, g.condition()), op(f, g.condition())})
	}
	switch {
	case r < 0.55:
		return op(g.field(), g.condition())
	case r < 0.75:
		f1, f2 := g.twoFields()
		return bson.D{{Key: f1, Value: g.condition()}, {Key: f2, Value: g.condition()}}
	case r < 0.88:
		branches := bson.A{}
		for n := g.between(1, 3); n > 0; n-- {
			branches = append(branches, op(g.field(), g.condition()))
		}
		return op("$or", branches)
	}
	return op("$and", bson.A{op(g.field(), g.condition()), op(g.field(), g.condition())})
}

func (g *generator) orFilter() bson.D {
	branches := bson.A{}
	for n := g.between(1, 4); n > 0; n-- {
		q := g.rng.Float64()
		switch {
		case q < 0.6:
			branches = append(branches, op(g.field(), g.condition()))
		case q < 0.8:
			f1, f2 := g.twoFields()
			branches = append(branches, bson.D{{Key: f1, Value: g.condition()}, {Key: f2, Value: g.condition()}})
		default:
			f := g.field()
			branches = append(branches, op("$and", bson.A{op(f, g.condition()), op(f, g.condition())}))
		}
	}
	or := op("$or", branches)
	switch {
	case g.chance(0.25):
		return op("$and", bson.A{or, op(g.field(), g.condition())})
	case g.chance(0.2):
		return append(or, bson.E{Key: g.field(), Value: g.condition()})
	}
	return or
}

var idOnly = bson.D{{Key: "_id", Value: 1}}

func readSteps(filter bson.D, hint string) []Step {
	step := func(kind string, cmd bson.D) Step {
		return Step{Kind: kind, Hint: hint, Filter: filter, Command: cmd}
	}
	return []Step{
		step("find", bson.D{{Key: "find", Value: Collection}, {Key: "filter", Value: filter}, {Key: "hint", Value: hint},
			{Key: "projection", Value: idOnly}, {Key: "batchSize", Value: 500}}),
		step("find-limit", bson.D{{Key: "find", Value: Collection}, {Key: "filter", Value: filter}, {Key: "hint", Value: hint},
			{Key: "projection", Value: idOnly}, {Key: "limit", Value: 2}}),
		step("count", bson.D{{Key: "count", Value: Collection}, {Key: "query", Value: filter}, {Key: "hint", Value: hint}}),
		step("aggregate", bson.D{{Key: "aggregate", Value: Collection},
			{Key: "pipeline", Value: bson.A{op("$match", filter), op("$project", idOnly)}},
			{Key: "hint", Value: hint}, {Key: "cursor", Value: op("batchSize", 500)}}),
		step("distinct", bson.D{{Key: "distinct", Value: Collection}, {Key: "key", Value: "_id"},
			{Key: "query", Value: filter}, {Key: "hint", Value: hint}}),
	}
}

var writeKinds = []string{"fam", "fam-remove", "fam-new", "delete", "update", "update-pipeline", "replace"}

// StateCheck reads the whole collection in _id order with the field writes set.
var StateCheck = bson.D{{Key: "find", Value: Collection}, {Key: "filter", Value: bson.D{}},
	{Key: "projection", Value: bson.D{{Key: "_id", Value: 1}, {Key: "m", Value: 1}}},
	{Key: "sort", Value: idOnly}, {Key: "batchSize", Value: 500}}

func (g *generator) writeStep(filter bson.D, hint string) Step {
	kind := g.pick(writeKinds).(string)
	set := op("$set", op("m", 1))
	var cmd bson.D
	switch kind {
	case "fam":
		cmd = bson.D{{Key: "findAndModify", Value: Collection}, {Key: "query", Value: filter}, {Key: "hint", Value: hint},
			{Key: "update", Value: set}, {Key: "fields", Value: idOnly}}
	case "fam-new":
		cmd = bson.D{{Key: "findAndModify", Value: Collection}, {Key: "query", Value: filter}, {Key: "hint", Value: hint},
			{Key: "update", Value: set}, {Key: "new", Value: true}, {Key: "fields", Value: bson.D{{Key: "_id", Value: 1}, {Key: "m", Value: 1}}}}
	case "fam-remove":
		cmd = bson.D{{Key: "findAndModify", Value: Collection}, {Key: "query", Value: filter}, {Key: "hint", Value: hint},
			{Key: "remove", Value: true}, {Key: "fields", Value: idOnly}}
	case "delete":
		cmd = bson.D{{Key: "delete", Value: Collection}, {Key: "deletes", Value: bson.A{
			bson.D{{Key: "q", Value: filter}, {Key: "limit", Value: 1}, {Key: "hint", Value: hint}}}}}
	case "update":
		cmd = updateOne(filter, set, hint)
	case "replace":
		cmd = updateOne(filter, op("m", 1), hint)
	default:
		cmd = updateOne(filter, bson.A{set}, hint)
	}
	return Step{Kind: kind, Hint: hint, Filter: filter, Command: cmd, Check: StateCheck}
}

func updateOne(filter bson.D, u any, hint string) bson.D {
	return bson.D{{Key: "update", Value: Collection}, {Key: "updates", Value: bson.A{
		bson.D{{Key: "q", Value: filter}, {Key: "u", Value: u}, {Key: "hint", Value: hint}}}}}
}
