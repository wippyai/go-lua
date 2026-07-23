package equation

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// FactStatus says whether a runtime fact is backed by the admitted artifact.
// The zero value is deliberately Unknown: a newly added field cannot become a
// JIT assumption merely because an older producer did not populate it.
type FactStatus uint8

const (
	FactUnknown FactStatus = iota
	FactProven
)

func (s FactStatus) valid() bool { return s == FactUnknown || s == FactProven }

// FactProvenance identifies the artifact and contract that supplied a fact.
// Operation is the dense, admission-frozen operation index.
type FactProvenance struct {
	ArtifactID ContentID
	ContractID ContentID
	Operation  uint32
}

func (p FactProvenance) validFor(artifact ContentID) bool {
	return p.ArtifactID == artifact && p.ContractID.Valid()
}

// RuntimeStorage is a VM storage location.  It intentionally mirrors only
// storage classes proven by source lowering; Unknown is never stack storage.
type RuntimeStorage uint8

const (
	RuntimeStorageUnknown RuntimeStorage = iota
	RuntimeStorageCanonicalArena
	RuntimeStorageEntryProjection
	RuntimeStorageEquationCell
	RuntimeStorageLocalFrame
	RuntimeStorageCaptureCell
	RuntimeStorageHeapPath
	RuntimeStorageCallResult
)

func (s RuntimeStorage) valid() bool {
	return s >= RuntimeStorageUnknown && s <= RuntimeStorageCallResult
}

func runtimeStorageForOperand(kind OperandKind) (RuntimeStorage, bool) {
	switch kind {
	case OperandCanonicalConstant:
		return RuntimeStorageCanonicalArena, true
	case OperandEntryProjection:
		return RuntimeStorageEntryProjection, true
	// These types are reserved by the compiled-artifact foundation.  They are
	// not admitted until a lowering can supply the corresponding proof.
	default:
		return RuntimeStorageUnknown, false
	}
}

// RuntimeValueTag describes a VM value shape.  Opaque equation terms produce
// no value-shape evidence, so Unknown is the only value currently admitted.
type RuntimeValueTag uint8

const (
	RuntimeValueUnknown RuntimeValueTag = iota
	RuntimeValueNil
	RuntimeValueBool
	RuntimeValueNumber
	RuntimeValueString
	RuntimeValueTable
	RuntimeValueFunction
	RuntimeValueUserdata
)

func (v RuntimeValueTag) valid() bool { return v <= RuntimeValueUserdata }

// RuntimeBoolFact is a boolean whose value is usable only when Status is
// FactProven.  Its zero value means "unknown", not "known false".
type RuntimeBoolFact struct {
	Value      bool
	Status     FactStatus
	Provenance FactProvenance
}

// RuntimeValueFact carries a tag/shape conclusion.
type RuntimeValueFact struct {
	Tag        RuntimeValueTag
	Status     FactStatus
	Provenance FactProvenance
}

// CaptureCellFact describes a concrete upvalue/capture cell.  No such cells
// are inferred from opaque operand bytes; later lowerings may publish them
// only with FactProven provenance.
type CaptureCellFact struct {
	CellID        uint32
	Mutable       RuntimeBoolFact
	Aliases       RuntimeBoolFact
	Owned         RuntimeBoolFact
	SurvivesYield RuntimeBoolFact
	Status        FactStatus
	Provenance    FactProvenance
}

// RuntimeEffectSummary reports effects relevant to JIT motion and placement.
// Every field has its own status so an absent field cannot be interpreted as a
// proven non-effect.
type RuntimeEffectSummary struct {
	HeapWrite    RuntimeBoolFact
	GlobalWrite  RuntimeBoolFact
	UpvalueWrite RuntimeBoolFact
	Allocation   RuntimeBoolFact
	Error        RuntimeBoolFact
	UserYield    RuntimeBoolFact
	SystemYield  RuntimeBoolFact
}

type RuntimeEscape uint8

const (
	RuntimeEscapeUnknown RuntimeEscape = iota
	RuntimeEscapeNone
	RuntimeEscapeReturn
	RuntimeEscapeStore
	RuntimeEscapeShare
)

type RuntimeOwnership uint8

const (
	RuntimeOwnershipUnknown RuntimeOwnership = iota
	RuntimeOwnershipBorrowed
	RuntimeOwnershipOwned
	RuntimeOwnershipShared
)

type RuntimeResidence uint8

