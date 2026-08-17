// Package artifact owns Program's one portable persistence boundary.
//
// The artifact stream stores only the immutable target/envelope identity and
// the four authored owner sections.  Flow, Source positions, Static indexes,
// Module resolution, and every other derived projection are rebuilt through
// the ordinary owner assembly on decode.
package artifact

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/relations"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const (
	artifactDomain  = "program/artifact"
	artifactVersion = 19

	// These limits are deliberately owned by this package.  They bound both
	// the encoded byte stream and the amount of reconstruction work admitted
	// from an untrusted byte stream.
	artifactMaxBytes   = 256 << 20
	artifactMaxEvents  = 32 << 20
	artifactMaxStrings = 64 << 20
	artifactMaxInt     = int(^uint(0) >> 1)
	// Record + String + Bytes(32-byte ContentID), using canonical's smallest
	// one-byte length varint for each frame. This is a pre-allocation floor;
	// the child decoders perform their own exact row preflights.
	artifactDependencyMin = 3 + 3 + 34
)

// Envelope records are intentionally local to the root codec. Child owners
// retain their own record spaces and are called in the fixed S/F/T/M order.
const (
	recordEnvelope uint64 = iota + 1
	recordDependency
)

// Dependency names one exact external artifact prerequisite.  It is an
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

var (
	ErrUnavailableTarget  = errors.New("program artifact: unavailable target contract")
	ErrUnavailableProgram = errors.New("program artifact: unavailable Program")
	ErrUnavailableSchema  = errors.New("program artifact: unavailable canonical relations schema")
	ErrTargetMismatch     = errors.New("program artifact: target identity mismatch")
	ErrSchemaMismatch     = errors.New("program artifact: relations schema digest mismatch")
	ErrDependencyMismatch = errors.New("program artifact: dependency manifest mismatch")
	ErrNoncanonical       = errors.New("program artifact: noncanonical encoding")
	ErrLimit              = errors.New("program artifact: resource limit")
)

// Encode binds one published Program to one exact sealed target Contract.
// There is no unbound, legacy, or compatibility representation.
func Encode(p *program.Program, contract *target.Contract, metadata Metadata) ([]byte, error) {
	targetID, ok := targetIdentity(contract)
	if !ok {
		return nil, ErrUnavailableTarget
	}
	if p == nil || !p.ContentID().Available() || !ownerViewsAvailable(p) {
		return nil, ErrUnavailableProgram
	}
	relationsSchema, err := canonicalRelationsSchemaDigest()
	if err != nil {
		return nil, err
	}
	dependencies, err := canonicalDependencies(metadata)
	if err != nil {
		return nil, err
	}
	entry, ok := p.Source().Index().Entry()
	if !ok {
		return nil, ErrUnavailableProgram
	}

	destination := newArtifactBuffer(artifactMaxBytes)
	var writer framing.Writer
	if err := writer.Reset(destination, artifactDomain, artifactVersion); err != nil {
		return nil, encodeError(err)
	}
	if err := writeEnvelope(&writer, targetID, p.ContentID(), relationsSchema, entry, metadata.Provenance, dependencies); err != nil {
		return nil, encodeError(err)
	}
	// These are the only four payload authorities and their order is part of
	// the v19 stream grammar.
	if err := source.WriteArtifactSection(&writer, p.Source()); err != nil {
		return nil, encodeError(err)
	}
	if err := flow.WriteArtifactSection(&writer, p.Flow()); err != nil {
		return nil, encodeError(err)
	}
	if err := static.WriteArtifactSection(&writer, p.Static()); err != nil {
		return nil, encodeError(err)
	}
	if err := imports.WriteArtifactSection(&writer, p.Module()); err != nil {
		return nil, encodeError(err)
	}
	if err := writer.Finish(); err != nil {
		return nil, encodeError(err)
	}

	data := destination.Bytes()
	measure, err := framing.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, encodeError(err)
	}
	if !artifactMeasureAllowed(measure) {
		return nil, ErrLimit
	}
	return data, nil
}

