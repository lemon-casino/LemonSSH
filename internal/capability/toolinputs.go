package capability

// ToolInputField is one agent-facing input parameter. JSON tags match the
// buildZodShape projection emitted into generated tool specs.
type ToolInputField struct {
	Type        string `json:"type"`
	Optional    bool   `json:"optional"`
	Description string `json:"description"`
}

// ToolInputFields returns the input schema for a capability. The second
// result reports whether the capability has an input schema entry at all —
// eligibility requires one, even when it is empty.
func ToolInputFields(capabilityID string) (map[string]ToolInputField, bool) {
	fields, ok := toolInputFields[capabilityID]
	return fields, ok
}

// HasToolInputFields reports whether the capability has an input schema
// entry (possibly empty), the CJS hasOwnProperty check.
func HasToolInputFields(capabilityID string) bool {
	_, ok := toolInputFields[capabilityID]
	return ok
}

// ModelDescriptionHint returns the long-form model guidance for a
// capability, or "".
func ModelDescriptionHint(capabilityID string) string {
	return modelDescriptionHints[capabilityID]
}
