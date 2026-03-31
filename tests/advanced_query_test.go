package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// advancedQueryDocs covers text search, regex, $mod, and $jsonSchema scenarios.
var advancedQueryDocs = []interface{}{
	bson.D{{Key: "_id", Value: "aq1"}, {Key: "title", Value: "The quick brown fox"}, {Key: "body", Value: "jumps over the lazy dog"}, {Key: "num", Value: int32(10)}, {Key: "score", Value: 8.5}},
	bson.D{{Key: "_id", Value: "aq2"}, {Key: "title", Value: "MongoDB performance tuning"}, {Key: "body", Value: "indexes and query plans"}, {Key: "num", Value: int32(15)}, {Key: "score", Value: 6.0}},
	bson.D{{Key: "_id", Value: "aq3"}, {Key: "title", Value: "Quick start guide"}, {Key: "body", Value: "fast and easy setup"}, {Key: "num", Value: int32(20)}, {Key: "score", Value: 9.0}},
	bson.D{{Key: "_id", Value: "aq4"}, {Key: "title", Value: "Database administration"}, {Key: "body", Value: "backup restore and recovery"}, {Key: "num", Value: int32(9)}, {Key: "score", Value: 4.0}},
	bson.D{{Key: "_id", Value: "aq5"}, {Key: "title", Value: "Go programming language"}, {Key: "body", Value: "concurrency channels goroutines"}, {Key: "num", Value: int32(3)}, {Key: "score", Value: 7.5}},
	bson.D{{Key: "_id", Value: "aq6"}, {Key: "title", Value: "Fox and hound stories"}, {Key: "body", Value: "animal tales quick fox"}, {Key: "num", Value: int32(25)}, {Key: "score", Value: 5.0}},
	bson.D{{Key: "_id", Value: "aq7"}, {Key: "title", Value: "Advanced indexing"}, {Key: "body", Value: "compound partial sparse indexes"}, {Key: "num", Value: int32(7)}, {Key: "score", Value: 8.0}},
	bson.D{{Key: "_id", Value: "aq8"}, {Key: "title", Value: "Query optimization"}, {Key: "body", Value: "explain and hint usage"}, {Key: "num", Value: int32(12)}, {Key: "score", Value: 3.0}},
}

func insertAdvancedQueryDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, advancedQueryDocs)
	return err
}

func insertAdvancedQueryDocsWithTextIndex(ctx context.Context, col *mongo.Collection) error {
	if err := insertAdvancedQueryDocs(ctx, col); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "title", Value: "text"}, {Key: "body", Value: "text"}},
	})
	return err
}

// ─── Text Search (DongoXFail) ───────────────────────────────────────────────

func TestAdvancedQuery_TextSearch_CreateTextIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_CreateTextIndex",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{})
		},
	})
}

func TestAdvancedQuery_TextSearch_BasicPhrase(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_BasicPhrase",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_MultipleTerms(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_MultipleTerms",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick fox"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_ExactPhrase(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_ExactPhrase",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "\"quick brown fox\""}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_Negation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_Negation",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick -fox"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_LanguageOption(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_LanguageOption",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick"}, {Key: "$language", Value: "en"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_CaseSensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_CaseSensitive",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "Quick"}, {Key: "$caseSensitive", Value: true}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_MetaTextScore_Sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_MetaTextScore_Sort",
		Support: harness.DongoXFail,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick fox"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_MetaTextScore_Projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_MetaTextScore_Projection",
		Support: harness.DongoXFail,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "textScore", Value: bson.D{{Key: "$meta", Value: "textScore"}}}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "index"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_Count",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "fox"}}}})
		},
	})
}

func TestAdvancedQuery_TextSearch_NoResults(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_NoResults",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "xyznotfound"}}}})
		},
	})
}

func TestAdvancedQuery_TextSearch_WithAdditionalFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_WithAdditionalFilter",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			filter := bson.D{
				{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick"}}},
				{Key: "num", Value: bson.D{{Key: "$gt", Value: int32(5)}}},
			}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── Regex (DongoFull for basic, DongoXFail for advanced flags) ─────────────

