package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

// RuleAdmission is the one sealed authority that permits a Rule evaluator to
// contribute ordinary State. It has exactly two constructors: an explicitly
// named/versioned trusted theorem (a reviewed TCB obligation), or a versioned
// local derivation checker. The zero value is deliberately not an admission.
//
// A trusted theorem is not a checked artifact, an exhaustive proof, or a
// verifier result. It is intentionally classified as trust until a future
// artifact basis arrives together with its one authoritative verifier.
//
// A checker is not a second evaluator.  Wave E supplies it a sealed local
// derivation built from one evaluator result, its resolved operands and its
// exact occurrence anchor; the checker can only accept or reject that
// derivation and return opaque evidence.  It receives no State, scheduler,
// carrier, or Patch capability.
type RuleAdmission[V, O any] struct {
	kind     ruleAdmissionKind
	identity SemanticKey
	check    RuleDerivationChecker[V, O]
}

type ruleAdmissionKind uint8

const (
	ruleAdmissionInvalid ruleAdmissionKind = iota
	ruleAdmissionTrustedTheorem
	ruleAdmissionDerivation
)

// RuleAdmissionBasis is the complete public provenance classification of a
// sealed Rule admission. It is reported from Composition's canonical cold
// schema, rather than inferred from a callback or package convention.
type RuleAdmissionBasis uint8

const (
	RuleAdmissionBasisInvalid RuleAdmissionBasis = iota
	// RuleAdmissionBasisTrustedTheorem is a named/versioned reviewed TCB
	// obligation. It is deliberately not a claim that an artifact was checked
	// or that a proof is exhaustive.
	RuleAdmissionBasisTrustedTheorem
	// RuleAdmissionBasisDerivation is admitted only by the exact local
	// derivation checker named by the row identity.
	RuleAdmissionBasisDerivation
)

// RuleDerivationChecker validates exactly one evaluator result.  The runtime
// invokes it before Patch admission; a false result, panic, stale identity,
// or foreign derivation rejects the candidate.  Its argument and return value
// are opaque capabilities issued by the runtime, not mutable engine state.
type RuleDerivationChecker[V, O any] func(RuleDerivation[V, O]) (RuleEvidence, bool)

// RuleDerivation is the sealed local-check input.  Its concrete operand,
// resolved-target, guard, and result accessors are introduced with the Wave-E
// binding surface; this cold declaration layer intentionally cannot mint one.
// Keeping the type opaque prevents a checker from fabricating a successful
// admission or using an unrelated evaluator result.
type RuleDerivation[V, O any] struct {
	proof          *ruleRuntimeProof
	composition    CompositionID
	identity       SemanticKey
	epoch          identity.Generation
	anchor         SemanticKey
	operandContent [32]byte
	coordinates    ActivationCoordinates
	inputs         []RuleInput
	reads          []RuleRead
	dispositions   []RuleDisposition[V]
	product        *productSession
	ticket         *ruleAdmissionTicket
	operand        O
}

// Rule returns the exact Rule schema identity that produced this derivation.
func (derivation RuleDerivation[V, O]) Rule() SemanticKey {
	if derivation.proof == nil || !derivation.proof.valid() {
		return SemanticKey{}
	}
	return semanticKeyFromComposition(derivation.proof.semantic)
}

// Composition returns the sealed composition identity under which the
// derivation was issued.  A checker can use it to reject foreign proof input.
func (derivation RuleDerivation[V, O]) Composition() CompositionID {
	return derivation.composition
}

// Anchor returns the exact compiled Rule-instance identity. It is derived by
// the sole equation compiler and includes that instance's sealed structural
// anchor and resolved surface, never a caller-supplied occurrence label.
func (derivation RuleDerivation[V, O]) Anchor() SemanticKey {
	if !derivation.valid() {
		return SemanticKey{}
	}
	return derivation.anchor
}

// Operand returns the exact immutable O installed for this canonical Rule
// instance. The typed value is carried directly by the bound Rule; neither a
// source lookup nor an erased runtime registry participates in admission.
func (derivation RuleDerivation[V, O]) Operand() (O, bool) {
	var zero O
	if !derivation.valid() {
		return zero, false
	}
	return derivation.operand, true
}

// Coordinates exposes the exact accepted dynamic relation when this
// derivation came from an activation-materialized Rule row. It does not
// expose the equation Member, family axes, or any selection authority.
func (derivation RuleDerivation[V, O]) Coordinates() (ActivationCoordinates, bool) {
	if !derivation.valid() || !derivation.coordinates.Available() {
		return ActivationCoordinates{}, false
	}
	return derivation.coordinates, true
}

// OperandContentMatches proves that the typed O was bound to an equation
// Operand carrying this canonical content digest. The private source-encoding
// version is not exposed to domain proof code.
func (derivation RuleDerivation[V, O]) OperandContentMatches(digest [32]byte) bool {
	return derivation.valid() && digest != [32]byte{} && derivation.operandContent == digest
}

