package source

// This file is the Source-owned payload codec used by Program artifacts. It
// intentionally imports only the canonical framing primitive and keyspace:
// the Source child is responsible for decoding its own authored Input, while
// the parent artifact codec owns the surrounding envelope and seal replay.

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sourceArtifactRecordIdentity uint64 = iota + 1
	sourceArtifactRecordSpans
	sourceArtifactRecordLiterals
	sourceArtifactRecordOrder
	sourceArtifactRecordKeys
	sourceArtifactRecordSpellings
)

// WriteArtifactSection writes only the Source-authored payload. The caller
// owns the enclosing artifact stream, including its Header and Finish; this
// method therefore never resets or finishes the supplied Writer. The direct
// Source View is the only accepted owner capability; an unavailable view
// fails closed before any payload frame is emitted.
func WriteArtifactSection(writer *framing.Writer, view View) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	authority := liveAuthority(view.authority, nil)
	if authority == nil || !authority.content.Available() {
		return framing.ErrMalformed
	}
	return writeAuthoredPayload(writer, authority)
}

// ReadArtifactSection consumes exactly one Source-authored payload and leaves
// any following parent-artifact events unread. It returns construction Input,
// not a Component: the parent performs the ordinary sibling assembly and
// Source Commit, which is where derived Outcomes and all seal indexes belong.
// The decoder deliberately never calls Build.
func ReadArtifactSection(reader *framing.Reader) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}

	if err := sourceRecord(reader, sourceArtifactRecordIdentity); err != nil {
		return Input{}, err
	}
	nameBytes, err := sourceStringBytes(reader)
	if err != nil {
		return Input{}, err
	}
	if len(nameBytes) == 0 {
		return Input{}, framing.ErrMalformed
	}
	authoredTermCount, err := reader.Count()
	if err != nil {
		return Input{}, err
	}
	if authoredTermCount == 0 || authoredTermCount > uint64(^uint32(0)) {
		return Input{}, framing.ErrLimit
	}

	// Probe the complete payload on a value copy before copying the source
	// name or reserving any authored collection. A malformed final key/fault
	// row must not leave earlier spans, literals, or order pools allocated.
	probe := *reader
	if err := sourceRecord(&probe, sourceArtifactRecordSpans); err != nil {
		return Input{}, err
	}
	var counts [keyspace.FamilyCount]uint32
	if err := preflightSourceSpans(&probe, &counts); err != nil {
		return Input{}, err
	}
	var computedTermCount uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		computedTermCount += uint64(counts[family])
	}
	if computedTermCount == 0 || computedTermCount != authoredTermCount || computedTermCount > uint64(^uint32(0)) {
		return Input{}, framing.ErrMalformed
	}

	if err := sourceRecord(&probe, sourceArtifactRecordLiterals); err != nil {
		return Input{}, err
	}
	if err := preflightSourceLiterals(&probe, counts); err != nil {
		return Input{}, err
	}

	if err := sourceRecord(&probe, sourceArtifactRecordOrder); err != nil {
		return Input{}, err
	}
	faultOwners, err := preflightSourceOrder(&probe, counts)
	if err != nil {
		return Input{}, err
	}

	if err := sourceRecord(&probe, sourceArtifactRecordKeys); err != nil {
		return Input{}, err
	}
	if err := preflightSourceKeys(&probe, counts, faultOwners); err != nil {
		return Input{}, err
	}
	if err := sourceRecord(&probe, sourceArtifactRecordSpellings); err != nil {
		return Input{}, err
	}
	if err := preflightSourceSpellings(&probe, counts); err != nil {
		return Input{}, err
	}

	// The copied-reader proof above is complete. Only now may the real pass
	// copy the filename and allocate the Input-owned authored collections.
	name := string(nameBytes)
	if err := sourceRecord(reader, sourceArtifactRecordSpans); err != nil {
		return Input{}, err
	}
	decodedCounts, families, err := readSourceSpans(reader, name)
	if err != nil {
		return Input{}, err
	}
	if decodedCounts != counts {
		return Input{}, framing.ErrMalformed
	}
	input := Input{Name: name, Families: families}
	if err := sourceRecord(reader, sourceArtifactRecordLiterals); err != nil {
		return Input{}, err
	}
	if err := readSourceLiterals(reader, &input, counts); err != nil {
		return Input{}, err
	}

	if err := sourceRecord(reader, sourceArtifactRecordOrder); err != nil {
		return Input{}, err
	}
	faultOwners, err = readSourceOrder(reader, &input, counts)
	if err != nil {
		return Input{}, err
	}

	if err := sourceRecord(reader, sourceArtifactRecordKeys); err != nil {
		return Input{}, err
	}
	if err := readSourceKeys(reader, &input, counts, faultOwners); err != nil {
		return Input{}, err
	}
	if err := sourceRecord(reader, sourceArtifactRecordSpellings); err != nil {
		return Input{}, err
	}
	if err := readSourceSpellings(reader, &input, counts); err != nil {
		return Input{}, err
	}
	return input, nil
}

func sourceRecord(reader *framing.Reader, want uint64) error {
	got, err := reader.Record()
	if err != nil {
		return err
	}
	if got != want {
		return framing.ErrMalformed
	}
	return nil
}

// sourceCount checks an untrusted Count before any Go slice capacity is
// derived from it. minimumBytes is a conservative lower bound for one row's
// remaining canonical frames; using Remaining catches impossible arities even
// when a caller gives the Reader a very large external limit.
func sourceCount(reader *framing.Reader, maximum uint64, minimumBytes int) (int, error) {
	value, err := reader.Count()
	if err != nil {
		return 0, err
	}
	if value > maximum || value > sourceMaxInt() {
		return 0, framing.ErrLimit
	}
	if minimumBytes > 0 && value > uint64(reader.Remaining()/minimumBytes) {
		return 0, framing.ErrLimit
	}
	return int(value), nil
}

func sourceMaxInt() uint64 { return uint64(^uint(0) >> 1) }

func sourceString(reader *framing.Reader) (string, error) {
	value, err := sourceStringBytes(reader)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func sourceStringBytes(reader *framing.Reader) ([]byte, error) {
	limit := reader.Remaining()
	if uint64(limit) > sourceMaxInt() {
		limit = int(sourceMaxInt())
	}
	return reader.StringBytes(limit)
}