func TestAdvancedQuery_Regex_CaseInsensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_CaseInsensitive",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "quick"}, {Key: "$options", Value: "i"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_CaseInsensitive_InlineOptions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_CaseInsensitive_InlineOptions",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "(?i)quick"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Anchors_StartOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Anchors_StartOf",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "^The"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Anchors_EndOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Anchors_EndOf",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "guide$"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_CharacterClass(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_CharacterClass",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "[Gg]o"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Alternation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Alternation",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "fox|Fox"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Quantifiers(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Quantifiers",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "advanc(ed|ing)+"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_InlineSyntax_vs_ObjectSyntax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_InlineSyntax_vs_ObjectSyntax",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// Use $regex object syntax explicitly
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "programming"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Multiline_m_Flag(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Multiline_m_Flag",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ml1"}, {Key: "text", Value: "line one\nline two\nline three"}},
				bson.D{{Key: "_id", Value: "ml2"}, {Key: "text", Value: "first line\nsecond line"}},
				bson.D{{Key: "_id", Value: "ml3"}, {Key: "text", Value: "single line only"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// m flag: ^ and $ match start/end of each line
			cursor, err := col.Find(ctx, bson.D{{Key: "text", Value: bson.D{{Key: "$regex", Value: "^line"}, {Key: "$options", Value: "m"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Multiline_EndAnchor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Multiline_EndAnchor",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ml1"}, {Key: "text", Value: "line one\nend here"}},
				bson.D{{Key: "_id", Value: "ml2"}, {Key: "text", Value: "no match\nmiddle end"}},
				bson.D{{Key: "_id", Value: "ml3"}, {Key: "text", Value: "only here"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "text", Value: bson.D{{Key: "$regex", Value: "here$"}, {Key: "$options", Value: "m"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_DotAll_s_Flag(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_DotAll_s_Flag",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "da1"}, {Key: "text", Value: "start\nmiddle\nend"}},
				bson.D{{Key: "_id", Value: "da2"}, {Key: "text", Value: "start end single line"}},
				bson.D{{Key: "_id", Value: "da3"}, {Key: "text", Value: "no match here"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// s flag: dot matches newline
			cursor, err := col.Find(ctx, bson.D{{Key: "text", Value: bson.D{{Key: "$regex", Value: "start.*end"}, {Key: "$options", Value: "s"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_DotAll_WithoutFlag_NoNewlineMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_DotAll_WithoutFlag_NoNewlineMatch",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "da1"}, {Key: "text", Value: "start\nmiddle\nend"}},
				bson.D{{Key: "_id", Value: "da2"}, {Key: "text", Value: "start end single line"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Without 's', dot does NOT match newline — should match only da2
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "text", Value: bson.D{{Key: "$regex", Value: "start.*end"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_ExtendedWhitespace_x_Flag(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_ExtendedWhitespace_x_Flag",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// x flag: whitespace in pattern is ignored, # comments allowed
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "quick # find quick\n fox"}, {Key: "$options", Value: "x"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_NoMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_NoMatch",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: "xyznotfound"}}}})
		},
	})
}

func TestAdvancedQuery_Regex_DigitCharClass(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_DigitCharClass",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "r1"}, {Key: "code", Value: "abc123"}},
				bson.D{{Key: "_id", Value: "r2"}, {Key: "code", Value: "no digits here"}},
				bson.D{{Key: "_id", Value: "r3"}, {Key: "code", Value: "xyz456"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "code", Value: bson.D{{Key: "$regex", Value: `\d+`}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── $mod ───────────────────────────────────────────────────────────────────

func TestAdvancedQuery_Mod_BasicDivisorRemainder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_BasicDivisorRemainder",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 5 == 0 → aq1(10), aq3(20), aq6(25)
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_RemainderNonZero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_RemainderNonZero",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 4 == 3 → aq1(10%4=2 no), aq2(15%4=3 yes), aq4(9%4=1 no), aq5(3%4=3 yes), aq7(7%4=3 yes)
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{4, 3}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_Divisor2_Even(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_Divisor2_Even",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 2 == 0 → even numbers: 10, 20, 12
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{2, 0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_Divisor2_Odd(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_Divisor2_Odd",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 2 == 1 → odd numbers
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{2, 1}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_LargeDivisor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_LargeDivisor",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 100 == 10 → only aq1(num=10)
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{100, 10}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_NoResults(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_NoResults",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// num % 7 == 6 → none in set
			return col.CountDocuments(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{7, 6}}}}})
		},
	})
}

func TestAdvancedQuery_Mod_NegativeRemainder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_NegativeRemainder",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "val", Value: int32(-10)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "val", Value: int32(-7)}},
				bson.D{{Key: "_id", Value: "m3"}, {Key: "val", Value: int32(10)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// MongoDB truncates floats toward zero for $mod
			cursor, err := col.Find(ctx, bson.D{{Key: "val", Value: bson.D{{Key: "$mod", Value: bson.A{5, -0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_FloatTruncation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_FloatTruncation",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "f1"}, {Key: "val", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "f2"}, {Key: "val", Value: int32(11)}},
				bson.D{{Key: "_id", Value: "f3"}, {Key: "val", Value: int32(12)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// MongoDB truncates float divisor: 5.8 → 5; remainder 2.3 → 2
			// val % 5 == 0 → f1(10), f3(12? no, 12%5=2)
			cursor, err := col.Find(ctx, bson.D{{Key: "val", Value: bson.D{{Key: "$mod", Value: bson.A{5.8, 0.0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_MissingField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_MissingField",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "x1"}, {Key: "num", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "x2"}}, // no 'num' field
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Missing field should not match
			return col.CountDocuments(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}}})
		},
	})
}

func TestAdvancedQuery_Mod_WithAndOperator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_WithAndOperator",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 5 == 0 AND score > 5
			filter := bson.D{
				{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}},
				{Key: "score", Value: bson.D{{Key: "$gt", Value: 5.0}}},
			}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── $jsonSchema ─────────────────────────────────────────────────────────────

var jsonSchemaDocs = []interface{}{
	bson.D{{Key: "_id", Value: "js1"}, {Key: "name", Value: "Alice"}, {Key: "age", Value: int32(25)}, {Key: "email", Value: "alice@example.com"}},
	bson.D{{Key: "_id", Value: "js2"}, {Key: "name", Value: "Bob"}, {Key: "age", Value: int32(30)}}, // missing email
	bson.D{{Key: "_id", Value: "js3"}, {Key: "age", Value: int32(22)}, {Key: "email", Value: "carol@example.com"}}, // missing name
	bson.D{{Key: "_id", Value: "js4"}, {Key: "name", Value: "Dave"}, {Key: "age", Value: "thirty"}}, // age is string not int
	bson.D{{Key: "_id", Value: "js5"}, {Key: "name", Value: "Eve"}, {Key: "age", Value: int32(-5)}, {Key: "email", Value: "eve@example.com"}}, // negative age
	bson.D{{Key: "_id", Value: "js6"}, {Key: "name", Value: "Frank"}, {Key: "age", Value: int32(150)}, {Key: "email", Value: "frank@example.com"}}, // age too high
	bson.D{{Key: "_id", Value: "js7"}, {Key: "name", Value: "Grace"}, {Key: "age", Value: int32(28)}, {Key: "email", Value: "grace@example.com"}, {Key: "role", Value: "admin"}},
	bson.D{{Key: "_id", Value: "js8"}, {Key: "name", Value: "Henry"}, {Key: "age", Value: int32(35)}, {Key: "email", Value: "henry@example.com"}, {Key: "role", Value: "user"}},
}

func insertJsonSchemaDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, jsonSchemaDocs)
	return err
}

func TestAdvancedQuery_JsonSchema_RequiredFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_RequiredFields",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"name", "age", "email"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_String(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_String",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// Match docs where 'name' is a string
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}},
				}},
				{Key: "required", Value: bson.A{"name"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_Int(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_Int",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// Match only docs where 'age' is int (not string)
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
				}},
				{Key: "required", Value: bson.A{"age"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Minimum_Maximum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Minimum_Maximum",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// age must be int, 0 <= age <= 120
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{
						{Key: "bsonType", Value: "int"},
						{Key: "minimum", Value: 0},
						{Key: "maximum", Value: 120},
					}},
				}},
				{Key: "required", Value: bson.A{"age"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Enum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Enum",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// role must be one of: "admin", "user", "moderator"
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "role", Value: bson.D{{Key: "enum", Value: bson.A{"admin", "user", "moderator"}}}},
				}},
				{Key: "required", Value: bson.A{"role"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_NestedProperties(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_NestedProperties",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "n1"}, {Key: "address", Value: bson.D{{Key: "city", Value: "NYC"}, {Key: "zip", Value: "10001"}}}},
				bson.D{{Key: "_id", Value: "n2"}, {Key: "address", Value: bson.D{{Key: "city", Value: "LA"}}}}, // missing zip
				bson.D{{Key: "_id", Value: "n3"}, {Key: "name", Value: "no address"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "address", Value: bson.D{
						{Key: "bsonType", Value: "object"},
						{Key: "required", Value: bson.A{"city", "zip"}},
						{Key: "properties", Value: bson.D{
							{Key: "city", Value: bson.D{{Key: "bsonType", Value: "string"}}},
							{Key: "zip", Value: bson.D{{Key: "bsonType", Value: "string"}}},
						}},
					}},
				}},
				{Key: "required", Value: bson.A{"address"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_PartialMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_PartialMatch",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Only docs with valid email (string type) and valid age (int, >= 0, <= 100)
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"name", "age", "email"}},
				{Key: "properties", Value: bson.D{
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "age", Value: bson.D{
						{Key: "bsonType", Value: "int"},
						{Key: "minimum", Value: 0},
						{Key: "maximum", Value: 100},
					}},
					{Key: "email", Value: bson.D{{Key: "bsonType", Value: "string"}}},
				}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Count",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"name", "age"}},
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
				}},
			}}}
			return col.CountDocuments(ctx, schema)
		},
	})
}

func TestAdvancedQuery_JsonSchema_NoMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_NoMatch",
		Support: harness.DongoXFail,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Impossible constraint: age must be int AND string simultaneously
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{{Key: "bsonType", Value: bson.A{"int", "string"}}}},
				}},
				{Key: "required", Value: bson.A{"age"}},
				// field 'nonexistent' required → nothing matches
				{Key: "required", Value: bson.A{"nonexistent_field_xyz"}},
			}}}
			return col.CountDocuments(ctx, schema)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Title_Description(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Title_Description",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// title and description annotations are ignored by the validator
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "title", Value: "User schema"},
				{Key: "description", Value: "Validates user documents"},
				{Key: "required", Value: bson.A{"name"}},
				{Key: "properties", Value: bson.D{
					{Key: "name", Value: bson.D{
						{Key: "bsonType", Value: "string"},
						{Key: "title", Value: "Full name"},
					}},
				}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_MinLength_MaxLength(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_MinLength_MaxLength",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "code", Value: "AB"}},   // too short
				bson.D{{Key: "_id", Value: "s2"}, {Key: "code", Value: "ABC"}},  // ok
				bson.D{{Key: "_id", Value: "s3"}, {Key: "code", Value: "ABCDE"}},// ok
				bson.D{{Key: "_id", Value: "s4"}, {Key: "code", Value: "ABCDEF"}},// too long
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "code", Value: bson.D{
						{Key: "bsonType", Value: "string"},
						{Key: "minLength", Value: 3},
						{Key: "maxLength", Value: 5},
					}},
				}},
				{Key: "required", Value: bson.A{"code"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Pattern(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Pattern",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "email", Value: "user@example.com"}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "email", Value: "notanemail"}},
				bson.D{{Key: "_id", Value: "p3"}, {Key: "email", Value: "another@test.org"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "email", Value: bson.D{
						{Key: "bsonType", Value: "string"},
						{Key: "pattern", Value: `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`},
					}},
				}},
				{Key: "required", Value: bson.A{"email"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_ArrayItems(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_ArrayItems",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a1"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "a2"}, {Key: "tags", Value: bson.A{"go", 42}}},   // mixed types
				bson.D{{Key: "_id", Value: "a3"}, {Key: "tags", Value: bson.A{}}},            // empty array
				bson.D{{Key: "_id", Value: "a4"}, {Key: "name", Value: "no tags"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "bsonType", Value: "array"},
						{Key: "items", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					}},
				}},
				{Key: "required", Value: bson.A{"tags"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_MinItems_MaxItems(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_MinItems_MaxItems",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mi1"}, {Key: "tags", Value: bson.A{"a"}}},
				bson.D{{Key: "_id", Value: "mi2"}, {Key: "tags", Value: bson.A{"a", "b"}}},
				bson.D{{Key: "_id", Value: "mi3"}, {Key: "tags", Value: bson.A{"a", "b", "c"}}},
				bson.D{{Key: "_id", Value: "mi4"}, {Key: "tags", Value: bson.A{"a", "b", "c", "d"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "bsonType", Value: "array"},
						{Key: "minItems", Value: 2},
						{Key: "maxItems", Value: 3},
					}},
				}},
				{Key: "required", Value: bson.A{"tags"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_MultipleOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_MultipleOf",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "o1"}, {Key: "val", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "o2"}, {Key: "val", Value: int32(15)}},
				bson.D{{Key: "_id", Value: "o3"}, {Key: "val", Value: int32(20)}},
				bson.D{{Key: "_id", Value: "o4"}, {Key: "val", Value: int32(7)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "val", Value: bson.D{
						{Key: "bsonType", Value: "int"},
						{Key: "multipleOf", Value: 5},
					}},
				}},
				{Key: "required", Value: bson.A{"val"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_AdditionalProperties_False(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_AdditionalProperties_False",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ap1"}, {Key: "name", Value: "Alice"}, {Key: "age", Value: int32(25)}},
				bson.D{{Key: "_id", Value: "ap2"}, {Key: "name", Value: "Bob"}, {Key: "age", Value: int32(30)}, {Key: "extra", Value: "field"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "additionalProperties", Value: false},
				{Key: "properties", Value: bson.D{
					{Key: "_id", Value: bson.D{}},
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}},
					{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
				}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Combined_WithRegularQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Combined_WithRegularQuery",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			filter := bson.D{
				{Key: "$jsonSchema", Value: bson.D{
					{Key: "required", Value: bson.A{"name", "age"}},
					{Key: "properties", Value: bson.D{
						{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
					}},
				}},
				{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(25)}}},
			}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_Double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_Double",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "val", Value: 3.14}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "val", Value: int32(3)}},
				bson.D{{Key: "_id", Value: "d3"}, {Key: "val", Value: "3.14"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "val", Value: bson.D{{Key: "bsonType", Value: "double"}}},
				}},
				{Key: "required", Value: bson.A{"val"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_Bool(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_Bool",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "b1"}, {Key: "active", Value: true}},
				bson.D{{Key: "_id", Value: "b2"}, {Key: "active", Value: false}},
				bson.D{{Key: "_id", Value: "b3"}, {Key: "active", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b4"}, {Key: "active", Value: "true"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "active", Value: bson.D{{Key: "bsonType", Value: "bool"}}},
				}},
				{Key: "required", Value: bson.A{"active"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_Array_Constraint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_Array_Constraint",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ar1"}, {Key: "items", Value: bson.A{1, 2, 3}}},
				bson.D{{Key: "_id", Value: "ar2"}, {Key: "items", Value: "not an array"}},
				bson.D{{Key: "_id", Value: "ar3"}, {Key: "items", Value: bson.A{}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "items", Value: bson.D{{Key: "bsonType", Value: "array"}}},
				}},
				{Key: "required", Value: bson.A{"items"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_BsonType_Null(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_BsonType_Null",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nl1"}, {Key: "val", Value: nil}},
				bson.D{{Key: "_id", Value: "nl2"}, {Key: "val", Value: int32(0)}},
				bson.D{{Key: "_id", Value: "nl3"}, {Key: "name", Value: "no val field"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "val", Value: bson.D{{Key: "bsonType", Value: "null"}}},
				}},
				{Key: "required", Value: bson.A{"val"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── Additional Text Search tests ────────────────────────────────────────────

func TestAdvancedQuery_TextSearch_DiacriticSensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_DiacriticSensitive",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{
				{Key: "$search", Value: "quick"},
				{Key: "$diacriticSensitive", Value: true},
			}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_EmptyCollection",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "title", Value: "text"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "anything"}}}})
		},
	})
}

func TestAdvancedQuery_TextSearch_FindOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_FindOne",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOne(ctx,
				bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "database"}}}},
				options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}),
			).Decode(&doc)
			if err == mongo.ErrNoDocuments {
				return nil, nil
			}
			return doc, err
		},
	})
}