// InputCount, InputAt, ReadCount, ReadAt, DispositionCount, and
// DispositionAt expose only immutable local proof operands. They deliberately
// provide no carrier, scheduler, Patch, or ambient State capability.
func (derivation RuleDerivation[V, O]) InputCount() int {
	if !derivation.valid() {
		return 0
	}
	return len(derivation.inputs)
}

func (derivation RuleDerivation[V, O]) InputAt(index int) (RuleInput, bool) {
	if !derivation.valid() || index < 0 || index >= len(derivation.inputs) {
		return RuleInput{}, false
	}
	return derivation.inputs[index], true
}

func (derivation RuleDerivation[V, O]) ReadCount() int {
	if !derivation.valid() {
		return 0
	}
	return len(derivation.reads)
}

func (derivation RuleDerivation[V, O]) ReadAt(index int) (RuleRead, bool) {
	if !derivation.valid() || index < 0 || index >= len(derivation.reads) {
		return RuleRead{}, false
	}
	return derivation.reads[index], true
}

func (derivation RuleDerivation[V, O]) DispositionCount() int {
	if !derivation.valid() {
		return 0
	}
	return len(derivation.dispositions)
}

func (derivation RuleDerivation[V, O]) DispositionAt(index int) (RuleDisposition[V], bool) {
	if !derivation.valid() || index < 0 || index >= len(derivation.dispositions) {
		return RuleDisposition[V]{}, false
	}
	disposition := derivation.dispositions[index]
	// RuleTarget has no mutation capability and its backing comes solely from
	// sealed assembly. Returning this value therefore needs no allocation.
	return disposition, true
}

// ruleResultRow is one exact Product row attached internally to a Rule
// disposition. Proof code receives a complete disposition, never a freely
// pairable row, so a typed proof read is structurally about that same row.
type ruleResultRow struct {
	ticket *ruleAdmissionTicket
	index  int
}

// DerivationDispositionReadValue resolves precisely the typed input observed
// by disposition's own Product row. It reuses the one live Product session:
// no erased value, State access, or second product is created. The
// disposition membership witness and canonical row must agree, so a foreign,
// stale, swapped, forged, or post-callback access fails closed.
func DerivationDispositionReadValue[V, O, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[S]) (S, bool) {
	var zero S
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal || derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || disposition.row.index < 0 || disposition.row.index >= len(derivation.product.values) || !read.matchesRuleProof(derivation.proof) || read.index >= len(derivation.product.reads) || read.resolve == nil {
		return zero, false
	}
	id, found := derivation.product.readID(disposition.row.index, read.index)
	if !found {
		return zero, false
	}
	return read.resolve(derivation.product, read.index, id)
}

// DerivationDispositionSelectionCount exposes the cardinality of one live
// staged read for this exact disposition row. It is proof-time only: neither
// the dynamic Ref route, Unit, State, nor a mutable execution Access escapes.
func DerivationDispositionSelectionCount[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]]) (int, bool) {
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || selection.count == nil {
		return 0, false
	}
	return selection.count(row)
}

// DerivationDispositionSelectionAt exposes one canonical Tag/value pair from
// a live staged read for this exact disposition row. It is intentionally not
// an Access-based Selection operation, which lets a cross-package checker
// replay its selection evidence without receiving a mutation capability.
func DerivationDispositionSelectionAt[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], ordinal int) (Tag, S, bool) {
	var tag Tag
	var value S
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || selection.at == nil || ordinal < 0 {
		return tag, value, false
	}
	return selection.at(row, ordinal)
}

// DerivationDispositionSelectionMatchesRef proves that one selected staged
// route was resolved to exactly expected. It is deliberately a predicate:
// evidence can authenticate a tag-to-Ref association without receiving a
// route, Factor key, carrier Unit, or mutable selection capability.
//
// This is distinct from DerivationReadMatchesRef. A staged read has no one
// fixed Ref, and distinct selected routes may carry equal normalized facts.
// It is also distinct from DerivationDispositionRouteValue, which applies
// only when the selected route is itself a RouteWrite output target.
func DerivationDispositionSelectionMatchesRef[V any, O any, Tag selectionTag, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], ordinal int, expected Ref[K]) bool {
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || ordinal < 0 || selection.count == nil || selection.route == nil {
		return false
	}
	count, counted := selection.count(row)
	if !counted || ordinal >= count {
		return false
	}
	route, routed := selection.route(row, ordinal)
	return routed && selectionRouteMatchesRef(route, expected)
}

// selectionRefIdentity is the sealed identity shared by a public expected
// Ref and the private exactRef retained by one staged route. Its fields never
// leave engine: external evidence receives only the boolean result above.
// The actual route was already validated against its staged target Factor by
// stagedRouteSink, so this comparison authenticates the same owner-issued
// exact coordinate without constructing a second route authority.
type selectionRefIdentity struct {
	compositionID    CompositionID
	bindingAuthority *schemaBindingAuthority
	factorKey        composition.Key
	factorIndex      uint64
	raw              uint64
}

type selectionRefIdentityProvider interface {
	selectionRefIdentity() selectionRefIdentity
}

