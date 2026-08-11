package engine

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// Measure is a key-aware, well-founded transition witness for a Factor's
// recurrence operation. It is semantic input, not a work budget. Wave D
// retains the law but does not attach it to a carrier or scheduler.
type Measure[K ~uint32 | ~uint64, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

// FactorSpec is the cold, complete typed algebra for one Factor. KeyEnd may
// be a finite universe derived from sealed Link structure. It deliberately
// creates neither a guard/fact binding nor any concrete read or write handle;
// those are exact Wave-E template products.
type FactorSpec[K ~uint32 | ~uint64, V any] struct {
	Semantic SemanticKey
	KeyEnd   uint64
	Lattice  lattice.Lattice[V]
	Default  V
	// AdmitAt is the Factor owner's total coordinate/value fence. It is the
	// sole authority that decides whether V may inhabit K; the engine carries
	// it unchanged into factbinding and never reconstructs it from a lattice
	// or from Link structure. Default must be admitted at every dense key.
	AdmitAt func(key K, value V) bool
	// Fingerprint is a semantic comparison index, never an equality verdict.
	// It must be pure and deterministic, and Lattice.Equal(a,b) implies
	// Fingerprint(a)==Fingerprint(b). Equality resolves every collision.
	Fingerprint func(V) uint64
	WidenRank   Measure[K, V]
	NarrowRank  Measure[K, V]
}

// Factor is a typed cold owner capability. K and V remain available to its
// owner and the later template binder, but they never enter a mixed engine
// carrier during Composition declaration.
type Factor[K ~uint32 | ~uint64, V any] struct {
	composition *Composition
	schema      *factorSchema
	algebra     *factbinding.Algebra[K, V]
	open        bool

	forms     map[SemanticKey]struct{}
	formBinds map[*formSchema]formBinder
}

// Ref is an opaque, sealed exact-key capability issued by a Factor. Its key
// remains zero-based even though later private equation binding may choose a
// different representation. It records canonical sealed identities plus one
// private live seal-authority ticket, has no public constructor or inspection
// surface, and the zero-sized function member deliberately makes it
// uncomparable.
//
// Ref is only a cold identity capability. It is not a Program handle, an
// equation coordinate, or a runtime binding handle.
type Ref[K ~uint32 | ~uint64] struct {
	compositionID CompositionID
	sealAuthority uint64
	factorKey     composition.Key
	factorIndex   uint64
	raw           K
	_             [0]func()
}

// ClosedRefs is one Factor-issued, seal-once vector of exact Ref
// capabilities. It deliberately exposes append/close rather than raw
// coordinates: callers with an owner-private K can construct it by type
// inference, while only Assembly later reads its canonical Ref vector.
type ClosedRefs[K ~uint32 | ~uint64] struct {
	factor *factorSchema
	refs   []Ref[K]
	closed bool
}

type factorSchema struct {
	composition *Composition
	semantic    SemanticKey
	keyEnd      uint64
	open        bool

	forms      []*formSchema
	exactRead  *formSchema
	exactWrite *formSchema
	bind       factorBinder
	bindIndex  uint64
	bound      bool
}

// formSchema is the serializable structural identity of one Factor-issued
// form. Typed callbacks remain on their typed owner capabilities; the sealed
// Composition retains this schema so Wave E can enumerate every form without
// rediscovering it through a live Go closure.
type formSchema struct {
	factor    *factorSchema
	semantic  SemanticKey
	readKind  readFormKind
	writeKind writeFormKind
	source    *formSchema
}

// Output is the typed schema capability for the sole Factor a Rule writes.
// The private bindOutput closure is a cold double-dispatch witness for the
// Factor's owner-private K. It is installed once by Factor.Output and used
// only while compiling a Rule instance; it avoids a runtime type switch over
// K, so named domain key types remain valid without reflection or a second
// key representation.
type Output[V any] struct {
	composition *Composition
	factor      *factorSchema
	bindOutput  func(runtimeFactor, anyRule, *outputRuntime) (outputAccess[V], bool)
}

// ReadForm is a Factor-issued, unanchored observation form. It describes
// exact or summary shape but never an exact key, Unit, Point, or
// Program/Link anchor. The typed normalizer callback is retained only in this
// typed capability and is excluded from cold identity.
type ReadForm[V, S any] struct {
	schema      *formSchema
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
	bindQuery   func(*querySchema, QueryRead[S], runtimeFactor, equation.Surface) (queryReadRuntime, bool)
	bindRule    func(readBinding, equation.RuleMember, Read[S], runtimeFactor) bool
}

type readFormKind uint8

const (
	exactReadForm readFormKind = iota + 1
	summaryReadForm
)

// WriteForm is a Factor-issued, unanchored write-target form. The exact
// coordinate and concrete Target are introduced only when an E template binds
// one Rule instance.
type WriteForm[V any] struct {
	schema *formSchema
}

type writeFormKind uint8

const (
	exactWriteForm writeFormKind = iota + 1
	selectorWriteForm
)

// CarryForm is the explicit whole-Factor predecessor relation. It is kept
// separate from a read form so an E binder cannot accidentally manufacture a
// whole-plane fallback from a coordinate read.
type CarryForm struct {
	composition *Composition
	factor      *factorSchema
}

// Normalizer is a typed Factor-owned summary law. The callback is monomorphic
// and never serialized or hashed; Semantic names its reviewed semantics.
type Normalizer[V, S any] struct {
	schema      *formSchema
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
	bindQuery   func(*querySchema, QueryRead[S], runtimeFactor, equation.Surface) (queryReadRuntime, bool)
	bindRule    func(readBinding, equation.RuleMember, Read[S], runtimeFactor) bool
}

func runtimeQueryReadBinder[K ~uint32 | ~uint64, V, S any](normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) func(*querySchema, QueryRead[S], runtimeFactor, equation.Surface) (queryReadRuntime, bool) {
	return func(schema *querySchema, read QueryRead[S], factor runtimeFactor, surface equation.Surface) (queryReadRuntime, bool) {
		bound, ok := factor.(*boundFactor[K, V])
		if !ok {
			return nil, false
		}
		return bindQueryRead(schema, read, bound, surface, normalize, equal, fingerprint)
	}
}

func runtimeExactRuleReadBinder[K ~uint32 | ~uint64, V, S any](normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) func(readBinding, equation.RuleMember, Read[S], runtimeFactor) bool {
	return func(bound readBinding, member equation.RuleMember, read Read[S], factor runtimeFactor) bool {
		output, ok := factor.(*boundFactor[K, V])
		return ok && bindMemberExactRead(bound, member, read, output, normalize, equal, fingerprint)
	}
}

func runtimeSummaryRuleReadBinder[K ~uint32 | ~uint64, V, S any](form ReadForm[V, S]) func(readBinding, equation.RuleMember, Read[S], runtimeFactor) bool {
	return func(bound readBinding, member equation.RuleMember, read Read[S], factor runtimeFactor) bool {
		output, ok := factor.(*boundFactor[K, V])
		return ok && bindMemberSummaryRead(bound, member, read, output, form)
	}
}

// DeclareFactor creates one Factor schema under the open Composition. The
// owner callback can declare only its own summary forms. A failed
// callback or invalid declaration poisons the whole Composition.
func DeclareFactor[K ~uint32 | ~uint64, V any](composition *Composition, spec FactorSpec[K, V], declare func(*Factor[K, V]) bool) (*Factor[K, V], bool) {
	if composition == nil || !composition.acceptsFactor(spec.Semantic) || declare == nil {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	algebra, admitted := factbinding.Admit(spec.KeyEnd, spec.Default, spec.Lattice, spec.AdmitAt, spec.Fingerprint,
		factbinding.Measure[K, V]{Width: spec.WidenRank.Width, At: spec.WidenRank.At},
		factbinding.Measure[K, V]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At})
	if !admitted || algebra == nil {
		composition.poison()
		return nil, false
	}
	schema := &factorSchema{composition: composition, semantic: spec.Semantic, keyEnd: spec.KeyEnd, open: true}
	schema.exactRead = &formSchema{factor: schema, semantic: spec.Semantic, readKind: exactReadForm}
	schema.exactWrite = &formSchema{factor: schema, semantic: spec.Semantic, writeKind: exactWriteForm}
	schema.forms = []*formSchema{schema.exactRead, schema.exactWrite}
	factor := &Factor[K, V]{composition: composition, schema: schema, algebra: algebra, open: true, forms: make(map[SemanticKey]struct{}), formBinds: make(map[*formSchema]formBinder, 2)}
	schema.bind = factorBind[K, V]{owner: factor}
	exactNormalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	exactEqual := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, factor.equal)
	}
	exactFingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, factor.algebra.Fingerprint)
	}
	factor.formBinds[schema.exactRead] = readFormBind[V, OrderedCells[V]]{form: ReadForm[V, OrderedCells[V]]{schema: schema.exactRead, normalize: exactNormalize, equal: exactEqual, fingerprint: exactFingerprint, bindQuery: runtimeQueryReadBinder[K, V](exactNormalize, exactEqual, exactFingerprint), bindRule: runtimeExactRuleReadBinder[K, V](exactNormalize, exactEqual, exactFingerprint)}}
	factor.formBinds[schema.exactWrite] = writeFormBind[V]{form: WriteForm[V]{schema: schema.exactWrite}}
	composition.factors = append(composition.factors, schema)
	composition.activeFactor = schema
	declarationOK := false
	func() {
		defer func() {
			recovered := recover()
			if composition.activeFactor != schema {
				composition.poison()
			}
			// Clear before the outer success decision. A poisoned callback cannot
			// recover into a later declaration because usable rejects it below.
			composition.activeFactor = nil
			factor.open = false
			schema.open = false
			if recovered != nil {
				composition.poison()
				declarationOK = false
			}
		}()
		declarationOK = declare(factor)
	}()
	if !declarationOK || !composition.usable() {
		composition.poison()
		return nil, false
	}
	return factor, true
}

