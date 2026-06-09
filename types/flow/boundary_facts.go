package flow

import (
	"slices"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// BoundaryPathKind identifies a caller-visible function boundary root. Boundary
// facts are projected from point-local facts by rebasing symbols to either a
// runtime parameter slot or a return slot.
type BoundaryPathKind uint8

const (
	BoundaryPathParam BoundaryPathKind = iota + 1
	BoundaryPathReturn
)

// BoundaryPath is a canonical path relative to a function boundary root.
type BoundaryPath struct {
	Kind     BoundaryPathKind
	Index    int
	Segments []constraint.Segment
}

// BoundaryKeyPresenceFact proves Table[Key] is present after the call returns.
type BoundaryKeyPresenceFact struct {
	Table BoundaryPath
	Key   BoundaryPath
}

// BoundaryKeyArrayFact proves every current element of Array is a key of Table
// after the call returns.
type BoundaryKeyArrayFact struct {
	Array BoundaryPath
	Table BoundaryPath
}

// BoundaryKeyArrayValueFact proves every current element of Array is a key of
// Table and Table[element] has Value after the call returns.
type BoundaryKeyArrayValueFact struct {
	Array BoundaryPath
	Table BoundaryPath
	Value product.AbstractValue
}

// BoundaryAppendKeyFact proves that Key was appended to Array during the call.
// When HasTable is true, the append is tied to a table whose key-array relation
// was being preserved. Without HasTable, callers may materialize a table
// relation only from their own fresh-empty array state.
type BoundaryAppendKeyFact struct {
	Array    BoundaryPath
	Key      BoundaryPath
	Table    BoundaryPath
	HasTable bool
}

// BoundaryAppendHistoryBaseFact proves Array's append history remains tracked
// after the call. It mirrors AppendHistoryBaseFact so conditional append field
// origins can survive function-boundary joins under the same local law.
type BoundaryAppendHistoryBaseFact struct {
	Array BoundaryPath
}

// BoundaryAppendHistoryEventFact records one possible key appended to Array
// during the call. It mirrors AppendHistoryEventFact and is intentionally
// weaker than BoundaryAppendKeyFact: branch joins may preserve possible events
// without claiming a single definite append key.
type BoundaryAppendHistoryEventFact struct {
	Array BoundaryPath
	Key   BoundaryPath
}

// BoundaryAppendHistoryCoverageFact proves that a tracked append event is
// covered by Table[Key] with Value after the call. Together with append bases
// and events, this carries branch-sensitive key-array readback across function
// boundaries.
type BoundaryAppendHistoryCoverageFact struct {
	Array BoundaryPath
	Key   BoundaryPath
	Table BoundaryPath
	Value product.AbstractValue
}

// BoundaryAppendHistoryTableCoverageFact proves that append-history events for
// Array are covered by Table with Value, without naming the event keys. It is
// the portable boundary form for branch-local append keys that cannot be
// rebased into the caller.
type BoundaryAppendHistoryTableCoverageFact struct {
	Array BoundaryPath
	Table BoundaryPath
	Value product.AbstractValue
}

// BoundaryAppendElementFieldOriginFact records that a tracked append to Array
// may have sourced the appended element-relative Field from Source. It is a
// boundary-relative counterpart of AppendElementFieldOriginFact, used to carry
// demand-routing provenance across function calls and returns.
type BoundaryAppendElementFieldOriginFact struct {
	Array       BoundaryPath
	Field       []constraint.Segment
	Source      BoundaryPath
	SourceField []constraint.Segment
}

// BoundaryLengthLowerBound proves len(Target) >= Lower after the call returns.
type BoundaryLengthLowerBound struct {
	Target BoundaryPath
	Lower  int64
}

// BoundaryLengthUpperBound proves len(Target) <= Upper after the call returns.
// It carries exact-empty sequence facts such as table.create capacity
// allocations across constructor/method boundaries.
type BoundaryLengthUpperBound struct {
	Target BoundaryPath
	Upper  int64
}

// BoundaryLengthRelationFact proves len(Target) >= len(Source) after the call
// returns. Both sides are boundary-relative paths so callers can replay the
// relation against concrete call arguments and assignment targets.
type BoundaryLengthRelationFact struct {
	Target BoundaryPath
	Source BoundaryPath
}

// BoundaryIndexWriteFact is the boundary-relative form of
// IndexWriteAdmissionAddressFact. KeyPath/ValuePath carry identities only when
// they cross the boundary; KeyValue/Value carry the product-domain admission
// proof needed for dynamic readback.
type BoundaryIndexWriteFact struct {
	Table        BoundaryPath
	KeyPath      BoundaryPath
	HasKeyPath   bool
	KeyValue     product.AbstractValue
	ValuePath    BoundaryPath
	HasValuePath bool
	Value        product.AbstractValue
}

// BoundaryStaticMemberFact proves Target has Value after boundary rebasing. It
// carries exact static-member PointState facts across function entry/return
// boundaries without forcing those facts into product type structure.
type BoundaryStaticMemberFact struct {
	Target BoundaryPath
	Value  product.AbstractValue
}

// BoundaryFacts is the finite caller-visible postcondition component for facts
// that already exist point-locally but must cross a function boundary. It is a
// must-fact lattice: join keeps only facts proved by every normal return path.
// Projection creates boundary-relative facts from PointState; call transfer
// replays them by rebasing boundary roots onto concrete caller places. This
// carrier does not infer product shape, repair diagnostics, or own mutation
// footprints.
type BoundaryFacts struct {
	bottom              bool
	keyPresence         []BoundaryKeyPresenceFact
	keyArrays           []BoundaryKeyArrayFact
	keyArrayValues      []BoundaryKeyArrayValueFact
	appendKeys          []BoundaryAppendKeyFact
	appendBases         []BoundaryAppendHistoryBaseFact
	appendEvents        []BoundaryAppendHistoryEventFact
	appendCoverage      []BoundaryAppendHistoryCoverageFact
	appendTableCoverage []BoundaryAppendHistoryTableCoverageFact
	appendOrigins       []BoundaryAppendElementFieldOriginFact
	lenLower            []BoundaryLengthLowerBound
	lenUpper            []BoundaryLengthUpperBound
	lenRelations        []BoundaryLengthRelationFact
	indexWrites         []BoundaryIndexWriteFact
	staticMembers       []BoundaryStaticMemberFact
}

// BoundaryFactParts is the construction surface for the boundary-fact carrier.
// It is intentionally lane-shaped: projection code may assemble facts by
// semantic family, while BoundaryFactsFromParts owns compaction and canonical
// identity for the whole carrier.
type BoundaryFactParts struct {
	KeyPresence         []BoundaryKeyPresenceFact
	KeyArrays           []BoundaryKeyArrayFact
	KeyArrayValues      []BoundaryKeyArrayValueFact
	AppendKeys          []BoundaryAppendKeyFact
	AppendBases         []BoundaryAppendHistoryBaseFact
	AppendEvents        []BoundaryAppendHistoryEventFact
	AppendCoverage      []BoundaryAppendHistoryCoverageFact
	AppendTableCoverage []BoundaryAppendHistoryTableCoverageFact
	AppendOrigins       []BoundaryAppendElementFieldOriginFact
	LengthLower         []BoundaryLengthLowerBound
	LengthUpper         []BoundaryLengthUpperBound
	LengthRelations     []BoundaryLengthRelationFact
	IndexWrites         []BoundaryIndexWriteFact
	StaticMembers       []BoundaryStaticMemberFact
}

// BoundaryFactsFromParts builds a canonical finite boundary-fact value from all
// currently-supported lanes. New lanes enter the carrier through this function
// before callers receive public construction helpers.
func BoundaryFactsFromParts(parts BoundaryFactParts) BoundaryFacts {
	return BoundaryFacts{
		keyPresence:         compactBoundaryKeyPresence(parts.KeyPresence),
		keyArrays:           compactBoundaryKeyArrays(parts.KeyArrays),
		keyArrayValues:      compactBoundaryKeyArrayValues(parts.KeyArrayValues),
		appendKeys:          compactBoundaryAppendKeys(parts.AppendKeys),
		appendBases:         compactBoundaryAppendHistoryBases(parts.AppendBases),
		appendEvents:        compactBoundaryAppendHistoryEvents(parts.AppendEvents),
		appendCoverage:      compactBoundaryAppendHistoryCoverage(parts.AppendCoverage),
		appendTableCoverage: compactBoundaryAppendHistoryTableCoverage(parts.AppendTableCoverage),
		appendOrigins:       compactBoundaryAppendElementFieldOrigins(parts.AppendOrigins),
		lenLower:            compactBoundaryLengthLower(parts.LengthLower),
		lenUpper:            compactBoundaryLengthUpper(parts.LengthUpper),
		lenRelations:        compactBoundaryLengthRelations(parts.LengthRelations),
		indexWrites:         compactBoundaryIndexWrites(parts.IndexWrites),
		staticMembers:       compactBoundaryStaticMembers(parts.StaticMembers),
	}
}

// Parts returns an alias-free lane view of f. Bottom and Top both expose no
// finite parts; callers that care about lattice sentinels must inspect f first.
func (f BoundaryFacts) Parts() BoundaryFactParts {
	if f.bottom || !f.HasProof() {
		return BoundaryFactParts{}
	}
	return BoundaryFactParts{
		KeyPresence:         f.KeyPresence(),
		KeyArrays:           f.KeyArrays(),
		KeyArrayValues:      f.KeyArrayValues(),
		AppendKeys:          f.AppendKeys(),
		AppendBases:         f.AppendHistoryBases(),
		AppendEvents:        f.AppendHistoryEvents(),
		AppendCoverage:      f.AppendHistoryCoverage(),
		AppendTableCoverage: f.AppendHistoryTableCoverage(),
		AppendOrigins:       f.AppendElementFieldOrigins(),
		LengthLower:         f.LengthLowerBounds(),
		LengthUpper:         f.LengthUpperBounds(),
		LengthRelations:     f.LengthRelations(),
		IndexWrites:         f.IndexWrites(),
		StaticMembers:       f.StaticMembers(),
	}
}

// appendBoundaryFactParts returns p plus other without canonicalizing.
// BoundaryFactsFromParts is the only canonicalization boundary.
func appendBoundaryFactParts(p, other BoundaryFactParts) BoundaryFactParts {
	p.KeyPresence = append(p.KeyPresence, other.KeyPresence...)
	p.KeyArrays = append(p.KeyArrays, other.KeyArrays...)
	p.KeyArrayValues = append(p.KeyArrayValues, other.KeyArrayValues...)
	p.AppendKeys = append(p.AppendKeys, other.AppendKeys...)
	p.AppendBases = append(p.AppendBases, other.AppendBases...)
	p.AppendEvents = append(p.AppendEvents, other.AppendEvents...)
	p.AppendCoverage = append(p.AppendCoverage, other.AppendCoverage...)
	p.AppendTableCoverage = append(p.AppendTableCoverage, other.AppendTableCoverage...)
	p.AppendOrigins = append(p.AppendOrigins, other.AppendOrigins...)
	p.LengthLower = append(p.LengthLower, other.LengthLower...)
	p.LengthUpper = append(p.LengthUpper, other.LengthUpper...)
	p.LengthRelations = append(p.LengthRelations, other.LengthRelations...)
	p.IndexWrites = append(p.IndexWrites, other.IndexWrites...)
	p.StaticMembers = append(p.StaticMembers, other.StaticMembers...)
	return p
}

// Clone returns a canonical, alias-free copy of f. BoundaryFacts owns its lane
// set; external packages should not rebuild it field by field because that makes
// every new fact lane a cross-package migration hazard.
func (f BoundaryFacts) Clone() BoundaryFacts {
	if f.bottom || !f.HasProof() {
		return BoundaryFactsDomain.Top()
	}
	return BoundaryFactsFromParts(f.Parts())
}

// IdentityHash returns a canonical hash for the exact boundary-fact set. It is
// intentionally owned with the lane compactors/comparators so adding a fact lane
// updates equality and hash identity in one package.
func (f BoundaryFacts) IdentityHash(seed string) uint64 {
	h := internal.FnvString(seed)
	if f.bottom || !f.HasProof() {
		return h
	}
	h = hashBoundaryKeyIndexFacts(h, f)
	h = hashBoundaryAppendFacts(h, f)
	h = hashBoundaryLengthFacts(h, f)
	h = hashBoundaryStaticMemberFacts(h, f)
	return h
}

// BoundaryReturnFactBucket groups finite boundary facts by the return slots
// their paths mention. Summary projection uses these buckets to apply the
// must-fact join only to return points where those slots are actually bound.
type BoundaryReturnFactBucket struct {
	indices []int
	facts   BoundaryFacts
}

type boundaryReturnFactAdder func(indices []int, add func(*BoundaryFactParts))

type boundaryPathMapper func(BoundaryPath) (BoundaryPath, bool)

// Indices returns the sorted return-slot indices mentioned by the bucket.
func (b BoundaryReturnFactBucket) Indices() []int {
	return append([]int(nil), b.indices...)
}

// Facts returns the finite facts carried by this return-slot bucket.
func (b BoundaryReturnFactBucket) Facts() BoundaryFacts {
	return b.facts
}

// UnionBoundaryFactProofs combines independently-proven finite boundary facts.
// It is a proof builder, not the lattice Join: if two derivations both hold on
// the same path, callers may consume the union of their facts. Top contributes
// no finite proof; Bottom is treated as unreachable/no consumable proof here.
func UnionBoundaryFactProofs(a, b BoundaryFacts) BoundaryFacts {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return BoundaryFactsFromParts(appendBoundaryFactParts(a.Parts(), b.Parts()))
}

// RebaseBoundaryReturnFactsToParam maps facts proven about one returned value
// into facts proven about the callee parameter receiving that value. It is used
// for call-result arguments such as `make():use()`, where no caller-local path
// exists but the selected call outcome already proved return-relative facts.
// Facts mentioning any other boundary root are intentionally dropped; composing
// those requires a full call-boundary map, not a partial guess.
func RebaseBoundaryReturnFactsToParam(facts BoundaryFacts, returnIndex, paramSlot int) BoundaryFacts {
	if facts.bottom {
		return facts
	}
	if !facts.HasProof() || returnIndex < 0 || paramSlot < 0 {
		return BoundaryFactsDomain.Top()
	}
	mapPath := func(path BoundaryPath) (BoundaryPath, bool) {
		if path.Kind != BoundaryPathReturn || path.Index != returnIndex {
			return BoundaryPath{}, false
		}
		return BoundaryPath{
			Kind:     BoundaryPathParam,
			Index:    paramSlot,
			Segments: append([]constraint.Segment(nil), path.Segments...),
		}, true
	}
	parts := rebaseBoundaryKeyIndexReturnFactsToParam(facts, mapPath)
	parts = appendBoundaryFactParts(parts, rebaseBoundaryAppendReturnFactsToParam(facts, mapPath))
	parts = appendBoundaryFactParts(parts, rebaseBoundaryLengthReturnFactsToParam(facts, mapPath))
	return BoundaryFactsFromParts(appendBoundaryFactParts(parts, rebaseBoundaryStaticMemberReturnFactsToParam(facts, mapPath)))
}

// BoundaryFactsDomain is the lattice over boundary postconditions. Its Join is
// conjunction over possible returns/targets: a fact missing from one branch is
// not caller-visible proof.
var BoundaryFactsDomain = lattice.Lattice[BoundaryFacts]{
	Bottom: func() BoundaryFacts { return BoundaryFacts{bottom: true} },
	Top:    func() BoundaryFacts { return BoundaryFacts{} },
	Equal: func(a, b BoundaryFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return boundaryKeyIndexFactsEqual(a, b) &&
			boundaryAppendFactsEqual(a, b) &&
			boundaryLengthFactsEqual(a, b) &&
			boundaryStaticMemberFactsEqual(a, b)
	},
	LessOrEq: func(a, b BoundaryFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return boundaryKeyIndexFactsLessOrEq(a, b) &&
			boundaryAppendFactsLessOrEq(a, b) &&
			boundaryLengthFactsLessOrEq(a, b) &&
			boundaryStaticMemberFactsLessOrEq(a, b)
	},
	Join:  joinBoundaryFacts,
	Meet:  nil,
	Widen: widenBoundaryFacts,
}

func (f BoundaryFacts) IsBottom() bool { return f.bottom }

func (f BoundaryFacts) HasProof() bool {
	return !f.bottom && (len(f.keyPresence) > 0 || len(f.keyArrays) > 0 || len(f.keyArrayValues) > 0 || len(f.appendKeys) > 0 || len(f.appendBases) > 0 || len(f.appendEvents) > 0 || len(f.appendCoverage) > 0 || len(f.appendTableCoverage) > 0 || len(f.appendOrigins) > 0 || len(f.lenLower) > 0 || len(f.lenUpper) > 0 || len(f.lenRelations) > 0 || len(f.indexWrites) > 0 || len(f.staticMembers) > 0)
}

// PartitionByReturnIndices separates parameter-only facts from facts whose
// boundary paths mention return slots. Return-relative facts are grouped by the
// sorted set of return indices they mention, so must-fact projection can reason
// about eligibility per returned value instead of treating all returns as one
// unconditional postcondition stream.
func (f BoundaryFacts) PartitionByReturnIndices() (BoundaryFacts, []BoundaryReturnFactBucket) {
	if f.bottom || !f.HasProof() {
		return f, nil
	}
	var params BoundaryFactParts
	buckets := make(map[string]BoundaryFactParts)
	bucketIndices := make(map[string][]int)
	addReturnFact := func(indices []int, add func(*BoundaryFactParts)) {
		key := boundaryReturnIndicesKey(indices)
		parts := buckets[key]
		add(&parts)
		buckets[key] = parts
		if _, ok := bucketIndices[key]; !ok {
			bucketIndices[key] = append([]int(nil), indices...)
		}
	}
	partitionBoundaryKeyIndexFactsByReturnIndices(f, &params, addReturnFact)
	partitionBoundaryAppendFactsByReturnIndices(f, &params, addReturnFact)
	partitionBoundaryLengthFactsByReturnIndices(f, &params, addReturnFact)
	partitionBoundaryStaticMemberFactsByReturnIndices(f, &params, addReturnFact)
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]BoundaryReturnFactBucket, 0, len(keys))
	for _, key := range keys {
		out = append(out, BoundaryReturnFactBucket{
			indices: append([]int(nil), bucketIndices[key]...),
			facts:   buckets[key].facts(),
		})
	}
	return params.facts(), out
}

