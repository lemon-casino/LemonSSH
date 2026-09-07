package migrationprobe

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

const (
	ProtocolVersion   = 1
	MaxFrameBytes     = 192 * 1024
	MaxRecords        = 64
	MaxPlaintextBytes = 128 * 1024
	MaxTotalBytes     = 1024 * 1024
	defaultTimeout    = 30 * time.Second

	metadataSourceFormat = "classified-metadata"
	finalReceiptID       = "migration.final-receipt"
	finalReceiptFormat   = "sanitized-receipt"
	finalReceiptPurpose  = "profile-secret-migration/final-receipt"
)

type serverHello struct {
	Version   int    `json:"v"`
	Type      string `json:"type"`
	RunID     string `json:"runId"`
	Challenge string `json:"challenge"`
	PublicKey string `json:"publicKey"`
}

type clientHello struct {
	Version   int    `json:"v"`
	Type      string `json:"type"`
	RunID     string `json:"runId"`
	Challenge string `json:"challenge"`
	PublicKey string `json:"publicKey"`
}

type confirmation struct {
	Version int    `json:"v"`
	Type    string `json:"type"`
	Proof   string `json:"proof"`
}

type sealedFrame struct {
	Version      int    `json:"v"`
	Type         string `json:"type"`
	Sequence     uint64 `json:"seq"`
	FixtureID    string `json:"fixtureId"`
	SourceFormat string `json:"sourceFormat"`
	Purpose      string `json:"purpose"`
	Ciphertext   string `json:"ciphertext"`
}

type sealedMetadataFrame struct {
	Version        int    `json:"v"`
	Type           string `json:"type"`
	Sequence       uint64 `json:"seq"`
	MetadataID     string `json:"metadataId"`
	Classification string `json:"classification"`
	Ciphertext     string `json:"ciphertext"`
}

type sealedFinalReceiptFrame struct {
	Version    int    `json:"v"`
	Type       string `json:"type"`
	Sequence   uint64 `json:"seq"`
	Ciphertext string `json:"ciphertext"`
}

type secretPayload struct {
	Plaintext string `json:"plaintext"`
}

type metadataPayload struct {
	Data string `json:"data"`
}

type receiptPayload struct {
	Passed bool `json:"passed"`
}

type metadataReceiptPayload struct {
	Passed bool   `json:"passed"`
	Data   string `json:"data"`
}

type finishFrame struct {
	Version int    `json:"v"`
	Type    string `json:"type"`
	Count   int    `json:"count"`
}

type receiptCounts struct {
	TotalRecords    int `json:"totalRecords"`
	SecretRecords   int `json:"secretRecords"`
	MetadataRecords int `json:"metadataRecords"`
}

type fixtureReceipt struct {
	FixtureID    string `json:"fixtureId"`
	Purpose      string `json:"purpose"`
	SourceFormat string `json:"sourceFormat"`
}

type metadataReceipt struct {
	MetadataID     string `json:"metadataId"`
	Classification string `json:"classification"`
}

type sanitizedReceipt struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Provider        string            `json:"provider"`
	ProviderVersion int               `json:"providerVersion"`
	EnvelopeVersion int               `json:"envelopeVersion"`
	Counts          receiptCounts     `json:"counts"`
	Fixtures        []fixtureReceipt  `json:"fixtures"`
	Metadata        []metadataReceipt `json:"metadata"`
	AllSealed       bool              `json:"allSealed"`
	AllOpened       bool              `json:"allOpened"`
	ExactMatches    bool              `json:"exactMatches"`
	Passed          bool              `json:"passed"`
	ErrorCodes      []string          `json:"errorCodes"`
}

type frameReader struct {
	r io.Reader
}

func (r frameReader) Read(timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.ReadContext(ctx)
}