func (ref Ref[K]) selectionRefIdentity() selectionRefIdentity {
	return selectionRefIdentity{
		compositionID:    ref.compositionID,
		bindingAuthority: ref.bindingAuthority,
		factorKey:        ref.factorKey,
		factorIndex:      ref.factorIndex,
		raw:              uint64(ref.raw),
	}
}

func selectionRouteMatchesRef[K ~uint32 | ~uint64](route exactRef, expected Ref[K]) bool {
	if route == nil {
		return false
	}
	actual, ok := route.(selectionRefIdentityProvider)
	return ok && actual.selectionRefIdentity() == expected.selectionRefIdentity()
}

// derivationSelectedRead authenticates one selected-read capability through
// the sealed selected-read proof.
func derivationSelectedRead[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], read Read[Selection[Tag, S]]) bool {
	if derivation.proof == nil || !derivation.proof.valid() || read.index < 0 || read.resolve == nil || !read.matchesRuleProof(derivation.proof) {
		return false
	}
	selected := derivation.proof.selectedReadAt(uint64(read.index))
	return selected != nil && selected.Valid() && selected.read == uint64(read.index)
}

func derivationDispositionSelection[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]]) (Selection[Tag, S], int, bool) {
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal ||
		derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || !derivationSelectedRead(derivation, read) {
		return Selection[Tag, S]{}, 0, false
	}
	id, found := derivation.product.readID(disposition.row.index, read.index)
	if !found {
		return Selection[Tag, S]{}, 0, false
	}
	selection, resolved := read.resolve(derivation.product, read.index, id)
	if !resolved || selection.count == nil {
		return Selection[Tag, S]{}, 0, false
	}
	return selection, disposition.row.index, true
}

// DerivationDispositionRouteValue resolves the exact tag/value pair that
// justified one atomic route output. It is deliberately narrower than a
// general Selection projection: the requested Read must be the Rule's one
// declared RouteWrite input, the disposition must belong to this live
// derivation row, and the output ordinal must name the same canonical route.
// Neither Ref, Unit, State, nor an alternate fact projection escapes.
func DerivationDispositionRouteValue[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], output RuleOutput[V]) (Tag, S, bool) {
	var tag Tag
	var value S
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal ||
		derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || !derivationSelectedRead(derivation, read) || output.ordinal < 0 {
		return tag, value, false
	}
	var routeRead uint64
	if derivation.proof != nil && derivation.proof.routeWrite != nil && derivation.proof.routeWrite.Valid() {
		routeRead = derivation.proof.routeWrite.read + 1
	}
	if routeRead == 0 || int(routeRead-1) != read.index {
		return tag, value, false
	}
	// A target and ordinal are not an output identity: another derivation may
	// lawfully stage the same target at the same ordinal with a distinct V.
	// The private witness is installed before the checker sees this derivation
	// and ties the public output handle to this exact ticket, row, and route
	// ordinal without asking V to be comparable.
	candidate, candidateOK := disposition.OutputAt(output.ordinal)
	if !candidateOK || candidate.ordinal != output.ordinal || !candidate.target.Same(output.target) ||
		candidate.witness.ticket == nil || candidate.witness != output.witness || candidate.witness.ticket != derivation.ticket ||
		candidate.witness.row != ordinal || candidate.witness.ordinal != output.ordinal {
		return tag, value, false
	}
	selection, row, resolved := derivationDispositionSelection(derivation, disposition, read)
	if !resolved || selection.route == nil {
		return tag, value, false
	}
	count, counted := selection.count(row)
	if !counted || output.ordinal >= count {
		return tag, value, false
	}
	return DerivationDispositionSelectionAt(derivation, disposition, read, output.ordinal)
}

// RuleInput is an opaque immutable input-State snapshot. It supports only
// identity and common-support comparison; a derivation checker cannot read a
// Factor root or attach a new one.
type RuleInput struct{ state carrier.State }

func (input RuleInput) Same(other RuleInput) bool { return input.state.SamePublishedInput(other.state) }
func (input RuleInput) Guard() RuleGuard          { return RuleGuard{mask: input.state.Support()} }

// RuleRead is one exact input-qualified resolved read observation. Unit stays
// opaque; Same is the only capability exposed to local proof code.
type RuleRead struct {
	input uint64
	unit  carrier.Unit
}

func (read RuleRead) Input() uint64 { return read.input }
func (read RuleRead) Same(other RuleRead) bool {
	return read.input == other.input && read.unit.Same(other.unit)
}

// ruleReadProof is the compact sealed identity retained only for exact reads.
// Summary and selector runtimes carry the zero value and therefore cannot
// satisfy an exact Ref proof.
type ruleReadProof struct {
	schema           *Schema
	bindingAuthority *schemaBindingAuthority
	factorIndex      uint64
	raw              uint64
	exact            bool
}

func newRuleReadReceiptProof(receipt factorRuntimeReceipt, surface equation.Surface) (ruleReadProof, bool) {
	if !receipt.valid() || surface.Factor != receipt.semantic || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > receipt.keyEnd {
		return ruleReadProof{}, false
	}
	return ruleReadProof{schema: receipt.schema, bindingAuthority: receipt.authority, factorIndex: receipt.ordinal, raw: surface.Local - 1, exact: true}, true
}

