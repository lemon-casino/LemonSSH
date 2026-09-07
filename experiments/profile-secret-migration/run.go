package migrationprobe

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

func Run(input io.Reader, output io.Writer, provider Provider, timeout time.Duration) (sanitizedReceipt, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runContext(ctx, input, output, provider)
}

func runContext(ctx context.Context, input io.Reader, output io.Writer, provider Provider) (sanitizedReceipt, error) {
	if ctx == nil || input == nil || output == nil {
		return sanitizedReceipt{}, errors.New("invalid protocol endpoint")
	}
	if provider == nil || !provider.Available() || provider.Name() != ProviderName {
		return sanitizedReceipt{}, ErrProviderUnavailable
	}
	reader := frameReader{r: input}

	privateKey, hello, err := newServerHandshake()
	if err != nil {
		return sanitizedReceipt{}, errors.New("handshake failed")
	}
	if err := writeJSONFrameContext(ctx, output, hello); err != nil {
		return sanitizedReceipt{}, protocolError("server hello write")
	}
	data, err := reader.ReadContext(ctx)
	if err != nil {
		return sanitizedReceipt{}, protocolError("client hello read")
	}
	var client clientHello
	decodeErr := decodeStrictJSON(data, &client)
	zero(data)
	if decodeErr != nil {
		return sanitizedReceipt{}, protocolError("client hello decode")
	}
	channel, serverProof, expectedClientProof, err := finishServerHandshake(privateKey, hello, client)
	if err != nil {
		return sanitizedReceipt{}, err
	}
	defer channel.destroy()
	defer zero(serverProof)
	defer zero(expectedClientProof)
	if err := writeJSONFrameContext(ctx, output, confirmation{
		Version: ProtocolVersion,
		Type:    "server_confirm",
		Proof:   base64.StdEncoding.EncodeToString(serverProof),
	}); err != nil {
		return sanitizedReceipt{}, protocolError("server confirmation write")
	}
	data, err = reader.ReadContext(ctx)
	if err != nil {
		return sanitizedReceipt{}, protocolError("client confirmation read")
	}
	var confirm confirmation
	decodeErr = decodeStrictJSON(data, &confirm)
	zero(data)
	if decodeErr != nil || confirm.Version != ProtocolVersion || confirm.Type != "client_confirm" {
		return sanitizedReceipt{}, protocolError("client confirmation decode")
	}
	clientProof, err := decodeFixedBase64(confirm.Proof, 32)
	confirm.Proof = ""
	if err != nil || !hmac.Equal(clientProof, expectedClientProof) {
		zero(clientProof)
		return sanitizedReceipt{}, protocolError("client confirmation proof")
	}
	zero(clientProof)

	state := newProtocolState()
	for {
		data, err = reader.ReadContext(ctx)
		if err != nil {
			return sanitizedReceipt{}, protocolError("record read")
		}
		frameType, kindErr := decodeFrameType(data)
		if kindErr != nil {
			zero(data)
			return sanitizedReceipt{}, protocolError("frame discriminator")
		}
		switch frameType {
		case "secret":
			err = processSecretFrame(ctx, output, channel, provider, state, data)
		case "metadata":
			err = processMetadataFrame(ctx, output, channel, state, data)
		case "finish":
			var finish finishFrame
			decodeErr = decodeStrictJSON(data, &finish)
			zero(data)
			if decodeErr != nil {
				return sanitizedReceipt{}, protocolError("finish decode")
			}
			return sendFinalReceipt(ctx, output, channel, provider, state, finish)
		default:
			err = protocolError("unexpected frame")
		}
		zero(data)
		if err != nil {
			return sanitizedReceipt{}, err
		}
	}
}

func processSecretFrame(ctx context.Context, output io.Writer, channel *secureChannel, provider Provider, state *protocolState, data []byte) error {
	var frame sealedFrame
	if decodeStrictJSON(data, &frame) != nil || frame.Version != ProtocolVersion || frame.Type != "secret" {
		return protocolError("secret frame")
	}
	if validateSecretDescriptor(frame.FixtureID, frame.SourceFormat, frame.Purpose) != nil {
		return protocolError("secret descriptor")
	}
	if state.checkRecord(frame.Sequence, frame.FixtureID, 0) != nil {
		return protocolError("secret state")
	}
	opened, err := channel.open("electron-to-go", frame.Sequence, frame.FixtureID, frame.SourceFormat, frame.Purpose, frame.Ciphertext)
	frame.Ciphertext = ""
	if err != nil {
		return protocolError("secret authentication")
	}
	var payload secretPayload
	if decodeStrictJSON(opened, &payload) != nil {
		zero(opened)
		return protocolError("secret payload")
	}
	plaintext := []byte(payload.Plaintext)
	plaintextSize := len(plaintext)
	payload.Plaintext = ""
	zero(opened)
	defer zero(plaintext)
	if plaintextSize == 0 || state.checkRecord(frame.Sequence, frame.FixtureID, plaintextSize) != nil {
		return protocolError("secret bounds")
	}
	envelope, err := provider.Seal(plaintext, frame.Purpose)
	if err != nil {
		zero(envelope)
		return errors.New("provider seal failed")
	}
	roundtrip, err := provider.Open(envelope, frame.Purpose)
	zero(envelope)
	exact := err == nil && hmac.Equal(plaintext, roundtrip)
	zero(roundtrip)
	if !exact {
		return errors.New("provider roundtrip failed")
	}

	receiptBody, err := json.Marshal(receiptPayload{Passed: true})
	if err != nil {
		return protocolError("receipt encode")
	}
	receiptCiphertext, err := channel.seal("go-to-electron", frame.Sequence, frame.FixtureID, frame.SourceFormat, frame.Purpose, receiptBody)
	zero(receiptBody)
	if err != nil {
		return protocolError("receipt seal")
	}
	if err := writeJSONFrameContext(ctx, output, sealedFrame{
		Version:      ProtocolVersion,
		Type:         "receipt",
		Sequence:     frame.Sequence,
		FixtureID:    frame.FixtureID,
		SourceFormat: frame.SourceFormat,
		Purpose:      frame.Purpose,
		Ciphertext:   receiptCiphertext,
	}); err != nil {
		return protocolError("receipt write")
	}
	state.commitSecret(frame, plaintextSize, true, true, exact)
	return nil
}

