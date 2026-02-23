package main

import (
	"testing"
)

// --- Tokenizer tests ---

func TestTokenizeSimple(t *testing.T) {
	tokens := tokenize("hello world")
	// hello, world, EOF
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[0].kind != tokWord || tokens[0].text != "hello" {
		t.Errorf("token 0: got %v", tokens[0])
	}
	if tokens[1].kind != tokWord || tokens[1].text != "world" {
		t.Errorf("token 1: got %v", tokens[1])
	}
	if tokens[2].kind != tokEOF {
		t.Errorf("token 2: expected EOF")
	}
}

func TestTokenizeKeywords(t *testing.T) {
	tokens := tokenize("a AND b OR c NOT d")
	expected := []struct {
		kind tokenKind
		text string
	}{
		{tokWord, "a"},
		{tokAnd, "AND"},
		{tokWord, "b"},
		{tokOr, "OR"},
		{tokWord, "c"},
		{tokNot, "NOT"},
		{tokWord, "d"},
		{tokEOF, ""},
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, e := range expected {
		if tokens[i].kind != e.kind || tokens[i].text != e.text {
			t.Errorf("token %d: expected {%d, %q}, got {%d, %q}", i, e.kind, e.text, tokens[i].kind, tokens[i].text)
		}
	}
}

func TestTokenizeLowercaseKeywordsArePlainWords(t *testing.T) {
	tokens := tokenize("and or not")
	for i := 0; i < 3; i++ {
		if tokens[i].kind != tokWord {
			t.Errorf("token %d: expected Word, got %d", i, tokens[i].kind)
		}
	}
}

func TestTokenizeQuoted(t *testing.T) {
	tokens := tokenize(`"Hello World" foo`)
	if tokens[0].kind != tokWord || tokens[0].text != "hello world" {
		t.Errorf("quoted token: got {%d, %q}", tokens[0].kind, tokens[0].text)
	}
	if tokens[1].kind != tokWord || tokens[1].text != "foo" {
		t.Errorf("token 1: got {%d, %q}", tokens[1].kind, tokens[1].text)
	}
}

func TestTokenizeFieldSyntax(t *testing.T) {
	tokens := tokenize("name:jack")
	expected := []tokenKind{tokWord, tokColon, tokWord, tokEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, e := range expected {
		if tokens[i].kind != e {
			t.Errorf("token %d: expected kind %d, got %d", i, e, tokens[i].kind)
		}
	}
}

func TestTokenizeMinus(t *testing.T) {
	tokens := tokenize("-bot")
	if tokens[0].kind != tokMinus {
		t.Errorf("expected Minus, got %d", tokens[0].kind)
	}
	if tokens[1].kind != tokWord || tokens[1].text != "bot" {
		t.Errorf("expected Word 'bot', got {%d, %q}", tokens[1].kind, tokens[1].text)
	}
}

func TestTokenizeParens(t *testing.T) {
	tokens := tokenize("(a OR b)")
	kinds := []tokenKind{tokLParen, tokWord, tokOr, tokWord, tokRParen, tokEOF}
	if len(tokens) != len(kinds) {
		t.Fatalf("expected %d tokens, got %d", len(kinds), len(tokens))
	}
	for i, k := range kinds {
		if tokens[i].kind != k {
			t.Errorf("token %d: expected kind %d, got %d", i, k, tokens[i].kind)
		}
	}
}

// --- Parser tests ---

func TestParseSimpleTerm(t *testing.T) {
	q, err := Parse("jack")
	if err != nil {
		t.Fatal(err)
	}
	tq, ok := q.(*TermQuery)
	if !ok {
		t.Fatalf("expected TermQuery, got %T", q)
	}
	if tq.Term != "jack" {
		t.Errorf("expected term 'jack', got %q", tq.Term)
	}
}

func TestParseFieldTerm(t *testing.T) {
	q, err := Parse("name:jack")
	if err != nil {
		t.Fatal(err)
	}
	fq, ok := q.(*FieldQuery)
	if !ok {
		t.Fatalf("expected FieldQuery, got %T", q)
	}
	if fq.Field != FieldName || fq.Term != "jack" {
		t.Errorf("expected name:jack, got %v:%q", fq.Field, fq.Term)
	}
}

func TestParseAnd(t *testing.T) {
	q, err := Parse("jack AND bitcoin")
	if err != nil {
		t.Fatal(err)
	}
	aq, ok := q.(*AndQuery)
	if !ok {
		t.Fatalf("expected AndQuery, got %T", q)
	}
	if len(aq.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(aq.Children))
	}
}

func TestParsePlainTextAsPhrase(t *testing.T) {
	q, err := Parse("jack bitcoin")
	if err != nil {
		t.Fatal(err)
	}
	tq, ok := q.(*TermQuery)
	if !ok {
		t.Fatalf("expected TermQuery (phrase), got %T", q)
	}
	if tq.Term != "jack bitcoin" {
		t.Errorf("expected phrase 'jack bitcoin', got %q", tq.Term)
	}
}

func TestParseMultiWordPhrase(t *testing.T) {
	q, err := Parse("hello world foo bar")
	if err != nil {
		t.Fatal(err)
	}
	tq, ok := q.(*TermQuery)
	if !ok {
		t.Fatalf("expected TermQuery (phrase), got %T", q)
	}
	if tq.Term != "hello world foo bar" {
		t.Errorf("expected phrase 'hello world foo bar', got %q", tq.Term)
	}
}

func TestParseOperatorTriggerssDSL(t *testing.T) {
	// AND keyword triggers DSL parsing, not phrase mode
	q, err := Parse("jack AND bitcoin")
	if err != nil {
		t.Fatal(err)
	}
	_, ok := q.(*AndQuery)
	if !ok {
		t.Fatalf("expected AndQuery, got %T", q)
	}
}

func TestParseLowercaseAndOrNotArePhrase(t *testing.T) {
	// lowercase and/or/not are plain words, so "and or not" is a phrase
	q, err := Parse("and or not")
	if err != nil {
		t.Fatal(err)
	}
	tq, ok := q.(*TermQuery)
	if !ok {
		t.Fatalf("expected TermQuery (phrase), got %T", q)
	}
	if tq.Term != "and or not" {
		t.Errorf("expected phrase 'and or not', got %q", tq.Term)
	}
}

func TestParseOr(t *testing.T) {
	q, err := Parse("jack OR dorsey")
	if err != nil {
		t.Fatal(err)
	}
	oq, ok := q.(*OrQuery)
	if !ok {
		t.Fatalf("expected OrQuery, got %T", q)
	}
	if len(oq.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(oq.Children))
	}
}