const (
	RuntimeResidenceUnknown RuntimeResidence = iota
	RuntimeResidenceTransient
	RuntimeResidenceFrame
	RuntimeResidenceHeap
	RuntimeResidenceSharedHeap
)

type RuntimePlacement uint8

const (
	RuntimePlacementUnknown RuntimePlacement = iota
	RuntimePlacementInterpreter
	RuntimePlacementStack
	RuntimePlacementRegister
	RuntimePlacementOwnedHeap
	RuntimePlacementSharedHeap
)

// RuntimePlacementVerdicts records the independent escape/ownership/residence
// conclusions needed before a JIT moves an allocation or value.
type RuntimePlacementVerdicts struct {
	Escape     RuntimeEscape
	Ownership  RuntimeOwnership
	Residence  RuntimeResidence
	Placement  RuntimePlacement
	Status     FactStatus
	Provenance FactProvenance
}

// RuntimeAllocationTemplateFact is an identity, not an allocation itself.
// A zero identity is valid only for an unknown template.
type RuntimeAllocationTemplateFact struct {
	TemplateID ContentID
	Status     FactStatus
	Provenance FactProvenance
}

// RuntimeBoundaryFacts identifies control-flow boundaries.  Acyclic admission
// can prove that an operation is outside a loop/WTO region; whether opaque
// kernels suspend remains unknown.
type RuntimeBoundaryFacts struct {
	InLoop   RuntimeBoolFact
	InWTO    RuntimeBoolFact
	Suspends RuntimeBoolFact
}

// RuntimeOperandProjection is artifact-bound operand metadata.  Kind and
// Storage are always provenance-carrying; Shape is conservative for opaque
// term encodings.
type RuntimeOperandProjection struct {
	Kind       OperandKind
	Storage    RuntimeStorage
	Shape      RuntimeValueFact
	Status     FactStatus
	Provenance FactProvenance
}

// RuntimeGuardRequirement records that the operation has a guard.  Guard
// failure always deoptimizes; a projection does not grant permission to fold a
// guard's opaque expression.
type RuntimeGuardRequirement struct {
	Status     FactStatus
	Provenance FactProvenance
}

// RuntimeOperationProjection contains all runtime facts for one compact
// operation.  Slice ranges refer to immutable projection-owned arrays.
type RuntimeOperationProjection struct {
	ContractID                 ContentID
	OperandStart, OperandCount uint32
	GuardStart, GuardCount     uint32
	CaptureStart, CaptureCount uint32
	Effects                    RuntimeEffectSummary
	AllocationTemplate         RuntimeAllocationTemplateFact
	Placement                  RuntimePlacementVerdicts
	Boundaries                 RuntimeBoundaryFacts
}

// RuntimeProjection is the canonical, immutable runtime contract emitted by
// artifact admission.  Its slices are private; accessors return values or
// copies, so callers cannot alter a retained projection.
type RuntimeProjection struct {
	artifactID ContentID
	id         ContentID
	admitted   bool
	canonical  []byte
	ops        []RuntimeOperationProjection
	operands   []RuntimeOperandProjection
	guards     []RuntimeGuardRequirement
	captures   []CaptureCellFact
}

func (p RuntimeProjection) ArtifactID() ContentID { return p.artifactID }
func (p RuntimeProjection) ContentID() ContentID  { return p.id }

// Admitted reports whether this projection passed the artifact-bound runtime
// admission gate. Decoding validates wire shape, not artifact origin.
func (p RuntimeProjection) Admitted() bool         { return p.admitted }
func (p RuntimeProjection) CanonicalBytes() []byte { return append([]byte(nil), p.canonical...) }
func (p RuntimeProjection) OperationCount() int    { return len(p.ops) }
func (p RuntimeProjection) OperandCount() int      { return len(p.operands) }
func (p RuntimeProjection) GuardCount() int        { return len(p.guards) }
func (p RuntimeProjection) CaptureCount() int      { return len(p.captures) }

func (p RuntimeProjection) OperationAt(index int) (RuntimeOperationProjection, bool) {
	if index < 0 || index >= len(p.ops) {
		return RuntimeOperationProjection{}, false
	}
	return p.ops[index], true
}
func (p RuntimeProjection) OperandAt(index int) (RuntimeOperandProjection, bool) {
	if index < 0 || index >= len(p.operands) {
		return RuntimeOperandProjection{}, false
	}
	return p.operands[index], true
}
func (p RuntimeProjection) GuardAt(index int) (RuntimeGuardRequirement, bool) {
	if index < 0 || index >= len(p.guards) {
		return RuntimeGuardRequirement{}, false
	}
	return p.guards[index], true
}
func (p RuntimeProjection) CaptureAt(index int) (CaptureCellFact, bool) {
	if index < 0 || index >= len(p.captures) {
		return CaptureCellFact{}, false
	}
	return p.captures[index], true
}

