package core

// DispatchOptions configures log entry dispatch behavior.
type DispatchOptions struct {
	SyncMode   bool
	JSONFormat bool
	UseRing    bool

	TryPushRing func() bool
	WriteSync   func()
	FormatJSON  func() []byte
	FormatText  func() []byte
	PushBytes   func([]byte)
	Release     func()
}

// DispatchEntry orchestrates sync/ring/channel fallback dispatch.
func DispatchEntry(opts DispatchOptions) {
	if opts.SyncMode {
		opts.WriteSync()
		opts.Release()
		return
	}

	if opts.UseRing && opts.TryPushRing != nil && opts.TryPushRing() {
		return
	}

	var msgBytes []byte
	if opts.JSONFormat {
		jsonBytes := opts.FormatJSON()
		if jsonBytes == nil {
			opts.Release()
			return
		}
		msgBytes = make([]byte, len(jsonBytes)+1)
		copy(msgBytes, jsonBytes)
		msgBytes[len(jsonBytes)] = '\n'
		opts.Release()
	} else {
		textBytes := opts.FormatText()
		if textBytes == nil {
			opts.Release()
			return
		}
		msgBytes = append([]byte(nil), textBytes...)
		opts.Release()
	}

	opts.PushBytes(msgBytes)
}
