package rename

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	sqlparser "github.com/marianogappa/sqlparser"
	"github.com/marianogappa/sqlparser/query"
)

type renameCase struct {
	name    string
	sql     string
	tables  map[string]string
	columns map[string]string
	want    string
}

func TestRename(t *testing.T) {
	cases := []renameCase{
		{
			name:   "table in SELECT FROM is renamed",
			sql:    "SELECT a, b FROM 't1' WHERE a > '1'",
			tables: map[string]string{"t1": "t2"},
			want:   "SELECT a, b FROM 't2' WHERE a > '1'",
		},
		{
			name:   "table in INSERT INTO is renamed",
			sql:    "INSERT INTO 't1' (a, b) VALUES ('1', '2')",
			tables: map[string]string{"t1": "t2"},
			want:   "INSERT INTO 't2' (a, b) VALUES ('1', '2')",
		},
		{
			name:   "table in UPDATE is renamed",
			sql:    "UPDATE 't1' SET a = 'x' WHERE x = '2'",
			tables: map[string]string{"t1": "t2"},
			want:   "UPDATE 't2' SET a = 'x' WHERE x = '2'",
		},
		{
			name:   "table in DELETE FROM is renamed",
			sql:    "DELETE FROM 't1' WHERE a = '2'",
			tables: map[string]string{"t1": "t2"},
			want:   "DELETE FROM 't2' WHERE a = '2'",
		},
		{
			name:   "no mappings returns the statement byte for byte",
			sql:    "SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
			tables: map[string]string{},
			want:   "SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
		},
		{
			name:    "SELECT fields and WHERE fields are renamed, literals are not",
			sql:     "SELECT a, b FROM 't1' WHERE a > '1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "SELECT c, b FROM 't2' WHERE c > '1'",
		},
		{
			name:    "SET literal named like a WHERE field stays untouched",
			sql:     "UPDATE 't1' SET a = 'x' WHERE x = '2'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"x": "y"},
			want:    "UPDATE 't2' SET a = 'x' WHERE y = '2'",
		},
		{
			name:    "field-typed right operand is renamed, quoted operand is not",
			sql:     "SELECT a, c, d FROM 'b' WHERE a != b",
			columns: map[string]string{"a": "aa", "b": "bb"},
			want:    "SELECT aa, c, d FROM 'b' WHERE aa != bb",
		},
		{
			name:    "every condition field is renamed across AND chains",
			sql:     "SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
			tables:  map[string]string{"b": "t2"},
			columns: map[string]string{"a": "x", "b": "y"},
			want:    "SELECT x, c, d FROM 't2' WHERE x != '1' AND y = '2'",
		},
		{
			name:    "INSERT columns are renamed, values stay literal",
			sql:     "INSERT INTO 't1' (a, b) VALUES ('1', '2')",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "INSERT INTO 't2' (c, b) VALUES ('1', '2')",
		},
		{
			name:    "SELECT alias is renamed through the column mapping",
			sql:     "SELECT a AS x FROM 't1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"x": "y"},
			want:    "SELECT a AS y FROM 't2'",
		},
		{
			name:    "UPDATE SET field is renamed, assigned literal is not",
			sql:     "UPDATE 't1' SET a = '1' WHERE b = '2'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "z"},
			want:    "UPDATE 't2' SET z = '1' WHERE b = '2'",
		},
		{
			name:    "same name is resolved independently at table and column",
			sql:     "SELECT a FROM 'a'",
			tables:  map[string]string{"a": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "SELECT c FROM 't2'",
		},
		{
			name:    "multiple aliases and fields are renamed independently",
			sql:     "SELECT a as z, b as y, c FROM 'b'",
			columns: map[string]string{"z": "zz", "y": "yy"},
			want:    "SELECT a as zz, b as yy, c FROM 'b'",
		},
		{
			name:    "column absent from mapping is untouched",
			sql:     "SELECT z FROM 't1' WHERE z > '1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"other": "x"},
			want:    "SELECT z FROM 't2' WHERE z > '1'",
		},
		{
			name:    "escaped quote inside a literal keeps the literal intact",
			sql:     "UPDATE 'a' SET b = 'hello\\'world' WHERE a = '1'",
			tables:  map[string]string{"a": "t2"},
			columns: map[string]string{"b": "c"},
			want:    "UPDATE 't2' SET c = 'hello\\'world' WHERE a = '1'",
		},
		{
			name:    "irregular spacing is copied verbatim",
			sql:     "INSERT INTO 'a' (b,c,    d) VALUES ('1','2' ,  '3' ),('4','5' ,'6' )",
			tables:  map[string]string{"a": "t2"},
			columns: map[string]string{"b": "bb", "c": "cc", "d": "dd"},
			want:    "INSERT INTO 't2' (bb,cc,    dd) VALUES ('1','2' ,  '3' ),('4','5' ,'6' )",
		},
		{
			name:    "lowercase keyword casing is preserved",
			sql:     "select a FROM 't1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "select c FROM 't2'",
		},
		{
			name:    "a name inside a string literal is never replaced",
			sql:     "SELECT a FROM 't1' WHERE a = 't1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "SELECT c FROM 't2' WHERE c = 't1'",
		},
		{
			name:    "SELECT wildcard is not treated as a column name",
			sql:     "SELECT a, * FROM 'b'",
			tables:  map[string]string{"b": "t2"},
			columns: map[string]string{"a": "c", "*": "x"},
			want:    "SELECT c, * FROM 't2'",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseSilent(tc.sql)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			got, err := Rename(tc.sql, q, tc.tables, tc.columns)
			if err != nil {
				t.Fatalf("Rename returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Rename() = %q, want %q", got, tc.want)
			}
		})
	}
}