// AdmitRuntimeProjection is the runtime-contract admission seam.  It accepts
// only an already canonical compiled artifact, verifies the artifact bytes,
// and publishes no fact that cannot be derived from its compact descriptors.
func AdmitRuntimeProjection(artifact CompiledArtifact) (RuntimeProjection, error) {
	if !artifact.id.Valid() || len(artifact.canonical) == 0 || len(artifact.ops) == 0 {
		return RuntimeProjection{}, fmt.Errorf("equation: runtime projection requires an admitted artifact")
	}
	reconstructed, err := artifact.ReferenceArtifact()
	if err != nil || reconstructed.ContentID() != artifact.id || !bytes.Equal(reconstructed.CanonicalBytes(), artifact.canonical) {
		return RuntimeProjection{}, fmt.Errorf("equation: runtime projection artifact confirmation failed")
	}
	p := RuntimeProjection{artifactID: artifact.id, ops: make([]RuntimeOperationProjection, len(artifact.ops)), operands: make([]RuntimeOperandProjection, len(artifact.operands)), guards: make([]RuntimeGuardRequirement, len(artifact.guards))}
	for index, op := range artifact.ops {
		if !op.ContractID.Valid() || uint64(op.OperandStart)+uint64(op.OperandCount) > uint64(len(artifact.operands)) || uint64(op.GuardStart)+uint64(op.GuardCount) > uint64(len(artifact.guards)) {
			return RuntimeProjection{}, fmt.Errorf("equation: runtime projection operation range is invalid")
		}
		provenance := FactProvenance{ArtifactID: artifact.id, ContractID: op.ContractID, Operation: uint32(index)}
		p.ops[index] = RuntimeOperationProjection{
			ContractID: op.ContractID, OperandStart: op.OperandStart, OperandCount: op.OperandCount, GuardStart: op.GuardStart, GuardCount: op.GuardCount,
			Effects: unknownEffects(provenance), AllocationTemplate: RuntimeAllocationTemplateFact{Status: FactUnknown, Provenance: provenance},
			Placement:  RuntimePlacementVerdicts{Status: FactUnknown, Provenance: provenance},
			Boundaries: RuntimeBoundaryFacts{InLoop: provenBool(false, provenance), InWTO: provenBool(false, provenance), Suspends: unknownBool(provenance)},
		}
		for operandIndex := op.OperandStart; operandIndex < op.OperandStart+op.OperandCount; operandIndex++ {
			storage, ok := runtimeStorageForOperand(artifact.operands[operandIndex].Kind)
			if !ok {
				return RuntimeProjection{}, fmt.Errorf("equation: runtime projection operand %d has unproven kind %d", operandIndex, artifact.operands[operandIndex].Kind)
			}
			p.operands[operandIndex] = RuntimeOperandProjection{Kind: artifact.operands[operandIndex].Kind, Storage: storage, Status: FactProven, Provenance: provenance, Shape: RuntimeValueFact{Status: FactUnknown, Provenance: provenance}}
		}
		for guardIndex := op.GuardStart; guardIndex < op.GuardStart+op.GuardCount; guardIndex++ {
			p.guards[guardIndex] = RuntimeGuardRequirement{Status: FactProven, Provenance: provenance}
		}
	}
	p.canonical = encodeRuntimeProjection(p)
	p.id = contentID(p.canonical)
	p.admitted = true
	return p, nil
}

// AdmitRuntimeProjectionEncoding is the transport-side admission gate. A
// codec decoder may validate syntax, but a runtime must bind bytes back to the
// exact compiled artifact before treating any fact as optimization evidence.
// Today the artifact proves only the conservative projection constructed by
// AdmitRuntimeProjection, so byte-for-byte equality is required.
func AdmitRuntimeProjectionEncoding(artifact CompiledArtifact, encoded []byte) (RuntimeProjection, error) {
	decoded, err := DecodeRuntimeProjection(encoded)
	if err != nil {
		return RuntimeProjection{}, err
	}
	expected, err := AdmitRuntimeProjection(artifact)
	if err != nil {
		return RuntimeProjection{}, err
	}
	if decoded.artifactID != expected.artifactID || !bytes.Equal(decoded.canonical, expected.canonical) {
		return RuntimeProjection{}, fmt.Errorf("equation: runtime projection encoding is not admitted for artifact")
	}
	return expected, nil
}

