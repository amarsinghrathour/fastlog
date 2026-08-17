package fastlog

import (
	"fmt"
	"strconv"

	internalformat "github.com/amarsinghrathour/fastlog/internal/format"
)

func (l *Logger) formatTextMessage(e *entry) []byte {
	if e == nil {
		return nil
	}

	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	} else {
		e.buf = e.buf[:0]
		if cap(e.buf) < 256 {
			e.buf = make([]byte, 0, 256)
		}
	}

	if e.levelStr == "" {
		e.levelStr = e.level.String()
	}

	if e.timestamp != nil {
		e.buf = append(e.buf, e.timestamp...)
	}

	e.buf = append(e.buf, " ["...)
	if e.levelStr != "" {
		e.buf = append(e.buf, e.levelStr...)
	} else {

		e.buf = append(e.buf, e.level.String()...)
	}
	e.buf = append(e.buf, "] "...)

	if e.format != "" {

		e.buf = fmt.Appendf(e.buf, e.format, e.args...)
	} else {

		for i, arg := range e.args {
			if i > 0 {
				e.buf = append(e.buf, ' ')
			}
			e.buf = internalformat.AppendValue(e.buf, arg)
		}
	}

	if len(e.fields) > 0 {
		e.buf = append(e.buf, " fields=["...)
		for i, f := range e.fields {
			if i > 0 {
				e.buf = append(e.buf, ' ')
			}
			e.buf = append(e.buf, f.Key...)
			e.buf = append(e.buf, '=')

			switch f.Kind {
			case FieldString:
				e.buf = append(e.buf, f.Str...)
			case FieldInt, FieldInt64:
				e.buf = strconv.AppendInt(e.buf, f.Int, 10)
			case FieldUint, FieldUint64:
				e.buf = strconv.AppendUint(e.buf, f.Uint, 10)
			case FieldFloat64:
				e.buf = strconv.AppendFloat(e.buf, f.Float, 'f', -1, 64)
			case FieldBool:
				e.buf = strconv.AppendBool(e.buf, f.Bool)
			case FieldBytes:
				e.buf = append(e.buf, f.Bytes...)
			}
		}
		e.buf = append(e.buf, ']')
	}

	if e.caller != "" {
		e.buf = append(e.buf, " ["...)
		e.buf = append(e.buf, e.caller...)
		e.buf = append(e.buf, ']')
	}

	e.buf = append(e.buf, '\n')

	return e.buf
}

func (l *Logger) formatJSONMessage(e *entry) []byte {
	if e == nil {
		return nil
	}

	if e.buf == nil {
		e.buf = make([]byte, 0, 512)
	} else {
		e.buf = e.buf[:0]
		if cap(e.buf) < 512 {
			e.buf = make([]byte, 0, 512)
		}
	}

	e.buf = append(e.buf, '{')

	e.buf = append(e.buf, `"timestamp":"`...)
	if e.timestamp != nil {
		e.buf = append(e.buf, e.timestamp...)
	}
	e.buf = append(e.buf, '"')

	e.buf = append(e.buf, `,"level":"`...)
	if e.levelStr != "" {
		e.buf = append(e.buf, e.levelStr...)
	} else {
		e.buf = append(e.buf, e.level.String()...)
	}
	e.buf = append(e.buf, '"')

	e.buf = append(e.buf, `,"message":`...)

	if e.format != "" {

		formatted := fmt.Sprintf(e.format, e.args...)
		e.buf = internalformat.AppendJSONString(e.buf, formatted)
	} else if len(e.args) == 0 {
		e.buf = append(e.buf, `null`...)
	} else if len(e.args) == 1 {

		e.buf = internalformat.AppendJSONValue(e.buf, e.args[0])
	} else {

		e.buf = append(e.buf, '[')
		for i, arg := range e.args {
			if i > 0 {
				e.buf = append(e.buf, ',')
			}
			e.buf = internalformat.AppendJSONValue(e.buf, arg)
		}
		e.buf = append(e.buf, ']')
	}

	if len(e.fields) > 0 {
		e.buf = append(e.buf, `,"fields":{`...)
		for i, f := range e.fields {
			if i > 0 {
				e.buf = append(e.buf, ',')
			}

			e.buf = internalformat.AppendJSONString(e.buf, f.Key)
			e.buf = append(e.buf, ':')

			switch f.Kind {
			case FieldString:
				e.buf = internalformat.AppendJSONString(e.buf, f.Str)
			case FieldInt, FieldInt64:
				e.buf = strconv.AppendInt(e.buf, f.Int, 10)
			case FieldUint, FieldUint64:
				e.buf = strconv.AppendUint(e.buf, f.Uint, 10)
			case FieldFloat64:
				e.buf = strconv.AppendFloat(e.buf, f.Float, 'f', -1, 64)
			case FieldBool:
				e.buf = strconv.AppendBool(e.buf, f.Bool)
			case FieldBytes:

				e.buf = internalformat.AppendJSONString(e.buf, string(f.Bytes))
			}
		}
		e.buf = append(e.buf, '}')
	}

	if e.caller != "" {
		e.buf = append(e.buf, `,"caller":"`...)
		e.buf = internalformat.AppendJSONString(e.buf, e.caller)
		e.buf = append(e.buf, '"')
	}

	e.buf = append(e.buf, '}')

	return e.buf
}
