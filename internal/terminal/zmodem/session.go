// Raw ZMODEM session state. Callbacks run synchronously on the transport reader.
package zmodem

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Cancel writes the standard CAN burst. The transport owner must also unblock
// any outstanding read when its context is cancelled.
func Cancel(write func([]byte) error) error {
	if write == nil {
		return nil
	}
	return write([]byte{24, 24, 24, 24, 24, 24, 24, 24, 8, 8, 8, 8, 8, 8, 8, 8})
}

type feedState struct {
	meta         FileMeta
	written      int64
	inFile       bool
	packetKind   byte
	packetActive bool
	use32        bool
}

func (r *Receiver) maxBytes() int64 {
	if r.MaxFileBytes > 0 {
		return r.MaxFileBytes
	}
	return MaxFileBytes
}
func (r *Receiver) reply(kind byte, pos uint32) error {
	if r.Write == nil {
		return nil
	}
	return r.Write(hexHeader(kind, pos))
}

// Start advertises full duplex, overlapped I/O and CRC32 receive support.
func (r *Receiver) Start(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return r.reply(ZRINIT, 0x23000000)
}

func (r *Receiver) FeedSession(ctx context.Context, data []byte) (consumed int, err error) {
	if r.state == nil {
		r.state = &feedState{}
	}
	if r.decoder == nil {
		r.decoder = &wireDecoder{}
	}
	defer func() {
		if err != nil {
			_ = Cancel(r.Write)
		}
	}()
	if ctx.Err() != nil {
		return 0, ErrCancelled
	}
	// Bound retained data even when the peer never terminates a subpacket.
	for len(data) > 0 {
		n := min(len(data), 4096)
		r.decoder.buffer = append(r.decoder.buffer, data[:n]...)
		data = data[n:]
		for {
			if ctx.Err() != nil {
				return consumed, ErrCancelled
			}
			if r.Done {
				r.decoder.buffer = nil
				return consumed, nil
			}
			if r.state.packetActive {
				payload, end, ok, e := r.decoder.packet(r.state.use32)
				if e != nil {
					return consumed, e
				}
				if !ok {
					break
				}
				consumed += len(payload)
				switch r.state.packetKind {
				case ZFILE:
					if r.state.inFile {
						return consumed, fmt.Errorf("%w: nested ZFILE", ErrFrameMalformed)
					}
					meta, e := ParseFileMeta(payload, r.maxBytes())
					if e != nil {
						return consumed, e
					}
					if r.OnFileStart != nil {
						if e = r.OnFileStart(meta); e != nil {
							return consumed, e
						}
					}
					r.state.meta = meta
					r.state.written = 0
					r.state.inFile = true
					if e = r.reply(ZRPOS, 0); e != nil {
						return consumed, e
					}
				case ZSINIT:
					if e = r.reply(ZACK, 1); e != nil {
						return consumed, e
					}
				case ZDATA:
					total := r.state.written + int64(len(payload))
					if e = ValidateSize(total, r.maxBytes()); e != nil {
						return consumed, e
					}
					if r.state.meta.Size > 0 && total > r.state.meta.Size {
						return consumed, fmt.Errorf("%w: data exceeds declared size", ErrFileTooLarge)
					}
					if r.OnChunk != nil && len(payload) > 0 {
						if e = r.OnChunk(payload); e != nil {
							return consumed, e
						}
					}
					r.state.written = total
					if end == zcrcq || end == zcrcw {
						if e = r.reply(ZACK, uint32(total)); e != nil {
							return consumed, e
						}
					}
				}
				r.state.packetActive = end == zcrcg || end == zcrcq
				continue
			}
			h, ok, e := r.decoder.header()
			if e != nil {
				return consumed, e
			}
			if !ok {
				break
			}
			switch h.kind {
			case ZRQINIT:
				if e = r.Start(ctx); e != nil {
					return consumed, e
				}
			case ZFILE, ZSINIT, ZDATA:
				if h.kind == ZDATA {
					if !r.state.inFile {
						return consumed, fmt.Errorf("%w: ZDATA before ZFILE", ErrFrameMalformed)
					}
					if int64(h.position) != r.state.written {
						return consumed, fmt.Errorf("%w: unexpected data offset", ErrFrameMalformed)
					}
				}
				r.state.packetKind = h.kind
				r.state.packetActive = true
				r.state.use32 = h.crc32
			case ZEOF:
				if !r.state.inFile || int64(h.position) != r.state.written || (r.state.meta.Size > 0 && r.state.written != r.state.meta.Size) {
					return consumed, fmt.Errorf("%w: incomplete file", ErrFrameMalformed)
				}
				if r.OnFileEnd != nil {
					if e = r.OnFileEnd(r.state.meta); e != nil {
						return consumed, e
					}
				}
				r.state.inFile = false
				if e = r.Start(ctx); e != nil {
					return consumed, e
				}
			case ZFIN:
				if r.state.inFile {
					return consumed, fmt.Errorf("%w: finish during file", ErrFrameMalformed)
				}
				if e = r.reply(ZFIN, 0); e != nil {
					return consumed, e
				}
				r.Done = true
			case ZABORT, 16:
				return consumed, ErrCancelled
			default:
				return consumed, fmt.Errorf("%w: unexpected header %d", ErrFrameMalformed, h.kind)
			}
		}
	}
	return consumed, nil
}