func provenBool(value bool, provenance FactProvenance) RuntimeBoolFact {
	return RuntimeBoolFact{Value: value, Status: FactProven, Provenance: provenance}
}
func unknownBool(provenance FactProvenance) RuntimeBoolFact {
	return RuntimeBoolFact{Status: FactUnknown, Provenance: provenance}
}
func unknownEffects(provenance FactProvenance) RuntimeEffectSummary {
	unknown := unknownBool(provenance)
	return RuntimeEffectSummary{HeapWrite: unknown, GlobalWrite: unknown, UpvalueWrite: unknown, Allocation: unknown, Error: unknown, UserYield: unknown, SystemYield: unknown}
}

// cyclicRuntimeProjection clears acyclic-only negative boundary facts before
// the compact operation schema is attached to a frozen WTO artifact.
func cyclicRuntimeProjection(p RuntimeProjection) RuntimeProjection {
	p.admitted = false
	for index := range p.ops {
		provenance := FactProvenance{ArtifactID: p.artifactID, ContractID: p.ops[index].ContractID, Operation: uint32(index)}
		p.ops[index].Boundaries.InLoop = unknownBool(provenance)
		p.ops[index].Boundaries.InWTO = unknownBool(provenance)
	}
	p.canonical = encodeRuntimeProjection(p)
	p.id = contentID(p.canonical)
	p.admitted = true
	return p
}

// JITAdmission is the whole-picture consumer contract.  A consumer may use a
// true permission only for the named optimization and must deopt when a guard
// fails.  Unknown facts produce the interpreter-safe zero-permission plan.
type JITAdmission struct {
	ArtifactID          ContentID
	ContractID          ContentID
	InterpreterSafe     bool
	DeoptOnGuardFailure bool
	StackPlacement      bool
	AllocationSinking   bool
	Inlining            bool
	BoundsSpecialize    bool
	TypeSpecialize      bool
	RegisterResidency   bool
}

// JITAdmissionAt is allocation-free and is safe on a warmed hot path. Every
// permission is a conjunction of the corresponding proven facts; a missing,
// unknown, or malformed fact returns interpreter-safe behavior instead.
func (p RuntimeProjection) JITAdmissionAt(index int) JITAdmission {
	if !p.admitted {
		return JITAdmission{InterpreterSafe: true, DeoptOnGuardFailure: true}
	}
	if index < 0 || index >= len(p.ops) {
		return JITAdmission{InterpreterSafe: true, DeoptOnGuardFailure: true}
	}
	op := p.ops[index]
	if !runtimeOperationRangeValid(p, op) {
		return JITAdmission{InterpreterSafe: true, DeoptOnGuardFailure: true}
	}
	noSuspend := provenFalse(op.Boundaries.Suspends) && provenFalse(op.Effects.UserYield) && provenFalse(op.Effects.SystemYield)
	noLoop := provenFalse(op.Boundaries.InLoop) && provenFalse(op.Boundaries.InWTO)
	stack := op.Placement.Status == FactProven && op.Placement.Escape == RuntimeEscapeNone && op.Placement.Ownership == RuntimeOwnershipOwned && (op.Placement.Residence == RuntimeResidenceTransient || op.Placement.Residence == RuntimeResidenceFrame) && op.Placement.Placement == RuntimePlacementStack && noSuspend && noLoop
	allocationSink := stack && op.AllocationTemplate.Status == FactProven && op.AllocationTemplate.TemplateID.Valid() && provenTrue(op.Effects.Allocation) && provenFalse(op.Effects.HeapWrite) && provenFalse(op.Effects.GlobalWrite) && provenFalse(op.Effects.UpvalueWrite)
	noObservableEffect := provenFalse(op.Effects.HeapWrite) && provenFalse(op.Effects.GlobalWrite) && provenFalse(op.Effects.UpvalueWrite) && provenFalse(op.Effects.Allocation) && provenFalse(op.Effects.Error) && noSuspend && noLoop
	typed := allOperandsHaveProvenShape(p, op)
	register := op.Placement.Status == FactProven && op.Placement.Placement == RuntimePlacementRegister && noSuspend
	return JITAdmission{
		ArtifactID: p.artifactID, ContractID: op.ContractID, InterpreterSafe: true, DeoptOnGuardFailure: true,
		StackPlacement: stack, AllocationSinking: allocationSink, Inlining: noObservableEffect,
		BoundsSpecialize: typed, TypeSpecialize: typed, RegisterResidency: register,
	}
}

