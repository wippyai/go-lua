package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// Evidence is the private Heap-aligned state produced by the suspension
// producer. Missing is the sparse Factor default; it remains distinct from
// Unknown until the query projects the complete route result into the public
// allocation evidence plane.
type Evidence uint8

const (
	EvidenceMissing Evidence = iota
	EvidenceUnknown
	EvidenceRefuted
	EvidenceProven
)

const invalidEvidence Evidence = ^Evidence(0)

func (state Evidence) Valid() bool { return state <= EvidenceProven }

func (state Evidence) Known() bool {
	return state == EvidenceRefuted || state == EvidenceProven
}

// Public projects this producer's private state into the public Placement
// proof vocabulary. The projection is distinction-preserving: Missing is the
// sparse Factor default and publishes as absence, while Unknown is the
// authenticated all-routes verdict and publishes as Unknown. A state outside
// this vocabulary has no public projection and yields an inadmissible state
// that every public boundary rejects.
func (state Evidence) Public() placement.EvidenceState {
	switch state {
	case EvidenceMissing:
		return placement.EvidenceAbsent
	case EvidenceUnknown:
		return placement.EvidenceUnknown
	case EvidenceRefuted:
		return placement.EvidenceRefuted
	case EvidenceProven:
		return placement.EvidenceProven
	default:
		return placement.InvalidEvidenceState()
	}
}

// JoinChecked is the all-routes law. Missing is the identity for a coordinate,
// Unknown absorbs opaque/missing/mixed evidence, and a proof survives only
// when every contributing route has the same explicit answer. Invalid input
// refuses and never becomes semantic Unknown.
func (state Evidence) JoinChecked(other Evidence) (Evidence, bool) {
	if !state.Valid() || !other.Valid() {
		return invalidEvidence, false
	}
	if state == EvidenceMissing {
		return other, true
	}
	if other == EvidenceMissing {
		return state, true
	}
	if state == EvidenceUnknown || other == EvidenceUnknown {
		return EvidenceUnknown, true
	}
	if state == other {
		return state, true
	}
	return EvidenceUnknown, true
}

// Join is the generic lattice callback spelling. The engine admits only valid
// states; the out-of-domain sentinel prevents a malformed direct invocation
// from being normalized into a semantic verdict.
func (state Evidence) Join(other Evidence) Evidence {
	joined, ok := state.JoinChecked(other)
	if !ok {
		return invalidEvidence
	}
	return joined
}

func meetChecked(left, right Evidence) (Evidence, bool) {
	if !left.Valid() || !right.Valid() {
		return invalidEvidence, false
	}
	if left == EvidenceUnknown {
		return right, true
	}
	if right == EvidenceUnknown {
		return left, true
	}
	if left == EvidenceMissing || right == EvidenceMissing {
		return EvidenceMissing, true
	}
	if left == right {
		return left, true
	}
	return EvidenceMissing, true
}

func Lattice() lattice.Lattice[Evidence] {
	return lattice.Lattice[Evidence]{
		Bottom: func() Evidence { return EvidenceMissing },
		Top:    func() Evidence { return EvidenceUnknown },
		Equal:  func(left, right Evidence) bool { return left.Valid() && right.Valid() && left == right },
		LessOrEq: func(left, right Evidence) bool {
			if !left.Valid() || !right.Valid() {
				return false
			}
			return left == right || left == EvidenceMissing || right == EvidenceUnknown
		},
		Join: func(left, right Evidence) Evidence { return left.Join(right) },
		Meet: func(left, right Evidence) Evidence {
			met, ok := meetChecked(left, right)
			if !ok {
				return invalidEvidence
			}
			return met
		},
		Widen: func(left, right Evidence) Evidence { return left.Join(right) },
	}
}

// EvidenceFactorFragment is the callback-free cold Factor shape for the
// suspension evidence axis. It is separate from Placement's class Factor.
type EvidenceFactorFragment struct {
	slot        *engine.FactorSlot[Evidence]
	ref         engine.FactorRef[Evidence]
	exactRead   engine.SchemaReadForm[Evidence]
	summaryRead engine.SchemaReadForm[Evidence]
	exactWrite  engine.SchemaWriteForm[Evidence]
}

func (fragment *EvidenceFactorFragment) Ref() engine.FactorRef[Evidence] {
	if fragment == nil {
		return engine.FactorRef[Evidence]{}
	}
	return fragment.ref
}

func (fragment *EvidenceFactorFragment) ExactRead() engine.SchemaReadForm[Evidence] {
	if fragment == nil {
		return engine.SchemaReadForm[Evidence]{}
	}
	return fragment.exactRead
}

func (fragment *EvidenceFactorFragment) SummaryRead() engine.SchemaReadForm[Evidence] {
	if fragment == nil {
		return engine.SchemaReadForm[Evidence]{}
	}
	return fragment.summaryRead
}

func (fragment *EvidenceFactorFragment) ExactWrite() engine.SchemaWriteForm[Evidence] {
	if fragment == nil {
		return engine.SchemaWriteForm[Evidence]{}
	}
	return fragment.exactWrite
}