// Output returns the sole cold typed write authority for this Factor.
func (factor *Factor[K, V]) Output() Output[V] {
	if !factor.valid() {
		if factor != nil {
			factor.composition.poison()
		}
		return Output[V]{}
	}
	return Output[V]{
		composition: factor.composition,
		factor:      factor.schema,
		bindOutput: func(runtime runtimeFactor, owner anyRule, projection *outputRuntime) (outputAccess[V], bool) {
			bound, ok := runtime.(*boundFactor[K, V])
			if !ok || bound == nil {
				return outputAccess[V]{}, false
			}
			return newTypedOutputAccess(bound, owner, projection)
		},
	}
}

// Ref issues this Factor's sealed exact-key capability. It is deliberately
// unavailable while the owner callback is open and until the entire owning
// Composition has sealed, so a failed or incomplete declaration cannot leak
// a usable coordinate identity.
func (factor *Factor[K, V]) Ref(key K) (Ref[K], bool) {
	if factor == nil || factor.composition == nil || factor.schema == nil {
		return Ref[K]{}, false
	}
	ref := Ref[K]{
		compositionID: factor.composition.ID(),
		sealAuthority: factor.composition.sealAuthority,
		factorKey:     factor.schema.semantic.compositionKey(),
		factorIndex:   factor.schema.bindIndex,
		raw:           key,
	}
	if !validateRefForWaveE(factor, ref) {
		return Ref[K]{}, false
	}
	return ref, true
}