func provenFalse(fact RuntimeBoolFact) bool { return fact.Status == FactProven && !fact.Value }
func provenTrue(fact RuntimeBoolFact) bool  { return fact.Status == FactProven && fact.Value }
func runtimeOperationRangeValid(p RuntimeProjection, op RuntimeOperationProjection) bool {
	return uint64(op.OperandStart)+uint64(op.OperandCount) <= uint64(len(p.operands)) && uint64(op.GuardStart)+uint64(op.GuardCount) <= uint64(len(p.guards)) && uint64(op.CaptureStart)+uint64(op.CaptureCount) <= uint64(len(p.captures))
}
func allOperandsHaveProvenShape(p RuntimeProjection, op RuntimeOperationProjection) bool {
	if op.OperandCount == 0 {
		return false
	}
	for index := op.OperandStart; index < op.OperandStart+op.OperandCount; index++ {
		shape := p.operands[index].Shape
		if shape.Status != FactProven || shape.Tag == RuntimeValueUnknown {
			return false
		}
	}
	return true
}

const runtimeProjectionEncoding = "equation-runtime-projection/v1"

func encodeRuntimeProjection(p RuntimeProjection) []byte {
	out := appendText(nil, runtimeProjectionEncoding)
	out = append(out, p.artifactID[:]...)
	out = appendU64(out, uint64(len(p.ops)))
	out = appendU64(out, uint64(len(p.operands)))
	out = appendU64(out, uint64(len(p.guards)))
	out = appendU64(out, uint64(len(p.captures)))
	for _, op := range p.ops {
		out = append(out, op.ContractID[:]...)
		out = appendU64(out, uint64(op.OperandStart))
		out = appendU64(out, uint64(op.OperandCount))
		out = appendU64(out, uint64(op.GuardStart))
		out = appendU64(out, uint64(op.GuardCount))
		out = appendU64(out, uint64(op.CaptureStart))
		out = appendU64(out, uint64(op.CaptureCount))
		out = appendOperationFacts(out, op)
	}
	for _, operand := range p.operands {
		out = append(out, byte(operand.Kind), byte(operand.Storage), byte(operand.Status))
		out = appendValueFact(out, operand.Shape)
		out = appendProvenance(out, operand.Provenance)
	}
	for _, guard := range p.guards {
		out = append(out, byte(guard.Status))
		out = appendProvenance(out, guard.Provenance)
	}
	for _, capture := range p.captures {
		out = appendU64(out, uint64(capture.CellID))
		out = append(out, byte(capture.Status))
		out = appendProvenance(out, capture.Provenance)
		out = appendBoolFact(out, capture.Mutable)
		out = appendBoolFact(out, capture.Aliases)
		out = appendBoolFact(out, capture.Owned)
		out = appendBoolFact(out, capture.SurvivesYield)
	}
	return out
}

func appendOperationFacts(out []byte, op RuntimeOperationProjection) []byte {
	for _, fact := range []RuntimeBoolFact{op.Effects.HeapWrite, op.Effects.GlobalWrite, op.Effects.UpvalueWrite, op.Effects.Allocation, op.Effects.Error, op.Effects.UserYield, op.Effects.SystemYield, op.Boundaries.InLoop, op.Boundaries.InWTO, op.Boundaries.Suspends} {
		out = appendBoolFact(out, fact)
	}
	out = append(out, byte(op.AllocationTemplate.Status))
	out = append(out, op.AllocationTemplate.TemplateID[:]...)
	out = appendProvenance(out, op.AllocationTemplate.Provenance)
	out = append(out, byte(op.Placement.Escape), byte(op.Placement.Ownership), byte(op.Placement.Residence), byte(op.Placement.Placement), byte(op.Placement.Status))
	return appendProvenance(out, op.Placement.Provenance)
}
func appendBoolFact(out []byte, fact RuntimeBoolFact) []byte {
	if fact.Value {
		out = append(out, 1)
	} else {
		out = append(out, 0)
	}
	out = append(out, byte(fact.Status))
	return appendProvenance(out, fact.Provenance)
}
func appendValueFact(out []byte, fact RuntimeValueFact) []byte {
	out = append(out, byte(fact.Tag), byte(fact.Status))
	return appendProvenance(out, fact.Provenance)
}
func appendProvenance(out []byte, provenance FactProvenance) []byte {
	out = append(out, provenance.ArtifactID[:]...)
	out = append(out, provenance.ContractID[:]...)
	return appendU64(out, uint64(provenance.Operation))
}

