package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/binaricat/netcatty/internal/capability"
)

// textAttachmentExtensions is the text-detection allowlist ported from
// mcpServerBridge.cjs TEXT_ATTACHMENT_EXTENSIONS.
var textAttachmentExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".json": true,
	".jsonc": true, ".jsonl": true, ".yaml": true, ".yml": true,
	".toml": true, ".ini": true, ".csv": true, ".tsv": true,
	".xml": true, ".html": true, ".css": true, ".js": true,
	".jsx": true, ".ts": true, ".tsx": true, ".mjs": true,
	".cjs": true, ".py": true, ".sh": true, ".bash": true,
	".zsh": true, ".fish": true,
}

var structuredMediaTypePattern = regexp.MustCompile(`(?i)\+(json|xml)$`)
var structuredJSONXMLPattern = regexp.MustCompile(`(?i)^application/(json|xml|javascript|x-javascript|typescript|yaml|x-yaml|toml|csv|x-ndjson|ndjson)$`)

// isLikelyTextAttachment mirrors isLikelyTextAttachment: text media types,
// structured-data types and known text extensions decode to text.
func isLikelyTextAttachment(mediaType, filename string) bool {
	if strings.HasPrefix(strings.ToLower(mediaType), "text/") {
		return true
	}
	if structuredJSONXMLPattern.MatchString(mediaType) || structuredMediaTypePattern.MatchString(mediaType) {
		return true
	}
	return textAttachmentExtensions[strings.ToLower(filepath.Ext(filename))]
}

// loadAttachmentContent resolves an attachment's bytes: inline base64 when
// registered, otherwise the registered host-readable path.
func loadAttachmentContent(attachment Attachment) ([]byte, error) {
	if attachment.Base64 != "" {
		return base64.StdEncoding.DecodeString(attachment.Base64)
	}
	if attachment.FilePath != "" {
		return os.ReadFile(attachment.FilePath)
	}
	return nil, fmt.Errorf("Attachment content is unavailable.")
}

// attachmentListHandler serves netcatty/listAttachments: identity
// summaries without content.
func (h *AgentHost) attachmentListHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chatSessionID, _ := params["chatSessionId"].(string)
	if chatSessionID == "" {
		return nil, fmt.Errorf("chatSessionId is required.")
	}
	return map[string]any{"ok": true, "attachments": h.attachments.List(chatSessionID)}, nil
}

// attachmentReadHandler serves netcatty/readAttachment: resolves the
// attachment inside the chat scope, loads its bytes and adds text for
// likely-text attachments. Content leaves the host only to the scoped
// chat that owns it.
func (h *AgentHost) attachmentReadHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chatSessionID, _ := params["chatSessionId"].(string)
	filePath, _ := params["filePath"].(string)
	filename, _ := params["filename"].(string)

	attachment, err := h.attachments.Find(chatSessionID, filePath, filename)
	if err != nil {
		return nil, err
	}
	content, err := loadAttachmentContent(attachment)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"ok":         true,
		"filename":   attachment.Filename,
		"mediaType":  attachment.MediaType,
		"sizeBytes":  len(content),
		"base64Data": base64.StdEncoding.EncodeToString(content),
	}
	if attachment.FilePath != "" {
		payload["filePath"] = attachment.FilePath
	}
	if isLikelyTextAttachment(attachment.MediaType, attachment.Filename) {
		payload["text"] = string(content)
	}
	return payload, nil
}
