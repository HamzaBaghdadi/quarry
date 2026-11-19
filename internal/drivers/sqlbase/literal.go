package sqlbase

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// LiteralStyle captures how a dialect spells values in SQL text.
type LiteralStyle struct {
	TrueFalse    [2]string // e.g. {"true","false"} or {"1","0"}
	Bytes        func(b []byte) string
	Backslashes  bool   // escape backslashes inside strings (MySQL)
	TimeLayout   string // layout for time.Time
	QuoteNaN     bool   // write NaN/Infinity as quoted strings
	NationalText bool   // N'…' prefix (SQL Server)
}

// Literal renders a database/sql driver value as a SQL literal.
func (s LiteralStyle) Literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return s.TrueFalse[0]
		}
		return s.TrueFalse[1]
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return s.Literal(float64(x))
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			if s.QuoteNaN {
				return s.Str(map[bool]string{true: "NaN"}[math.IsNaN(x)] + infName(x))
			}
			return "NULL"
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []byte:
		return s.Bytes(x)
	case time.Time:
		return s.Str(x.Format(s.TimeLayout))
	case string:
		return s.Str(x)
	case fmt.Stringer:
		return s.Str(x.String())
	}
	return s.Str(fmt.Sprint(v))
}

func infName(x float64) string {
	switch {
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	return ""
}

// Str quotes a string literal.
func (s LiteralStyle) Str(v string) string {
	if s.Backslashes {
		v = strings.ReplaceAll(v, `\`, `\\`)
	}
	v = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	if s.NationalText {
		return "N" + v
	}
	return v
}

// HexBytes renders X'…' (MySQL, SQLite).
func HexBytes(b []byte) string { return "X'" + hex.EncodeToString(b) + "'" }