func (f BoundaryFacts) KeyPresence() []BoundaryKeyPresenceFact {
	if f.bottom || len(f.keyPresence) == 0 {
		return nil
	}
	out := make([]BoundaryKeyPresenceFact, 0, len(f.keyPresence))
	for _, fact := range f.keyPresence {
		out = append(out, cloneBoundaryKeyPresence(fact))
	}
	return out
}

func (f BoundaryFacts) KeyArrays() []BoundaryKeyArrayFact {
	if f.bottom || len(f.keyArrays) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayFact, 0, len(f.keyArrays))
	for _, fact := range f.keyArrays {
		out = append(out, cloneBoundaryKeyArray(fact))
	}
	return out
}

func (f BoundaryFacts) KeyArrayValues() []BoundaryKeyArrayValueFact {
	if f.bottom || len(f.keyArrayValues) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayValueFact, 0, len(f.keyArrayValues))
	for _, fact := range f.keyArrayValues {
		out = append(out, cloneBoundaryKeyArrayValue(fact))
	}
	return out
}

func (f BoundaryFacts) AppendKeys() []BoundaryAppendKeyFact {
	if f.bottom || len(f.appendKeys) == 0 {
		return nil
	}
	out := make([]BoundaryAppendKeyFact, 0, len(f.appendKeys))
	for _, fact := range f.appendKeys {
		out = append(out, cloneBoundaryAppendKey(fact))
	}
	return out
}

