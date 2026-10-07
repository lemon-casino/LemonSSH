package main

import (
	"fmt"
	"strings"
	"sync"
)

// Attachment is one user-attached file registered for a chat session.
// Content arrives either inline (base64Data) or as a host-readable path.
type Attachment struct {
	Filename  string `json:"filename"`
	MediaType string `json:"mediaType"`
	FilePath  string `json:"filePath,omitempty"`
	Base64    string `json:"base64Data,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// AttachmentRegistry is the chat-scoped attachment store (W13). The
// renderer registers attachments for its chat sessions; agents list and
// read them through the capability methods. Scope is enforced at lookup:
// a session only ever sees its own registrations.
type AttachmentRegistry struct {
	mu     sync.Mutex
	scoped map[string]map[string]Attachment // chatSessionID -> key -> attachment
}

func newAttachmentRegistry() *AttachmentRegistry {
	return &AttachmentRegistry{scoped: map[string]map[string]Attachment{}}
}

func attachmentKey(filePath, filename string) string {
	if filePath != "" {
		return filePath
	}
	return filename
}

// Register replaces the attachment set of one chat session, mirroring
// updateAttachmentMetadata: entries key by filePath (or filename) and
// later registrations overwrite earlier ones with the same key.
func (r *AttachmentRegistry) Register(chatSessionID string, attachments []Attachment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.scoped[chatSessionID]
	if existing == nil {
		existing = map[string]Attachment{}
	}
	for _, attachment := range attachments {
		attachment.Filename = strings.TrimSpace(attachment.Filename)
		if attachment.Filename == "" {
			attachment.Filename = "attachment"
		}
		attachment.MediaType = strings.TrimSpace(attachment.MediaType)
		if attachment.MediaType == "" {
			attachment.MediaType = "application/octet-stream"
		}
		attachment.FilePath = strings.TrimSpace(attachment.FilePath)
		existing[attachmentKey(attachment.FilePath, attachment.Filename)] = attachment
	}
	r.scoped[chatSessionID] = existing
}

// List returns attachment summaries for one chat session (no content).
func (r *AttachmentRegistry) List(chatSessionID string) []AttachmentSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	attachments := r.scoped[chatSessionID]
	summaries := make([]AttachmentSummary, 0, len(attachments))
	for _, attachment := range attachments {
		summaries = append(summaries, AttachmentSummary{
			Filename:  attachment.Filename,
			MediaType: attachment.MediaType,
			FilePath:  attachment.FilePath,
			SizeBytes: attachment.SizeBytes,
		})
	}
	return summaries
}

// AttachmentSummary is the listing projection: identity without content.
type AttachmentSummary struct {
	Filename  string `json:"filename"`
	MediaType string `json:"mediaType"`
	FilePath  string `json:"filePath,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// Find resolves one attachment by filePath or filename within a chat
// session's scope; content loading is the caller's decision.
func (r *AttachmentRegistry) Find(chatSessionID, filePath, filename string) (Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if chatSessionID == "" {
		return Attachment{}, fmt.Errorf("chatSessionId is required.")
	}
	attachments := r.scoped[chatSessionID]
	requestedPath := strings.TrimSpace(filePath)
	requestedName := strings.TrimSpace(filename)
	if requestedPath == "" && requestedName == "" {
		return Attachment{}, fmt.Errorf("filePath or filename is required.")
	}
	for _, attachment := range attachments {
		if requestedPath != "" && attachment.FilePath == requestedPath {
			return attachment, nil
		}
		if requestedName != "" && attachment.Filename == requestedName {
			return attachment, nil
		}
	}
	return Attachment{}, fmt.Errorf("Attachment is not registered for this chat session.")
}