// DecodeRuntimeProjection validates and reconstructs a canonical projection.
// Decoding is admission/cold-path work; JITAdmissionAt itself does not decode,
// allocate, hash, or consult a map.
func DecodeRuntimeProjection(encoded []byte) (RuntimeProjection, error) {
	r := projectionReader{bytes: encoded}
	magic, err := r.text()
	if err != nil || magic != runtimeProjectionEncoding {
		return RuntimeProjection{}, fmt.Errorf("equation: invalid runtime projection encoding")
	}
	p := RuntimeProjection{}
	if p.artifactID, err = r.id(); err != nil || !p.artifactID.Valid() {
		return RuntimeProjection{}, fmt.Errorf("equation: invalid runtime projection artifact ID")
	}
	opCount, err := r.count()
	if err != nil {
		return RuntimeProjection{}, err
	}
	operandCount, err := r.count()
	if err != nil {
		return RuntimeProjection{}, err
	}
	guardCount, err := r.count()
	if err != nil {
		return RuntimeProjection{}, err
	}
	captureCount, err := r.count()
	if err != nil {
		return RuntimeProjection{}, err
	}
	p.ops = make([]RuntimeOperationProjection, opCount)
	p.operands = make([]RuntimeOperandProjection, operandCount)
	p.guards = make([]RuntimeGuardRequirement, guardCount)
	p.captures = make([]CaptureCellFact, captureCount)
	for index := range p.ops {
		if p.ops[index], err = r.operation(); err != nil {
			return RuntimeProjection{}, err
		}
	}
	for index := range p.operands {
		if p.operands[index], err = r.operand(); err != nil {
			return RuntimeProjection{}, err
		}
	}
	for index := range p.guards {
		if p.guards[index], err = r.guard(); err != nil {
			return RuntimeProjection{}, err
		}
	}
	for index := range p.captures {
		if p.captures[index], err = r.capture(); err != nil {
			return RuntimeProjection{}, err
		}
	}
	if r.remaining() != 0 || !validRuntimeProjection(p) {
		return RuntimeProjection{}, fmt.Errorf("equation: invalid runtime projection facts")
	}
	p.canonical = append([]byte(nil), encoded...)
	p.id = contentID(p.canonical)
	return p, nil
}

type projectionReader struct {
	bytes  []byte
	offset int
}

