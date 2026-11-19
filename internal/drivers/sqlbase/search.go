package sqlbase

import (
	"context"
	"strings"
)

// Searcher lets a dialect say how to match free text against any column.
type Searcher interface {
	// SearchExpr returns a boolean SQL expression testing whether quotedCol,
	// rendered as text, contains term (case-insensitively where possible).
	SearchExpr(quotedCol, term string) string
}

// QuoteLike renders %term% as a string literal. backslashes doubles
// backslashes for dialects that treat them as escapes (MySQL).
func QuoteLike(term string, backslashes bool) string {
	if backslashes {
		term = strings.ReplaceAll(term, `\`, `\\`)
	}
	return "'%" + strings.ReplaceAll(term, "'", "''") + "%'"
}

// searchClause builds "(c1 matches OR c2 matches …)" for every column of ref.
func (e *Engine) searchClause(ctx context.Context, q Querier, ref, term string) (string, error) {
	s, ok := e.Dialect.(Searcher)
	if !ok {
		return "", nil
	}
	// Discover columns without reading data.
	rows, err := q.QueryContext(ctx, "SELECT * FROM "+ref+" WHERE 1 = 0")
	if err != nil {
		return "", err
	}
	cols, err := rows.Columns()
	rows.Close()
	if err != nil || len(cols) == 0 {
		return "", err
	}
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = s.SearchExpr(e.Dialect.Quote(c), term)
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}
