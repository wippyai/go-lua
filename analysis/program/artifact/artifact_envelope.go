package artifact

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/internal/framing"
)

// Envelope records are intentionally local to the root codec. Child owners
// retain their own record spaces and are called in the fixed S/F/T/M order.
const (
	recordEnvelope uint64 = iota + 1
	recordDependency
)

// Dependency names one exact external artifact prerequisite. It is an
// envelope row, not a semantic owner or a second Program identity authority.
type Dependency struct {
	Name string
	ID   identity.ContentID
}

// Metadata is the caller-supplied immutable envelope evidence. Dependencies
// are defensively copied and emitted in canonical name order by Encode.
type Metadata struct {
	Dependencies []Dependency
	Provenance   string
}

type envelope struct {
	Target       identity.ContentID
	Program      identity.ContentID
	Entry        keyspace.Term
	Provenance   string
	Dependencies []Dependency
}

func writeEnvelope(
	writer *framing.Writer,
	targetID, programID identity.ContentID,
	entry keyspace.Term,
	provenance string,
	dependencies []Dependency,
) error {
	if err := writer.Record(recordEnvelope); err != nil {
		return err
	}
	if err := writer.Bytes(targetID[:]); err != nil {
		return err
	}
	if err := writer.Bytes(programID[:]); err != nil {
		return err
	}
	if err := writer.Uint(uint64(entry)); err != nil {
		return err
	}
	if err := writer.String(provenance); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(dependencies))); err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if err := writer.Record(recordDependency); err != nil {
			return err
		}
		if err := writer.String(dependency.Name); err != nil {
			return err
		}
		if err := writer.Bytes(dependency.ID[:]); err != nil {
			return err
		}
	}
	return nil
}

func readEnvelope(
	reader *framing.Reader,
	expectedTarget identity.ContentID,
	stringBytes uint64,
	expected []Dependency,
) (envelope, error) {
	if reader == nil {
		return envelope{}, framing.ErrMalformed
	}
	record, err := reader.Record()
	if err != nil || record != recordEnvelope {
		return envelope{}, framing.ErrMalformed
	}
	targetID, err := readID(reader)
	if err != nil {
		return envelope{}, err
	}
	if targetID != expectedTarget {
		return envelope{}, ErrTargetMismatch
	}
	programID, err := readID(reader)
	if err != nil || !programID.Available() {
		return envelope{}, framing.ErrMalformed
	}
	entryValue, err := reader.Uint()
	if err != nil {
		return envelope{}, err
	}
	if entryValue > uint64(^uint32(0)) {
		return envelope{}, framing.ErrMalformed
	}
	entry := keyspace.Term(entryValue)
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 {
		return envelope{}, framing.ErrMalformed
	}
	provenance, err := reader.String(artifactMaxStrings)
	if err != nil {
		return envelope{}, err
	}
	if uint64(len(provenance)) > stringBytes {
		return envelope{}, ErrLimit
	}
	stringBytes -= uint64(len(provenance))
	count, err := reader.Count()
	if err != nil {
		return envelope{}, err
	}
	if count > uint64(reader.Remaining())/uint64(artifactDependencyMin) || count > uint64(artifactMaxInt) {
		return envelope{}, ErrLimit
	}
	// Prove every dependency row against the caller's canonical manifest on a
	// copied Reader before reserving the final dependency slice. The probe uses
	// raw string payloads, so malformed or unauthenticated rows do not become
	// Go allocations.
	probe := *reader
	if err := preflightDependencies(&probe, count, expected, stringBytes); err != nil {
		return envelope{}, err
	}
	if count != uint64(len(expected)) {
		return envelope{}, ErrDependencyMismatch
	}
	dependencies := make([]Dependency, int(count))
	for index := range dependencies {
		kind, err := reader.Record()
		if err != nil || kind != recordDependency {
			return envelope{}, framing.ErrMalformed
		}
		name, err := reader.String(artifactMaxStrings)
		if err != nil {
			return envelope{}, err
		}
		if uint64(len(name)) > stringBytes {
			return envelope{}, ErrLimit
		}
		stringBytes -= uint64(len(name))
		id, err := readID(reader)
		if err != nil {
			return envelope{}, err
		}
		if name == "" || !id.Available() {
			return envelope{}, framing.ErrMalformed
		}
		if name != expected[index].Name || id != expected[index].ID {
			return envelope{}, ErrDependencyMismatch
		}
		dependencies[index] = Dependency{Name: name, ID: id}
	}
	return envelope{
		Target:       targetID,
		Program:      programID,
		Entry:        entry,
		Provenance:   provenance,
		Dependencies: dependencies,
	}, nil
}