func (r frameReader) ReadContext(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if setter, ok := r.r.(interface{ SetReadDeadline(time.Time) error }); ok {
		deadline, hasDeadline := ctx.Deadline()
		if !hasDeadline {
			deadline = time.Time{}
		}
		if err := setter.SetReadDeadline(deadline); err == nil {
			stop := context.AfterFunc(ctx, func() {
				_ = setter.SetReadDeadline(time.Now())
			})
			data, err := readFramePayload(r.r)
			stop()
			_ = setter.SetReadDeadline(time.Time{})
			if ctxErr := ctx.Err(); ctxErr != nil {
				zero(data)
				return nil, ctxErr
			}
			return data, err
		}
	}

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := readFramePayload(r.r)
		ch <- result{data: data, err: err}
	}()
	select {
	case got := <-ch:
		return got.data, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func readFramePayload(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxFrameBytes {
		return nil, errors.New("invalid frame size")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		zero(data)
		return nil, err
	}
	return data, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxFrameBytes {
		return errors.New("invalid frame size")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func writeJSONFrame(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer zero(payload)
	return writeFrame(w, payload)
}

func writeJSONFrameContext(ctx context.Context, w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer zero(payload)
	return writeFrameContext(ctx, w, payload)
}

func writeFrameContext(ctx context.Context, w io.Writer, payload []byte) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > MaxFrameBytes {
		return errors.New("invalid frame size")
	}
	if setter, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
		deadline, hasDeadline := ctx.Deadline()
		if !hasDeadline {
			deadline = time.Time{}
		}
		if err := setter.SetWriteDeadline(deadline); err == nil {
			stop := context.AfterFunc(ctx, func() {
				_ = setter.SetWriteDeadline(time.Now())
			})
			err := writeFrame(w, payload)
			stop()
			_ = setter.SetWriteDeadline(time.Time{})
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	ch := make(chan error, 1)
	go func() {
		err := writeAll(w, frame)
		zero(frame)
		ch <- err
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8")
	}
	if err := validateJSONDocument(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func validateJSONDocument(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			if _, exists := seen[key]; exists {
				return errors.New("duplicate JSON field")
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid JSON array")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("trailing JSON")
	}
	return nil
}

func decodeFrameType(data []byte) (string, error) {
	var fields map[string]json.RawMessage
	if err := decodeStrictJSON(data, &fields); err != nil {
		return "", err
	}
	versionData, hasVersion := fields["v"]
	typeData, hasType := fields["type"]
	if !hasVersion || !hasType {
		return "", errors.New("missing frame discriminator")
	}
	var version int
	var frameType string
	if err := json.Unmarshal(versionData, &version); err != nil || version != ProtocolVersion {
		return "", errors.New("invalid protocol version")
	}
	if err := json.Unmarshal(typeData, &frameType); err != nil || frameType == "" {
		return "", errors.New("invalid frame type")
	}
	return frameType, nil
}

func decodeFixedBase64(value string, size int) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != size {
		zero(decoded)
		return nil, errors.New("invalid encoded field")
	}
	return decoded, nil
}

func validateSecretDescriptor(fixtureID, sourceFormat, purpose string) error {
	if !validID(fixtureID) {
		return errors.New("invalid fixture id")
	}
	if sourceFormat != "enc:v1" && sourceFormat != "safeStorage-raw" {
		return errors.New("invalid source format")
	}
	if err := validatePurpose(purpose); err != nil {
		return err
	}
	if purpose != "profile-secret-migration/"+fixtureID {
		return errors.New("fixture purpose mismatch")
	}
	return nil
}

func validateMetadataDescriptor(metadataID, classification string) error {
	if !validID(metadataID) || len(classification) > 64 || !isToken(classification) {
		return errors.New("invalid metadata descriptor")
	}
	return nil
}

func validID(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && isToken(value)
}

func isToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func metadataPurpose(classification string) string {
	return "profile-secret-migration/metadata/" + classification
}

func protocolError(stage string) error {
	return fmt.Errorf("protocol failure at %s", stage)
}
