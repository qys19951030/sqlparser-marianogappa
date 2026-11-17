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
			name: "table name absent from mapping is untouched",
			sql:  "SELECT z FROM 't1' WHERE z > '1'",
			want: "SELECT z FROM 't1' WHERE z > '1'",
		},
		{
			name:   "no mappings returns the statement byte for byte",
			sql:    "SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
			tables: map[string]string{},
			want:   "SELECT a, c, d FROM 'b' WHERE a != '1' AND b = '2'",
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
