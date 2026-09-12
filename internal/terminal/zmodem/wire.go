package zmodem

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
)

const (
	zcrce        byte = 'h'
	zcrcg        byte = 'i'
	zcrcq        byte = 'j'
	zcrcw        byte = 'k'
	maxSubpacket      = 8192
)

type wireHeader struct {
	kind     byte
	position uint32
	crc32    bool
}

// EncodeEscaped quotes controls, including ZDLE itself, using standard XOR quoting.
func EncodeEscaped(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if b&0x60 == 0 {
			out = append(out, ZDLE, b^0x40)
		} else {
			out = append(out, b)
		}
	}
	return out
}

func unquote(data []byte, i int) (byte, int, error) {
	for i < len(data) && (data[i] == 0x11 || data[i] == 0x13 || data[i] == 0x91 || data[i] == 0x93) {
		i++
	}
	if i >= len(data) {
		return 0, i, ErrFrameMalformed
	}
	b := data[i]
	i++
	if b != ZDLE {
		return b, i, nil
	}
	if i >= len(data) {
		return 0, i, ErrFrameMalformed
	}
	b = data[i]
	i++
	switch b {
	case 'l':
		return 0x7f, i, nil
	case 'm':
		return 0xff, i, nil
	}
	if b&0x60 != 0x40 {
		return 0, i, ErrFrameMalformed
	}
	return b ^ 0x40, i, nil
}

func DecodeEscaped(data []byte) ([]byte, int) {
	var out []byte
	i := 0
	for i < len(data) {
		b, n, err := unquote(data, i)
		if err != nil {
			return out, i
		}
		out = append(out, b)
		i = n
	}
	return out, i
}

func headerBytes(kind byte, position uint32) []byte {
	body := make([]byte, 5)
	body[0] = kind
	binary.LittleEndian.PutUint32(body[1:], position)
	return body
}
func hexHeader(kind byte, position uint32) []byte {
	body := headerBytes(kind, position)
	crc := CRC16(body)
	body = append(body, byte(crc>>8), byte(crc))
	out := append([]byte{'*', '*', ZDLE, 'B'}, []byte(hex.EncodeToString(body))...)
	out = append(out, '\r', '\n')
	if kind != ZACK && kind != ZFIN {
		out = append(out, 0x11)
	}
	return out
}
func binaryHeader(kind byte, position uint32) []byte {
	body := headerBytes(kind, position)
	crc := CRC16(body)
	body = append(body, byte(crc>>8), byte(crc))
	return append([]byte{'*', ZDLE, 'A'}, EncodeEscaped(body)...)
}
func dataPacket(data []byte, end byte) []byte {
	body := append(append([]byte(nil), data...), end)
	crc := CRC16(body)
	out := append(EncodeEscaped(data), ZDLE, end)
	out = append(out, EncodeEscaped([]byte{byte(crc >> 8), byte(crc)})...)
	if end == zcrcw {
		out = append(out, 0x11)
	}
	return out
}

func parseHeader(data []byte) (wireHeader, int, error) {
	if bytes.Contains(data, bytes.Repeat([]byte{ZDLE}, 5)) {
		return wireHeader{}, 0, ErrCancelled
	}
	for start := 0; start < len(data); start++ {
		if data[start] != '*' {
			continue
		}
		i := start
		for i < len(data) && data[i] == '*' {
			i++
		}
		if i+1 >= len(data) {
			return wireHeader{}, 0, ErrFrameMalformed
		}
		if data[i] != ZDLE {
			continue
		}
		format := data[i+1]
		i += 2
		var body []byte
		switch format {
		case 'B':
			if len(data)-i < 14 {
				return wireHeader{}, 0, ErrFrameMalformed
			}
			var err error
			body, err = hex.DecodeString(string(data[i : i+14]))
			if err != nil {
				return wireHeader{}, 0, ErrFrameMalformed
			}
			i += 14
			// Hex headers end in CR/LF before an optional XON. Wait for LF
			// so split line endings cannot become subpacket payload.
			if i+1 >= len(data) {
				return wireHeader{}, 0, ErrFrameMalformed
			}
			if data[i]&0x7f != '\r' || data[i+1]&0x7f != '\n' {
				return wireHeader{}, 0, ErrFrameMalformed
			}
			i += 2
		case 'A', 'C':
			n := 7
			if format == 'C' {
				n = 9
			}
			for len(body) < n {
				b, next, err := unquote(data, i)
				if err != nil {
					return wireHeader{}, 0, err
				}
				body = append(body, b)
				i = next
			}
		default:
			continue
		}
		if format == 'C' {
			if crc32.ChecksumIEEE(body[:5]) != binary.LittleEndian.Uint32(body[5:]) {
				return wireHeader{}, 0, ErrCRCMismatch
			}
		} else if CRC16(body[:5]) != binary.BigEndian.Uint16(body[5:]) {
			return wireHeader{}, 0, ErrCRCMismatch
		}
		return wireHeader{body[0], binary.LittleEndian.Uint32(body[1:5]), format == 'C'}, i, nil
	}
	return wireHeader{}, 0, ErrFrameMalformed
}

type wireDecoder struct{ buffer []byte }

func (d *wireDecoder) header() (wireHeader, bool, error) {
	h, n, err := parseHeader(d.buffer)
	if err == ErrFrameMalformed {
		// Retain an incomplete candidate; discard prompt noise without growing forever.
		i := bytes.IndexByte(d.buffer, '*')
		if i < 0 {
			if len(d.buffer) > 4 {
				d.buffer = d.buffer[len(d.buffer)-4:]
			}
		} else if i > 0 {
			d.buffer = d.buffer[i:]
		}
		if len(d.buffer) > 128 {
			return h, false, ErrFrameMalformed
		}
		return h, false, nil
	}
	if err != nil {
		return h, false, err
	}
	d.buffer = d.buffer[n:]
	return h, true, nil
}
func (d *wireDecoder) packet(use32 bool) ([]byte, byte, bool, error) {
	var out []byte
	i := 0
	for i < len(d.buffer) {
		if len(d.buffer)-i >= 5 && bytes.Equal(d.buffer[i:i+5], bytes.Repeat([]byte{ZDLE}, 5)) {
			return nil, 0, false, ErrCancelled
		}
		if d.buffer[i] == ZDLE && i+1 < len(d.buffer) && d.buffer[i+1] >= zcrce && d.buffer[i+1] <= zcrcw {
			end := d.buffer[i+1]
			i += 2
			n := 2
			if use32 {
				n = 4
			}
			var check []byte
			for len(check) < n {
				b, next, err := unquote(d.buffer, i)
				if err != nil {
					return nil, 0, false, nil
				}
				check = append(check, b)
				i = next
			}
			body := append(append([]byte(nil), out...), end)
			valid := false
			if use32 {
				valid = crc32.ChecksumIEEE(body) == binary.LittleEndian.Uint32(check)
			} else {
				valid = CRC16(body) == binary.BigEndian.Uint16(check)
			}
			d.buffer = d.buffer[i:]
			if !valid {
				return nil, end, false, ErrCRCMismatch
			}
			return out, end, true, nil
		}
		b, next, err := unquote(d.buffer, i)
		if err != nil {
			return nil, 0, false, nil
		}
		out = append(out, b)
		i = next
		if len(out) > maxSubpacket {
			return nil, 0, false, ErrFileTooLarge
		}
	}
	return nil, 0, false, nil
}
