package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	WorkloadSustained    = "sustained"
	WorkloadLongLine     = "long-line"
	WorkloadMillionLines = "million-lines"
	WorkloadMetadataOnly = "metadata-only"
)

const canonicalPayloadSHA256 = "a72132b19b586c26f76ee1c635870b5dddb6727f744cd2099e9104745114c0e1"

type FixtureDistribution struct {
	Bytes int `json:"bytes"`
	Count int `json:"count"`
}

type FixtureParameters struct {
	Date                         string `json:"date"`
	ErrorText                    string `json:"errorText"`
	FailureText                  string `json:"failureText"`
	InfoText                     string `json:"infoText"`
	IPOctetModulo                int    `json:"ipOctetModulo"`
	IPPrefix                     string `json:"ipPrefix"`
	LineEnding                   string `json:"lineEnding"`
	LinesPerChunk                int    `json:"linesPerChunk"`
	PayloadCharacter             string `json:"payloadCharacter"`
	PayloadLength                int    `json:"payloadLength"`
	SecondIPOctetChunkMultiplier int    `json:"secondIpOctetChunkMultiplier"`
	WarnText                     string `json:"warnText"`
	WorkerModulo                 int    `json:"workerModulo"`
}

type SustainedFixture struct {
	FormatVersion int `json:"formatVersion"`
	Summary       struct {
		ChunkCount            int                   `json:"chunkCount"`
		ChunkSizeDistribution []FixtureDistribution `json:"chunkSizeDistribution"`
		PayloadSHA256         string                `json:"payloadSha256"`
		TotalBytes            int                   `json:"totalBytes"`
		TotalChars            int                   `json:"totalChars"`
	} `json:"summary"`
	Workload struct {
		CanonicalBenchmarkSource string            `json:"canonicalBenchmarkSource"`
		DefaultChunkCount        int               `json:"defaultChunkCount"`
		Encoding                 string            `json:"encoding"`
		GeneratorSource          string            `json:"generatorSource"`
		ID                       string            `json:"id"`
		Parameters               FixtureParameters `json:"parameters"`
		Version                  int               `json:"version"`
	} `json:"workload"`
}

type WorkloadOptions struct {
	ChunkCount     int `json:"chunkCount"`
	TotalBytes     int `json:"totalBytes"`
	LineCount      int `json:"lineCount"`
	MetadataFrames int `json:"metadataFrames"`
}

type WorkloadDescriptor struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var workloadDescriptors = []WorkloadDescriptor{
	{ID: WorkloadSustained, Label: "Canonical sustained output"},
	{ID: WorkloadLongLine, Label: "Long unbroken line"},
	{ID: WorkloadMillionLines, Label: "One million short lines"},
	{ID: WorkloadMetadataOnly, Label: "Metadata-only ingress"},
}

type generatedChunk struct {
	payload []byte
	cost    uint32
}

type workloadIterator interface {
	Next() (generatedChunk, bool, error)
	Steps() uint64
	Expectation() workloadExpectation
}

type workloadExpectation struct {
	payloadBytes uint64
	creditBytes  uint64
	frames       uint64
}

func loadCanonicalFixture() (SustainedFixture, error) {
	paths := []string{
		filepath.Join("..", "..", "testdata", "migration", "electron", "terminal-sustained-output-workload.json"),
		filepath.Join("testdata", "migration", "electron", "terminal-sustained-output-workload.json"),
	}
	if _, source, _, ok := runtime.Caller(0); ok {
		paths = append(paths, filepath.Join(filepath.Dir(source), "..", "..", "testdata", "migration", "electron", "terminal-sustained-output-workload.json"))
	}
	var data []byte
	var err error
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return SustainedFixture{}, fmt.Errorf("read canonical sustained fixture: %w", err)
	}
	var fixture SustainedFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return SustainedFixture{}, fmt.Errorf("decode canonical sustained fixture: %w", err)
	}
	if err := validateCanonicalFixture(fixture); err != nil {
		return SustainedFixture{}, err
	}
	return fixture, nil
}

