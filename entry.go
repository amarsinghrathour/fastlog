package fastlog

import (
	"sync"
)

var entryPool = sync.Pool{
	New: func() interface{} {
		return &entry{
			buf:       make([]byte, 0, 256),
			timestamp: make([]byte, 0, 64),
			fields:    make([]Field, 0, 8),
		}
	},
}

type entry struct {
	buf       []byte // Reusable buffer for formatting
	timestamp []byte // Cached timestamp bytes
	level     LogLevel
	levelStr  string
	caller    string
	fields    []Field
	args      []interface{} // Still interface{} for variadic args, but optimized
	format    string
}

func (e *entry) Reset() {
	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	} else {
		e.buf = e.buf[:0]
		if cap(e.buf) < 256 {
			e.buf = make([]byte, 0, 256)
		}
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	} else {
		e.timestamp = e.timestamp[:0]
	}
	e.level = 0
	e.levelStr = ""
	e.caller = ""
	e.format = ""
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	} else {
		e.fields = e.fields[:0]
	}
	e.args = nil
}
