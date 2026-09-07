package main

import (
	"encoding/binary"
	"testing"
)

func TestFrameRoundTripAndRejection(t *testing.T) {
	source := Frame{
		Kind:            FrameOutput,
		Generation:      7,
		Sequence:        42,
		CreditCost:      3,
		Correlation:     9,
		TimestampMicros: 123456,
		Payload:         []byte{1, 2, 3},
	}
	encoded, err := source.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalFrame(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != source.Kind || decoded.Generation != source.Generation ||
		decoded.Sequence != source.Sequence || decoded.CreditCost != source.CreditCost ||
		decoded.Correlation != source.Correlation || decoded.TimestampMicros != source.TimestampMicros ||
		string(decoded.Payload) != string(source.Payload) {
		t.Fatalf("roundtrip mismatch: %#v", decoded)
	}

	tests := map[string]func([]byte){
		"magic":    func(data []byte) { binary.BigEndian.PutUint32(data[0:4], 0) },
		"version":  func(data []byte) { data[4]++ },
		"kind":     func(data []byte) { data[5] = 99 },
		"flags":    func(data []byte) { data[6] = 1 },
		"reserved": func(data []byte) { data[7] = 1 },
		"length":   func(data []byte) { binary.BigEndian.PutUint32(data[24:28], 2) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := append([]byte(nil), encoded...)
			mutate(candidate)
			if _, err := UnmarshalFrame(candidate); err == nil {
				t.Fatal("malformed frame was accepted")
			}
		})
	}
	if _, err := UnmarshalFrame(encoded[:frameHeaderSize-1]); err == nil {
		t.Fatal("short frame was accepted")
	}
	oversized := make([]byte, maxFrameBytes+1)
	if _, err := UnmarshalFrame(oversized); err == nil {
		t.Fatal("oversized frame was accepted")
	}
}