// NewClosedRefs starts one owner-fenced exact Ref vector after this Factor has
// sealed. It retains no raw K values and cannot be constructed for a foreign
// or still-open Factor.
func (factor *Factor[K, V]) NewClosedRefs() *ClosedRefs[K] {
	if factor == nil || factor.schema == nil || !factor.valid() || !factor.schema.bound || factor.open || factor.schema.open {
		return nil
	}
	return &ClosedRefs[K]{factor: factor.schema}
}

// OwnsClosedRefs authenticates a seal-once summary vector against this exact
// Factor issuer.  It is intentionally a predicate rather than a projection:
// callers cannot recover the vector's dense coordinate, Factor schema, or
// composition authority.  Domain owners use it to reject a foreign vector
// before invoking Append/Close laws.
func (factor *Factor[K, V]) OwnsClosedRefs(refs *ClosedRefs[K]) bool {
	return factor != nil && refs != nil && factor.schema != nil && refs.factor == factor.schema && factor.valid() && factor.schema.bound && !factor.open && !factor.schema.open
}

// Append records one exact Ref from this vector's sole issuing Factor. It is
// legal only before Close; duplicates are rejected before they can alter the
// canonical summary set.
func (refs *ClosedRefs[K]) Append(ref Ref[K]) bool {
	if refs == nil || refs.closed || !validateRefForSchema(refs.factor, ref) {
		return false
	}
	for _, present := range refs.refs {
		if present.raw == ref.raw {
			return false
		}
	}
	refs.refs = append(refs.refs, ref)
	return true
}