func validateCanonicalFixture(fixture SustainedFixture) error {
	p := fixture.Workload.Parameters
	if fixture.FormatVersion != 1 || fixture.Workload.Version != 1 ||
		fixture.Workload.ID != "xterm-keyword-highlight-sustained-output" ||
		fixture.Workload.Encoding != "utf8" || fixture.Workload.DefaultChunkCount != 1600 {
		return errors.New("canonical sustained fixture identity or version changed")
	}
	if fixture.Summary.ChunkCount != 1600 || fixture.Summary.TotalBytes != 9_708_106 ||
		fixture.Summary.TotalChars != 9_708_106 || fixture.Summary.PayloadSHA256 != canonicalPayloadSHA256 {
		return errors.New("canonical sustained fixture summary changed")
	}
	if p.Date != "2026-08-13" || p.InfoText != "INFO" || p.WarnText != "WARN" ||
		p.ErrorText != "ERROR" || p.FailureText != "failed" || p.LinesPerChunk != 64 ||
		p.WorkerModulo != 32 || p.IPPrefix != "10.2" || p.IPOctetModulo != 255 ||
		p.SecondIPOctetChunkMultiplier != 7 || p.PayloadCharacter != "x" ||
		p.PayloadLength != 24 || p.LineEnding != "\r\n" {
		return errors.New("canonical sustained fixture parameters changed")
	}
	count, bytes := 0, 0
	for _, item := range fixture.Summary.ChunkSizeDistribution {
		if item.Bytes <= 0 || item.Count <= 0 {
			return errors.New("canonical sustained fixture has invalid distribution values")
		}
		count += item.Count
		bytes += item.Bytes * item.Count
	}
	if count != fixture.Summary.ChunkCount || bytes != fixture.Summary.TotalBytes {
		return errors.New("canonical sustained fixture distribution does not match its summary")
	}
	return nil
}

func newWorkload(id string, options WorkloadOptions, fixture SustainedFixture) (workloadIterator, error) {
	switch id {
	case WorkloadSustained:
		count := options.ChunkCount
		if count == 0 {
			count = fixture.Workload.DefaultChunkCount
		}
		if count < 1 || count > fixture.Workload.DefaultChunkCount {
			return nil, fmt.Errorf("sustained chunkCount must be between 1 and %d", fixture.Workload.DefaultChunkCount)
		}
		expectedBytes := uint64(0)
		if count == fixture.Workload.DefaultChunkCount {
			expectedBytes = uint64(fixture.Summary.TotalBytes)
		} else {
			for index := 0; index < count; index++ {
				expectedBytes += uint64(len(makeSustainedPayload(fixture, index)))
			}
		}
		return &sustainedWorkload{fixture: fixture, count: count, expectedBytes: expectedBytes}, nil
	case WorkloadLongLine:
		total := options.TotalBytes
		if total == 0 {
			total = 2 * receiveWindowBytes
		}
		if total < 1 || total > 64*receiveWindowBytes {
			return nil, errors.New("long-line totalBytes must be between 1 byte and 64 receive windows")
		}
		return &longLineWorkload{remaining: total, total: total}, nil
	case WorkloadMillionLines:
		count := options.LineCount
		if count == 0 {
			count = 1_000_000
		}
		if count < 1 || count > 1_000_000 {
			return nil, errors.New("lineCount must be between 1 and 1000000")
		}
		return &shortLinesWorkload{remaining: count, total: count}, nil
	case WorkloadMetadataOnly:
		count := options.MetadataFrames
		if count == 0 {
			count = 512
		}
		if count < 1 || count > 100_000 {
			return nil, errors.New("metadataFrames must be between 1 and 100000")
		}
		return &metadataWorkload{remaining: count, total: count}, nil
	default:
		return nil, fmt.Errorf("unknown workload %q", id)
	}
}

type sustainedWorkload struct {
	fixture       SustainedFixture
	count         int
	index         int
	steps         uint64
	expectedBytes uint64
}