func (r *projectionReader) remaining() int { return len(r.bytes) - r.offset }
func (r *projectionReader) take(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, fmt.Errorf("equation: truncated runtime projection")
	}
	out := r.bytes[r.offset : r.offset+n]
	r.offset += n
	return out, nil
}
func (r *projectionReader) text() (string, error) {
	n, err := r.u64()
	if err != nil || n > uint64(r.remaining()) {
		return "", fmt.Errorf("equation: invalid runtime projection text")
	}
	raw, err := r.take(int(n))
	return string(raw), err
}
func (r *projectionReader) u64() (uint64, error) {
	raw, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(raw), nil
}
func (r *projectionReader) count() (int, error) {
	n, err := r.u64()
	if err != nil || n > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("equation: invalid runtime projection count")
	}
	return int(n), nil
}
func (r *projectionReader) byte() (byte, error) {
	raw, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return raw[0], nil
}
func (r *projectionReader) id() (ContentID, error) {
	raw, err := r.take(len(ContentID{}))
	if err != nil {
		return ContentID{}, err
	}
	var id ContentID
	copy(id[:], raw)
	return id, nil
}
func (r *projectionReader) provenance() (FactProvenance, error) {
	artifact, err := r.id()
	if err != nil {
		return FactProvenance{}, err
	}
	contract, err := r.id()
	if err != nil {
		return FactProvenance{}, err
	}
	operation, err := r.u64()
	if err != nil || operation > uint64(^uint32(0)) {
		return FactProvenance{}, fmt.Errorf("equation: invalid runtime projection provenance")
	}
	return FactProvenance{ArtifactID: artifact, ContractID: contract, Operation: uint32(operation)}, nil
}
func (r *projectionReader) boolFact() (RuntimeBoolFact, error) {
	value, err := r.byte()
	if err != nil || value > 1 {
		return RuntimeBoolFact{}, fmt.Errorf("equation: invalid runtime projection boolean")
	}
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return RuntimeBoolFact{}, fmt.Errorf("equation: invalid runtime projection status")
	}
	provenance, err := r.provenance()
	return RuntimeBoolFact{Value: value == 1, Status: FactStatus(status), Provenance: provenance}, err
}
func (r *projectionReader) valueFact() (RuntimeValueFact, error) {
	tag, err := r.byte()
	if err != nil || !RuntimeValueTag(tag).valid() {
		return RuntimeValueFact{}, fmt.Errorf("equation: invalid runtime projection tag")
	}
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return RuntimeValueFact{}, fmt.Errorf("equation: invalid runtime projection status")
	}
	provenance, err := r.provenance()
	return RuntimeValueFact{Tag: RuntimeValueTag(tag), Status: FactStatus(status), Provenance: provenance}, err
}
func (r *projectionReader) operation() (RuntimeOperationProjection, error) {
	var op RuntimeOperationProjection
	var err error
	if op.ContractID, err = r.id(); err != nil {
		return op, err
	}
	ranges := []*uint32{&op.OperandStart, &op.OperandCount, &op.GuardStart, &op.GuardCount, &op.CaptureStart, &op.CaptureCount}
	for _, target := range ranges {
		value, readErr := r.u64()
		if readErr != nil || value > uint64(^uint32(0)) {
			return op, fmt.Errorf("equation: invalid runtime projection range")
		}
		*target = uint32(value)
	}
	effects := []*RuntimeBoolFact{&op.Effects.HeapWrite, &op.Effects.GlobalWrite, &op.Effects.UpvalueWrite, &op.Effects.Allocation, &op.Effects.Error, &op.Effects.UserYield, &op.Effects.SystemYield, &op.Boundaries.InLoop, &op.Boundaries.InWTO, &op.Boundaries.Suspends}
	for _, target := range effects {
		if *target, err = r.boolFact(); err != nil {
			return op, err
		}
	}
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return op, fmt.Errorf("equation: invalid runtime projection status")
	}
	op.AllocationTemplate.Status = FactStatus(status)
	if op.AllocationTemplate.TemplateID, err = r.id(); err != nil {
		return op, err
	}
	if op.AllocationTemplate.Provenance, err = r.provenance(); err != nil {
		return op, err
	}
	values, err := r.take(5)
	if err != nil || values[0] > byte(RuntimeEscapeShare) || values[1] > byte(RuntimeOwnershipShared) || values[2] > byte(RuntimeResidenceSharedHeap) || values[3] > byte(RuntimePlacementSharedHeap) || !FactStatus(values[4]).valid() {
		return op, fmt.Errorf("equation: invalid runtime projection placement")
	}
	op.Placement = RuntimePlacementVerdicts{Escape: RuntimeEscape(values[0]), Ownership: RuntimeOwnership(values[1]), Residence: RuntimeResidence(values[2]), Placement: RuntimePlacement(values[3]), Status: FactStatus(values[4])}
	op.Placement.Provenance, err = r.provenance()
	return op, err
}
func (r *projectionReader) operand() (RuntimeOperandProjection, error) {
	kind, err := r.byte()
	if err != nil || kind < byte(OperandCanonicalConstant) || kind > byte(OperandCallResult) {
		return RuntimeOperandProjection{}, fmt.Errorf("equation: invalid runtime projection operand kind")
	}
	storage, err := r.byte()
	if err != nil || !RuntimeStorage(storage).valid() {
		return RuntimeOperandProjection{}, fmt.Errorf("equation: invalid runtime projection storage")
	}
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return RuntimeOperandProjection{}, fmt.Errorf("equation: invalid runtime projection status")
	}
	shape, err := r.valueFact()
	if err != nil {
		return RuntimeOperandProjection{}, err
	}
	provenance, err := r.provenance()
	return RuntimeOperandProjection{Kind: OperandKind(kind), Storage: RuntimeStorage(storage), Status: FactStatus(status), Shape: shape, Provenance: provenance}, err
}
func (r *projectionReader) guard() (RuntimeGuardRequirement, error) {
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return RuntimeGuardRequirement{}, fmt.Errorf("equation: invalid runtime projection guard")
	}
	provenance, err := r.provenance()
	return RuntimeGuardRequirement{Status: FactStatus(status), Provenance: provenance}, err
}
func (r *projectionReader) capture() (CaptureCellFact, error) {
	cell, err := r.u64()
	if err != nil || cell > uint64(^uint32(0)) {
		return CaptureCellFact{}, fmt.Errorf("equation: invalid runtime projection capture")
	}
	status, err := r.byte()
	if err != nil || !FactStatus(status).valid() {
		return CaptureCellFact{}, fmt.Errorf("equation: invalid runtime projection capture status")
	}
	provenance, err := r.provenance()
	if err != nil {
		return CaptureCellFact{}, err
	}
	capture := CaptureCellFact{CellID: uint32(cell), Status: FactStatus(status), Provenance: provenance}
	fields := []*RuntimeBoolFact{&capture.Mutable, &capture.Aliases, &capture.Owned, &capture.SurvivesYield}
	for _, target := range fields {
		if *target, err = r.boolFact(); err != nil {
			return CaptureCellFact{}, err
		}
	}
	return capture, nil
}