// Close fixes one immutable canonical Ref order. It is intentionally
// idempotence-free: a vector has one admission episode and any second close
// is rejected rather than treated as a parallel construction path.
func (refs *ClosedRefs[K]) Close() bool {
	if refs == nil || refs.closed || refs.factor == nil || len(refs.refs) == 0 {
		return false
	}
	for _, ref := range refs.refs {
		if !validateRefForSchema(refs.factor, ref) {
			return false
		}
	}
	sort.Slice(refs.refs, func(left, right int) bool { return refs.refs[left].raw < refs.refs[right].raw })
	for index := 1; index < len(refs.refs); index++ {
		if refs.refs[index-1].raw >= refs.refs[index].raw {
			return false
		}
	}
	refs.closed = true
	return true
}

// sealedRefsForAssembly is the only vector projection. It remains private so
// no Program/Link user can recover raw coordinates or evade the close fence.
func (refs *ClosedRefs[K]) sealedRefsForAssembly(factor *factorSchema) ([]Ref[K], bool) {
	if refs == nil || !refs.closed || refs.factor != factor || len(refs.refs) == 0 {
		return nil, false
	}
	for index, ref := range refs.refs {
		if !validateRefForSchema(factor, ref) || index > 0 && refs.refs[index-1].raw >= ref.raw {
			return nil, false
		}
	}
	return append([]Ref[K](nil), refs.refs...), true
}

// validateRefForWaveE is the Factor-owner form of the private Wave-E admission
// hook. A Ref carries no live schema pointer: validation re-establishes the
// exact sealed Composition and canonical Factor binding before a consumer may
// use it.
func validateRefForWaveE[K ~uint32 | ~uint64, V any](factor *Factor[K, V], ref Ref[K]) bool {
	if factor == nil || factor.composition == nil || factor.schema == nil ||
		factor.schema.composition != factor.composition || factor.open || factor.schema.open ||
		!factor.valid() {
		return false
	}
	return validateRefForSchema(factor.schema, ref)
}

// validateRefForSchema is the schema-only Wave-E admission hook. It lets one
// private body-template consumer validate typed Ref vectors with different K
// widths without erasing them to any. It accepts only the canonical sealed
// CompositionID, matching private live seal authority, Factor semantic/bind
// identity, and in-range raw key.
func validateRefForSchema[K ~uint32 | ~uint64](schema *factorSchema, ref Ref[K]) bool {
	if schema == nil || schema.composition == nil || schema.open || !schema.composition.Sealed() ||
		!schema.bound || uint64(ref.raw) >= schema.keyEnd {
		return false
	}
	sealed := schema.composition.coldComposition()
	if sealed == nil {
		return false
	}
	factorKey := schema.semantic.compositionKey()
	factorIndex, found := sealed.FactorIndex(factorKey)
	if !found || factorIndex != schema.bindIndex {
		return false
	}
	return ref.compositionID == schema.composition.ID() && ref.sealAuthority == schema.composition.sealAuthority && ref.factorKey == factorKey && ref.factorIndex == schema.bindIndex
}

// ExactReadForm returns the one unanchored exact-read form for this Factor. It
// does not name any K coordinate; `template` supplies that coordinate later.
func ExactReadForm[K ~uint32 | ~uint64, V any](factor *Factor[K, V]) (ReadForm[V, OrderedCells[V]], bool) {
	if !factor.valid() {
		if factor != nil {
			factor.composition.poison()
		}
		return ReadForm[V, OrderedCells[V]]{}, false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, factor.equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, factor.algebra.Fingerprint)
	}
	return ReadForm[V, OrderedCells[V]]{schema: factor.schema.exactRead, normalize: normalize, equal: equal, fingerprint: fingerprint, bindQuery: runtimeQueryReadBinder[K, V](normalize, equal, fingerprint), bindRule: runtimeExactRuleReadBinder[K, V](normalize, equal, fingerprint)}, true
}

