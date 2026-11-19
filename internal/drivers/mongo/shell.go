package mongo

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// call is one `.name(args)` segment of a shell expression.
type call struct {
	Name string
	Args string // raw source between the parentheses
}

// shellExpr is a parsed `db.<collection>.<method>(...).<chain>(...)` expression.
type shellExpr struct {
	Collection string
	Calls      []call
}

// parseShell understands the subset of mongosh syntax people actually type:
//
//	db.users.find({ age: { $gt: 30 } }).sort({ name: 1 }).limit(10)
//	db.getCollection("weird.name").aggregate([...])
func parseShell(src string) (*shellExpr, error) {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(src), ";"))
	if !strings.HasPrefix(s, "db.") {
		return nil, errors.New(`expected an expression like db.collection.find({...})`)
	}
	s = s[3:]
	expr := &shellExpr{}
	if strings.HasPrefix(s, "getCollection(") {
		args, rest, err := balanced(s[len("getCollection"):])
		if err != nil {
			return nil, err
		}
		expr.Collection = strings.Trim(strings.TrimSpace(args), `"'`)
		s = rest
	} else {
		i := 0
		for i < len(s) && s[i] != '.' && s[i] != '(' {
			i++
		}
		expr.Collection, s = s[:i], s[i:]
	}
	for len(strings.TrimSpace(s)) > 0 {
		s = strings.TrimSpace(s)
		if s[0] != '.' {
			return nil, fmt.Errorf("unexpected %q", s)
		}
		s = s[1:]
		i := 0
		for i < len(s) && (unicode.IsLetter(rune(s[i])) || unicode.IsDigit(rune(s[i])) || s[i] == '_') {
			i++
		}
		name := s[:i]
		args, rest, err := balanced(s[i:])
		if err != nil {
			return nil, err
		}
		expr.Calls = append(expr.Calls, call{Name: name, Args: args})
		s = rest
	}
	if expr.Collection == "" || len(expr.Calls) == 0 {
		return nil, errors.New("expected db.collection.method(...)")
	}
	return expr, nil
}

// balanced reads "( ... )" from the start of s, honouring nested brackets and
// string literals, and returns the inner text plus the remainder.
func balanced(s string) (inner, rest string, err error) {
	s = strings.TrimLeft(s, " \t\r\n")
	if s == "" || s[0] != '(' {
		return "", "", errors.New("expected '('")
	}
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return s[1:i], s[i+1:], nil
			}
		}
	}
	return "", "", errors.New("unbalanced parentheses")
}

// shellHelpers maps mongosh constructors onto Extended JSON.
var shellHelpers = map[string]string{
	"ObjectId": "$oid", "ISODate": "$date", "NumberLong": "$numberLong",
	"NumberInt": "$numberInt", "NumberDecimal": "$numberDecimal", "Date": "$date",
}

// toJSON converts relaxed JS object syntax (unquoted keys, single quotes,
// trailing commas, ObjectId("…")) into strict Extended JSON.
func toJSON(src string) (string, error) {
	var b strings.Builder
	n := len(src)
	for i := 0; i < n; i++ {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			b.WriteByte('"')
			for ; j < n && src[j] != c; j++ {
				if src[j] == '\\' && j+1 < n {
					b.WriteByte(src[j])
					j++
					b.WriteByte(src[j])
					continue
				}
				if c == '\'' && src[j] == '"' {
					b.WriteString(`\"`)
					continue
				}
				b.WriteByte(src[j])
			}
			if j >= n {
				return "", errors.New("unterminated string")
			}
			b.WriteByte('"')
			i = j
		case c == '$' || c == '_' || unicode.IsLetter(rune(c)):
			j := i
			for j < n && (src[j] == '$' || src[j] == '_' || src[j] == '.' || unicode.IsLetter(rune(src[j])) || unicode.IsDigit(rune(src[j]))) {
				j++
			}
			word := src[i:j]
			k := j
			for k < n && unicode.IsSpace(rune(src[k])) {
				k++
			}
			switch {
			case k < n && src[k] == ':':
				fmt.Fprintf(&b, "%q", word)
			case k < n && src[k] == '(':
				key, ok := shellHelpers[strings.TrimPrefix(word, "new ")]
				if !ok {
					return "", fmt.Errorf("unsupported function %s()", word)
				}
				inner, rest, err := balanced(src[k:])
				if err != nil {
					return "", err
				}
				arg := strings.TrimSpace(inner)
				if arg == "" && key == "$date" {
					arg = `"` + time.Now().UTC().Format(time.RFC3339) + `"`
				}
				conv, err := toJSON(arg)
				if err != nil {
					return "", err
				}
				if !strings.HasPrefix(conv, `"`) {
					conv = `"` + conv + `"`
				}
				fmt.Fprintf(&b, `{"%s":%s}`, key, conv)
				i = n - len(rest) - 1
				continue
			case word == "new":
				// `new Date(...)` — drop the keyword.
			case word == "undefined":
				b.WriteString("null")
			default:
				b.WriteString(word)
			}
			i = j - 1
		case c == ',':
			k := i + 1
			for k < n && unicode.IsSpace(rune(src[k])) {
				k++
			}
			if k < n && (src[k] == '}' || src[k] == ']') {
				continue // trailing comma
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