func validRuntimeProjection(p RuntimeProjection) bool {
	if !p.artifactID.Valid() || len(p.ops) == 0 {
		return false
	}
	for index, op := range p.ops {
		if !op.ContractID.Valid() || uint64(op.OperandStart)+uint64(op.OperandCount) > uint64(len(p.operands)) || uint64(op.GuardStart)+uint64(op.GuardCount) > uint64(len(p.guards)) || uint64(op.CaptureStart)+uint64(op.CaptureCount) > uint64(len(p.captures)) {
			return false
		}
		provenance := FactProvenance{ArtifactID: p.artifactID, ContractID: op.ContractID, Operation: uint32(index)}
		for _, fact := range []RuntimeBoolFact{op.Effects.HeapWrite, op.Effects.GlobalWrite, op.Effects.UpvalueWrite, op.Effects.Allocation, op.Effects.Error, op.Effects.UserYield, op.Effects.SystemYield, op.Boundaries.InLoop, op.Boundaries.InWTO, op.Boundaries.Suspends} {
			if !validBoolFact(fact, p.artifactID) {
				return false
			}
		}
		if !validTemplateFact(op.AllocationTemplate, p.artifactID) || !validPlacementVerdict(op.Placement, p.artifactID) {
			return false
		}
		for operandIndex := op.OperandStart; operandIndex < op.OperandStart+op.OperandCount; operandIndex++ {
			operand := p.operands[operandIndex]
			storage, known := runtimeStorageForOperand(operand.Kind)
			if !known || operand.Storage != storage || operand.Status != FactProven || operand.Provenance != provenance || !validValueFact(operand.Shape, p.artifactID) {
				return false
			}
		}
		for guardIndex := op.GuardStart; guardIndex < op.GuardStart+op.GuardCount; guardIndex++ {
			guard := p.guards[guardIndex]
			if guard.Status != FactProven || guard.Provenance != provenance {
				return false
			}
		}
	}
	for _, capture := range p.captures {
		if capture.Status != FactProven || !capture.Provenance.validFor(p.artifactID) || !validBoolFact(capture.Mutable, p.artifactID) || !validBoolFact(capture.Aliases, p.artifactID) || !validBoolFact(capture.Owned, p.artifactID) || !validBoolFact(capture.SurvivesYield, p.artifactID) {
			return false
		}
	}
	return true
}
func validBoolFact(f RuntimeBoolFact, artifact ContentID) bool {
	return f.Status.valid() && f.Provenance.validFor(artifact)
}
func validValueFact(f RuntimeValueFact, artifact ContentID) bool {
	return f.Tag.valid() && f.Status.valid() && f.Provenance.validFor(artifact)
}
func validTemplateFact(f RuntimeAllocationTemplateFact, artifact ContentID) bool {
	return f.Status.valid() && f.Provenance.validFor(artifact) && (f.Status != FactProven || f.TemplateID.Valid())
}
func validPlacementVerdict(f RuntimePlacementVerdicts, artifact ContentID) bool {
	return f.Escape <= RuntimeEscapeShare && f.Ownership <= RuntimeOwnershipShared && f.Residence <= RuntimeResidenceSharedHeap && f.Placement <= RuntimePlacementSharedHeap && f.Status.valid() && f.Provenance.validFor(artifact)
}