func (r *Receiver) SessionReader(ctx context.Context, source io.Reader) error {
	buffer := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			_ = Cancel(r.Write)
			return ErrCancelled
		}
		n, err := source.Read(buffer)
		if n > 0 {
			if _, e := r.FeedSession(ctx, buffer[:n]); e != nil {
				return e
			}
			if r.Done {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && r.Done {
				return nil
			}
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
}

// Sender requires duplex transport. Read must honor ctx (including deadlines).
// Read and Write may not race another consumer of the terminal byte stream.
type Sender struct {
	Write func([]byte) error
	Read  func(context.Context, []byte) (int, error)
	// OnProgress reports bytes acknowledged by the peer, never merely sent.
	OnProgress func(transferred, total int64)
}

func (s *Sender) SendFile(ctx context.Context, meta FileMeta, data []byte) (err error) {
	if e := validateName(meta.Name); e != nil {
		return e
	}
	if e := ValidateSize(int64(len(data)), MaxFileBytes); e != nil {
		return e
	}
	if s.Write == nil || s.Read == nil {
		return errors.New("zmodem: duplex transport required")
	}
	defer func() {
		if err != nil {
			_ = Cancel(s.Write)
		}
	}()
	decoder := &wireDecoder{}
	readHeader := func() (wireHeader, error) {
		buffer := make([]byte, 4096)
		for {
			if ctx.Err() != nil {
				return wireHeader{}, ErrCancelled
			}
			h, ok, e := decoder.header()
			if e != nil {
				return h, e
			}
			if ok {
				if h.kind == ZABORT || h.kind == 16 {
					return h, ErrCancelled
				}
				return h, nil
			}
			n, e := s.Read(ctx, buffer)
			if n > 0 {
				decoder.buffer = append(decoder.buffer, buffer[:n]...)
			}
			if e != nil {
				return h, e
			}
			if n == 0 {
				return h, io.ErrNoProgress
			}
		}
	}
	send := func(kind byte, pos uint32) error {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		return s.Write(hexHeader(kind, pos))
	}
	expect := func(kind byte) (wireHeader, error) {
		for i := 0; i < 16; i++ {
			h, e := readHeader()
			if e != nil {
				return h, e
			}
			if h.kind == kind {
				return h, nil
			}
			if h.kind == ZSKIP {
				return h, errors.New("zmodem: peer skipped file")
			}
			if h.kind != ZRINIT && h.kind != ZACK {
				return h, fmt.Errorf("%w: expected %d got %d", ErrFrameMalformed, kind, h.kind)
			}
		}
		return wireHeader{}, ErrFrameMalformed
	}
	if err = send(ZRQINIT, 0); err != nil {
		return err
	}
	if _, err = expect(ZRINIT); err != nil {
		return err
	}
	if err = s.Write(append(binaryHeader(ZFILE, 0), dataPacket([]byte(fmt.Sprintf("%s\x00%d 0 100644 0\x00", meta.Name, len(data))), zcrcw)...)); err != nil {
		return err
	}
	h, err := expect(ZRPOS)
	if err != nil {
		return err
	}
	offset := int(h.position)
	if offset > len(data) {
		return fmt.Errorf("%w: resume beyond file", ErrFrameMalformed)
	}
	if err = s.Write(binaryHeader(ZDATA, uint32(offset))); err != nil {
		return err
	}
	for offset < len(data) {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		end := min(offset+1024, len(data))
		if err = s.Write(dataPacket(data[offset:end], zcrcq)); err != nil {
			return err
		}
		h, err = expect(ZACK)
		if err != nil {
			return err
		}
		if h.position != uint32(end) {
			return fmt.Errorf("%w: incorrect ACK offset", ErrFrameMalformed)
		}
		offset = end
		if s.OnProgress != nil {
			s.OnProgress(int64(offset), int64(len(data)))
		}
	}
	if err = s.Write(dataPacket(nil, zcrce)); err != nil {
		return err
	}
	if err = send(ZEOF, uint32(offset)); err != nil {
		return err
	}
	if _, err = expect(ZRINIT); err != nil {
		return err
	}
	if err = send(ZFIN, 0); err != nil {
		return err
	}
	if _, err = expect(ZFIN); err != nil {
		return err
	}
	return s.Write([]byte("OO"))
}
