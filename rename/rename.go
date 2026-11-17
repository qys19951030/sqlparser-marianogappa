// Package rename rewrites SQL statements by replacing table and column names
// at their syntactic positions. Every byte that is not a replaced name is
// preserved verbatim, including whitespace, casing, quoting and literals.
package rename

import (
	"fmt"
	"sort"
	"strings"

	"github.com/marianogappa/sqlparser/query"
)

// tokenKind distinguishes the three lexical shapes the parser recognises.
type tokenKind int

const (
	kindReserved tokenKind = iota
	kindQuoted
	kindIdent
)

// token is a lexical token with its byte offsets in the source statement.
// sql[start:end] is the token's original text, val is the inner content for
// quoted strings, the raw text for identifiers and the upper-cased word for
// reserved words and operators.
type token struct {
	kind       tokenKind
	start, end int
	val        string
}

// reservedWords mirrors sqlparser.reservedWords, including its order: longer
// operators must be probed before their single-character prefixes.
var reservedWords = []string{
	"(", ")", ">=", "<=", "!=", ",", "=", ">", "<", "SELECT", "INSERT INTO", "VALUES", "UPDATE", "DELETE FROM",
	"WHERE", "FROM", "SET", "AS",
}

// tokenize splits sql into offset-annotated tokens using the same lexical
// rules as the parser, so every token corresponds one-to-one with what
// parser.peek would return.
func tokenize(sql string) ([]token, error) {
	tokens := []token{}
	for i := 0; i < len(sql); {
		if isSpace(sql[i]) {
			i++
			continue
		}
		matched := false
		for _, rWord := range reservedWords {
			end := i + len(rWord)
			if end > len(sql) {
				end = len(sql)
			}
			if strings.ToUpper(sql[i:end]) == rWord {
				tokens = append(tokens, token{kind: kindReserved, start: i, end: end, val: rWord})
				i = end
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if sql[i] == '\'' {
			j := i + 1
			for ; j < len(sql); j++ {
				if sql[j] == '\'' && sql[j-1] != '\\' {
					break
				}
			}
			if j >= len(sql) {
				return nil, fmt.Errorf("unterminated quoted string")
			}
			tokens = append(tokens, token{kind: kindQuoted, start: i, end: j + 1, val: sql[i+1 : j]})
			i = j + 1
			continue
		}
		j := i
		for ; j < len(sql) && isIdentChar(sql[j]); j++ {
		}
		if j == i {
			return nil, fmt.Errorf("unexpected character %q", sql[i])
		}
		tokens = append(tokens, token{kind: kindIdent, start: i, end: j, val: sql[i:j]})
		i = j
	}
	return tokens, nil
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '*'
}

// edit replaces sql[start:end] with replacement.
type edit struct {
	start, end  int
	replacement string
}

// rewriter walks the token stream with a state machine that mirrors the
// parser's steps, recording edits only at syntactic name positions.
type rewriter struct {
	sql         string
	q           query.Query
	tokens      []token
	pos         int
	tableNames  map[string]string
	columnNames map[string]string
	edits       []edit
}

// Rename returns sql with table and column names replaced according to
// tableNames and columnNames. Only the names at the syntactic positions
// recorded in q are eligible for replacement; keywords and literals are never
// touched. A name absent from the relevant mapping stays as it is. When both
// maps contain the same name, each position uses the map for its own role.
func Rename(sql string, q query.Query, tableNames, columnNames map[string]string) (string, error) {
	tokens, err := tokenize(sql)
	if err != nil {
		return "", err
	}
	r := &rewriter{
		sql:         sql,
		q:           q,
		tokens:      tokens,
		tableNames:  tableNames,
		columnNames: columnNames,
	}
	switch q.Type {
	case query.Select:
		err = r.rewriteSelect()
	case query.Insert:
		err = r.rewriteInsert()
	case query.Update:
		err = r.rewriteUpdate()
	case query.Delete:
		err = r.rewriteDelete()
	default:
		err = fmt.Errorf("unknown query type")
	}
	if err != nil {
		return "", err
	}
	if r.pos != len(r.tokens) {
		return "", fmt.Errorf("unexpected trailing tokens")
	}
	return r.apply(), nil
}

func (r *rewriter) rewriteSelect() error {
	if err := r.expectReserved("SELECT"); err != nil {
		return err
	}
	// SELECT field [AS alias] (, ...)*
	fieldIdx := 0
	for {
		tok, err := r.next()
		if err != nil {
			return err
		}
		if tok.kind != kindIdent {
			return fmt.Errorf("at SELECT: expected field")
		}
		if fieldIdx >= len(r.q.Fields) || tok.val != r.q.Fields[fieldIdx] {
			return fmt.Errorf("SELECT field mismatch at position %d", fieldIdx)
		}
		r.renameColumn(tok)
		fieldIdx++
		if r.peekReserved("AS") {
			if _, err := r.next(); err != nil {
				return err
			}
			alias, err := r.next()
			if err != nil {
				return err
			}
			if alias.kind != kindIdent {
				return fmt.Errorf("at SELECT: expected field alias")
			}
			if parsedAlias, ok := r.q.Aliases[tok.val]; !ok || alias.val != parsedAlias {
				return fmt.Errorf("alias mismatch for field %q", tok.val)
			}
			r.renameColumn(alias)
		}
		if r.peekReserved("FROM") {
			break
		}
		if err := r.expectReserved(","); err != nil {
			return err
		}
	}
	if fieldIdx != len(r.q.Fields) {
		return fmt.Errorf("SELECT field count mismatch")
	}
	if err := r.expectReserved("FROM"); err != nil {
		return err
	}
	if err := r.renameTable(); err != nil {
		return err
	}
	return r.consumeWhere()
}

func (r *rewriter) rewriteInsert() error {
	if err := r.expectReserved("INSERT INTO"); err != nil {
		return err
	}
	if err := r.renameTable(); err != nil {
		return err
	}
	if err := r.expectReserved("("); err != nil {
		return err
	}
	// Inserted column list.
	fieldIdx := 0
	for {
		tok, err := r.next()
		if err != nil {
			return err
		}
		if tok.kind != kindIdent {
			return fmt.Errorf("at INSERT INTO: expected field")
		}
		if fieldIdx >= len(r.q.Fields) || tok.val != r.q.Fields[fieldIdx] {
			return fmt.Errorf("INSERT column mismatch at position %d", fieldIdx)
		}
		r.renameColumn(tok)
		fieldIdx++
		if r.peekReserved(")") {
			r.pos++
			break
		}
		if err := r.expectReserved(","); err != nil {
			return err
		}
	}
	if fieldIdx != len(r.q.Fields) {
		return fmt.Errorf("INSERT column count mismatch")
	}
	if err := r.expectReserved("VALUES"); err != nil {
		return err
	}
	// One parenthesised row of quoted literals at a time; values are never
	// renamed, only walked over so offsets line up.
	for {
		if err := r.expectReserved("("); err != nil {
			return err
		}
		for {
			tok, err := r.next()
			if err != nil {
				return err
			}
			if tok.kind != kindQuoted {
				return fmt.Errorf("at INSERT INTO: expected quoted value")
			}
			if r.peekReserved(")") {
				r.pos++
				break
			}
			if err := r.expectReserved(","); err != nil {
				return err
			}
		}
		if !r.peekReserved(",") {
			break
		}
		r.pos++
	}
	return nil
}

func (r *rewriter) rewriteUpdate() error {
	if err := r.expectReserved("UPDATE"); err != nil {
		return err
	}
	if err := r.renameTable(); err != nil {
		return err
	}
	if err := r.expectReserved("SET"); err != nil {
		return err
	}
	// SET field = quoted-literal (, ...)*; the field is a column position,
	// the assigned literal is not.
	setIdx := 0
	for {
		field, err := r.next()
		if err != nil {
			return err
		}
		if field.kind != kindIdent {
			return fmt.Errorf("at UPDATE: expected field")
		}
		if _, ok := r.q.Updates[field.val]; !ok {
			return fmt.Errorf("SET field %q not found in parsed query", field.val)
		}
		r.renameColumn(field)
		setIdx++
		if err := r.expectReserved("="); err != nil {
			return err
		}
		value, err := r.next()
		if err != nil {
			return err
		}
		if value.kind != kindQuoted {
			return fmt.Errorf("at UPDATE: expected quoted value")
		}
		if value.val != r.q.Updates[field.val] {
			return fmt.Errorf("SET value mismatch for field %q", field.val)
		}
		if r.peekReserved("WHERE") {
			break
		}
		if err := r.expectReserved(","); err != nil {
			return err
		}
	}
	if setIdx != len(r.q.Updates) {
		return fmt.Errorf("SET field count mismatch")
	}
	return r.consumeWhere()
}

func (r *rewriter) rewriteDelete() error {
	if err := r.expectReserved("DELETE FROM"); err != nil {
		return err
	}
	if err := r.renameTable(); err != nil {
		return err
	}
	return r.consumeWhere()
}

// consumeWhere walks an optional WHERE clause, renaming exactly the operands
// that the parser classified as fields. Quoted literals are field positions
// nowhere in a condition, so they are only walked over.
func (r *rewriter) consumeWhere() error {
	if !r.peekReserved("WHERE") {
		if len(r.q.Conditions) != 0 {
			return fmt.Errorf("missing WHERE clause for %d parsed conditions", len(r.q.Conditions))
		}
		return nil
	}
	r.pos++
	conditionIdx := 0
	for {
		field, err := r.next()
		if err != nil {
			return err
		}
		if field.kind != kindIdent {
			return fmt.Errorf("at WHERE: expected field")
		}
		if conditionIdx >= len(r.q.Conditions) {
			return fmt.Errorf("more conditions than parsed")
		}
		condition := r.q.Conditions[conditionIdx]
		if !condition.Operand1IsField || field.val != condition.Operand1 {
			return fmt.Errorf("WHERE left operand mismatch at condition %d", conditionIdx)
		}
		r.renameColumn(field)
		operator, err := r.next()
		if err != nil {
			return err
		}
		if !isOperator(operator.val) {
			return fmt.Errorf("at WHERE: unknown operator")
		}
		operand, err := r.next()
		if err != nil {
			return err
		}
		if operand.kind != kindIdent && operand.kind != kindQuoted {
			return fmt.Errorf("at WHERE: expected value")
		}
		if operand.val != condition.Operand2 {
			return fmt.Errorf("WHERE right operand mismatch at condition %d", conditionIdx)
		}
		if condition.Operand2IsField {
			if operand.kind != kindIdent {
				return fmt.Errorf("parsed field right operand is not an identifier")
			}
			r.renameColumn(operand)
		}
		conditionIdx++
		if !r.peekWord("AND") {
			break
		}
		r.pos++
	}
	if conditionIdx != len(r.q.Conditions) {
		return fmt.Errorf("WHERE condition count mismatch")
	}
	return nil
}

func isOperator(s string) bool {
	switch s {
	case "=", "!=", ">", "<", ">=", "<=":
		return true
	}
	return false
}

// renameTable records an edit for the table name following FROM/INTO/UPDATE,
// cross-checking the scanned name against the parser result.
func (r *rewriter) renameTable() error {
	tok, err := r.next()
	if err != nil {
		return err
	}
	if tok.kind != kindQuoted && tok.kind != kindIdent {
		return fmt.Errorf("expected table name")
	}
	if tok.val != r.q.TableName {
		return fmt.Errorf("table name mismatch: scanned %q, parsed %q", tok.val, r.q.TableName)
	}
	if newName, ok := r.tableNames[tok.val]; ok {
		r.edits = append(r.edits, edit{
			start:       tok.start,
			end:         tok.end,
			replacement: r.replacement(tok, newName),
		})
	}
	return nil
}

// renameColumn records an edit when tok holds a name present in the column
// mapping. Quoted column positions keep their quoting style.
func (r *rewriter) renameColumn(tok token) {
	newName, ok := r.columnNames[tok.val]
	if !ok {
		return
	}
	r.edits = append(r.edits, edit{
		start:       tok.start,
		end:         tok.end,
		replacement: r.replacement(tok, newName),
	})
}

// replacement renders newName in the quoting style of the original token.
func (r *rewriter) replacement(tok token, newName string) string {
	if tok.kind == kindQuoted {
		return "'" + newName + "'"
	}
	return newName
}

// apply performs the recorded edits on the original bytes. With no edits the
// statement is returned untouched, which keeps the byte-identical guarantee.
func (r *rewriter) apply() string {
	sort.SliceStable(r.edits, func(i, j int) bool { return r.edits[i].start < r.edits[j].start })
	var b strings.Builder
	cursor := 0
	for _, e := range r.edits {
		b.WriteString(r.sql[cursor:e.start])
		b.WriteString(e.replacement)
		cursor = e.end
	}
	b.WriteString(r.sql[cursor:])
	return b.String()
}

func (r *rewriter) next() (token, error) {
	if r.pos >= len(r.tokens) {
		return token{}, fmt.Errorf("unexpected end of statement")
	}
	tok := r.tokens[r.pos]
	r.pos++
	return tok, nil
}

func (r *rewriter) peekReserved(word string) bool {
	if r.pos >= len(r.tokens) {
		return false
	}
	tok := r.tokens[r.pos]
	return tok.kind == kindReserved && tok.val == word
}

// peekWord matches a case-insensitive keyword. The parser keeps AND out of
// its reserved-word list, so it arrives as an identifier token and needs an
// upper-case comparison rather than a reserved-token match.
func (r *rewriter) peekWord(word string) bool {
	if r.pos >= len(r.tokens) {
		return false
	}
	tok := r.tokens[r.pos]
	if tok.kind == kindReserved {
		return tok.val == word
	}
	if tok.kind == kindIdent {
		return strings.ToUpper(tok.val) == word
	}
	return false
}

func (r *rewriter) expectReserved(word string) error {
	tok, err := r.next()
	if err != nil {
		return err
	}
	if tok.kind != kindReserved || tok.val != word {
		return fmt.Errorf("expected %s", word)
	}
	return nil
}
