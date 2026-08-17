package core

import (
	"time"
)

// RingReader is the minimal ring behavior needed by the async loop.
type RingReader[T any] interface {
	DrainBatch(dst []T, max int) ([]T, bool)
	IsEmpty() bool
}

// AsyncLoopConfig configures the generic async processing loop.
type AsyncLoopConfig[T any] struct {
	Done           <-chan struct{}
	Queue          <-chan []byte
	Ring           RingReader[T]
	FlushInterval  time.Duration
	BatchSize      int
	FlushBatch     func([]T)
	ProcessMessage func([]byte)
}

// RunAsyncLoop runs a queue/ring consumer loop with batch flushing.
func RunAsyncLoop[T any](cfg AsyncLoopConfig[T]) {
	if cfg.Ring == nil && cfg.Queue == nil {
		return
	}

	ticker := time.NewTicker(cfg.FlushInterval / 2)
	defer ticker.Stop()

	batch := make([]T, 0, cfg.BatchSize)

	for {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				cfg.FlushBatch(batch)
				batch = batch[:0]
			}
			if cfg.Ring != nil {
				batch, _ = cfg.Ring.DrainBatch(batch, cfg.BatchSize)
			}

		case <-cfg.Done:
			if cfg.Ring != nil {
				for {
					var read bool
					batch, read = cfg.Ring.DrainBatch(batch, cfg.BatchSize)
					if !read {
						break
					}
				}
			}
			if len(batch) > 0 {
				cfg.FlushBatch(batch)
			}
			if cfg.Queue != nil {
				for {
					select {
					case msgBytes := <-cfg.Queue:
						cfg.ProcessMessage(msgBytes)
					default:
						return
					}
				}
			}
			return

		default:
			readSomething := false
			if cfg.Ring != nil {
				var read bool
				batch, read = cfg.Ring.DrainBatch(batch, cfg.BatchSize)
				readSomething = read
				if len(batch) >= cfg.BatchSize {
					cfg.FlushBatch(batch)
					batch = batch[:0]
				} else if len(batch) > 0 && cfg.Ring.IsEmpty() {
					cfg.FlushBatch(batch)
					batch = batch[:0]
				}
			}

			if cfg.Queue != nil {
				select {
				case msgBytes := <-cfg.Queue:
					cfg.ProcessMessage(msgBytes)
				default:
					if !readSomething {
						time.Sleep(100 * time.Microsecond)
					}
				}
			} else if cfg.Ring != nil && !readSomething && cfg.Ring.IsEmpty() {
				time.Sleep(100 * time.Microsecond)
			}
		}
	}
}
