package dataplane

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	input := Frame{Kind: FrameOutput, Generation: 2, Sequence: 17, CreditCost: 4, Correlation: 9, TimestampMicros: 123, Payload: []byte("output")}
	encoded, err := input.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	output, err := UnmarshalFrame(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if output.Kind != input.Kind || output.Generation != input.Generation || output.Sequence != input.Sequence || output.CreditCost != input.CreditCost || output.Correlation != input.Correlation || output.TimestampMicros != input.TimestampMicros || !bytes.Equal(output.Payload, input.Payload) {
		t.Fatalf("round trip mismatch: %+v", output)
	}
}

func TestFrameRejectsMalformed(t *testing.T) {
	cases := [][]byte{nil, make([]byte, FrameHeaderBytes-1), make([]byte, MaxFrameBytes+1)}
	for _, value := range cases {
		if _, err := UnmarshalFrame(value); err == nil {
			t.Fatal("malformed frame accepted")
		}
	}
	frame := Frame{Kind: FrameOutput, Payload: []byte("x")}
	encoded, err := frame.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	encoded[0] ^= 1
	if _, err := UnmarshalFrame(encoded); err == nil {
		t.Fatal("bad magic accepted")
	}
}

func TestFrameBoundaries(t *testing.T) {
	frame := Frame{Kind: FrameOutput, Payload: make([]byte, MaxPayloadBytes)}
	if _, err := frame.MarshalBinary(); err != nil {
		t.Fatalf("max payload rejected: %v", err)
	}
	tooLarge := Frame{Kind: FrameOutput, Payload: make([]byte, MaxPayloadBytes+1)}
	if _, err := tooLarge.MarshalBinary(); err == nil {
		t.Fatal("oversized payload accepted")
	}
	invalid := Frame{Kind: 255}
	if _, err := invalid.MarshalBinary(); err == nil {
		t.Fatal("invalid kind accepted")
	}
}

func FuzzUnmarshalFrameNeverPanics(f *testing.F) {
	encoded, _ := (Frame{Kind: FrameOutput, Payload: []byte("seed")}).MarshalBinary()
	f.Add(encoded)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = UnmarshalFrame(data) })
}