// ruleSummaryReadProof is the private sealed identity of one summary Unit.
// The canonical key vector is retained for cold shape bookkeeping; hot
// evidence authenticates the sealed scalar digest and never replays it.
type ruleSummaryReadProof struct {
	receipt     factorRuntimeReceipt
	formReceipt factorFormReceipt
	keys        []uint64
	digest      [32]byte
}

func summaryProofMatchesRefs[K ~uint32 | ~uint64](proof ruleSummaryReadProof, refs *ClosedRefs[K]) bool {
	return proof.receipt.valid() && refs != nil && refs.closed && refs.validIssuer() && refs.receipt.state == proof.receipt.state && refs.receipt.authority == proof.receipt.authority && refs.receipt.schema == proof.receipt.schema && refs.receipt.ordinal == proof.receipt.ordinal && refs.receipt.semantic == proof.receipt.semantic && proof.formReceipt.kind == SchemaFormReadSummary && proof.formReceipt.semantic != (composition.Key{}) && len(refs.refs) == len(proof.keys) && refs.digest == proof.digest
}

// DerivationReadMatchesRef proves that one checker-visible typed read was
// bound to exactly the owner-issued Ref supplied by the domain. It inspects
// the live product's sealed read runtime; no coordinate, equation Surface, or
// carrier Unit is exposed or reconstructed.
func DerivationReadMatchesRef[V, O, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], read Read[S], ref Ref[K]) bool {
	if !derivation.liveProduct() || !read.matchesRuleProof(derivation.proof) ||
		read.index >= len(derivation.product.reads) || read.resolve == nil {
		return false
	}
	runtime := derivation.product.reads[read.index]
	if runtime == nil {
		return false
	}
	proof := runtime.exactProof()
	return proof.schema != nil && proof.exact && ref.bindingAuthority == proof.bindingAuthority && ref.compositionID == proof.schema.ID() && ref.factorKey == proof.schema.factorSemanticAt(proof.factorIndex) && ref.factorIndex == proof.factorIndex && proof.raw == uint64(ref.raw)
}

// DerivationReadMatchesSummaryRefs proves that one checker-visible typed
// summary read was bound to exactly the closed, owner-issued Ref vector
// supplied by the domain. The comparison remains inside the sealed runtime:
// it exposes neither coordinates nor the summary Unit, and accepts no
// alternate evidence path.
func DerivationReadMatchesSummaryRefs[V, O, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], read Read[S], refs *ClosedRefs[K]) bool {
	if !derivation.liveProduct() || !read.matchesRuleProof(derivation.proof) ||
		read.index >= len(derivation.product.reads) || read.resolve == nil {
		return false
	}
	runtime := derivation.product.reads[read.index]
	return runtime != nil && summaryProofMatchesRefs(runtime.summaryProof(), refs)
}

// RuleGuard is the exact resolved output region of a staged result. It is an
// opaque shared-support value and cannot create, widen, or restrict guards.
type RuleGuard struct{ mask support.Mask }

func (guard RuleGuard) Same(other RuleGuard) bool {
	return guard.mask.Valid() && other.mask.Valid() && guard.mask.Equal(other.mask)
}
func (guard RuleGuard) Empty() bool { return !guard.mask.Valid() || support.Empty(guard.mask) }

// RuleTarget is an opaque resolved write capability. It permits identity
// comparison only, so a checker can validate target correspondence without
// manufacturing or retargeting a Patch.
type RuleTarget struct {
	target carrier.Target
	proof  ruleTargetProof
}

type ruleTargetProof struct {
	schema           *Schema
	state            *schemaBindingState
	bindingAuthority *schemaBindingAuthority
	factorIndex      uint64
	raw              uint64
	strong           bool
}

func newRuleTargetReceiptProof(receipt factorRuntimeReceipt, surface equation.Surface) (ruleTargetProof, bool) {
	if !receipt.valid() || surface.Factor != receipt.semantic || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeStrong || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > receipt.keyEnd {
		return ruleTargetProof{}, false
	}
	return ruleTargetProof{schema: receipt.schema, state: receipt.state, bindingAuthority: receipt.authority, factorIndex: receipt.ordinal, raw: surface.Local - 1, strong: true}, true
}

func (proof ruleTargetProof) validOrigin() bool {
	return proof.schema != nil && proof.schema.Available() && proof.state != nil && proof.bindingAuthority != nil && proof.state.phase == schemaBindingSealed && proof.state.schema == proof.schema && proof.state.authority == proof.bindingAuthority && proof.factorIndex < proof.schema.factorCount() && proof.strong
}

func (target RuleTarget) Same(other RuleTarget) bool {
	return target.target.Same(other.target) && target.proof == other.proof
}