// Decode accepts only the v19 stream bound to contract and reconstructs a
// fresh owner quartet through the ordinary Build/Finalizer/Assemble/Publish
// path. No derived section is read or retained.
func Decode(data []byte, contract *target.Contract, expectedDependencies []Dependency) (*program.Program, Metadata, error) {
	targetID, ok := targetIdentity(contract)
	if !ok {
		return nil, Metadata{}, ErrUnavailableTarget
	}
	if len(data) > artifactMaxBytes {
		return nil, Metadata{}, ErrLimit
	}
	expected, err := canonicalDependencies(Metadata{Dependencies: expectedDependencies})
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return nil, Metadata{}, ErrLimit
		}
		return nil, Metadata{}, ErrDependencyMismatch
	}
	measure, err := framing.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if !artifactMeasureAllowed(measure) {
		return nil, Metadata{}, ErrLimit
	}
	relationsSchema, err := canonicalRelationsSchemaDigest()
	if err != nil {
		return nil, Metadata{}, err
	}
	reader, err := framing.NewReader(data, artifactMaxBytes)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if err := reader.Header(artifactDomain, artifactVersion); err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	envelope, err := readEnvelope(reader, targetID, relationsSchema, measure.StringBytes, expected)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}

	// Child sections are decoded in the same fixed order as Encode. Each child
	// parser performs its own value-copy preflight before allocating rows.
	sourceInput, err := source.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	flowInput, err := flow.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	staticInput, err := static.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	moduleInput, err := imports.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if err := reader.Finish(); err != nil {
		return nil, Metadata{}, decodeError(err)
	}

	p, err := rebuild(sourceInput, flowInput, staticInput, moduleInput, envelope.Entry)
	if err != nil {
		return nil, Metadata{}, decodeError(fmt.Errorf("rebuild: %w", err))
	}
	if p.ContentID() != envelope.Program {
		return nil, Metadata{}, ErrNoncanonical
	}
	metadata := Metadata{
		Provenance:   envelope.Provenance,
		Dependencies: append([]Dependency(nil), envelope.Dependencies...),
	}
	canonicalBytes, err := Encode(p, contract, metadata)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if !bytes.Equal(data, canonicalBytes) {
		return nil, Metadata{}, ErrNoncanonical
	}
	return p, metadata, nil
}

type envelope struct {
	Target       identity.ContentID
	Program      identity.ContentID
	Relations    identity.ContentID
	Entry        keyspace.Term
	Provenance   string
	Dependencies []Dependency
}

func writeEnvelope(
	writer *framing.Writer,
	targetID, programID identity.ContentID,
	relationsSchema identity.ContentID,
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
	if err := writer.Bytes(relationsSchema[:]); err != nil {
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
	expectedRelationsSchema identity.ContentID,
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
	relationsSchema, err := readID(reader)
	if err != nil || !relationsSchema.Available() {
		return envelope{}, framing.ErrMalformed
	}
	if relationsSchema != expectedRelationsSchema {
		return envelope{}, ErrSchemaMismatch
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
		Relations:    relationsSchema,
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

func canonicalRelationsSchemaDigest() (identity.ContentID, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil || schema == nil {
		return identity.ContentID{}, ErrUnavailableSchema
	}
	digest := schema.Digest()
	if !digest.Available() {
		return identity.ContentID{}, ErrUnavailableSchema
	}
	return digest, nil
}

func ownerViewsAvailable(p *program.Program) bool {
	return p.Source().Identity().ContentID().Available() &&
		p.Flow().ContentID().Available() &&
		p.Static().ContentID().Available() &&
		p.Module().ContentID().Available()
}

func artifactMeasureAllowed(measure framing.StreamMeasure) bool {
	return measure.Events <= artifactMaxEvents && measure.StringBytes <= artifactMaxStrings
}

// artifactBuffer is the local all-or-nothing persistence sink. A Writer error
// never exposes its partially filled bytes because Encode returns nil on all
// failures.
type artifactBuffer struct {
	data  []byte
	limit int
}

func newArtifactBuffer(limit int) *artifactBuffer { return &artifactBuffer{limit: limit} }

func (buffer *artifactBuffer) Write(value []byte) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(value) > buffer.limit-len(buffer.data) {
		return 0, ErrLimit
	}
	if !buffer.reserve(len(value)) {
		return 0, ErrLimit
	}
	buffer.data = append(buffer.data, value...)
	return len(value), nil
}

func (buffer *artifactBuffer) WriteString(value string) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(value) > buffer.limit-len(buffer.data) {
		return 0, ErrLimit
	}
	if !buffer.reserve(len(value)) {
		return 0, ErrLimit
	}
	buffer.data = append(buffer.data, value...)
	return len(value), nil
}

func (buffer *artifactBuffer) reserve(extra int) bool {
	if buffer == nil || buffer.limit < 0 || extra < 0 || len(buffer.data) > buffer.limit-extra {
		return false
	}
	need := len(buffer.data) + extra
	if cap(buffer.data) >= need {
		return true
	}
	// Grow geometrically for normal streams, but clamp capacity to the hard
	// persistence ceiling instead of letting append overshoot it.
	capacity := cap(buffer.data) * 2
	if capacity < need {
		capacity = need
	}
	if capacity > buffer.limit {
		capacity = buffer.limit
	}
	next := make([]byte, len(buffer.data), capacity)
	copy(next, buffer.data)
	buffer.data = next
	return true
}

func (buffer *artifactBuffer) Bytes() []byte {
	if buffer == nil {
		return nil
	}
	return buffer.data
}

func encodeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrLimit) || errors.Is(err, framing.ErrLimit) {
		return ErrLimit
	}
	return ErrUnavailableProgram
}

func decodeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrTargetMismatch) {
		return ErrTargetMismatch
	}
	if errors.Is(err, ErrSchemaMismatch) {
		return ErrSchemaMismatch
	}
	if errors.Is(err, ErrUnavailableSchema) {
		return ErrUnavailableSchema
	}
	if errors.Is(err, ErrDependencyMismatch) {
		return ErrDependencyMismatch
	}
	if errors.Is(err, ErrLimit) || errors.Is(err, framing.ErrLimit) {
		return ErrLimit
	}
	if errors.Is(err, ErrNoncanonical) {
		return ErrNoncanonical
	}
	return fmt.Errorf("%w: %v", ErrNoncanonical, err)
}

// rebuild injects the dense source/flow universes into the authored child
// Inputs. Child codecs intentionally decode only their owner relations; the
// four-way term census remains Source's canonical denominator.
func rebuild(
	sourceInput source.Input,
	flowInput flow.Input,
	staticInput static.Input,
	moduleInput imports.Input,
	entry keyspace.Term,
) (*program.Program, error) {
	counts := sourceCounts(sourceInput)
	if !ownerDenominatorsAgree(counts, flowInput, staticInput, moduleInput) {
		return nil, errors.New("artifact: child denominators disagree with Source")
	}
	flowInput.Counts = flowCounts(counts, flowInput)
	staticInput.Counts = staticCounts(counts, flowInput, staticInput)

	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		return nil, err
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, nil, nil))
	}
	moduleDraft, err := imports.Build(moduleInput)
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, staticDraft, nil))
	}

	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, staticDraft, moduleDraft))
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		cleanup := errors.Join(sourceFinalizer.Abort(), abortUnclaimedDrafts(nil, staticDraft, moduleDraft))
		return nil, errors.Join(err, cleanup)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		cleanup := errors.Join(staticFinalizer.Abort(), sourceFinalizer.Abort(), abortUnclaimedDrafts(nil, nil, moduleDraft))
		return nil, errors.Join(err, cleanup)
	}
	// Flow is built only after every sibling Draft has either been claimed or
	// terminalized. This removes the otherwise-unabortable unclaimed Flow
	// window: a Flow Build failure can cleanly abort the three already-claimed
	// owner finalizers, and every later failure is inside Assemble's lifecycle.
	flowDraft, err := flow.Build(flowInput)
	if err != nil {
		cleanup := errors.Join(abortModuleFinalizer(moduleFinalizer), staticFinalizer.Abort(), sourceFinalizer.Abort())
		return nil, errors.Join(err, cleanup)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, entry)
	if err != nil {
		// Assemble normally aborts every claimed owner itself. These finalizers
		// are also aborted here for the pre-claim failure path: it is valid for
		// the flow draft to reject before it owns the three other drafts. Abort
		// is terminal and idempotent at the owner boundary, so this does not
		// reopen or otherwise alter Flow ownership.
		_ = moduleFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		return nil, err
	}
	return program.Publish(assembly)
}

