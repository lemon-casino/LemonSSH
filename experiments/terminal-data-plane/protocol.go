package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	frameMagic      uint32 = 0x4e544450 // NTDP
	frameVersion    uint8  = 2
	frameHeaderSize        = 40
	maxPayloadBytes        = 128 * 1024
	maxFrameBytes          = frameHeaderSize + maxPayloadBytes
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
	result := make([]byte, frameHeaderSize+len(f.Payload))
	binary.BigEndian.PutUint32(result[0:4], frameMagic)
	result[4] = frameVersion
	result[5] = byte(f.Kind)
	// Bytes 6 and 7 are flags and reserved space. Version 2 requires both zero.
	binary.BigEndian.PutUint32(result[8:12], f.Generation)
	binary.BigEndian.PutUint64(result[12:20], f.Sequence)
	binary.BigEndian.PutUint32(result[20:24], f.CreditCost)
	binary.BigEndian.PutUint32(result[24:28], uint32(len(f.Payload)))
	binary.BigEndian.PutUint32(result[28:32], f.Correlation)
	binary.BigEndian.PutUint64(result[32:40], f.TimestampMicros)
	copy(result[frameHeaderSize:], f.Payload)
	return result, nil
}

func UnmarshalFrame(data []byte) (Frame, error) {
	if len(data) < frameHeaderSize {
		return Frame{}, errors.New("frame is shorter than the version 2 header")
	}
	if len(data) > maxFrameBytes {
		return Frame{}, fmt.Errorf("frame exceeds %d byte limit", maxFrameBytes)
	}
	if binary.BigEndian.Uint32(data[0:4]) != frameMagic {
		return Frame{}, errors.New("invalid frame magic")
	}
	if data[4] != frameVersion {
		return Frame{}, fmt.Errorf("unsupported frame version %d", data[4])
	}
	if data[6] != 0 || data[7] != 0 {
		return Frame{}, errors.New("version 2 flags and reserved fields must be zero")
	}
	payloadLength := binary.BigEndian.Uint32(data[24:28])
	if payloadLength > maxPayloadBytes {
		return Frame{}, fmt.Errorf("payload exceeds %d byte limit", maxPayloadBytes)
	}
	if int(payloadLength) != len(data)-frameHeaderSize {
		return Frame{}, errors.New("payload length does not match frame size")
	}
	frame := Frame{
		Kind:            FrameKind(data[5]),
		Generation:      binary.BigEndian.Uint32(data[8:12]),
		Sequence:        binary.BigEndian.Uint64(data[12:20]),
		CreditCost:      binary.BigEndian.Uint32(data[20:24]),
		Correlation:     binary.BigEndian.Uint32(data[28:32]),
		TimestampMicros: binary.BigEndian.Uint64(data[32:40]),
		Payload:         append([]byte(nil), data[frameHeaderSize:]...),
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
	if len(frame.Payload) > maxPayloadBytes {
		return fmt.Errorf("payload exceeds %d byte limit", maxPayloadBytes)
	}
	return nil
}
