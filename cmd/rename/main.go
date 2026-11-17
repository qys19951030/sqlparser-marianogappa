// Command rename rewrites table and column names in a SQL statement.
//
// The statement is read from standard input, or from the file named by -f.
// The -t and -c flags, each repeatable, supply old=new table and column
// renames. On success the rewritten statement is printed to standard output;
// on failure a single diagnostic line is written to standard error and the
// process exits with a non-zero status.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/marianogappa/sqlparser"
	"github.com/marianogappa/sqlparser/query"
	"github.com/marianogappa/sqlparser/rename"
)

// stringList is a flag.Value that collects every occurrence of a flag.
type stringList []string

func (l *stringList) String() string { return fmt.Sprintf("%v", []string(*l)) }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var tableFlags, columnFlags stringList
	fs.Var(&tableFlags, "t", "rename a table, format old=new (repeatable)")
	fs.Var(&columnFlags, "c", "rename a column, format old=new (repeatable)")
	filePath := fs.String("f", "", "read the SQL statement from `file` instead of standard input")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	tableNames, err := buildMapping(tableFlags)
	if err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 2
	}
	columnNames, err := buildMapping(columnFlags)
	if err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 2
	}

	var input []byte
	if *filePath != "" {
		input, err = ioutil.ReadFile(*filePath)
	} else {
		input, err = ioutil.ReadAll(in)
	}
	if err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 1
	}

	q, err := parseSilent(string(input))
	if err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 1
	}

	rewritten, err := rename.Rename(string(input), q, tableNames, columnNames)
	if err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 1
	}

	if _, err := fmt.Fprint(out, rewritten); err != nil {
		fmt.Fprintf(errOut, "rename: %v\n", err)
		return 1
	}
	return 0
}

// buildMapping turns repeatable old=new flag values into a name map.
func buildMapping(pairs []string) (map[string]string, error) {
	m := map[string]string{}
	for _, pair := range pairs {
		idx := strings.IndexByte(pair, '=')
		if idx <= 0 || idx == len(pair)-1 {
			return nil, fmt.Errorf("invalid mapping %q, expected old=new", pair)
		}
		m[pair[:idx]] = pair[idx+1:]
	}
	return m, nil
}

// parseSilent runs sqlparser.Parse while suppressing its diagnostics, which
// the parser writes to standard output. The rename command reports parse
// errors on standard error instead and never touches standard output on
// failure.
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
