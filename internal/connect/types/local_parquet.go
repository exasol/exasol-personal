// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"errors"
	"strings"
	"unicode"
)

const LocalParquetTableForm = "CREATE [OR REPLACE] TABLE <schema>.<table> AS " +
	"(IMPORT FROM LOCAL PARQUET FILE '<path>')"

var ErrUnsupportedLocalParquetTable = errors.New(
	"creating a table from a local Parquet file is only supported as " + LocalParquetTableForm,
)

type LocalParquetTable struct {
	Schema  string
	Table   string
	Path    string
	Replace bool
}

const (
	unterminated        = -1
	maxIdentifierLength = 128
	middleDot           = '\u00B7'
)

var (
	identifierStart = []*unicode.RangeTable{unicode.L, unicode.Nl}
	identifierPart  = []*unicode.RangeTable{
		unicode.L, unicode.Nl, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Cf,
	}
)

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenString
	tokenQuotedIdentifier
	tokenSymbol
)

type sqlToken struct {
	kind  tokenKind
	value string
}

func (token sqlToken) isWord(keyword string) bool {
	return token.kind == tokenWord && strings.EqualFold(token.value, keyword)
}

func (token sqlToken) isSymbol(symbol string) bool {
	return token.kind == tokenSymbol && token.value == symbol
}

func ParseLocalParquetTable(sql string) (*LocalParquetTable, error) {
	tokens, terminated := tokenizeSQL(sql)
	if !terminated {
		return nil, nil //nolint:nilnil // the database reports the malformed statement
	}

	prefix, isCreateTable := matchCreateTablePrefix(tokens)
	if !isCreateTable || !containsLocalParquet(prefix.rest) {
		return nil, nil //nolint:nilnil // nil means the statement is unrelated
	}

	table, matched := matchLocalParquetImport(prefix.rest)
	if !matched {
		return nil, ErrUnsupportedLocalParquetTable
	}
	table.Replace = prefix.replace

	return table, nil
}

type createTablePrefix struct {
	rest    []sqlToken
	replace bool
}

func matchCreateTablePrefix(tokens []sqlToken) (createTablePrefix, bool) {
	position := 0
	expect := func(keyword string) bool {
		if position < len(tokens) && tokens[position].isWord(keyword) {
			position++
			return true
		}

		return false
	}

	if !expect("CREATE") {
		return createTablePrefix{}, false
	}
	replace := expect("OR")
	if replace && !expect("REPLACE") {
		return createTablePrefix{}, false
	}
	if !expect("TABLE") {
		return createTablePrefix{}, false
	}

	return createTablePrefix{rest: tokens[position:], replace: replace}, true
}

func containsLocalParquet(tokens []sqlToken) bool {
	for i := 1; i < len(tokens); i++ {
		if tokens[i-1].isWord("LOCAL") && tokens[i].isWord("PARQUET") {
			return true
		}
	}

	return false
}

func matchLocalParquetImport(tokens []sqlToken) (*LocalParquetTable, bool) {
	if len(tokens) > 0 && tokens[len(tokens)-1].isSymbol(";") {
		tokens = tokens[:len(tokens)-1]
	}

	pattern := []func(sqlToken) bool{
		isRegularIdentifier,
		symbol("."),
		isRegularIdentifier,
		keyword("AS"),
		symbol("("),
		keyword("IMPORT"),
		keyword("FROM"),
		keyword("LOCAL"),
		keyword("PARQUET"),
		keyword("FILE"),
		func(token sqlToken) bool { return token.kind == tokenString },
		symbol(")"),
	}
	if len(tokens) != len(pattern) {
		return nil, false
	}
	for i, matches := range pattern {
		if !matches(tokens[i]) {
			return nil, false
		}
	}

	return &LocalParquetTable{
		Schema: strings.ToUpper(tokens[0].value),
		Table:  strings.ToUpper(tokens[2].value),
		Path:   tokens[10].value,
	}, true
}

func keyword(word string) func(sqlToken) bool {
	return func(token sqlToken) bool { return token.isWord(word) }
}

func symbol(value string) func(sqlToken) bool {
	return func(token sqlToken) bool { return token.isSymbol(value) }
}

// Exasol's grammar for unquoted identifiers, which the driver also enforces.
func isRegularIdentifier(token sqlToken) bool {
	if token.kind != tokenWord {
		return false
	}

	runes := []rune(token.value)

	return len(runes) <= maxIdentifierLength && unicode.In(runes[0], identifierStart...)
}

func tokenizeSQL(sql string) ([]sqlToken, bool) {
	var tokens []sqlToken
	runes := []rune(sql)

	for pos := 0; pos < len(runes); {
		char := runes[pos]
		switch {
		case unicode.IsSpace(char):
			pos++
		case hasPrefixAt(runes, pos, "--"):
			pos = skipLineComment(runes, pos)
		case hasPrefixAt(runes, pos, "/*"):
			pos = skipBlockComment(runes, pos)
		case char == '\'' || char == '"':
			var token sqlToken
			token, pos = readQuotedToken(runes, pos)
			tokens = append(tokens, token)
		case isWordRune(char):
			var token sqlToken
			token, pos = readWord(runes, pos)
			tokens = append(tokens, token)
		default:
			tokens = append(tokens, sqlToken{kind: tokenSymbol, value: string(char)})
			pos++
		}
		if pos == unterminated {
			return nil, false
		}
	}

	return tokens, true
}

func skipLineComment(runes []rune, pos int) int {
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}

	return pos
}

func skipBlockComment(runes []rune, pos int) int {
	const start, end = "/*", "*/"
	for pos += len(start); pos < len(runes); pos++ {
		if hasPrefixAt(runes, pos, end) {
			return pos + len(end)
		}
	}

	return unterminated
}

func readQuotedToken(runes []rune, pos int) (sqlToken, int) {
	value, next := readQuoted(runes, pos)
	if runes[pos] == '"' {
		return sqlToken{kind: tokenQuotedIdentifier, value: value}, next
	}

	return sqlToken{kind: tokenString, value: value}, next
}

func readWord(runes []rune, start int) (sqlToken, int) {
	pos := start
	for pos < len(runes) && isWordRune(runes[pos]) {
		pos++
	}

	return sqlToken{kind: tokenWord, value: string(runes[start:pos])}, pos
}

func readQuoted(runes []rune, start int) (string, int) {
	quote := runes[start]
	var builder strings.Builder

	for pos := start + 1; pos < len(runes); pos++ {
		if runes[pos] != quote {
			_, _ = builder.WriteRune(runes[pos])

			continue
		}
		if pos+1 < len(runes) && runes[pos+1] == quote {
			_, _ = builder.WriteRune(quote)
			pos++

			continue
		}

		return builder.String(), pos + 1
	}

	return "", unterminated
}

func isWordRune(char rune) bool {
	return unicode.In(char, identifierPart...) || char == middleDot
}

func hasPrefixAt(runes []rune, pos int, prefix string) bool {
	for _, char := range prefix {
		if pos >= len(runes) || runes[pos] != char {
			return false
		}
		pos++
	}

	return true
}
