package driver

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"
	"unicode/utf8"
)

// Normalize converts a driver-native value into something that survives the
// JSON bridge to the UI without losing meaning.
func Normalize(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(x) {
			return string(x)
		}
		return "\\x" + hex.EncodeToString(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Sprint(x)
		}
		return x
	case float32:
		return Normalize(float64(x))
	case int64:
		// JS numbers lose precision past 2^53; send big ints as strings.
		if x > 1<<53 || x < -(1<<53) {
			return fmt.Sprint(x)
		}
		return x
	case uint64:
		if x > 1<<53 {
			return fmt.Sprint(x)
		}
		return x
	case *big.Int:
		return x.String()
	case string, bool, int, int8, int16, int32, uint, uint8, uint16, uint32:
		return x
	case json.RawMessage:
		return string(x)
	case fmt.Stringer:
		return x.String()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return Normalize(rv.Elem().Interface())
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		b, err := json.Marshal(v)
		if err == nil {
			return string(b)
		}
	}
	return fmt.Sprint(v)
}