// TargetMatchesRef proves that one checker-visible staged target is exactly
// the owner-issued Ref supplied by the domain. It compares only authenticated
// sealed surface identity; neither the raw coordinate nor the equation
// representation is exposed.
func TargetMatchesRef[K ~uint32 | ~uint64](target RuleTarget, ref Ref[K]) bool {
	if target.target == (carrier.Target{}) || !target.proof.validOrigin() {
		return false
	}
	return ref.bindingAuthority == target.proof.bindingAuthority && ref.compositionID == target.proof.schema.ID() && ref.factorKey == target.proof.schema.factorSemanticAt(target.proof.factorIndex) && ref.factorIndex == target.proof.factorIndex && target.proof.raw == uint64(ref.raw)
}

// RuleDispositionKind is the exact transfer outcome for one Product row.
// The only alternatives are a staged Factor successor or an explicit empty
// successor. Invalid is never issued by a live derivation.
type RuleDispositionKind uint8

const (
	ruleDispositionInvalid RuleDispositionKind = iota
	RuleDispositionStaged
	RuleDispositionNoCandidate
)

// RuleDisposition records one exact Product-row outcome. It is the sole
// checker-visible result vocabulary: NoCandidate is an explicit row outcome,
// never a missing result or a synthetic Default fact.
type RuleDisposition[V any] struct {
	kind    RuleDispositionKind
	value   V
	guard   RuleGuard
	targets []RuleTarget
	// carryTransform is the cold semantic identity of the declared
	// transformed-carry form that was applied before this row's writes.  Its
	// callback never enters evidence; a checker can verify only this sealed
	// form identity.
	carryTransform SemanticKey
	transformOnly  bool
	// outputs is an atomic route-output batch. Each entry retains both the
	// route-local value and its authenticated exact target; no caller can
	// detach a value vector and accidentally re-pair it with another route.
	outputs []RuleOutput[V]
	row     ruleResultRow
	// ordinal identifies this entry in the exact row-indexed disposition
	// vector without allocating a per-row proof object.
	ordinal int
}

func (disposition RuleDisposition[V]) Kind() RuleDispositionKind { return disposition.kind }
func (disposition RuleDisposition[V]) Guard() RuleGuard          { return disposition.guard }
func (disposition RuleDisposition[V]) Value() (V, bool) {
	var zero V
	if disposition.kind != RuleDispositionStaged || disposition.transformOnly || len(disposition.outputs) != 0 {
		return zero, false
	}
	return disposition.value, true
}

// CarryTransform reports the exact declared transformed-carry form applied
// for this staged row.  NoCandidate is deliberately transform-free.
func (disposition RuleDisposition[V]) CarryTransform() (SemanticKey, bool) {
	return disposition.carryTransform, disposition.kind == RuleDispositionStaged && disposition.carryTransform.Available()
}

// TransformOnly distinguishes a staged carry-map successor from a staged
// value write without manufacturing a Default/sentinel Fact.
func (disposition RuleDisposition[V]) TransformOnly() bool {
	return disposition.kind == RuleDispositionStaged && disposition.transformOnly
}
func (disposition RuleDisposition[V]) TargetCount() int {
	if disposition.kind != RuleDispositionStaged || disposition.transformOnly || len(disposition.outputs) != 0 {
		return 0
	}
	return len(disposition.targets)
}
func (disposition RuleDisposition[V]) TargetAt(index int) (RuleTarget, bool) {
	if disposition.kind != RuleDispositionStaged || disposition.transformOnly || len(disposition.outputs) != 0 || index < 0 || index >= len(disposition.targets) {
		return RuleTarget{}, false
	}
	return disposition.targets[index], true
}

// RuleOutput is one authenticated route-local target/value pair in a staged
// row batch. Ordinal follows the canonical Selection order and remains part
// of the evidence even when two ordinals are lawfully joined at one target.
type RuleOutput[V any] struct {
	target  RuleTarget
	value   V
	ordinal int
	// witness is a private value capability installed before the checker sees a
	// derivation. It proves membership in one ticket, Product row, and
	// Selection ordinal without allocating one heap object per route output.
	witness ruleRouteOutputWitness
}

type ruleRouteOutputWitness struct {
	ticket  *ruleAdmissionTicket
	row     int
	ordinal int
}

func (output RuleOutput[V]) Target() RuleTarget { return output.target }
func (output RuleOutput[V]) Value() V           { return output.value }
func (output RuleOutput[V]) Ordinal() int       { return output.ordinal }

func (disposition RuleDisposition[V]) OutputCount() int {
	if disposition.kind != RuleDispositionStaged {
		return 0
	}
	return len(disposition.outputs)
}

func (disposition RuleDisposition[V]) OutputAt(index int) (RuleOutput[V], bool) {
	if disposition.kind != RuleDispositionStaged || index < 0 || index >= len(disposition.outputs) {
		return RuleOutput[V]{}, false
	}
	return disposition.outputs[index], true
}

// RuleEvidence is an opaque one-shot checker result.  Only an authentic
// derivation can turn into evidence, so a checker cannot mint admission for a
// different Rule, evaluator epoch, or Composition.
type RuleEvidence struct {
	proof       *ruleRuntimeProof
	composition CompositionID
	identity    SemanticKey
	epoch       identity.Generation
	ticket      *ruleAdmissionTicket
}