func (f BoundaryFacts) AppendHistoryBases() []BoundaryAppendHistoryBaseFact {
	if f.bottom || len(f.appendBases) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryBaseFact, 0, len(f.appendBases))
	for _, fact := range f.appendBases {
		out = append(out, cloneBoundaryAppendHistoryBase(fact))
	}
	return out
}

func (f BoundaryFacts) AppendHistoryEvents() []BoundaryAppendHistoryEventFact {
	if f.bottom || len(f.appendEvents) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryEventFact, 0, len(f.appendEvents))
	for _, fact := range f.appendEvents {
		out = append(out, cloneBoundaryAppendHistoryEvent(fact))
	}
	return out
}

func (f BoundaryFacts) AppendHistoryCoverage() []BoundaryAppendHistoryCoverageFact {
	if f.bottom || len(f.appendCoverage) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryCoverageFact, 0, len(f.appendCoverage))
	for _, fact := range f.appendCoverage {
		out = append(out, cloneBoundaryAppendHistoryCoverage(fact))
	}
	return out
}

func (f BoundaryFacts) AppendHistoryTableCoverage() []BoundaryAppendHistoryTableCoverageFact {
	if f.bottom || len(f.appendTableCoverage) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryTableCoverageFact, 0, len(f.appendTableCoverage))
	for _, fact := range f.appendTableCoverage {
		out = append(out, cloneBoundaryAppendHistoryTableCoverage(fact))
	}
	return out
}

