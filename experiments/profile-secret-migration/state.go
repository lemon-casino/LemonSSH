package migrationprobe

import "errors"

type protocolState struct {
	nextSequence uint64
	totalBytes   int
	seenIDs      map[string]struct{}
	fixtures     []fixtureReceipt
	metadata     []metadataReceipt
	allSealed    bool
	allOpened    bool
	exactMatches bool
}

func newProtocolState() *protocolState {
	return &protocolState{
		nextSequence: 1,
		seenIDs:      make(map[string]struct{}),
		allSealed:    true,
		allOpened:    true,
		exactMatches: true,
	}
}

func (s *protocolState) checkRecord(sequence uint64, id string, size int) error {
	if s.nextSequence == 0 || sequence == ^uint64(0) || sequence != s.nextSequence {
		return errors.New("invalid record sequence")
	}
	if len(s.fixtures)+len(s.metadata) >= MaxRecords {
		return errors.New("record limit exceeded")
	}
	if _, exists := s.seenIDs[id]; exists {
		return errors.New("duplicate record id")
	}
	if size < 0 || size > MaxPlaintextBytes || size > MaxTotalBytes-s.totalBytes {
		return errors.New("plaintext limit exceeded")
	}
	return nil
}

func (s *protocolState) commitSecret(frame sealedFrame, size int, sealed, opened, exact bool) {
	s.seenIDs[frame.FixtureID] = struct{}{}
	s.fixtures = append(s.fixtures, fixtureReceipt{
		FixtureID:    frame.FixtureID,
		Purpose:      frame.Purpose,
		SourceFormat: frame.SourceFormat,
	})
	s.totalBytes += size
	s.allSealed = s.allSealed && sealed
	s.allOpened = s.allOpened && opened
	s.exactMatches = s.exactMatches && exact
	s.nextSequence++
}

func (s *protocolState) commitMetadata(frame sealedMetadataFrame, size int, opened, exact bool) {
	s.seenIDs[frame.MetadataID] = struct{}{}
	s.metadata = append(s.metadata, metadataReceipt{
		MetadataID:     frame.MetadataID,
		Classification: frame.Classification,
	})
	s.totalBytes += size
	s.allOpened = s.allOpened && opened
	s.exactMatches = s.exactMatches && exact
	s.nextSequence++
}

func (s *protocolState) count() int {
	return len(s.fixtures) + len(s.metadata)
}

func (s *protocolState) finish(frame finishFrame, provider string) (sanitizedReceipt, error) {
	if frame.Version != ProtocolVersion || frame.Type != "finish" || frame.Count != s.count() {
		return sanitizedReceipt{}, errors.New("finish mismatch")
	}
	receipt := sanitizedReceipt{
		ProtocolVersion: ProtocolVersion,
		Provider:        provider,
		ProviderVersion: ProviderVersion,
		EnvelopeVersion: EnvelopeVersion,
		Counts: receiptCounts{
			TotalRecords:    s.count(),
			SecretRecords:   len(s.fixtures),
			MetadataRecords: len(s.metadata),
		},
		Fixtures:     append([]fixtureReceipt(nil), s.fixtures...),
		Metadata:     append([]metadataReceipt(nil), s.metadata...),
		AllSealed:    s.allSealed,
		AllOpened:    s.allOpened,
		ExactMatches: s.exactMatches,
		ErrorCodes:   []string{},
	}
	receipt.Passed = receipt.AllSealed && receipt.AllOpened && receipt.ExactMatches
	return receipt, nil
}