// Accept returns evidence bound to this exact derivation.  It is intentionally
// a method on the opaque checker input rather than a public constructor.
func (derivation RuleDerivation[V, O]) Accept() (RuleEvidence, bool) {
	if !derivation.liveProduct() {
		return RuleEvidence{}, false
	}
	return RuleEvidence{proof: derivation.proof, composition: derivation.composition, identity: derivation.identity, epoch: derivation.epoch, ticket: derivation.ticket}, true
}

// AdmitRuleByTrustedTheorem selects one explicitly named, versioned reviewed
// TCB theorem. It does not check an artifact or establish exhaustiveness; its
// identity denotes the trusted theorem, not the Rule evaluator closure. The
// resulting TrustedTheorem row is sealed into Rule/Composition identity and
// remains an explicit TCB obligation in Composition's admission inventory.
func AdmitRuleByTrustedTheorem[V, O any](identity SemanticKey) RuleAdmission[V, O] {
	return RuleAdmission[V, O]{kind: ruleAdmissionTrustedTheorem, identity: identity}
}

// AdmitRuleByDerivation selects a versioned total local derivation checker.
// Nil or unavailable inputs produce an invalid admission that declaration
// rejects.  Determinism and totality are law obligations of the named checker;
// runtime admission is fail-closed around it.
func AdmitRuleByDerivation[V, O any](identity SemanticKey, check RuleDerivationChecker[V, O]) RuleAdmission[V, O] {
	return RuleAdmission[V, O]{kind: ruleAdmissionDerivation, identity: identity, check: check}
}

func (admission RuleAdmission[V, O]) valid() bool {
	switch admission.kind {
	case ruleAdmissionTrustedTheorem:
		return admission.identity.Available() && admission.check == nil
	case ruleAdmissionDerivation:
		return admission.identity.Available() && admission.check != nil
	default:
		return false
	}
}

func (admission RuleAdmission[V, O]) cold() (ruleAdmissionSchema, bool) {
	if !admission.valid() {
		return ruleAdmissionSchema{}, false
	}
	return ruleAdmissionSchema{kind: admission.kind, identity: admission.identity}, true
}

func (admission RuleAdmission[V, O]) same(schema ruleAdmissionSchema) bool {
	return admission.valid() && schema.valid() && admission.kind == schema.kind && admission.identity == schema.identity
}

// admit is the sole future runtime handoff. Trusted admission authenticates
// only the live runtime ticket; derivation admission additionally receives the
// complete checker-visible payload. Keeping both cases here preserves one
// admission path and prevents a checker from publishing independently.
func (admission RuleAdmission[V, O]) admit(derivation RuleDerivation[V, O], proof *ruleRuntimeProof) (RuleEvidence, bool) {
	if !admission.valid() || proof == nil || !proof.valid() || !admission.same(proof.admission) {
		return RuleEvidence{}, false
	}
	switch admission.kind {
	case ruleAdmissionTrustedTheorem:
		return derivation.ticket.evidence(proof, proof.compositionID(), admission.identity)
	case ruleAdmissionDerivation:
		if derivation.proof != proof || derivation.composition != proof.compositionID() || derivation.identity != admission.identity || !derivation.liveProduct() {
			return RuleEvidence{}, false
		}
		var evidence RuleEvidence
		var accepted bool
		func() {
			defer func() {
				if recover() != nil {
					evidence, accepted = RuleEvidence{}, false
				}
			}()
			evidence, accepted = admission.check(derivation)
		}()
		if !accepted || evidence.proof != proof || evidence.composition != proof.compositionID() || evidence.identity != admission.identity || evidence.epoch != derivation.epoch || evidence.ticket != derivation.ticket {
			return RuleEvidence{}, false
		}
		return evidence, true
	default:
		return RuleEvidence{}, false
	}
}

type ruleAdmissionTicket struct {
	proof       *ruleRuntimeProof
	composition CompositionID
	identity    SemanticKey
	epoch       identity.Generation
	anchor      SemanticKey
	execution   *ruleExecution
	product     *productSession
	live        bool
	used        bool
}

// evidence is the trusted-theorem admission cut. It exposes no derivation
// operands, but still binds evidence to the exact live rule instance, product,
// anchor, epoch, composition, and one-shot ticket.
func (ticket *ruleAdmissionTicket) evidence(proof *ruleRuntimeProof, composition CompositionID, identity SemanticKey) (RuleEvidence, bool) {
	if !ticket.liveFor(proof, composition, identity) {
		return RuleEvidence{}, false
	}
	return RuleEvidence{proof: proof, composition: composition, identity: identity, epoch: ticket.epoch, ticket: ticket}, true
}

func (ticket *ruleAdmissionTicket) liveFor(proof *ruleRuntimeProof, composition CompositionID, identity SemanticKey) bool {
	return ticket != nil && ticket.live && !ticket.used && proof != nil && proof.valid() && composition.Available() && identity.Available() && ticket.proof == proof &&
		ticket.composition == composition && ticket.identity == identity && ticket.epoch.Available() && ticket.anchor.Available() &&
		ticket.execution != nil && ticket.product != nil && ticket.product.execution == ticket.execution &&
		ticket.execution.epoch == ticket.epoch && ticket.execution.active.holds(ticket.epoch) && ticket.product.valid(ticket.execution, ticket.epoch)
}