func (f BoundaryFacts) AppendElementFieldOrigins() []BoundaryAppendElementFieldOriginFact {
	if f.bottom || len(f.appendOrigins) == 0 {
		return nil
	}
	out := make([]BoundaryAppendElementFieldOriginFact, 0, len(f.appendOrigins))
	for _, fact := range f.appendOrigins {
		out = append(out, cloneBoundaryAppendElementFieldOrigin(fact))
	}
	return out
}

func (f BoundaryFacts) LengthLowerBounds() []BoundaryLengthLowerBound {
	if f.bottom || len(f.lenLower) == 0 {
		return nil
	}
	out := make([]BoundaryLengthLowerBound, 0, len(f.lenLower))
	for _, fact := range f.lenLower {
		out = append(out, cloneBoundaryLengthLower(fact))
	}
	return out
}

func (f BoundaryFacts) LengthUpperBounds() []BoundaryLengthUpperBound {
	if f.bottom || len(f.lenUpper) == 0 {
		return nil
	}
	out := make([]BoundaryLengthUpperBound, 0, len(f.lenUpper))
	for _, fact := range f.lenUpper {
		out = append(out, cloneBoundaryLengthUpper(fact))
	}
	return out
}

func (f BoundaryFacts) LengthRelations() []BoundaryLengthRelationFact {
	if f.bottom || len(f.lenRelations) == 0 {
		return nil
	}
	out := make([]BoundaryLengthRelationFact, 0, len(f.lenRelations))
	for _, fact := range f.lenRelations {
		out = append(out, cloneBoundaryLengthRelation(fact))
	}
	return out
}