// ExactWriteForm returns the one unanchored exact-write form for this Factor.
func ExactWriteForm[K ~uint32 | ~uint64, V any](factor *Factor[K, V]) (WriteForm[V], bool) {
	if !factor.valid() {
		if factor != nil {
			factor.composition.poison()
		}
		return WriteForm[V]{}, false
	}
	return WriteForm[V]{schema: factor.schema.exactWrite}, true
}

// Carry returns the explicit whole-Factor predecessor form.
func Carry[K ~uint32 | ~uint64, V any](factor *Factor[K, V]) (CarryForm, bool) {
	if !factor.valid() {
		if factor != nil {
			factor.composition.poison()
		}
		return CarryForm{}, false
	}
	return CarryForm{composition: factor.composition, factor: factor.schema}, true
}

// DeclareNormalizer declares one typed summary law while the Factor owner is
// open. It does not choose keys or create a concrete summary Unit. normalize,
// equal, and fingerprint are one reviewed semantic law: they are pure and
// deterministic, and equal(a,b) implies fingerprint(a)==fingerprint(b).
// Fingerprint only indexes candidate comparisons; equal resolves collisions.
func DeclareNormalizer[K ~uint32 | ~uint64, V, S any](factor *Factor[K, V], semantic SemanticKey, normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) (Normalizer[V, S], bool) {
	if factor == nil || !factor.activeOwner() || !semantic.Available() || semantic == factor.schema.semantic || normalize == nil || equal == nil || fingerprint == nil || !factor.claimForm(semantic) {
		if factor != nil {
			factor.composition.poison()
		}
		return Normalizer[V, S]{}, false
	}
	schema := &formSchema{factor: factor.schema, semantic: semantic, readKind: summaryReadForm}
	factor.schema.forms = append(factor.schema.forms, schema)
	form := ReadForm[V, S]{schema: schema, normalize: normalize, equal: equal, fingerprint: fingerprint}
	normalizer := Normalizer[V, S]{schema: schema, normalize: normalize, equal: equal, fingerprint: fingerprint, bindQuery: runtimeQueryReadBinder[K, V](normalize, equal, fingerprint), bindRule: runtimeSummaryRuleReadBinder[K, V](form)}
	factor.formBinds[schema] = readFormBind[V, S]{form: ReadForm[V, S]{schema: schema, normalize: normalize, equal: equal, fingerprint: fingerprint}}
	return normalizer, true
}

// DeclareIdentityNormalizer declares the canonical ordered-cell identity
// summary for one Factor.  It exists for Factor owners which need a variable
// number of exact inputs, but whose Rule semantics observe every selected
// coordinate directly.  The form deliberately reuses the same record
// equality and fingerprint law as ExactReadForm: it is a different declared
// summary shape, not a second interpretation of ordered cells.
//
// semantic remains explicit because the summary form is independently
// authored cold schema.  It must therefore be distinct from the Factor and
// every other form semantic, exactly as for DeclareNormalizer.
func DeclareIdentityNormalizer[K ~uint32 | ~uint64, V any](factor *Factor[K, V], semantic SemanticKey) (Normalizer[V, OrderedCells[V]], bool) {
	if factor == nil || factor.algebra == nil {
		if factor != nil && factor.composition != nil {
			factor.composition.poison()
		}
		return Normalizer[V, OrderedCells[V]]{}, false
	}
	normalize := func(cells OrderedCells[V]) OrderedCells[V] { return cells }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, factor.equal)
	}
	fingerprint := func(cells OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(cells.record, factor.algebra.Fingerprint)
	}
	return DeclareNormalizer(factor, semantic, normalize, equal, fingerprint)
}