func DeclareEvidenceFactorSchema(builder *engine.SchemaBuilder, semantic, summarySemantic identity.SemanticKey) (*EvidenceFactorFragment, bool) {
	if builder == nil || !semantic.Available() || !summarySemantic.Available() || semantic == summarySemantic {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[Evidence](builder, semantic)
	if !ok {
		return nil, false
	}
	exactRead, ok := slot.ExactRead()
	if !ok {
		return nil, false
	}
	summaryRead, ok := slot.DistributiveSummaryRead(summarySemantic)
	if !ok {
		return nil, false
	}
	exactWrite, ok := slot.ExactWrite()
	if !ok {
		return nil, false
	}
	return &EvidenceFactorFragment{slot: slot, ref: slot.Ref(), exactRead: exactRead, summaryRead: summaryRead, exactWrite: exactWrite}, true
}

type evidenceAxisInputs interface {
	PlacementInput() placement.Schema
}

type MountRejection uint8

const (
	MountRejectionNone MountRejection = iota
	MountRejectionInput
)

// AxisEntry declares the independent evidence Factor. It projects the exact
// Placement/Heap schema already mounted by the Placement axis; it does not
// mint a second coordinate directory or retain a duplicate root table.
func AxisEntry[A evidenceAxisInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:          "placement-suspension-evidence",
		Storage:      axis.StorageFactor,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeLink,
		Mutability:   axis.MutabilitySolve,
		Concurrency:  axis.ConcurrencySingleWriter,
		Dependencies: []schema.Key{"placement"},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: "placement/suspension-evidence/facts", Writer: "placement-suspension-evidence"}}},
		Semantic:     "semantic/factor/placement/suspension-evidence",
		Roles:        []schema.Key{"semantic/factor/placement/suspension-evidence/summary"},
		Mount: axis.NewMount(func(context axis.Mounting[A]) (placement.Schema, MountRejection, bool) {
			projected := context.Inputs.PlacementInput()
			if !projected.Valid() {
				return placement.Schema{}, MountRejectionInput, false
			}
			return projected, MountRejectionNone, true
		}),
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*EvidenceFactorFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/factor/placement/suspension-evidence")
	summary, summaryOK := context.Roles.Key("semantic/factor/placement/suspension-evidence/summary")
	if !semanticOK || !summaryOK {
		return nil, false
	}
	return DeclareEvidenceFactorSchema(builder, semantic, summary)
}

func BindAxis[A evidenceAxisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *EvidenceFactorFragment]) (*EvidenceOwner, bool) {
	schema := context.Inputs.PlacementInput()
	if !schema.Valid() {
		return nil, false
	}
	return BindEvidenceFactorHot(binding, context.Fragment, schema)
}

func AlgebraAxis(owner *EvidenceOwner) (axis.Algebra[Evidence], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[Evidence]{}, false
	}
	return axis.Adopt(axis.CarrierAlgebra[coordinate, Evidence]{
		KeyEnd: spec.KeyEnd, Lattice: spec.Lattice, Default: spec.Default,
		AdmitAt: spec.AdmitAt, Fingerprint: spec.Fingerprint,
		Widen:  axis.CarrierRank[coordinate, Evidence]{Width: spec.WidenRank.Width, At: spec.WidenRank.At},
		Narrow: axis.CarrierRank[coordinate, Evidence]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At},
	})
}

// EvidenceOwner is the Link-local owner of the independent evidence Factor.
// Its schema is the Placement schema it projects, so Heap coordinates remain
// single-issued and owner-fenced.
type EvidenceOwner struct {
	binding  *engine.SchemaBinding
	fragment *EvidenceFactorFragment
	schema   placement.Schema
}

func BindEvidenceFactorHot(binding *engine.SchemaBinding, fragment *EvidenceFactorFragment, schema placement.Schema) (*EvidenceOwner, bool) {
	if binding == nil || binding.Sealed() || binding.Poisoned() || binding.Schema() != nil || fragment == nil || !schema.Valid() {
		return nil, false
	}
	owner := &EvidenceOwner{binding: binding, fragment: fragment, schema: schema}
	spec, ok := owner.FactorSpec()
	if !ok || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		return nil, false
	}
	if !engine.BindIdentitySummaryReadForFactor[coordinate, Evidence](binding, fragment.slot, fragment.summaryRead) {
		return nil, false
	}
	return owner, true
}

func (owner *EvidenceOwner) Schema() placement.Schema {
	if owner == nil {
		return placement.Schema{}
	}
	return owner.schema
}

func (owner *EvidenceOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

func (owner *EvidenceOwner) FactorRef() engine.FactorRef[Evidence] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[Evidence]{}
	}
	return owner.fragment.Ref()
}

func (owner *EvidenceOwner) ExactRead() engine.SchemaReadForm[Evidence] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[Evidence]{}
	}
	return owner.fragment.ExactRead()
}

func (owner *EvidenceOwner) SummaryRead() engine.SchemaReadForm[Evidence] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[Evidence]{}
	}
	return owner.fragment.SummaryRead()
}

