package main

import (
	"fmt"
	"strings"
)

// Query is the interface for all AST nodes in the search query language.
type Query interface {
	Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField)
}

// --- Tokenizer ---

type tokenKind int

const (
	tokWord   tokenKind = iota
	tokAnd              // AND
	tokOr               // OR
	tokNot              // NOT
	tokMinus            // -
	tokLParen           // (
	tokRParen           // )
	tokColon            // :
	tokEOF
)

type token struct {
	kind tokenKind
	text string
}

func tokenize(input string) []token {
	var tokens []token
	i := 0

	for i < len(input) {
		ch := input[i]

		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		switch ch {
		case '(':
			tokens = append(tokens, token{tokLParen, "("})
			i++
		case ')':
			tokens = append(tokens, token{tokRParen, ")"})
			i++
		case ':':
			tokens = append(tokens, token{tokColon, ":"})
			i++
		case '-':
			tokens = append(tokens, token{tokMinus, "-"})
			i++
		case '"':
			// Quoted string
			i++ // skip opening quote
			start := i
			for i < len(input) && input[i] != '"' {
				i++
			}
			text := input[start:i]
			if i < len(input) {
				i++ // skip closing quote
			}
			tokens = append(tokens, token{tokWord, strings.ToLower(text)})
		default:
			// Bare word — collect until whitespace or special char
			start := i
			for i < len(input) {
				c := input[i]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
					c == '(' || c == ')' || c == ':' || c == '"' {
					break
				}
				i++
			}
			word := input[start:i]
			switch word {
			case "AND":
				tokens = append(tokens, token{tokAnd, word})
			case "OR":
				tokens = append(tokens, token{tokOr, word})
			case "NOT":
				tokens = append(tokens, token{tokNot, word})
			default:
				tokens = append(tokens, token{tokWord, strings.ToLower(word)})
			}
		}
	}

	tokens = append(tokens, token{tokEOF, ""})
	return tokens
}

// --- AST Nodes ---

// TermQuery matches a term against all fields, picking the best match.
type TermQuery struct {
	Term string
}

func (q *TermQuery) Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField) {
	bestQuality := MatchNone
	bestField := MatchedField(-1)

	for _, f := range []MatchedField{FieldName, FieldNip05, FieldDisplayName, FieldAbout, FieldContent} {
		value, ok := fields[f]
		if !ok || value == "" {
			continue
		}
		mq := matchString(value, q.Term)
		if mq == MatchNone {
			continue
		}
		if bestQuality == MatchNone || mq < bestQuality || (mq == bestQuality && f < bestField) {
			bestQuality = mq
			bestField = f
		}
	}

	return bestQuality, bestField
}

// FieldQuery matches a term against a specific field only.
type FieldQuery struct {
	Field MatchedField
	Term  string
}

func (q *FieldQuery) Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField) {
	value, ok := fields[q.Field]
	if !ok || value == "" {
		return MatchNone, q.Field
	}
	mq := matchString(value, q.Term)
	return mq, q.Field
}

// AndQuery requires all children to match. Quality = worst (max), field = best (min).
type AndQuery struct {
	Children []Query
}

func (q *AndQuery) Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField) {
	worstQuality := MatchQuality(-1)
	bestField := MatchedField(999)

	for _, child := range q.Children {
		mq, mf := child.Evaluate(fields)
		if mq == MatchNone {
			return MatchNone, MatchedField(-1)
		}
		if worstQuality == -1 || mq > worstQuality {
			worstQuality = mq
		}
		if mf < bestField {
			bestField = mf
		}
	}

	return worstQuality, bestField
}

// OrQuery matches if any child matches. Quality = best (min), field = best (min).
type OrQuery struct {
	Children []Query
}

func (q *OrQuery) Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField) {
	bestQuality := MatchNone
	bestField := MatchedField(999)

	for _, child := range q.Children {
		mq, mf := child.Evaluate(fields)
		if mq == MatchNone {
			continue
		}
		if bestQuality == MatchNone || mq < bestQuality {
			bestQuality = mq
		}
		if mf < bestField {
			bestField = mf
		}
	}

	return bestQuality, bestField
}

