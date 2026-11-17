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
			name:    "SELECT fields are renamed but literals are not",
			sql:     "SELECT a, b FROM 't1' WHERE a > '1'",
			tables:  map[string]string{"t1": "t2"},
			columns: map[string]string{"a": "c"},
			want:    "SELECT c, b FROM 't2' WHERE a > '1'",
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