func (derivation RuleDerivation[V, O]) valid() bool {
	ticket := derivation.ticket
	if derivation.proof == nil || !derivation.proof.valid() {
		return false
	}
	return derivation.composition.Available() && derivation.identity.Available() && derivation.epoch.Available() && derivation.anchor.Available() &&
		derivation.product != nil && ticket != nil && ticket.live && !ticket.used && ticket.proof == derivation.proof && ticket.composition == derivation.composition && ticket.identity == derivation.identity && ticket.epoch == derivation.epoch && ticket.anchor == derivation.anchor && ticket.execution == derivation.product.execution && ticket.product == derivation.product
}

func (derivation RuleDerivation[V, O]) liveProduct() bool {
	return derivation.product != nil && derivation.valid() && derivation.product.valid(derivation.product.execution, derivation.epoch) && derivation.product.execution != nil && derivation.product.execution.epoch == derivation.epoch
}

func (evidence *RuleEvidence) consume() bool {
	if evidence == nil || evidence.proof == nil || !evidence.proof.valid() || !evidence.composition.Available() || !evidence.identity.Available() || !evidence.epoch.Available() || evidence.ticket == nil {
		return false
	}
	ticket := evidence.ticket
	if !ticket.live || ticket.used || ticket.proof != evidence.proof || ticket.composition != evidence.composition || ticket.identity != evidence.identity || ticket.epoch != evidence.epoch {
		return false
	}
	ticket.used = true
	ticket.live = false
	return true
}

func (ticket *ruleAdmissionTicket) invalidate() {
	if ticket == nil {
		return
	}
	ticket.live = false
}

type ruleAdmissionSchema struct {
	kind     ruleAdmissionKind
	identity SemanticKey
}

// ruleRuntimeProof is the sole private runtime identity of one sealed receipt
// Rule implementation. It names the SchemaBinding state, canonical ordinal,
// semantic shape, and sealed read/write receipts used by admission.
type ruleRuntimeProof struct {
	schema           *Schema
	state            *schemaBindingState
	bindingAuthority *schemaBindingAuthority
	ordinal          uint64
	semantic         composition.Key
	operandFamily    composition.Key
	admission        ruleAdmissionSchema
	outputKind       composition.OutputKind
	output           composition.Key
	inputs           uint64
	reads            uint64
	carries          uint64
	writes           uint64
	selectedReads    []*SchemaSelectedReadReceipt
	selectWrite      *SchemaSelectWriteReceipt
	routeWrite       *SchemaRouteWriteReceipt
}

func (proof *ruleRuntimeProof) selectedReadAt(read uint64) *SchemaSelectedReadReceipt {
	if proof == nil || read >= uint64(len(proof.selectedReads)) {
		return nil
	}
	return proof.selectedReads[read]
}

func coldRuleAdmission(value composition.Admission) (ruleAdmissionSchema, bool) {
	identity := semanticKeyFromComposition(value.Identity)
	result := ruleAdmissionSchema{identity: identity}
	switch value.Kind {
	case composition.AdmissionTrustedTheorem:
		result.kind = ruleAdmissionTrustedTheorem
	case composition.AdmissionDerivation:
		result.kind = ruleAdmissionDerivation
	default:
		return ruleAdmissionSchema{}, false
	}
	return result, result.valid()
}

