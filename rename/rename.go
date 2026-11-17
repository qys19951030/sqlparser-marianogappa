// Package rename rewrites SQL statements by replacing table and column names
// at their syntactic positions. Every byte that is not a replaced name is
// preserved verbatim, including whitespace, casing, quoting and literals.
package rename

import (
	"github.com/marianogappa/sqlparser/query"
)

// Rename returns sql with table and column names replaced according to
// tableNames and columnNames. Only the names at the syntactic positions
// recorded in q are eligible for replacement; keywords and literals are never
// touched. A name absent from the relevant mapping stays as it is. When both
// maps contain the same name, each position uses the map for its own role.
func Rename(sql string, q query.Query, tableNames, columnNames map[string]string) (string, error) {
	// TODO: table names (FROM/INTO/UPDATE, DELETE via FROM)
	// TODO: SELECT fields and aliases, INSERT columns, UPDATE SET fields
	// TODO: WHERE condition fields marked as fields in q
	// TODO: quoted-name and string-literal boundaries
	return sql, nil
}