func TestParseNot(t *testing.T) {
	q, err := Parse("-bot")
	if err != nil {
		t.Fatal(err)
	}
	nq, ok := q.(*NotQuery)
	if !ok {
		t.Fatalf("expected NotQuery, got %T", q)
	}
	tq, ok := nq.Child.(*TermQuery)
	if !ok {
		t.Fatalf("expected TermQuery child, got %T", nq.Child)
	}
	if tq.Term != "bot" {
		t.Errorf("expected 'bot', got %q", tq.Term)
	}
}

func TestParseNotKeyword(t *testing.T) {
	q, err := Parse("NOT bot")
	if err != nil {
		t.Fatal(err)
	}
	_, ok := q.(*NotQuery)
	if !ok {
		t.Fatalf("expected NotQuery, got %T", q)
	}
}

func TestParseGrouping(t *testing.T) {
	q, err := Parse("(jack OR dorsey) -bot")
	if err != nil {
		t.Fatal(err)
	}
	aq, ok := q.(*AndQuery)
	if !ok {
		t.Fatalf("expected AndQuery, got %T", q)
	}
	if len(aq.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(aq.Children))
	}
	_, ok = aq.Children[0].(*OrQuery)
	if !ok {
		t.Fatalf("expected OrQuery as first child, got %T", aq.Children[0])
	}
	_, ok = aq.Children[1].(*NotQuery)
	if !ok {
		t.Fatalf("expected NotQuery as second child, got %T", aq.Children[1])
	}
}