func preflightDependencies(
	reader *framing.Reader,
	count uint64,
	expected []Dependency,
	stringBytes uint64,
) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	var previous []byte
	manifestMismatch := count != uint64(len(expected))
	noncanonical := false
	for index := uint64(0); index < count; index++ {
		kind, err := reader.Record()
		if err != nil || kind != recordDependency {
			return framing.ErrMalformed
		}
		name, err := reader.StringBytes(artifactMaxStrings)
		if err != nil {
			return err
		}
		if len(name) == 0 {
			return framing.ErrMalformed
		}
		if index != 0 && bytes.Compare(previous, name) >= 0 {
			noncanonical = true
		}
		if uint64(len(name)) > stringBytes {
			return ErrLimit
		}
		stringBytes -= uint64(len(name))
		id, err := readID(reader)
		if err != nil {
			return err
		}
		if !id.Available() {
			return framing.ErrMalformed
		}
		if index >= uint64(len(expected)) ||
			!equalBytesString(name, expected[index].Name) || id != expected[index].ID {
			manifestMismatch = true
		}
		previous = name
	}
	if noncanonical {
		return framing.ErrMalformed
	}
	if manifestMismatch {
		return ErrDependencyMismatch
	}
	return nil
}

func equalBytesString(value []byte, want string) bool {
	if len(value) != len(want) {
		return false
	}
	for index := range value {
		if value[index] != want[index] {
			return false
		}
	}
	return true
}

func readID(reader *framing.Reader) (identity.ContentID, error) {
	bytesValue, err := reader.Bytes(len(identity.ContentID{}))
	if err != nil {
		return identity.ContentID{}, err
	}
	if len(bytesValue) != len(identity.ContentID{}) {
		return identity.ContentID{}, framing.ErrMalformed
	}
	var id identity.ContentID
	copy(id[:], bytesValue)
	return id, nil
}

func canonicalDependencies(metadata Metadata) ([]Dependency, error) {
	if len(metadata.Provenance) > artifactMaxStrings {
		return nil, ErrLimit
	}
	stringBytes := uint64(len(metadata.Provenance))
	var nameBytes uint64
	for _, dependency := range metadata.Dependencies {
		if dependency.Name == "" || !dependency.ID.Available() {
			return nil, ErrUnavailableProgram
		}
		width := uint64(len(dependency.Name))
		if width > uint64(artifactMaxBytes)-nameBytes {
			return nil, ErrLimit
		}
		nameBytes += width
		if width > uint64(artifactMaxStrings)-stringBytes {
			return nil, ErrLimit
		}
		stringBytes += width
	}
	if uint64(len(metadata.Dependencies)) > uint64(artifactMaxBytes)/uint64(artifactDependencyMin) ||
		nameBytes > uint64(artifactMaxBytes) {
		return nil, ErrLimit
	}
	dependencies := append([]Dependency(nil), metadata.Dependencies...)
	sort.Slice(dependencies, func(left, right int) bool {
		return dependencies[left].Name < dependencies[right].Name
	})
	for index := 1; index < len(dependencies); index++ {
		if dependencies[index-1].Name == dependencies[index].Name {
			return nil, ErrUnavailableProgram
		}
	}
	return dependencies, nil
}

func targetIdentity(contract *target.Contract) (identity.ContentID, bool) {
	if contract == nil {
		return identity.ContentID{}, false
	}
	id := contract.ContentID()
	return id, id.Available()
}

func ownerViewsAvailable(p *program.Program) bool {
	return p.Source().Identity().ContentID().Available() &&
		p.Flow().ContentID().Available() &&
		p.Static().ContentID().Available() &&
		p.Module().ContentID().Available()
}