func abortModuleFinalizer(finalizer imports.Finalizer) error {
	if finalizer.Abort() {
		return nil
	}
	return errors.New("program/module: finalizer abort failed")
}

// abortUnclaimedDrafts closes every owner Draft that has been built but not
// yet claimed by a Finalizer. Build is intentionally performed before any
// cross-owner assembly, so a later sibling failure must not leave an earlier
// construction capability open. The helper is used only on failure paths;
// successful drafts are claimed below or consumed by Assemble.
func abortUnclaimedDrafts(
	sourceDraft *source.Draft,
	staticDraft *static.Draft,
	moduleDraft *imports.Draft,
) error {
	var cleanup error
	if sourceDraft != nil {
		finalizer, err := sourceDraft.Finalizer()
		if err == nil {
			err = finalizer.Abort()
		}
		cleanup = errors.Join(cleanup, err)
	}
	if staticDraft != nil {
		finalizer, err := staticDraft.Finalizer()
		if err == nil {
			err = finalizer.Abort()
		}
		cleanup = errors.Join(cleanup, err)
	}
	if moduleDraft != nil {
		finalizer, err := moduleDraft.Finalizer()
		if err == nil && !finalizer.Abort() {
			err = errors.New("program/module: draft abort failed")
		}
		cleanup = errors.Join(cleanup, err)
	}
	return cleanup
}

