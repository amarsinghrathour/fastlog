package queue

import "time"

// RetrySend tries to enqueue with bounded retries and interval.
func RetrySend(ch chan []byte, payload []byte, maxRetries int, retryInterval time.Duration) bool {
	if ch == nil {
		return false
	}

	select {
	case ch <- payload:
		return true
	default:
	}

	for retries := 0; retries < maxRetries; retries++ {
		select {
		case ch <- payload:
			return true
		case <-time.After(retryInterval):
		}
	}
	return false
}