func (owner *EvidenceOwner) ExactWrite() engine.SchemaWriteForm[Evidence] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[Evidence]{}
	}
	return owner.fragment.ExactWrite()
}

func (owner *EvidenceOwner) FactorSpec() (engine.HotFactorSpec[coordinate, Evidence], bool) {
	if owner == nil || !owner.schema.Valid() {
		return engine.HotFactorSpec[coordinate, Evidence]{}, false
	}
	keyEnd := owner.schema.KeyCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[coordinate, Evidence]{}, false
	}
	return engine.HotFactorSpec[coordinate, Evidence]{
		KeyEnd: uint64(keyEnd), Lattice: Lattice(), Default: EvidenceMissing,
		AdmitAt: owner.admits, Fingerprint: func(value Evidence) uint64 { return uint64(value) },
		WidenRank: engine.Measure[coordinate, Evidence]{Width: 1, At: owner.widenRank},
	}, true
}

func (owner *EvidenceOwner) Ref(key heap.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || !owner.schema.Valid() || !owner.schema.Heap().OwnsKey(key) {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.Heap().KeyIndex(key)
	if !ok || index < 0 || index >= owner.schema.KeyCount() {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, Evidence](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

func SelectRouteTyped[Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](owner *EvidenceOwner, context engine.SelectorContext, key heap.Key, tag Tag) bool {
	ref, ok := owner.Ref(key)
	return ok && engine.SelectRoute(context, ref, tag)
}

type EvidenceRuleImplementation[O any] struct {
	owner *EvidenceOwner
	slot  *engine.RuleSlot[Evidence, O]
}

func BindSelectedEvidenceRouteRuleDirect[O any](owner *EvidenceOwner, slot *engine.RuleSlot[Evidence, O], carry engine.SchemaCarrySlot[Evidence], write engine.SchemaWriteSlot[Evidence], spec engine.HotRuleSpec[Evidence, O], carrySpec engine.HotCarrySpec[Evidence, O]) (*EvidenceRuleImplementation[O], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || slot == nil || !engine.BindSelectedRouteRuleDirect[coordinate](owner.binding, slot, carry, write, owner.fragment.Ref(), spec, carrySpec, nil) {
		return nil, false
	}
	return &EvidenceRuleImplementation[O]{owner: owner, slot: slot}, true
}

func AddSelectedEvidenceRuleDirectExactRead[O any, RV any](issuer *EvidenceRuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], project func(O) (uint64, bool)) (engine.Read[engine.OrderedCells[RV]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.OrderedCells[RV]]{}, false
	}
	return engine.BindSelectedRuleDirectExactRead[coordinate](issuer.owner.binding, issuer.slot, slot, factor, project)
}

func AddSelectedEvidenceRuleDirectOperandRead[O any, RV any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](issuer *EvidenceRuleImplementation[O], slot engine.SchemaReadSlot[RV], factor engine.FactorRef[RV], locate func(engine.SelectorContext, O) bool) (engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.Read[engine.Selection[Tag, engine.OrderedCells[RV]]]{}, false
	}
	return engine.BindSelectedRuleDirectOperandRead[coordinate, Evidence, O, RV, Tag](issuer.owner.binding, issuer.slot, slot, factor, locate)
}

func (issuer *EvidenceRuleImplementation[O]) MountedCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.MountedCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

func (issuer *EvidenceRuleImplementation[O]) LinkCapability() (engine.RuleSlotCapability, bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return engine.RuleSlotCapability{}, false
	}
	return engine.LinkCapabilityForSlot(issuer.owner.binding, issuer.slot)
}

func ResolveEvidenceRuleImplementation[O any](issuer *EvidenceRuleImplementation[O]) (*engine.RuleImplementation[coordinate, Evidence, O], bool) {
	if issuer == nil || issuer.owner == nil || issuer.slot == nil {
		return nil, false
	}
	implementation, ok := engine.RuleImplementationAt[coordinate, Evidence, O](issuer.owner.binding, issuer.slot)
	if !ok {
		return nil, false
	}
	return implementation, true
}

type coordinate uint32

func (owner *EvidenceOwner) keyAt(index coordinate) (heap.Key, bool) {
	if owner == nil || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return heap.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

func (owner *EvidenceOwner) admits(index coordinate, state Evidence) bool {
	if owner == nil || !state.Valid() {
		return false
	}
	key, ok := owner.keyAt(index)
	if !ok {
		return false
	}
	if key.Kind() == heap.RootBoot {
		return state == EvidenceMissing || state == EvidenceUnknown
	}
	return key.Kind() == heap.RootAllocation
}

func (owner *EvidenceOwner) widenRank(index coordinate, state Evidence, component int) uint64 {
	if owner == nil || component != 0 || !owner.admits(index, state) {
		return 0
	}
	switch state {
	case EvidenceMissing:
		return 2
	case EvidenceRefuted, EvidenceProven:
		return 1
	case EvidenceUnknown:
		return 0
	default:
		return 0
	}
}

func FactorStructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/placement/suspension-evidence", "factor/placement/suspension-evidence/summary")
}
