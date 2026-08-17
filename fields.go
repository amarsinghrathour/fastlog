package fastlog

// FieldKind represents the type of a field value.
type FieldKind uint8

const (
	FieldString FieldKind = iota
	FieldInt
	FieldInt64
	FieldUint
	FieldUint64
	FieldFloat64
	FieldBool
	FieldBytes
)

// Field represents a typed log field.
type Field struct {
	Key   string
	Kind  FieldKind
	Str   string
	Int   int64
	Uint  uint64
	Float float64
	Bool  bool
	Bytes []byte
}

// String creates a string field.
func String(key, val string) Field {
	return Field{Key: key, Kind: FieldString, Str: val}
}

// Int creates an int field.
func Int(key string, val int) Field {
	return Field{Key: key, Kind: FieldInt, Int: int64(val)}
}

// Int64 creates an int64 field.
func Int64(key string, val int64) Field {
	return Field{Key: key, Kind: FieldInt64, Int: val}
}

// Uint creates a uint field.
func Uint(key string, val uint) Field {
	return Field{Key: key, Kind: FieldUint, Uint: uint64(val)}
}

// Uint64 creates a uint64 field.
func Uint64(key string, val uint64) Field {
	return Field{Key: key, Kind: FieldUint64, Uint: val}
}

// Float64 creates a float64 field.
func Float64(key string, val float64) Field {
	return Field{Key: key, Kind: FieldFloat64, Float: val}
}

// Bool creates a bool field.
func Bool(key string, val bool) Field {
	return Field{Key: key, Kind: FieldBool, Bool: val}
}

// Bytes creates a bytes field.
func Bytes(key string, val []byte) Field {
	return Field{Key: key, Kind: FieldBytes, Bytes: val}
}
