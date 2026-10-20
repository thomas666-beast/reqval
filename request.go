package reqval

// Request is the marker interface every request struct must satisfy.
// In Stage 1 it's empty on purpose; later stages will use it to hook
// in authorization, custom messages, and framework adapters.
type Request interface {
	isRequest()
}

// Base is meant to be embedded in every concrete request struct.
// Embedding it satisfies the Request interface automatically.
type Base struct{}

func (Base) isRequest() {}