// SummaryReadForm turns one declared normalizer into an unanchored summary
// read shape. Exact selected coordinates are deliberately absent until E.
func SummaryReadForm[V, S any](normalizer Normalizer[V, S]) (ReadForm[V, S], bool) {
	if !normalizer.valid() || normalizer.normalize == nil || normalizer.equal == nil || normalizer.fingerprint == nil {
		if normalizer.schema != nil && normalizer.schema.factor != nil && normalizer.schema.factor.composition != nil {
			normalizer.schema.factor.composition.poison()
		}
		return ReadForm[V, S]{}, false
	}
	return ReadForm[V, S]{schema: normalizer.schema, normalize: normalizer.normalize, equal: normalizer.equal, fingerprint: normalizer.fingerprint, bindQuery: normalizer.bindQuery, bindRule: normalizer.bindRule}, true
}

// DeclareWriteSelector creates an unanchored Factor-owned selector target
// form. Its concrete candidate targets are an E concern.
func DeclareWriteSelector[K ~uint32 | ~uint64, V any](factor *Factor[K, V], semantic SemanticKey) (WriteForm[V], bool) {
	if factor == nil || !factor.activeOwner() || !semantic.Available() || !factor.claimForm(semantic) {
		if factor != nil {
			factor.composition.poison()
		}
		return WriteForm[V]{}, false
	}
	schema := &formSchema{factor: factor.schema, semantic: semantic, writeKind: selectorWriteForm}
	factor.schema.forms = append(factor.schema.forms, schema)
	form := WriteForm[V]{schema: schema}
	factor.formBinds[schema] = writeFormBind[V]{form: form}
	return form, true
}

func (factor *Factor[K, V]) claimForm(semantic SemanticKey) bool {
	if factor == nil || !factor.activeOwner() || factor.forms == nil || !semantic.Available() || semantic == factor.schema.semantic {
		return false
	}
	if _, exists := factor.forms[semantic]; exists {
		return false
	}
	if !factor.composition.claim(semantic) {
		return false
	}
	factor.forms[semantic] = struct{}{}
	return true
}

func (factor *Factor[K, V]) valid() bool {
	return factor != nil && factor.composition != nil && factor.schema != nil && factor.schema.composition == factor.composition && factor.algebra != nil && factor.algebra.KeyEnd() == factor.schema.keyEnd && factor.composition.available()
}

func (factor *Factor[K, V]) equal(left, right V) bool {
	return factor != nil && factor.algebra != nil && factor.algebra.Equal(left, right)
}

// activeOwner gates the Factor-only schema mutation surface. Read-only forms
// remain available after Seal for Wave-E binding, but no callback can mutate a
// different or already-closed Factor declaration.
func (factor *Factor[K, V]) activeOwner() bool {
	return factor != nil && factor.open && factor.schema != nil && factor.schema.open && factor.composition != nil && factor.composition.phase == compositionFactors && factor.composition.activeFactor == factor.schema && factor.valid()
}

func (form ReadForm[V, S]) valid() bool {
	return form.schema != nil && form.schema.factor != nil && form.schema.factor.composition != nil && form.schema.readKind != 0 && form.schema.writeKind == 0 && form.schema.semantic.Available() && form.schema.factor.composition.available()
}

func (form WriteForm[V]) valid() bool {
	return form.schema != nil && form.schema.factor != nil && form.schema.factor.composition != nil && form.schema.readKind == 0 && form.schema.writeKind != 0 && form.schema.semantic.Available() && form.schema.factor.composition.available()
}

func (normalizer Normalizer[V, S]) valid() bool {
	return normalizer.schema != nil && normalizer.schema.factor != nil && normalizer.schema.factor.composition != nil && normalizer.schema.readKind == summaryReadForm && normalizer.schema.writeKind == 0 && normalizer.schema.semantic.Available() && normalizer.schema.factor.composition.available()
}