func processMetadataFrame(ctx context.Context, output io.Writer, channel *secureChannel, state *protocolState, data []byte) error {
	var frame sealedMetadataFrame
	if decodeStrictJSON(data, &frame) != nil || frame.Version != ProtocolVersion || frame.Type != "metadata" {
		return protocolError("metadata frame")
	}
	if validateMetadataDescriptor(frame.MetadataID, frame.Classification) != nil {
		return protocolError("metadata descriptor")
	}
	if state.checkRecord(frame.Sequence, frame.MetadataID, 0) != nil {
		return protocolError("metadata state")
	}
	purpose := metadataPurpose(frame.Classification)
	opened, err := channel.open("electron-to-go", frame.Sequence, frame.MetadataID, metadataSourceFormat, purpose, frame.Ciphertext)
	frame.Ciphertext = ""
	if err != nil {
		return protocolError("metadata authentication")
	}
	var payload metadataPayload
	if decodeStrictJSON(opened, &payload) != nil {
		zero(opened)
		return protocolError("metadata payload")
	}
	metadata := []byte(payload.Data)
	metadataSize := len(metadata)
	payload.Data = ""
	zero(opened)
	defer zero(metadata)
	if state.checkRecord(frame.Sequence, frame.MetadataID, metadataSize) != nil {
		return protocolError("metadata bounds")
	}
	preserved := append([]byte(nil), metadata...)
	exact := hmac.Equal(metadata, preserved)
	ack := metadataReceiptPayload{Passed: exact, Data: string(preserved)}
	zero(preserved)
	receiptBody, err := json.Marshal(ack)
	ack.Data = ""
	if err != nil {
		return protocolError("metadata receipt encode")
	}
	receiptCiphertext, err := channel.seal("go-to-electron", frame.Sequence, frame.MetadataID, metadataSourceFormat, purpose, receiptBody)
	zero(receiptBody)
	if err != nil {
		return protocolError("metadata receipt seal")
	}
	if err := writeJSONFrameContext(ctx, output, sealedMetadataFrame{
		Version:        ProtocolVersion,
		Type:           "metadata_receipt",
		Sequence:       frame.Sequence,
		MetadataID:     frame.MetadataID,
		Classification: frame.Classification,
		Ciphertext:     receiptCiphertext,
	}); err != nil {
		return protocolError("metadata receipt write")
	}
	state.commitMetadata(frame, metadataSize, true, exact)
	return nil
}

func sendFinalReceipt(ctx context.Context, output io.Writer, channel *secureChannel, provider Provider, state *protocolState, finish finishFrame) (sanitizedReceipt, error) {
	receipt, err := state.finish(finish, provider.Name())
	if err != nil {
		return sanitizedReceipt{}, protocolError("finish")
	}
	if err := validateCorpusReceipt(receipt); err != nil {
		return sanitizedReceipt{}, err
	}
	receiptBody, err := json.Marshal(receipt)
	if err != nil {
		return sanitizedReceipt{}, protocolError("final receipt encode")
	}
	sequence := state.nextSequence
	receiptCiphertext, err := channel.seal("go-to-electron", sequence, finalReceiptID, finalReceiptFormat, finalReceiptPurpose, receiptBody)
	zero(receiptBody)
	if err != nil {
		return sanitizedReceipt{}, protocolError("final receipt seal")
	}
	if err := writeJSONFrameContext(ctx, output, sealedFinalReceiptFrame{
		Version:    ProtocolVersion,
		Type:       "final_receipt",
		Sequence:   sequence,
		Ciphertext: receiptCiphertext,
	}); err != nil {
		return sanitizedReceipt{}, protocolError("final receipt write")
	}
	return receipt, nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