func TestAdvancedQuery_TextSearch_MultiFieldIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_MultiFieldIndex",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "indexes" appears in body of aq2 and aq7
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "indexes"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── Additional Regex tests ──────────────────────────────────────────────────

func TestAdvancedQuery_Regex_WordBoundary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_WordBoundary",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: `\bGo\b`}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_Multiline_Both_Flags(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_Multiline_Both_Flags",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mb1"}, {Key: "text", Value: "START\nmiddle\nEND"}},
				bson.D{{Key: "_id", Value: "mb2"}, {Key: "text", Value: "start end"}},
				bson.D{{Key: "_id", Value: "mb3"}, {Key: "text", Value: "other text"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// both m and i flags
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "text", Value: bson.D{{Key: "$regex", Value: "^start"}, {Key: "$options", Value: "mi"}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Regex_OnNonStringField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_OnNonStringField",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $regex on a numeric field — no matches expected
			return col.CountDocuments(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$regex", Value: "1"}}}})
		},
	})
}

func TestAdvancedQuery_Regex_CombinedWithMod(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_CombinedWithMod",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// title matches "^[A-Z]" (capital letter start) AND num % 5 == 0
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			filter := bson.D{
				{Key: "title", Value: bson.D{{Key: "$regex", Value: "^[A-Z]"}}},
				{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}},
			}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── Additional $mod tests ───────────────────────────────────────────────────