// newSchemaRuleRuntimeProof issues the private proof from the exact shared
// SchemaBinding state and canonical Rule ordinal.
func newSchemaRuleRuntimeProof(state *schemaBindingState, authority *schemaBindingAuthority, ordinal uint64) (*ruleRuntimeProof, bool) {
	if state == nil || authority == nil || state.phase != schemaBindingSealed || state.authority != authority || state.schema == nil || ordinal >= uint64(len(state.rules)) {
		return nil, false
	}
	cell, ok := state.rules[ordinal].(schemaRuleBindingCell)
	if !ok || cell == nil || cell.schemaBindingSchema() != state.schema || cell.schemaRuleOrdinal() != ordinal || !cell.schemaRuleComplete() {
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
	if !shapeOK {
		return nil, false
	}
	admission, admitted := coldRuleAdmission(shape.Admission)
	if !admitted {
		return nil, false
	}
	proof := &ruleRuntimeProof{
		schema: state.schema, state: state, bindingAuthority: authority,
		ordinal: ordinal, semantic: state.schema.ruleSemanticAt(ordinal),
		operandFamily: shape.OperandFamily, admission: admission, outputKind: shape.OutputKind,
		output: shape.Output, inputs: shape.Inputs, reads: shape.ReadCount,
		carries: shape.CarryCount, writes: shape.WriteCount,
	}
	if shape.ReadCount > uint64(^uint(0)>>1) {
		return nil, false
	}
	proof.selectedReads = make([]*SchemaSelectedReadReceipt, int(shape.ReadCount))
	fence := schemaRuleReceiptFence{state: state, authority: authority, schema: state.schema, rule: ordinal}
	if ordinal < uint64(len(state.rules)) {
		fence.cell, _ = state.rules[ordinal].(schemaRuleBindingCell)
	}
	for read := uint64(0); read < shape.ReadCount; read++ {
		readShape, readOK := state.schema.ruleReadShapeAt(ordinal, read)
		if !readOK {
			return nil, false
		}
		if readShape.Kind == composition.ReadSelect {
			receipt, receiptOK := issueSchemaSelectedReadReceiptFence(fence, fence.valid(), read)
			if !receiptOK {
				return nil, false
			}
			proof.selectedReads[read] = &receipt
		}
	}
	for write := uint64(0); write < shape.WriteCount; write++ {
		writeShape, writeOK := state.schema.ruleWriteShapeAt(ordinal, write)
		if !writeOK {
			return nil, false
		}
		switch writeShape.Kind {
		case composition.WriteSelect:
			receipt, receiptOK := issueSchemaSelectWriteReceiptFence(fence, fence.valid(), write)
			if !receiptOK {
				return nil, false
			}
			proof.selectWrite = &receipt
		case composition.WriteRoute:
			receipt, receiptOK := issueSchemaRouteWriteReceiptFence(fence, fence.valid(), write)
			if !receiptOK {
				return nil, false
			}
			proof.routeWrite = &receipt
		}
	}
	if !proof.valid() {
		return nil, false
	}
	return proof, true
}

func (proof *ruleRuntimeProof) valid() bool {
	if proof == nil || proof.schema == nil || !proof.schema.Available() || !proof.semantic.Available() || !proof.operandFamily.Available() || !proof.admission.valid() || proof.ordinal >= uint64(schemaRuleCount(proof.schema)) || proof.schema.ruleSemanticAt(proof.ordinal) != proof.semantic {
		return false
	}
	shape, ok := proof.schema.ruleShapeAt(proof.ordinal)
	admission, admitted := coldRuleAdmission(shape.Admission)
	if !ok || !admitted || shape.OperandFamily != proof.operandFamily || admission != proof.admission || shape.OutputKind != proof.outputKind || shape.Output != proof.output || shape.Inputs != proof.inputs || shape.ReadCount != proof.reads || shape.CarryCount != proof.carries || shape.WriteCount != proof.writes {
		return false
	}
	if proof.state == nil || proof.bindingAuthority == nil || proof.state.phase != schemaBindingSealed || proof.state.authority != proof.bindingAuthority || proof.state.schema != proof.schema || proof.ordinal >= uint64(len(proof.state.rules)) {
		return false
	}
	cell, ok := proof.state.rules[proof.ordinal].(schemaRuleBindingCell)
	if !ok || cell == nil || cell.schemaBindingSchema() != proof.schema || cell.schemaRuleOrdinal() != proof.ordinal || !cell.schemaRuleComplete() || !cell.schemaRuleProofMatches(proof) {
		return false
	}
	if uint64(len(proof.selectedReads)) != proof.reads {
		return false
	}
	for read := uint64(0); read < proof.reads; read++ {
		shape, shapeOK := proof.schema.ruleReadShapeAt(proof.ordinal, read)
		if !shapeOK {
			return false
		}
		if shape.Kind == composition.ReadSelect {
			selectedRead := proof.selectedReadAt(read)
			if selectedRead == nil || !selectedRead.Valid() || selectedRead.fence.state != proof.state || selectedRead.fence.authority != proof.bindingAuthority || selectedRead.fence.rule != proof.ordinal || selectedRead.read != read {
				return false
			}
		} else if proof.selectedReads[read] != nil {
			return false
		}
	}
	for write := uint64(0); write < proof.writes; write++ {
		shape, shapeOK := proof.schema.ruleWriteShapeAt(proof.ordinal, write)
		if !shapeOK {
			return false
		}
		switch shape.Kind {
		case composition.WriteSelect:
			if proof.selectWrite == nil || !proof.selectWrite.Valid() || proof.selectWrite.fence.state != proof.state || proof.selectWrite.fence.authority != proof.bindingAuthority || proof.selectWrite.fence.rule != proof.ordinal || proof.selectWrite.write != write {
				return false
			}
		case composition.WriteRoute:
			if proof.routeWrite == nil || !proof.routeWrite.Valid() || proof.routeWrite.fence.state != proof.state || proof.routeWrite.fence.authority != proof.bindingAuthority || proof.routeWrite.fence.rule != proof.ordinal || proof.routeWrite.write != write {
				return false
			}
		}
	}
	return true
}

func (proof *ruleRuntimeProof) compositionID() CompositionID {
	if proof == nil || !proof.valid() {
		return CompositionID{}
	}
	return proof.schema.ID()
}

func schemaRuleCount(schema *Schema) int {
	if schema == nil {
		return 0
	}
	_, rules, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return rules
}

func (schema ruleAdmissionSchema) valid() bool {
	return schema.identity.Available() && (schema.kind == ruleAdmissionTrustedTheorem || schema.kind == ruleAdmissionDerivation)
}