func (w *sustainedWorkload) Next() (generatedChunk, bool, error) {
	if w.index >= w.count {
		return generatedChunk{}, false, nil
	}
	payload := makeSustainedPayload(w.fixture, w.index)
	w.index++
	w.steps++
	return generatedChunk{payload: payload, cost: uint32(len(payload))}, true, nil
}

func makeSustainedPayload(fixture SustainedFixture, index int) []byte {
	p := fixture.Workload.Parameters
	var builder strings.Builder
	for line := 0; line < p.LinesPerChunk; line++ {
		fmt.Fprintf(&builder, "%s %s worker=%d %s %s %s from %s.%d.%d payload=%s%s",
			p.Date, p.InfoText, line%p.WorkerModulo, p.WarnText, p.ErrorText, p.FailureText,
			p.IPPrefix, (index+line)%p.IPOctetModulo,
			(index*p.SecondIPOctetChunkMultiplier+line)%p.IPOctetModulo,
			strings.Repeat(p.PayloadCharacter, p.PayloadLength), p.LineEnding)
	}
	return []byte(builder.String())
}

func (w *sustainedWorkload) Steps() uint64 { return w.steps }
func (w *sustainedWorkload) Expectation() workloadExpectation {
	return workloadExpectation{payloadBytes: w.expectedBytes, creditBytes: w.expectedBytes, frames: uint64(w.count)}
}

type longLineWorkload struct {
	remaining int
	steps     uint64
	total     int
}

func (w *longLineWorkload) Next() (generatedChunk, bool, error) {
	if w.remaining == 0 {
		return generatedChunk{}, false, nil
	}
	size := min(w.remaining, maxPayloadBytes)
	w.remaining -= size
	w.steps++
	return generatedChunk{payload: []byte(strings.Repeat("L", size)), cost: uint32(size)}, true, nil
}

func (w *longLineWorkload) Steps() uint64 { return w.steps }
func (w *longLineWorkload) Expectation() workloadExpectation {
	total := uint64(w.total)
	return workloadExpectation{payloadBytes: total, creditBytes: total, frames: uint64((w.total + maxPayloadBytes - 1) / maxPayloadBytes)}
}

type shortLinesWorkload struct {
	remaining int
	index     int
	steps     uint64
	total     int
}

func (w *shortLinesWorkload) Next() (generatedChunk, bool, error) {
	if w.remaining == 0 {
		return generatedChunk{}, false, nil
	}
	var builder strings.Builder
	for w.remaining > 0 {
		line := fmt.Sprintf("line %07d\r\n", w.index)
		if builder.Len() > 0 && builder.Len()+len(line) > maxPayloadBytes {
			break
		}
		builder.WriteString(line)
		w.index++
		w.remaining--
	}
	w.steps++
	payload := []byte(builder.String())
	return generatedChunk{payload: payload, cost: uint32(len(payload))}, true, nil
}

func (w *shortLinesWorkload) Steps() uint64 { return w.steps }
func (w *shortLinesWorkload) Expectation() workloadExpectation {
	const lineBytes = 14
	linesPerFrame := maxPayloadBytes / lineBytes
	bytes := uint64(w.total * lineBytes)
	return workloadExpectation{payloadBytes: bytes, creditBytes: bytes, frames: uint64((w.total + linesPerFrame - 1) / linesPerFrame)}
}

type metadataWorkload struct {
	remaining int
	steps     uint64
	total     int
}

func (w *metadataWorkload) Next() (generatedChunk, bool, error) {
	if w.remaining == 0 {
		return generatedChunk{}, false, nil
	}
	w.remaining--
	w.steps++
	return generatedChunk{cost: 4096}, true, nil
}

func (w *metadataWorkload) Steps() uint64 { return w.steps }
func (w *metadataWorkload) Expectation() workloadExpectation {
	return workloadExpectation{creditBytes: uint64(w.total * 4096), frames: uint64(w.total)}
}