func TestAdvancedQuery_Mod_Divisor3(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_Divisor3",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 3 == 0
			cursor, err := col.Find(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_NegativeDivisor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_NegativeDivisor",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nd1"}, {Key: "val", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "nd2"}, {Key: "val", Value: int32(15)}},
				bson.D{{Key: "_id", Value: "nd3"}, {Key: "val", Value: int32(7)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// MongoDB behavior: negative divisor: val % -5 == 0 same as val % 5 == 0
			cursor, err := col.Find(ctx, bson.D{{Key: "val", Value: bson.D{{Key: "$mod", Value: bson.A{-5, 0}}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_OnStringField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_OnStringField",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "sf1"}, {Key: "val", Value: "hello"}},
				bson.D{{Key: "_id", Value: "sf2"}, {Key: "val", Value: int32(10)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $mod on string field — string should not match
			return col.CountDocuments(ctx, bson.D{{Key: "val", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}}})
		},
	})
}

func TestAdvancedQuery_Mod_InOrClause(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_InOrClause",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// num % 3 == 0 OR num % 5 == 0
			filter := bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}},
				bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}}},
			}}}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

// ─── Additional $jsonSchema tests ────────────────────────────────────────────

func TestAdvancedQuery_JsonSchema_UniqueItems(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_UniqueItems",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ui1"}, {Key: "tags", Value: bson.A{"a", "b", "c"}}},
				bson.D{{Key: "_id", Value: "ui2"}, {Key: "tags", Value: bson.A{"a", "a", "c"}}}, // duplicates
				bson.D{{Key: "_id", Value: "ui3"}, {Key: "tags", Value: bson.A{"x"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "bsonType", Value: "array"},
						{Key: "uniqueItems", Value: true},
					}},
				}},
				{Key: "required", Value: bson.A{"tags"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_Not(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_Not",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// Documents that do NOT have 'age' as an int
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "not", Value: bson.D{
					{Key: "properties", Value: bson.D{
						{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
					}},
					{Key: "required", Value: bson.A{"age"}},
				}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_AnyOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_AnyOf",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// age is int OR string
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{
						{Key: "anyOf", Value: bson.A{
							bson.D{{Key: "bsonType", Value: "int"}},
							bson.D{{Key: "bsonType", Value: "string"}},
						}},
					}},
				}},
				{Key: "required", Value: bson.A{"age"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_AllOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_AllOf",
		Support: harness.DongoFull,
		Setup:   insertJsonSchemaDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "allOf", Value: bson.A{
					bson.D{{Key: "required", Value: bson.A{"name"}}},
					bson.D{{Key: "required", Value: bson.A{"email"}}},
				}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_OneOf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_OneOf",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "oo1"}, {Key: "val", Value: int32(5)}},   // odd only
				bson.D{{Key: "_id", Value: "oo2"}, {Key: "val", Value: int32(10)}},  // even only
				bson.D{{Key: "_id", Value: "oo3"}, {Key: "val", Value: int32(15)}},  // both (divisible by 3 and 5)
				bson.D{{Key: "_id", Value: "oo4"}, {Key: "val", Value: int32(7)}},   // neither
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			// oneOf: divisible by 3 XOR divisible by 5 (exactly one)
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "oneOf", Value: bson.A{
					bson.D{{Key: "properties", Value: bson.D{{Key: "val", Value: bson.D{
						{Key: "bsonType", Value: "int"}, {Key: "multipleOf", Value: 3},
					}}}}},
					bson.D{{Key: "properties", Value: bson.D{{Key: "val", Value: bson.D{
						{Key: "bsonType", Value: "int"}, {Key: "multipleOf", Value: 5},
					}}}}},
				}},
				{Key: "required", Value: bson.A{"val"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_Divisor1_AllMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_Divisor1_AllMatch",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// num % 1 == 0 — every integer matches
			return col.CountDocuments(ctx, bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{1, 0}}}}})
		},
	})
}