func (f BoundaryFacts) IndexWrites() []BoundaryIndexWriteFact {
	if f.bottom || len(f.indexWrites) == 0 {
		return nil
	}
	out := make([]BoundaryIndexWriteFact, 0, len(f.indexWrites))
	for _, fact := range f.indexWrites {
		out = append(out, cloneBoundaryIndexWrite(fact))
	}
	return out
}

func (f BoundaryFacts) StaticMembers() []BoundaryStaticMemberFact {
	if f.bottom || len(f.staticMembers) == 0 {
		return nil
	}
	out := make([]BoundaryStaticMemberFact, 0, len(f.staticMembers))
	for _, fact := range f.staticMembers {
		out = append(out, cloneBoundaryStaticMember(fact))
	}
	return out
}

func (f BoundaryFacts) HasKeyPresence(fact BoundaryKeyPresenceFact) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryKeyPresenceRowIdentity.Find(f.keyPresence, fact)
	return ok
}

func (f BoundaryFacts) HasKeyArray(fact BoundaryKeyArrayFact) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryKeyArrayRowIdentity.Find(f.keyArrays, fact)
	return ok
}

func (f BoundaryFacts) HasLengthLowerBound(fact BoundaryLengthLowerBound) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryLengthLowerRowIdentity.Find(f.lenLower, fact)
	return ok
}

