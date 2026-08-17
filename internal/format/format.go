package format

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// AppendValue appends a common Go value to a byte buffer.
func AppendValue(buf []byte, v interface{}) []byte {
	switch val := v.(type) {
	case string:
		return append(buf, val...)
	case int:
		return strconv.AppendInt(buf, int64(val), 10)
	case int8:
		return strconv.AppendInt(buf, int64(val), 10)
	case int16:
		return strconv.AppendInt(buf, int64(val), 10)
	case int32:
		return strconv.AppendInt(buf, int64(val), 10)
	case int64:
		return strconv.AppendInt(buf, val, 10)
	case uint:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint8:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint16:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint64:
		return strconv.AppendUint(buf, val, 10)
	case float32:
		return strconv.AppendFloat(buf, float64(val), 'f', -1, 32)
	case float64:
		return strconv.AppendFloat(buf, val, 'f', -1, 64)
	case bool:
		return strconv.AppendBool(buf, val)
	case []byte:
		return append(buf, val...)
	default:
		return append(buf, fmt.Sprint(val)...)
	}
}

// AppendJSONString appends a JSON-escaped string (including quotes) to buffer.
func AppendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf = append(buf, `\"`...)
		case '\\':
			buf = append(buf, `\\`...)
		case '\n':
			buf = append(buf, `\n`...)
		case '\r':
			buf = append(buf, `\r`...)
		case '\t':
			buf = append(buf, `\t`...)
		case '\b':
			buf = append(buf, `\b`...)
		case '\f':
			buf = append(buf, `\f`...)
		default:
			if c < 0x20 {
				buf = append(buf, `\u00`...)
				buf = append(buf, hexChar(c>>4))
				buf = append(buf, hexChar(c&0x0f))
			} else {
				buf = append(buf, c)
			}
		}
	}
	buf = append(buf, '"')
	return buf
}

// AppendJSONValue appends an arbitrary value as JSON.
func AppendJSONValue(buf []byte, v interface{}) []byte {
	switch val := v.(type) {
	case nil:
		return append(buf, "null"...)
	case string:
		return AppendJSONString(buf, val)
	case int:
		return strconv.AppendInt(buf, int64(val), 10)
	case int64:
		return strconv.AppendInt(buf, val, 10)
	case int32:
		return strconv.AppendInt(buf, int64(val), 10)
	case int16:
		return strconv.AppendInt(buf, int64(val), 10)
	case int8:
		return strconv.AppendInt(buf, int64(val), 10)
	case uint:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint64:
		return strconv.AppendUint(buf, val, 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint16:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint8:
		return strconv.AppendUint(buf, uint64(val), 10)
	case float64:
		return strconv.AppendFloat(buf, val, 'f', -1, 64)
	case float32:
		return strconv.AppendFloat(buf, float64(val), 'f', -1, 32)
	case bool:
		return strconv.AppendBool(buf, val)
	case []byte:
		return AppendJSONString(buf, string(val))
	default:
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return AppendJSONString(buf, fmt.Sprintf("%v", val))
		}
		return append(buf, jsonBytes...)
	}
}

// AppendRFC3339Timestamp appends RFC3339 timestamp bytes.
func AppendRFC3339Timestamp(buf []byte, t time.Time) []byte {
	var tsBuf [64]byte
	tsBytes := t.AppendFormat(tsBuf[:0], time.RFC3339)
	return append(buf, tsBytes...)
}

func hexChar(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + (v - 10)
}
