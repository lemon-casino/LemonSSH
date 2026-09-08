// Package dataplane owns the production terminal binary frame contract (P3-01).
// Terminal bytes do not use Wails JSON calls/events. This package intentionally
// contains only the bounded binary frame codec and flow-control primitives;
// WebSocket route lifecycle is added after the codec passes the same probe
// workload and race/fuzz evidence.
package dataplane

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FrameMagic       uint32 = 0x4e544450 // NTDP
	FrameVersion     uint8  = 2
	FrameHeaderBytes        = 40
	MaxPayloadBytes         = 128 * 1024
	MaxFrameBytes           = FrameHeaderBytes + MaxPayloadBytes
)

type FrameKind uint8

const (
	FrameOutput       FrameKind = 1
	FrameCredit       FrameKind = 2
	FrameUrgent       FrameKind = 3
	FrameUrgentACK    FrameKind = 4
	FrameComplete     FrameKind = 5
	FrameDrainRequest FrameKind = 6
	FrameDrainMarker  FrameKind = 7
	FrameDrainReady   FrameKind = 8
)

type Frame struct {
	Kind            FrameKind
	Generation      uint32
	Sequence        uint64
	CreditCost      uint32
	Correlation     uint32
	TimestampMicros uint64
	Payload         []byte
}

func (f Frame) MarshalBinary() ([]byte, error) {
	if err := validateFrame(f); err != nil {
		return nil, err
	}
	encoded := make([]byte, FrameHeaderBytes+len(f.Payload))
	binary.BigEndian.PutUint32(encoded[0:4], FrameMagic)
	encoded[4] = FrameVersion
	encoded[5] = byte(f.Kind)
	binary.BigEndian.PutUint32(encoded[8:12], f.Generation)
	binary.BigEndian.PutUint64(encoded[12:20], f.Sequence)
	binary.BigEndian.PutUint32(encoded[20:24], f.CreditCost)
	binary.BigEndian.PutUint32(encoded[24:28], uint32(len(f.Payload)))
	binary.BigEndian.PutUint32(encoded[28:32], f.Correlation)
	binary.BigEndian.PutUint64(encoded[32:40], f.TimestampMicros)
	copy(encoded[FrameHeaderBytes:], f.Payload)
	return encoded, nil
}

func UnmarshalFrame(data []byte) (Frame, error) {
	if len(data) < FrameHeaderBytes {
		return Frame{}, errors.New("frame is shorter than version 2 header")
	}
	if len(data) > MaxFrameBytes {
		return Frame{}, fmt.Errorf("frame exceeds %d byte limit", MaxFrameBytes)
	}
	if binary.BigEndian.Uint32(data[0:4]) != FrameMagic {
		return Frame{}, errors.New("invalid frame magic")
	}
	if data[4] != FrameVersion {
		return Frame{}, fmt.Errorf("unsupported frame version %d", data[4])
	}
	if data[6] != 0 || data[7] != 0 {
		return Frame{}, errors.New("version 2 flags and reserved fields must be zero")
	}
	payloadLength := binary.BigEndian.Uint32(data[24:28])
	if payloadLength > MaxPayloadBytes || int(payloadLength) != len(data)-FrameHeaderBytes {
		return Frame{}, errors.New("payload length is invalid")
	}
	frame := Frame{
		Kind:            FrameKind(data[5]),
		Generation:      binary.BigEndian.Uint32(data[8:12]),
		Sequence:        binary.BigEndian.Uint64(data[12:20]),
		CreditCost:      binary.BigEndian.Uint32(data[20:24]),
		Correlation:     binary.BigEndian.Uint32(data[28:32]),
		TimestampMicros: binary.BigEndian.Uint64(data[32:40]),
		Payload:         append([]byte(nil), data[FrameHeaderBytes:]...),
	}
	if err := validateFrame(frame); err != nil {
		return Frame{}, err
	}
	return frame, nil
}

func validateFrame(frame Frame) error {
	switch frame.Kind {
	case FrameOutput, FrameCredit, FrameUrgent, FrameUrgentACK, FrameComplete,
		FrameDrainRequest, FrameDrainMarker, FrameDrainReady:
	default:
		return fmt.Errorf("invalid frame kind %d", frame.Kind)
	}
	if len(frame.Payload) > MaxPayloadBytes {
		return fmt.Errorf("payload exceeds %d byte limit", MaxPayloadBytes)
	}
	return nil
}