func (f BoundaryFacts) HasLengthUpperBound(fact BoundaryLengthUpperBound) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryLengthUpperRowIdentity.Find(f.lenUpper, fact)
	return ok
}

func (f BoundaryFacts) HasLengthRelation(fact BoundaryLengthRelationFact) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryLengthRelationRowIdentity.Find(f.lenRelations, fact)
	return ok
}

func (f BoundaryFacts) HasIndexWrite(fact BoundaryIndexWriteFact) bool {
	if f.bottom {
		return false
	}
	idx, ok := boundaryIndexWriteRowIdentity.Find(f.indexWrites, fact)
	return ok && product.Domain.Equal(f.indexWrites[idx].Value, fact.Value)
}

func (f BoundaryFacts) HasStaticMember(fact BoundaryStaticMemberFact) bool {
	if f.bottom {
		return false
	}
	idx, ok := boundaryStaticMemberRowIdentity.Find(f.staticMembers, fact)
	return ok && product.Domain.Equal(f.staticMembers[idx].Value, fact.Value)
}

func joinBoundaryFacts(a, b BoundaryFacts) BoundaryFacts {
	return mergeBoundaryFacts(a, b, false)
}

func widenBoundaryFacts(prev, next BoundaryFacts) BoundaryFacts {
	return mergeBoundaryFacts(prev, next, true)
}