func TestParseComplex(t *testing.T) {
	q, err := Parse("name:jack AND about:bitcoin")
	if err != nil {
		t.Fatal(err)
	}
	aq, ok := q.(*AndQuery)
	if !ok {
		t.Fatalf("expected AndQuery, got %T", q)
	}
	if len(aq.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(aq.Children))
	}
	fq1, ok := aq.Children[0].(*FieldQuery)
	if !ok {
		t.Fatalf("expected FieldQuery, got %T", aq.Children[0])
	}
	if fq1.Field != FieldName || fq1.Term != "jack" {
		t.Errorf("child 0: expected name:jack, got %v:%q", fq1.Field, fq1.Term)
	}
	fq2, ok := aq.Children[1].(*FieldQuery)
	if !ok {
		t.Fatalf("expected FieldQuery, got %T", aq.Children[1])
	}
	if fq2.Field != FieldAbout || fq2.Term != "bitcoin" {
		t.Errorf("child 1: expected about:bitcoin, got %v:%q", fq2.Field, fq2.Term)
	}
}

func TestParseErrorUnclosedParen(t *testing.T) {
	_, err := Parse("(jack OR dorsey")
	if err == nil {
		t.Error("expected error for unclosed paren")
	}
}

func TestParseErrorUnknownField(t *testing.T) {
	_, err := Parse("unknown:value")
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestParseErrorEmpty(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Error("expected error for empty query")
	}
}

// --- Evaluation tests ---

func TestTermQueryEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName:  "jack",
		FieldAbout: "founder of twitter",
	}

	q := &TermQuery{Term: "jack"}
	mq, mf := q.Evaluate(fields)
	if mq != MatchExact || mf != FieldName {
		t.Errorf("expected exact/name, got %v/%v", mq, mf)
	}

	q2 := &TermQuery{Term: "twitter"}
	mq2, mf2 := q2.Evaluate(fields)
	if mq2 != MatchContains || mf2 != FieldAbout {
		t.Errorf("expected contains/about, got %v/%v", mq2, mf2)
	}
}

func TestFieldQueryEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName:  "jack",
		FieldAbout: "founder of twitter",
	}

	q := &FieldQuery{Field: FieldName, Term: "jack"}
	mq, mf := q.Evaluate(fields)
	if mq != MatchExact || mf != FieldName {
		t.Errorf("expected exact/name, got %v/%v", mq, mf)
	}

	// Field query should NOT match other fields
	q2 := &FieldQuery{Field: FieldName, Term: "twitter"}
	mq2, _ := q2.Evaluate(fields)
	if mq2 != MatchNone {
		t.Errorf("expected MatchNone, got %v", mq2)
	}
}

func TestAndQueryEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName:  "jack",
		FieldAbout: "bitcoin enthusiast",
	}

	q := &AndQuery{Children: []Query{
		&TermQuery{Term: "jack"},
		&TermQuery{Term: "bitcoin"},
	}}
	mq, mf := q.Evaluate(fields)
	// jack=exact(0), bitcoin=prefix on about(1) → worst quality = contains? No — bitcoin matches "bitcoin enthusiast" as prefix
	// Actually: matchString("bitcoin enthusiast", "bitcoin") → prefix
	// worst quality = max(exact=0, prefix=1) = prefix(1)
	// best field = min(name=0, about=3) = name(0)
	if mq != MatchPrefix {
		t.Errorf("expected MatchPrefix, got %v", mq)
	}
	if mf != FieldName {
		t.Errorf("expected FieldName, got %v", mf)
	}

	// One child doesn't match → AND fails
	q2 := &AndQuery{Children: []Query{
		&TermQuery{Term: "jack"},
		&TermQuery{Term: "nonexistent"},
	}}
	mq2, _ := q2.Evaluate(fields)
	if mq2 != MatchNone {
		t.Errorf("expected MatchNone, got %v", mq2)
	}
}

func TestOrQueryEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName:  "jack",
		FieldAbout: "bitcoin enthusiast",
	}

	q := &OrQuery{Children: []Query{
		&TermQuery{Term: "jack"},
		&TermQuery{Term: "nonexistent"},
	}}
	mq, mf := q.Evaluate(fields)
	if mq != MatchExact {
		t.Errorf("expected MatchExact, got %v", mq)
	}
	if mf != FieldName {
		t.Errorf("expected FieldName, got %v", mf)
	}

	// No children match → OR fails
	q2 := &OrQuery{Children: []Query{
		&TermQuery{Term: "xyz"},
		&TermQuery{Term: "abc"},
	}}
	mq2, _ := q2.Evaluate(fields)
	if mq2 != MatchNone {
		t.Errorf("expected MatchNone, got %v", mq2)
	}
}

func TestNotQueryEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName: "jack",
	}

	// NOT matching term → MatchNone
	q := &NotQuery{Child: &TermQuery{Term: "jack"}}
	mq, _ := q.Evaluate(fields)
	if mq != MatchNone {
		t.Errorf("expected MatchNone, got %v", mq)
	}

	// NOT non-matching term → MatchContains
	q2 := &NotQuery{Child: &TermQuery{Term: "bot"}}
	mq2, mf2 := q2.Evaluate(fields)
	if mq2 != MatchContains {
		t.Errorf("expected MatchContains, got %v", mq2)
	}
	if mf2 != FieldContent {
		t.Errorf("expected FieldContent, got %v", mf2)
	}
}

// --- Integration tests ---

func TestParseAndEvaluate(t *testing.T) {
	fields := map[MatchedField]string{
		FieldName:        "jack",
		FieldDisplayName: "jack dorsey",
		FieldAbout:       "bitcoin maximalist and founder",
		FieldNip05:       "jack@cash.app",
	}

	tests := []struct {
		input     string
		wantMatch bool
		wantQual  MatchQuality
	}{
		// Single word → phrase (single TermQuery)
		{"jack", true, MatchExact},
		// Multi-word plain text → phrase search
		{"jack dorsey", true, MatchExact},            // matches display_name "jack dorsey"
		{"bitcoin maximalist", true, MatchPrefix},     // matches about "bitcoin maximalist and founder"
		{"dorsey bitcoin", false, MatchNone},          // phrase not found in any field
		// DSL: operators trigger boolean parsing
		{"name:jack", true, MatchExact},
		{"name:jack AND about:bitcoin", true, MatchPrefix},
		{"name:dorsey", false, MatchNone},
		{"jack OR dorsey", true, MatchExact},
		{"-bot", true, MatchContains},
		{"-jack", false, MatchNone},
		{"(jack OR dorsey) -bot", true, MatchContains},
		{"(jack OR dorsey) -jack", false, MatchNone},
		{`"jack dorsey"`, true, MatchExact},
		{"about:maximalist", true, MatchContains},
		{"nip05:cash.app", true, MatchContains},
	}

	for _, tt := range tests {
		q, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.input, err)
			continue
		}
		mq, _ := q.Evaluate(fields)
		if tt.wantMatch && mq == MatchNone {
			t.Errorf("Parse(%q).Evaluate: expected match, got MatchNone", tt.input)
		} else if !tt.wantMatch && mq != MatchNone {
			t.Errorf("Parse(%q).Evaluate: expected MatchNone, got %v", tt.input, mq)
		} else if tt.wantMatch && mq != tt.wantQual {
			t.Errorf("Parse(%q).Evaluate: expected %v, got %v", tt.input, tt.wantQual, mq)
		}
	}
}