// parseSilent parses sql while suppressing the parser's standard-output
// diagnostics, mirroring how the command wrapper invokes the parser.
func parseSilent(sql string) (query.Query, error) {
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return query.Query{}, err
	}
	os.Stdout = w
	q, parseErr := sqlparser.Parse(sql)
	_ = w.Close()
	os.Stdout = original
	_, _ = io.Copy(ioutil.Discard, r)
	_ = r.Close()
	return q, parseErr
}

func TestRenameIdempotentAndDeterministic(t *testing.T) {
	statements := []string{
		"SELECT a, b FROM 't1' WHERE a > '1'",
		"INSERT INTO 't1' (a, b) VALUES ('1', '2')",
		"UPDATE 't1' SET a = 'x' WHERE x = '2'",
		"SELECT a AS x FROM 't1'",
		"DELETE FROM 't1' WHERE a = '2'",
		"UPDATE 't1' SET a = '1' WHERE b = '2'",
		"SELECT a FROM 'a'",
		"select a FROM 't1'",
		"SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
	}
	tables := map[string]string{"t1": "t2", "a": "t2", "b": "tt"}
	columns := map[string]string{"a": "c", "x": "y", "b": "z"}
	for _, sql := range statements {
		// Each pass re-parses the previous output: the parsed q always
		// describes the current text, and this also proves the rewrite
		// remains a statement the parser accepts.
		current := sql
		var first string
		for pass := 0; pass < 3; pass++ {
			q, err := parseSilent(current)
			if err != nil {
				t.Fatalf("%q: unexpected parse error: %v", current, err)
			}
			got, err := Rename(current, q, tables, columns)
			if err != nil {
				t.Fatalf("pass %d on %q: Rename failed: %v", pass, current, err)
			}
			if pass == 0 {
				first = got
			} else if got != first {
				t.Fatalf("%q: not stable across passes:\n first = %q\n later = %q", sql, first, got)
			}
			current = got
		}
	}
}

func TestEmptyMapsAreByteIdentical(t *testing.T) {
	// Every statement the parser accepts must come back untouched when no
	// renames are requested, including whitespace and casing quirks.
	statements := []string{
		"SELECT a FROM 'b'",
		"select a fRoM 'b'",
		"SELECT a, c, d FROM 'b'",
		"SELECT a as z, b as y, c FROM 'b'",
		"SELECT a, c, d FROM 'b' WHERE a = ''",
		"SELECT a, c, d FROM 'b' WHERE a < '1'",
		"SELECT a, c, d FROM 'b' WHERE a <= '1'",
		"SELECT a, c, d FROM 'b' WHERE a > '1'",
		"SELECT a, c, d FROM 'b' WHERE a >= '1'",
		"SELECT a, c, d FROM 'b' WHERE a != '1'",
		"SELECT a, c, d FROM 'b' WHERE a != b",
		"SELECT * FROM 'b'",
		"SELECT a, * FROM 'b'",
		"SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
		"UPDATE 'a' SET b = 'hello' WHERE a = '1'",
		"UPDATE 'a' SET b = 'hello\\'world' WHERE a = '1'",
		"UPDATE 'a' SET b = 'hello', c = 'bye' WHERE a = '1'",
		"UPDATE 'a' SET b = 'hello', c = 'bye' WHERE a = '1' AND b = '789'",
		"DELETE FROM 'a' WHERE b = '1'",
		"INSERT INTO 'a' (b) VALUES ('1')",
		"INSERT INTO 'a' (b,c,    d) VALUES ('1','2' ,  '3' )",
		"INSERT INTO 'a' (b,c,    d) VALUES ('1','2' ,  '3' ),('4','5' ,'6' )",
	}
	for _, sql := range statements {
		q, err := parseSilent(sql)
		if err != nil {
			t.Fatalf("%q: unexpected parse error: %v", sql, err)
		}
		got, err := Rename(sql, q, map[string]string{}, map[string]string{})
		if err != nil {
			t.Fatalf("%q: Rename failed: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: empty mappings changed the statement:\n got  = %q\n want = %q", sql, got, sql)
		}
	}
}