func mergeBoundaryFacts(a, b BoundaryFacts, widenPayload bool) BoundaryFacts {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return BoundaryFactsFromParts(intersectBoundaryFactParts(a, b, widenPayload))
}

func intersectBoundaryFactParts(a, b BoundaryFacts, widenPayload bool) BoundaryFactParts {
	parts := boundaryKeyIndexIntersectParts(a, b, widenPayload)
	parts = appendBoundaryFactParts(parts, boundaryAppendIntersectParts(a, b, widenPayload))
	parts = appendBoundaryFactParts(parts, boundaryLengthIntersectParts(a, b))
	return appendBoundaryFactParts(parts, boundaryStaticMemberIntersectParts(a, b, widenPayload))
}

func (p BoundaryFactParts) facts() BoundaryFacts {
	return BoundaryFactsFromParts(p)
}

func boundaryPathReturnIndices(paths ...BoundaryPath) []int {
	var out []int
	for _, path := range paths {
		if path.Kind == BoundaryPathReturn && path.Index >= 0 {
			out = append(out, path.Index)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func boundaryReturnIndicesKey(indices []int) string {
	var out []byte
	for i, idx := range indices {
		if i > 0 {
			out = append(out, ',')
		}
		out = appendBoundaryIndexKey(out, idx)
	}
	return string(out)
}

func appendBoundaryIndexKey(out []byte, idx int) []byte {
	if idx == 0 {
		return append(out, '0')
	}
	var buf [20]byte
	i := len(buf)
	for idx > 0 {
		i--
		buf[i] = byte('0' + idx%10)
		idx /= 10
	}
	return append(out, buf[i:]...)
}
