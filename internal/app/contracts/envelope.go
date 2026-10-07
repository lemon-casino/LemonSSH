package contracts

// Request is the envelope every request-bearing service call accepts. It
// carries correlation and deadline information; deadline zero means "no
// explicit deadline" and the shell may still impose its own.
type Request struct {
	RequestID RequestID `json:"requestId"`
	// DeadlineMS is a Unix epoch millisecond deadline; 0 means none.
	DeadlineMS int64 `json:"deadlineMs,omitempty"`
}

// SubscriptionOpen acknowledges a new subscription.
type SubscriptionOpen struct {
	SubscriptionID string `json:"subscriptionId"`
	Topic          string `json:"topic"`
}

// SubscriptionEvent is one event delivered on a subscription. Sequences are
// monotonic per subscription; Final marks the terminal event after which no
// further events may arrive.
type SubscriptionEvent struct {
	SubscriptionID string `json:"subscriptionId"`
	Topic          string `json:"topic"`
	Sequence       uint64 `json:"sequence"`
	// Payload is the topic-specific body; its schema belongs to the owning
	// service contract, not to this base envelope.
	Payload []byte `json:"payload,omitempty"`
	Final   bool   `json:"final,omitempty"`
}