func TestAdvancedQuery_Regex_LookaheadUnsupported(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Regex_LookaheadUnsupported",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Lookaheads are PCRE-only — may error or produce different results
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "title", Value: bson.D{{Key: "$regex", Value: `Quick(?= start)`}}}}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_TextSearch_AggregationMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_TextSearch_AggregationMatch",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocsWithTextIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $text in aggregation $match stage
			pipeline := bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "quick"}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_JsonSchema_ExclusiveMinimum_Maximum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_JsonSchema_ExclusiveMinimum_Maximum",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "em1"}, {Key: "val", Value: int32(0)}},   // at boundary
				bson.D{{Key: "_id", Value: "em2"}, {Key: "val", Value: int32(1)}},   // inside
				bson.D{{Key: "_id", Value: "em3"}, {Key: "val", Value: int32(9)}},   // inside
				bson.D{{Key: "_id", Value: "em4"}, {Key: "val", Value: int32(10)}},  // at boundary
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			schema := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "val", Value: bson.D{
						{Key: "bsonType", Value: "int"},
						{Key: "exclusiveMinimum", Value: true},
						{Key: "minimum", Value: 0},
						{Key: "exclusiveMaximum", Value: true},
						{Key: "maximum", Value: 10},
					}},
				}},
				{Key: "required", Value: bson.A{"val"}},
			}}}
			cursor, err := col.Find(ctx, schema, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			return docs, cursor.All(ctx, &docs)
		},
	})
}

func TestAdvancedQuery_Mod_CountWithFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AdvancedQuery_Mod_CountWithFilter",
		Support: harness.DongoFull,
		Setup:   insertAdvancedQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Count docs where num is divisible by 5 OR by 3
			filter := bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{5, 0}}}}},
				bson.D{{Key: "num", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}},
			}}}
			return col.CountDocuments(ctx, filter)
		},
	})
}