func validColdFactor(composition *Composition, factor *factorSchema) bool {
	if composition == nil || factor == nil || factor.composition != composition || factor.open || !factor.semantic.Available() || factor.exactRead == nil || factor.exactWrite == nil || len(factor.forms) < 2 || !validFactorBind(factor) {
		return false
	}
	forms := make(map[*formSchema]struct{}, len(factor.forms))
	semantics := make(map[SemanticKey]struct{}, len(factor.forms))
	for _, form := range factor.forms {
		if form == nil || form.factor != factor || !form.semantic.Available() || form.readKind != 0 && form.writeKind != 0 {
			return false
		}
		if _, claimed := composition.semantics[form.semantic]; !claimed {
			return false
		}
		if _, duplicate := forms[form]; duplicate {
			return false
		}
		forms[form] = struct{}{}
		if form == factor.exactRead {
			if form.semantic != factor.semantic || form.readKind != exactReadForm || form.writeKind != 0 || form.source != nil {
				return false
			}
			continue
		}
		if form == factor.exactWrite {
			if form.semantic != factor.semantic || form.readKind != 0 || form.writeKind != exactWriteForm || form.source != nil {
				return false
			}
			continue
		}
		if _, duplicate := semantics[form.semantic]; duplicate || form.semantic == factor.semantic {
			return false
		}
		semantics[form.semantic] = struct{}{}
		switch {
		case form.readKind == summaryReadForm && form.writeKind == 0 && form.source == nil:
		case form.readKind == 0 && form.writeKind == selectorWriteForm && form.source == nil:
		default:
			return false
		}
	}
	_, readPresent := forms[factor.exactRead]
	_, writePresent := forms[factor.exactWrite]
	if !readPresent || !writePresent {
		return false
	}
	return true
}

func (factor *factorSchema) hasForm(want *formSchema) bool {
	if factor == nil || want == nil {
		return false
	}
	for _, form := range factor.forms {
		if form == want {
			return true
		}
	}
	return false
}

// OrderedCells is the typed, read-only Factor observation handed to an E
// callback. D can name it in a form but cannot construct one.
type OrderedCells[V any] struct{ record *orderedCellsRecord[V] }

// Count reports the exact finite typed observation width while its Product or
// Query frame is live. A revoked observation reports zero rather than leaking
// stale abstract values into a later transfer or proof check.
func (cells OrderedCells[V]) Count() int {
	if cells.record == nil || !cells.record.live.Load() {
		return 0
	}
	return len(cells.record.cells)
}

// At returns one exact typed observation cell and its presence bit. It is a
// read-only snapshot capability: callers cannot mutate the underlying Factor
// root and a revoked Product/Query frame rejects the read.
func (cells OrderedCells[V]) At(index int) (V, bool, bool) {
	var zero V
	if cells.record == nil || !cells.record.live.Load() || index < 0 || index >= len(cells.record.cells) {
		return zero, false, false
	}
	cell := cells.record.cells[index]
	return cell.value, cell.present, true
}

// summaryCell is private runtime observation storage. Its fields stay hidden
// from domain declarations; an OrderedCells value is valid only while its
// synchronous Product or Query frame remains active.
type summaryCell[V any] struct {
	value   V
	present bool
}

type orderedCellsRecord[V any] struct {
	cells []summaryCell[V]
	live  atomic.Bool
}

func equalOrderedCellRecords[V any](left, right *orderedCellsRecord[V], equal func(V, V) bool) bool {
	if left == nil || right == nil || equal == nil || !left.live.Load() || !right.live.Load() || len(left.cells) != len(right.cells) {
		return false
	}
	for index := range left.cells {
		if left.cells[index].present != right.cells[index].present || left.cells[index].present && !equal(left.cells[index].value, right.cells[index].value) {
			return false
		}
	}
	return true
}

func fingerprintOrderedCellRecord[V any](record *orderedCellsRecord[V], fingerprint func(V) uint64) uint64 {
	if record == nil || fingerprint == nil || !record.live.Load() {
		return 0
	}
	result := uint64(len(record.cells)) ^ 0x9e3779b97f4a7c15
	for _, cell := range record.cells {
		value := uint64(0x517cc1b727220a95)
		if cell.present {
			value = fingerprint(cell.value) ^ 0x94d049bb133111eb
		}
		result ^= value + 0x9e3779b97f4a7c15 + result<<6 + result>>2
	}
	return result
}

func newOrderedCellsRecord[V any](cells []summaryCell[V]) *orderedCellsRecord[V] {
	record := &orderedCellsRecord[V]{cells: append([]summaryCell[V](nil), cells...)}
	record.live.Store(true)
	return record
}

func (record *orderedCellsRecord[V]) revoke() {
	if record == nil || !record.live.CompareAndSwap(true, false) {
		return
	}
	var zero V
	for index := range record.cells {
		record.cells[index].value = zero
		record.cells[index].present = false
	}
	record.cells = nil
}
