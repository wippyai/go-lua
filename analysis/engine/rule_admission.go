// rule_admission.go declares the Rule admission vocabulary, the derivation readers, the admit path, the ticket and the evidence.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
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
	identity identity.SemanticKey
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
	identity       identity.SemanticKey
	epoch          identity.Generation
	anchor         identity.SemanticKey
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
func (derivation RuleDerivation[V, O]) Rule() identity.SemanticKey {
	if derivation.proof == nil || !derivation.proof.valid() {
		return identity.SemanticKey{}
	}
	semantic, ok := semanticKeyFromComposition(derivation.proof.semantic)
	if !ok {
		return identity.SemanticKey{}
	}
	return semantic
}

// Composition returns the sealed composition identity under which the
// derivation was issued.  A checker can use it to reject foreign proof input.
func (derivation RuleDerivation[V, O]) Composition() CompositionID {
	return derivation.composition
}

// Anchor returns the exact compiled Rule-instance identity. It is derived by
// the sole equation compiler and includes that instance's sealed structural
// anchor and resolved surface, never a caller-supplied occurrence label.
func (derivation RuleDerivation[V, O]) Anchor() identity.SemanticKey {
	if !derivation.valid() {
		return identity.SemanticKey{}
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

// RuleInput is an opaque immutable input-State snapshot. It supports only
// identity and common-support comparison; a derivation checker cannot read a
// Factor root or attach a new one.
type RuleInput struct{ state carrier.State }

func (input RuleInput) Guard() RuleGuard { return RuleGuard{mask: input.state.Support()} }

// RuleRead is one exact input-qualified resolved read observation. Unit stays
// opaque; Same is the only capability exposed to local proof code.
type RuleRead struct {
	input uint64
	unit  carrier.Unit
}

// RuleGuard is the exact resolved output region of a staged result. It is an
// opaque shared-support value and cannot create, widen, or restrict guards.
type RuleGuard struct{ mask support.Mask }

func (guard RuleGuard) Empty() bool { return !guard.mask.Valid() || support.Empty(guard.mask) }

// RuleTarget is an opaque resolved write capability. It permits identity
// comparison only, so a checker can validate target correspondence without
// manufacturing or retargeting a Patch.
type RuleTarget struct {
	target carrier.Target
	proof  ruleTargetProof
}

func (target RuleTarget) Same(other RuleTarget) bool {
	return target.target.Same(other.target) && target.proof == other.proof
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
	carryTransform identity.SemanticKey
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

func (disposition RuleDisposition[V]) Guard() RuleGuard { return disposition.guard }

func (disposition RuleDisposition[V]) Value() (V, bool) {
	var zero V
	if disposition.kind != RuleDispositionStaged || disposition.transformOnly || len(disposition.outputs) != 0 {
		return zero, false
	}
	return disposition.value, true
}

// CarryTransform reports the exact declared transformed-carry form applied
// for this staged row.  NoCandidate is deliberately transform-free.
func (disposition RuleDisposition[V]) CarryTransform() (identity.SemanticKey, bool) {
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

func (output RuleOutput[V]) Value() V { return output.value }

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
	identity    identity.SemanticKey
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
func AdmitRuleByTrustedTheorem[V, O any](identity identity.SemanticKey) RuleAdmission[V, O] {
	return RuleAdmission[V, O]{kind: ruleAdmissionTrustedTheorem, identity: identity}
}

// AdmitRuleByDerivation selects a versioned total local derivation checker.
// Nil or unavailable inputs produce an invalid admission that declaration
// rejects.  Determinism and totality are law obligations of the named checker;
// runtime admission is fail-closed around it.
func AdmitRuleByDerivation[V, O any](identity identity.SemanticKey, check RuleDerivationChecker[V, O]) RuleAdmission[V, O] {
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
	identity    identity.SemanticKey
	epoch       identity.Generation
	anchor      identity.SemanticKey
	execution   *ruleExecution
	product     *productSession
	live        bool
	used        bool
}

// evidence is the trusted-theorem admission cut. It exposes no derivation
// operands, but still binds evidence to the exact live rule instance, product,
// anchor, epoch, composition, and one-shot ticket.
func (ticket *ruleAdmissionTicket) evidence(proof *ruleRuntimeProof, composition CompositionID, identity identity.SemanticKey) (RuleEvidence, bool) {
	if !ticket.liveFor(proof, composition, identity) {
		return RuleEvidence{}, false
	}
	return RuleEvidence{proof: proof, composition: composition, identity: identity, epoch: ticket.epoch, ticket: ticket}, true
}

func (ticket *ruleAdmissionTicket) liveFor(proof *ruleRuntimeProof, composition CompositionID, identity identity.SemanticKey) bool {
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

func coldRuleAdmission(value composition.Admission) (ruleAdmissionSchema, bool) {
	identity, identityOK := semanticKeyFromComposition(value.Identity)
	if !identityOK {
		return ruleAdmissionSchema{}, false
	}
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