// ownerDenominatorsAgree closes the one shared dense Term census before any
// child Build can reinterpret a foreign row. Sparse Static ClaimTarget rows
// are intentionally checked as a subset below; all other owner relations are
// dense by their canonical family.
func ownerDenominatorsAgree(
	counts [keyspace.FamilyCount]uint32,
	flowInput flow.Input,
	staticInput static.Input,
	moduleInput imports.Input,
) bool {
	length := func(family keyspace.Family, value int) bool {
		return value >= 0 && uint64(value) == uint64(counts[family])
	}
	if !length(keyspace.FamilyValues, len(flowInput.Values.Rows)) ||
		!length(keyspace.FamilyLensExact, len(flowInput.Access.Exact)) ||
		!length(keyspace.FamilyLensKey, len(flowInput.Access.Dynamic)) ||
		!length(keyspace.FamilyCell, len(flowInput.Storage.Cells)) ||
		!length(keyspace.FamilyRead, len(flowInput.Storage.Reads)) ||
		!length(keyspace.FamilyVararg, len(flowInput.Storage.Varargs)) ||
		!length(keyspace.FamilyBind, len(flowInput.Storage.Binds)) ||
		!length(keyspace.FamilyAssign, len(flowInput.Storage.Assigns)) ||
		!length(keyspace.FamilyWrite, len(flowInput.Storage.Writes)) ||
		!length(keyspace.FamilyTable, len(flowInput.Tables.Rows)) ||
		!length(keyspace.FamilyTableField, len(flowInput.Tables.Fields)) ||
		!length(keyspace.FamilyUnary, len(flowInput.Operators.Unaries)) ||
		!length(keyspace.FamilyBinary, len(flowInput.Operators.Binaries)) ||
		!length(keyspace.FamilySelect, len(flowInput.Operators.Selects)) ||
		!length(keyspace.FamilyFunction, len(flowInput.Functions.Rows)) ||
		!length(keyspace.FamilyCall, len(flowInput.Calls)) ||
		!length(keyspace.FamilyReturn, len(flowInput.Control.Returns)) ||
		!length(keyspace.FamilyBreak, len(flowInput.Control.Breaks)) ||
		!length(keyspace.FamilyLabel, len(flowInput.Control.Labels)) ||
		!length(keyspace.FamilyGoto, len(flowInput.Control.Gotos)) ||
		!length(keyspace.FamilyBranch, len(flowInput.Control.Branches)) ||
		!length(keyspace.FamilyLoop, len(flowInput.Control.Loops)) ||
		!length(keyspace.FamilyValueClaim, len(flowInput.Claims)) ||
		!length(keyspace.FamilyTypeValue, len(flowInput.TypeValues)) ||
		!length(keyspace.FamilyImport, len(moduleInput.Imports)) {
		return false
	}
	if uint64(len(staticInput.Operands.Claim)) > uint64(counts[keyspace.FamilyValueClaim]) {
		return false
	}
	staticLength := func(family keyspace.Family, value int) bool {
		return value >= 0 && uint64(value) == uint64(counts[family])
	}
	return staticLength(keyspace.FamilyTypePrimitive, len(staticInput.Types.Primitive)) &&
		staticLength(keyspace.FamilyTypeLiteral, len(staticInput.Types.Literal)) &&
		staticLength(keyspace.FamilyTypeOptional, len(staticInput.Types.Optional)) &&
		staticLength(keyspace.FamilyTypeUnion, len(staticInput.Types.Union)) &&
		staticLength(keyspace.FamilyTypeIntersection, len(staticInput.Types.Intersection)) &&
		staticLength(keyspace.FamilyTypeRef, len(staticInput.References.TypeRef)) &&
		staticLength(keyspace.FamilyTypeGeneric, len(staticInput.Types.Generic)) &&
		staticLength(keyspace.FamilyTypeArray, len(staticInput.Types.Array)) &&
		staticLength(keyspace.FamilyTypeMap, len(staticInput.Types.Map)) &&
		staticLength(keyspace.FamilyTypeRecord, len(staticInput.Types.Record)) &&
		staticLength(keyspace.FamilyTypeField, len(staticInput.Types.Field)) &&
		staticLength(keyspace.FamilyTypeAlias, len(staticInput.Declarations.Alias)) &&
		staticLength(keyspace.FamilyTypeParam, len(staticInput.Declarations.TypeParam)) &&
		staticLength(keyspace.FamilyTypeInterface, len(staticInput.Declarations.Interface)) &&
		staticLength(keyspace.FamilyDeclaredType, len(staticInput.Declarations.DeclaredType)) &&
		staticLength(keyspace.FamilyTypeFunction, len(staticInput.Signatures.TypeFunction)) &&
		staticLength(keyspace.FamilyTypeAsserts, len(staticInput.Signatures.TypeAsserts)) &&
		staticLength(keyspace.FamilyFunction, len(staticInput.Contracts.Function)) &&
		staticLength(keyspace.FamilyCall, len(staticInput.Contracts.Call)) &&
		staticLength(keyspace.FamilyTypePublication, len(staticInput.Publications.Type)) &&
		staticLength(keyspace.FamilyTypeValue, len(staticInput.Operands.TypeValue)) &&
		staticLength(keyspace.FamilyAnnotation, len(staticInput.Operands.Annotation)) &&
		staticLength(keyspace.FamilyTypeOf, len(staticInput.Operators.TypeOf)) &&
		staticLength(keyspace.FamilyTypeKeyOf, len(staticInput.Operators.KeyOf)) &&
		staticLength(keyspace.FamilyTypeIndexAccess, len(staticInput.Operators.IndexAccess)) &&
		staticLength(keyspace.FamilyTypeConditional, len(staticInput.Operators.Conditional))
}

func sourceCounts(input source.Input) [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	for _, row := range input.Families {
		if row.Family > keyspace.FamilyInvalid && row.Family < keyspace.FamilyCount {
			counts[row.Family] = uint32(len(row.Spans))
		}
	}
	return counts
}