// NotQuery negates its child. Returns MatchContains if child doesn't match, MatchNone if it does.
type NotQuery struct {
	Child Query
}

func (q *NotQuery) Evaluate(fields map[MatchedField]string) (MatchQuality, MatchedField) {
	mq, _ := q.Child.Evaluate(fields)
	if mq == MatchNone {
		return MatchContains, FieldContent
	}
	return MatchNone, MatchedField(-1)
}

// --- Parser ---

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{tokEOF, ""}
	}
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) expect(kind tokenKind) (token, error) {
	t := p.next()
	if t.kind != kind {
		return t, fmt.Errorf("expected %d, got %d (%q)", kind, t.kind, t.text)
	}
	return t, nil
}

// Parse parses a query string into a Query AST.
// If the query contains no operators (AND, OR, NOT, -, :, parens),
// it is treated as a single phrase search (e.g. "bitcoin rust" matches
// the literal phrase). Use explicit operators for boolean logic.
func Parse(input string) (Query, error) {
	tokens := tokenize(input)

	// Plain text (all words, no operators) → single phrase search
	if isPlainText(tokens) {
		phrase := strings.ToLower(strings.TrimSpace(input))
		if phrase == "" {
			return nil, fmt.Errorf("unexpected end of query")
		}
		return &TermQuery{Term: phrase}, nil
	}

	p := &parser{tokens: tokens}
	q, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return q, nil
}

// isPlainText returns true if the token list contains only bare words and EOF
// (no operators, parens, colons, minus signs, or quotes in the original input).
// A single word is also plain text (no need for phrase wrapping since it's equivalent).
func isPlainText(tokens []token) bool {
	wordCount := 0
	for _, t := range tokens {
		switch t.kind {
		case tokWord:
			wordCount++
		case tokEOF:
			continue
		default:
			return false
		}
	}
	return wordCount > 1
}

func (p *parser) parseOr() (Query, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	children := []Query{left}
	for p.peek().kind == tokOr {
		p.next() // consume OR
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}

	if len(children) == 1 {
		return children[0], nil
	}
	return &OrQuery{Children: children}, nil
}

func (p *parser) parseAnd() (Query, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	children := []Query{left}
	for {
		// Explicit AND
		if p.peek().kind == tokAnd {
			p.next() // consume AND
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			children = append(children, right)
			continue
		}

		// Implicit AND: next token starts a new unary (word, quote, minus, NOT, lparen)
		pk := p.peek().kind
		if pk == tokWord || pk == tokMinus || pk == tokNot || pk == tokLParen {
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			children = append(children, right)
			continue
		}

		break
	}

	if len(children) == 1 {
		return children[0], nil
	}
	return &AndQuery{Children: children}, nil
}

func (p *parser) parseUnary() (Query, error) {
	if p.peek().kind == tokMinus || p.peek().kind == tokNot {
		p.next() // consume - or NOT
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &NotQuery{Child: child}, nil
	}
	return p.parsePrimary()
}

var fieldNames = map[string]MatchedField{
	"name":         FieldName,
	"nip05":        FieldNip05,
	"display_name": FieldDisplayName,
	"about":        FieldAbout,
	"content":      FieldContent,
}

func (p *parser) parsePrimary() (Query, error) {
	t := p.peek()

	if t.kind == tokLParen {
		p.next() // consume (
		q, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen); err != nil {
			return nil, fmt.Errorf("unclosed parenthesis")
		}
		return q, nil
	}

	if t.kind == tokWord {
		p.next() // consume word

		// Check for field:value syntax
		if p.peek().kind == tokColon {
			p.next() // consume :
			field, ok := fieldNames[t.text]
			if !ok {
				return nil, fmt.Errorf("unknown field %q", t.text)
			}
			val := p.next()
			if val.kind != tokWord {
				return nil, fmt.Errorf("expected value after %s:, got %q", t.text, val.text)
			}
			return &FieldQuery{Field: field, Term: val.text}, nil
		}

		return &TermQuery{Term: t.text}, nil
	}

	if t.kind == tokEOF {
		return nil, fmt.Errorf("unexpected end of query")
	}

	return nil, fmt.Errorf("unexpected token %q", t.text)
}
