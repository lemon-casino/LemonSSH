package contracts

// Agent wire DTOs (W03, P7-01, AI-01). Large counters and revisions travel as
// decimal strings so JavaScript clients never lose uint64 precision. The
// shell-neutral contract imports nothing beyond this package.

// TurnScope is the host-narrowed set of operations a turn may use. The
// request carries a wish; the host records what it actually granted.
type TurnScope struct {
	TerminalRead  bool `json:"terminalRead,omitempty"`
	TerminalWrite bool `json:"terminalWrite,omitempty"`
	Exec          bool `json:"exec,omitempty"`
	SFTPRead      bool `json:"sftpRead,omitempty"`
	SFTPWrite     bool `json:"sftpWrite,omitempty"`
}

// TurnInput is the user-supplied text plus attachment references.
type TurnInput struct {
	Text          string   `json:"text"`
	AttachmentIDs []string `json:"attachmentIds,omitempty"`
}

// PrepareTurnRequest asks the host to reserve one turn slot. The host
// validates scope and revisions and never contacts a provider.
type PrepareTurnRequest struct {
	RequestID            RequestID     `json:"requestId"`
	ChatSessionID        ChatSessionID `json:"chatSessionId"`
	ExpectedChatRevision string        `json:"expectedChatRevision,omitempty"`
	AgentID              AgentID       `json:"agentId"`
	ProviderConfigID     string        `json:"providerConfigId,omitempty"`
	ModelID              string        `json:"modelId,omitempty"`
	Input                TurnInput     `json:"input"`
	RequestedScope       TurnScope     `json:"requestedScope"`
}

// PreparedTurn is the host reservation for one turn. It carries no secret
// material and expires when the lease lapses.
type PreparedTurn struct {
	TurnID                  TurnID    `json:"turnId"`
	LeaseExpiresAtMS        int64     `json:"leaseExpiresAtMs"`
	Cursor                  string    `json:"cursor"`
	SnapshotRevision        string    `json:"snapshotRevision"`
	EffectiveScope          TurnScope `json:"effectiveScope"`
	EffectiveConfigRevision string    `json:"effectiveConfigRevision"`
	PolicyRevision          string    `json:"policyRevision"`
}

// TurnCommandKind selects the lifecycle command for a prepared or running turn.
type TurnCommandKind string

const (
	TurnCommandStart TurnCommandKind = "start"
	TurnCommandStop  TurnCommandKind = "stop"
	TurnCommandSteer TurnCommandKind = "steer"
)

// TurnCommand starts, stops or steers a turn. Start requires the prepare
// lease; Stop carries a reason; Steer carries input plus the expected turn
// revision so concurrent writers fail closed.
type TurnCommand struct {
	RequestID            RequestID       `json:"requestId"`
	Kind                 TurnCommandKind `json:"kind"`
	TurnID               TurnID          `json:"turnId"`
	Reason               string          `json:"reason,omitempty"`
	Input                *TurnInput      `json:"input,omitempty"`
	ExpectedTurnRevision string          `json:"expectedTurnRevision,omitempty"`
}

// ReadEventsRequest asks for the next event page of one turn. Ownership is
// validated by the host principal, not by knowing the ID.
type ReadEventsRequest struct {
	RequestID     RequestID `json:"requestId"`
	TurnID        TurnID    `json:"turnId"`
	AfterSequence string    `json:"afterSequence"`
	Limit         int       `json:"limit,omitempty"`
}

// AgentEventEnvelope is one turn event. Sequence is a decimal string so the
// JavaScript side never loses precision on uint64 counters.
type AgentEventEnvelope struct {
	SchemaVersion int           `json:"schemaVersion"`
	InstanceID    InstanceID    `json:"instanceId"`
	ChatSessionID ChatSessionID `json:"chatSessionId"`
	TurnID        TurnID        `json:"turnId"`
	Sequence      string        `json:"sequence"`
	Type          string        `json:"type"`
	Backend       string        `json:"backend"`
	TimestampMS   int64         `json:"timestampMs"`
	MessageID     string        `json:"messageId,omitempty"`
	ModelCallID   string        `json:"modelCallId,omitempty"`
	ToolCallID    string        `json:"toolCallId,omitempty"`
	Payload       []byte        `json:"payload,omitempty"`
}

// TurnSnapshot is the authoritative projection replacing stale cursors. It
// carries no opaque provider continuation.
type TurnSnapshot struct {
	Status              string `json:"status"`
	Revision            string `json:"revision"`
	ThroughSequence     string `json:"throughSequence"`
	TerminalReason      string `json:"terminalReason,omitempty"`
	UsageComplete       bool   `json:"usageComplete,omitempty"`
	PendingInteractions int    `json:"pendingInteractions,omitempty"`
	ActiveJobs          int    `json:"activeJobs,omitempty"`
}

// EventPage is one ReadEvents response. Snapshot arrives only when the host
// evicted the ring section the cursor points into.
type EventPage struct {
	Events        []AgentEventEnvelope `json:"events"`
	NextCursor    string               `json:"nextCursor,omitempty"`
	HasMore       bool                 `json:"hasMore,omitempty"`
	CursorExpired bool                 `json:"cursorExpired,omitempty"`
	Snapshot      *TurnSnapshot        `json:"snapshot,omitempty"`
}