func flowCounts(counts [keyspace.FamilyCount]uint32, input flow.Input) [keyspace.FamilyCount]uint32 {
	counts[keyspace.FamilyValues] = uint32(len(input.Values.Rows))
	counts[keyspace.FamilyLensExact] = uint32(len(input.Access.Exact))
	counts[keyspace.FamilyLensKey] = uint32(len(input.Access.Dynamic))
	counts[keyspace.FamilyCell] = uint32(len(input.Storage.Cells))
	counts[keyspace.FamilyRead] = uint32(len(input.Storage.Reads))
	counts[keyspace.FamilyVararg] = uint32(len(input.Storage.Varargs))
	counts[keyspace.FamilyBind] = uint32(len(input.Storage.Binds))
	counts[keyspace.FamilyAssign] = uint32(len(input.Storage.Assigns))
	counts[keyspace.FamilyWrite] = uint32(len(input.Storage.Writes))
	counts[keyspace.FamilyTable] = uint32(len(input.Tables.Rows))
	counts[keyspace.FamilyTableField] = uint32(len(input.Tables.Fields))
	counts[keyspace.FamilyUnary] = uint32(len(input.Operators.Unaries))
	counts[keyspace.FamilyBinary] = uint32(len(input.Operators.Binaries))
	counts[keyspace.FamilySelect] = uint32(len(input.Operators.Selects))
	counts[keyspace.FamilyFunction] = uint32(len(input.Functions.Rows))
	counts[keyspace.FamilyCall] = uint32(len(input.Calls))
	counts[keyspace.FamilyReturn] = uint32(len(input.Control.Returns))
	counts[keyspace.FamilyBreak] = uint32(len(input.Control.Breaks))
	counts[keyspace.FamilyLabel] = uint32(len(input.Control.Labels))
	counts[keyspace.FamilyGoto] = uint32(len(input.Control.Gotos))
	counts[keyspace.FamilyBranch] = uint32(len(input.Control.Branches))
	counts[keyspace.FamilyLoop] = uint32(len(input.Control.Loops))
	counts[keyspace.FamilyValueClaim] = uint32(len(input.Claims))
	counts[keyspace.FamilyTypeValue] = uint32(len(input.TypeValues))
	return counts
}

func staticCounts(counts [keyspace.FamilyCount]uint32, flowInput flow.Input, input static.Input) [keyspace.FamilyCount]uint32 {
	counts[keyspace.FamilyTypePrimitive] = uint32(len(input.Types.Primitive))
	counts[keyspace.FamilyTypeLiteral] = uint32(len(input.Types.Literal))
	counts[keyspace.FamilyTypeOptional] = uint32(len(input.Types.Optional))
	counts[keyspace.FamilyTypeUnion] = uint32(len(input.Types.Union))
	counts[keyspace.FamilyTypeIntersection] = uint32(len(input.Types.Intersection))
	counts[keyspace.FamilyTypeRef] = uint32(len(input.References.TypeRef))
	counts[keyspace.FamilyTypeGeneric] = uint32(len(input.Types.Generic))
	counts[keyspace.FamilyTypeArray] = uint32(len(input.Types.Array))
	counts[keyspace.FamilyTypeMap] = uint32(len(input.Types.Map))
	counts[keyspace.FamilyTypeRecord] = uint32(len(input.Types.Record))
	counts[keyspace.FamilyTypeField] = uint32(len(input.Types.Field))
	counts[keyspace.FamilyTypeAlias] = uint32(len(input.Declarations.Alias))
	counts[keyspace.FamilyTypeParam] = uint32(len(input.Declarations.TypeParam))
	counts[keyspace.FamilyTypeInterface] = uint32(len(input.Declarations.Interface))
	counts[keyspace.FamilyDeclaredType] = uint32(len(input.Declarations.DeclaredType))
	counts[keyspace.FamilyTypeFunction] = uint32(len(input.Signatures.TypeFunction))
	counts[keyspace.FamilyTypeAsserts] = uint32(len(input.Signatures.TypeAsserts))
	counts[keyspace.FamilyTypePublication] = uint32(len(input.Publications.Type))
	counts[keyspace.FamilyTypeValue] = uint32(len(input.Operands.TypeValue))
	counts[keyspace.FamilyAnnotation] = uint32(len(input.Operands.Annotation))
	counts[keyspace.FamilyTypeOf] = uint32(len(input.Operators.TypeOf))
	counts[keyspace.FamilyTypeKeyOf] = uint32(len(input.Operators.KeyOf))
	counts[keyspace.FamilyTypeIndexAccess] = uint32(len(input.Operators.IndexAccess))
	counts[keyspace.FamilyTypeConditional] = uint32(len(input.Operators.Conditional))
	// Static's dense Function/Call universe and sparse ValueClaim relation are
	// owned by Flow; preserve those exact rows from the already decoded input.
	counts[keyspace.FamilyFunction] = uint32(len(flowInput.Functions.Rows))
	counts[keyspace.FamilyCall] = uint32(len(flowInput.Calls))
	counts[keyspace.FamilyValueClaim] = uint32(len(flowInput.Claims))
	return counts
}
