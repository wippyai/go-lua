package flow

import (
	"cmp"
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

// Append returns p plus other without canonicalizing. BoundaryFactsFromParts is
// the only canonicalization boundary.
func (p BoundaryFactParts) Append(other BoundaryFactParts) BoundaryFactParts {
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
	for _, fact := range f.KeyPresence() {
		h = internal.HashCombine(h, internal.FnvString("kp"))
		h = hashBoundaryPath(h, fact.Table)
		h = hashBoundaryPath(h, fact.Key)
	}
	for _, fact := range f.KeyArrays() {
		h = internal.HashCombine(h, internal.FnvString("ka"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Table)
	}
	for _, fact := range f.KeyArrayValues() {
		h = internal.HashCombine(h, internal.FnvString("kav"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Table)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	for _, fact := range f.AppendKeys() {
		h = internal.HashCombine(h, internal.FnvString("ak"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Key)
		if fact.HasTable {
			h = internal.HashCombine(h, 1)
			h = hashBoundaryPath(h, fact.Table)
		} else {
			h = internal.HashCombine(h, 0)
		}
	}
	for _, fact := range f.AppendHistoryBases() {
		h = internal.HashCombine(h, internal.FnvString("ahb"))
		h = hashBoundaryPath(h, fact.Array)
	}
	for _, fact := range f.AppendHistoryEvents() {
		h = internal.HashCombine(h, internal.FnvString("ahe"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Key)
	}
	for _, fact := range f.AppendHistoryCoverage() {
		h = internal.HashCombine(h, internal.FnvString("ahc"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Key)
		h = hashBoundaryPath(h, fact.Table)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	for _, fact := range f.AppendHistoryTableCoverage() {
		h = internal.HashCombine(h, internal.FnvString("ahtc"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Table)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	for _, fact := range f.AppendElementFieldOrigins() {
		h = internal.HashCombine(h, internal.FnvString("aefo"))
		h = hashBoundaryPath(h, fact.Array)
		h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(fact.Field)))
		h = hashBoundaryPath(h, fact.Source)
		h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(fact.SourceField)))
	}
	for _, fact := range f.LengthLowerBounds() {
		h = internal.HashCombine(h, internal.FnvString("len"))
		h = hashBoundaryPath(h, fact.Target)
		h = internal.HashCombine(h, uint64(fact.Lower))
	}
	for _, fact := range f.LengthUpperBounds() {
		h = internal.HashCombine(h, internal.FnvString("lenu"))
		h = hashBoundaryPath(h, fact.Target)
		h = internal.HashCombine(h, uint64(fact.Upper))
	}
	for _, fact := range f.LengthRelations() {
		h = internal.HashCombine(h, internal.FnvString("lenrel"))
		h = hashBoundaryPath(h, fact.Target)
		h = hashBoundaryPath(h, fact.Source)
	}
	for _, fact := range f.IndexWrites() {
		h = internal.HashCombine(h, internal.FnvString("iw"))
		h = hashBoundaryPath(h, fact.Table)
		h = hashBoundaryIndexKey(h, fact)
		if fact.HasValuePath {
			h = internal.HashCombine(h, 1)
			h = hashBoundaryPath(h, fact.ValuePath)
		} else {
			h = internal.HashCombine(h, 0)
		}
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	for _, fact := range f.StaticMembers() {
		h = internal.HashCombine(h, internal.FnvString("sm"))
		h = hashBoundaryPath(h, fact.Target)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	return h
}

// BoundaryReturnFactBucket groups finite boundary facts by the return slots
// their paths mention. Summary projection uses these buckets to apply the
// must-fact join only to return points where those slots are actually bound.
type BoundaryReturnFactBucket struct {
	indices []int
	facts   BoundaryFacts
}

// Indices returns the sorted return-slot indices mentioned by the bucket.
func (b BoundaryReturnFactBucket) Indices() []int {
	return append([]int(nil), b.indices...)
}

// Facts returns the finite facts carried by this return-slot bucket.
func (b BoundaryReturnFactBucket) Facts() BoundaryFacts {
	return b.facts
}

// MergeBoundaryFactProofs combines independently-proven finite boundary facts.
// It is a proof builder, not the lattice Join: if two derivations both hold on
// the same path, callers may consume the union of their facts. Top contributes
// no finite proof; Bottom is treated as unreachable/no consumable proof here.
func MergeBoundaryFactProofs(a, b BoundaryFacts) BoundaryFacts {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return BoundaryFactsFromParts(a.Parts().Append(b.Parts()))
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
	var keyPresence []BoundaryKeyPresenceFact
	for _, fact := range facts.KeyPresence() {
		table, tableOK := mapPath(fact.Table)
		key, keyOK := mapPath(fact.Key)
		if tableOK && keyOK {
			keyPresence = append(keyPresence, BoundaryKeyPresenceFact{Table: table, Key: key})
		}
	}
	var keyArrays []BoundaryKeyArrayFact
	for _, fact := range facts.KeyArrays() {
		array, arrayOK := mapPath(fact.Array)
		table, tableOK := mapPath(fact.Table)
		if arrayOK && tableOK {
			keyArrays = append(keyArrays, BoundaryKeyArrayFact{Array: array, Table: table})
		}
	}
	var keyArrayValues []BoundaryKeyArrayValueFact
	for _, fact := range facts.KeyArrayValues() {
		array, arrayOK := mapPath(fact.Array)
		table, tableOK := mapPath(fact.Table)
		if arrayOK && tableOK {
			keyArrayValues = append(keyArrayValues, BoundaryKeyArrayValueFact{Array: array, Table: table, Value: fact.Value})
		}
	}
	var appendKeys []BoundaryAppendKeyFact
	for _, fact := range facts.AppendKeys() {
		array, arrayOK := mapPath(fact.Array)
		key, keyOK := mapPath(fact.Key)
		if !arrayOK || !keyOK {
			continue
		}
		next := BoundaryAppendKeyFact{Array: array, Key: key}
		if fact.HasTable {
			table, tableOK := mapPath(fact.Table)
			if !tableOK {
				continue
			}
			next.Table = table
			next.HasTable = true
		}
		appendKeys = append(appendKeys, next)
	}
	var appendBases []BoundaryAppendHistoryBaseFact
	for _, fact := range facts.AppendHistoryBases() {
		array, ok := mapPath(fact.Array)
		if ok {
			appendBases = append(appendBases, BoundaryAppendHistoryBaseFact{Array: array})
		}
	}
	var appendEvents []BoundaryAppendHistoryEventFact
	for _, fact := range facts.AppendHistoryEvents() {
		array, arrayOK := mapPath(fact.Array)
		key, keyOK := mapPath(fact.Key)
		if arrayOK && keyOK {
			appendEvents = append(appendEvents, BoundaryAppendHistoryEventFact{Array: array, Key: key})
		}
	}
	var appendCoverage []BoundaryAppendHistoryCoverageFact
	for _, fact := range facts.AppendHistoryCoverage() {
		array, arrayOK := mapPath(fact.Array)
		key, keyOK := mapPath(fact.Key)
		table, tableOK := mapPath(fact.Table)
		if arrayOK && keyOK && tableOK {
			appendCoverage = append(appendCoverage, BoundaryAppendHistoryCoverageFact{
				Array: array,
				Key:   key,
				Table: table,
				Value: fact.Value,
			})
		}
	}
	var appendTableCoverage []BoundaryAppendHistoryTableCoverageFact
	for _, fact := range facts.AppendHistoryTableCoverage() {
		array, arrayOK := mapPath(fact.Array)
		table, tableOK := mapPath(fact.Table)
		if arrayOK && tableOK {
			appendTableCoverage = append(appendTableCoverage, BoundaryAppendHistoryTableCoverageFact{
				Array: array,
				Table: table,
				Value: fact.Value,
			})
		}
	}
	var appendOrigins []BoundaryAppendElementFieldOriginFact
	for _, fact := range facts.AppendElementFieldOrigins() {
		array, arrayOK := mapPath(fact.Array)
		source, sourceOK := mapPath(fact.Source)
		if arrayOK && sourceOK {
			appendOrigins = append(appendOrigins, BoundaryAppendElementFieldOriginFact{
				Array:       array,
				Field:       append([]constraint.Segment(nil), fact.Field...),
				Source:      source,
				SourceField: append([]constraint.Segment(nil), fact.SourceField...),
			})
		}
	}
	var lenLower []BoundaryLengthLowerBound
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := mapPath(fact.Target)
		if ok {
			lenLower = append(lenLower, BoundaryLengthLowerBound{Target: target, Lower: fact.Lower})
		}
	}
	var lenUpper []BoundaryLengthUpperBound
	for _, fact := range facts.LengthUpperBounds() {
		target, ok := mapPath(fact.Target)
		if ok {
			lenUpper = append(lenUpper, BoundaryLengthUpperBound{Target: target, Upper: fact.Upper})
		}
	}
	var lenRelations []BoundaryLengthRelationFact
	for _, fact := range facts.LengthRelations() {
		target, targetOK := mapPath(fact.Target)
		source, sourceOK := mapPath(fact.Source)
		if targetOK && sourceOK {
			lenRelations = append(lenRelations, BoundaryLengthRelationFact{Target: target, Source: source})
		}
	}
	var indexWrites []BoundaryIndexWriteFact
	for _, fact := range facts.IndexWrites() {
		table, tableOK := mapPath(fact.Table)
		if !tableOK {
			continue
		}
		next := BoundaryIndexWriteFact{
			Table:    table,
			KeyValue: fact.KeyValue,
			Value:    fact.Value,
		}
		if fact.HasKeyPath {
			key, keyOK := mapPath(fact.KeyPath)
			if !keyOK {
				continue
			}
			next.KeyPath = key
			next.HasKeyPath = true
		}
		if fact.HasValuePath {
			value, valueOK := mapPath(fact.ValuePath)
			if !valueOK {
				continue
			}
			next.ValuePath = value
			next.HasValuePath = true
		}
		indexWrites = append(indexWrites, next)
	}
	var staticMembers []BoundaryStaticMemberFact
	for _, fact := range facts.StaticMembers() {
		target, ok := mapPath(fact.Target)
		if ok {
			staticMembers = append(staticMembers, BoundaryStaticMemberFact{Target: target, Value: fact.Value})
		}
	}
	return BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:         keyPresence,
		KeyArrays:           keyArrays,
		KeyArrayValues:      keyArrayValues,
		AppendKeys:          appendKeys,
		AppendBases:         appendBases,
		AppendEvents:        appendEvents,
		AppendCoverage:      appendCoverage,
		AppendTableCoverage: appendTableCoverage,
		AppendOrigins:       appendOrigins,
		LengthLower:         lenLower,
		LengthUpper:         lenUpper,
		LengthRelations:     lenRelations,
		IndexWrites:         indexWrites,
		StaticMembers:       staticMembers,
	})
}

// BoundaryFactsDomain is the lattice over boundary postconditions.
var BoundaryFactsDomain = lattice.Lattice[BoundaryFacts]{
	Bottom: func() BoundaryFacts { return BoundaryFacts{bottom: true} },
	Top:    func() BoundaryFacts { return BoundaryFacts{} },
	Equal: func(a, b BoundaryFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return boundaryKeyPresenceRowIdentity.Equal(a.keyPresence, b.keyPresence) &&
			boundaryKeyArrayRowIdentity.Equal(a.keyArrays, b.keyArrays) &&
			boundaryKeyArrayValueRowIdentity.EqualBy(a.keyArrayValues, b.keyArrayValues, func(x, y BoundaryKeyArrayValueFact) bool {
				return compareBoundaryKeyArrayValue(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			}) &&
			boundaryAppendKeyRowIdentity.Equal(a.appendKeys, b.appendKeys) &&
			boundaryAppendHistoryBaseRowIdentity.Equal(a.appendBases, b.appendBases) &&
			boundaryAppendHistoryEventRowIdentity.Equal(a.appendEvents, b.appendEvents) &&
			boundaryAppendHistoryCoverageRowIdentity.EqualBy(a.appendCoverage, b.appendCoverage, func(x, y BoundaryAppendHistoryCoverageFact) bool {
				return compareBoundaryAppendHistoryCoverage(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			}) &&
			boundaryAppendHistoryTableCoverageRowIdentity.EqualBy(a.appendTableCoverage, b.appendTableCoverage, func(x, y BoundaryAppendHistoryTableCoverageFact) bool {
				return compareBoundaryAppendHistoryTableCoverage(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			}) &&
			boundaryAppendElementFieldOriginRowIdentity.Equal(a.appendOrigins, b.appendOrigins) &&
			boundaryLengthLowerRowIdentity.Equal(a.lenLower, b.lenLower) &&
			boundaryLengthUpperRowIdentity.Equal(a.lenUpper, b.lenUpper) &&
			boundaryLengthRelationRowIdentity.Equal(a.lenRelations, b.lenRelations) &&
			boundaryIndexWriteRowIdentity.EqualBy(a.indexWrites, b.indexWrites, func(x, y BoundaryIndexWriteFact) bool {
				return compareBoundaryIndexWrite(x, y) == 0 &&
					product.Domain.Equal(x.KeyValue, y.KeyValue) &&
					product.Domain.Equal(x.Value, y.Value)
			}) &&
			boundaryStaticMemberRowIdentity.EqualBy(a.staticMembers, b.staticMembers, func(x, y BoundaryStaticMemberFact) bool {
				return compareBoundaryStaticMember(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			})
	},
	LessOrEq: func(a, b BoundaryFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return boundaryKeyPresenceContainAll(a.keyPresence, b.keyPresence) &&
			boundaryKeyArraysContainAll(a.keyArrays, b.keyArrays) &&
			boundaryKeyArrayValuesContainAll(a.keyArrayValues, b.keyArrayValues) &&
			boundaryAppendKeysContainAll(a.appendKeys, b.appendKeys) &&
			boundaryAppendHistoryBasesContainAll(a.appendBases, b.appendBases) &&
			boundaryAppendHistoryEventsContainAll(a.appendEvents, b.appendEvents) &&
			boundaryAppendHistoryCoverageContainAll(a.appendCoverage, b.appendCoverage) &&
			boundaryAppendHistoryTableCoverageContainAll(a.appendTableCoverage, b.appendTableCoverage) &&
			boundaryAppendElementFieldOriginsContainAll(a.appendOrigins, b.appendOrigins) &&
			boundaryLengthLowerContainAll(a.lenLower, b.lenLower) &&
			boundaryLengthUpperContainAll(a.lenUpper, b.lenUpper) &&
			boundaryLengthRelationContainAll(a.lenRelations, b.lenRelations) &&
			boundaryIndexWritesContainAll(a.indexWrites, b.indexWrites) &&
			boundaryStaticMembersContainAll(a.staticMembers, b.staticMembers)
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
	for _, fact := range f.KeyPresence() {
		indices := boundaryPathReturnIndices(fact.Table, fact.Key)
		if len(indices) == 0 {
			params.KeyPresence = append(params.KeyPresence, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.KeyPresence = append(parts.KeyPresence, fact)
		})
	}
	for _, fact := range f.KeyArrays() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Table)
		if len(indices) == 0 {
			params.KeyArrays = append(params.KeyArrays, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.KeyArrays = append(parts.KeyArrays, fact)
		})
	}
	for _, fact := range f.KeyArrayValues() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Table)
		if len(indices) == 0 {
			params.KeyArrayValues = append(params.KeyArrayValues, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.KeyArrayValues = append(parts.KeyArrayValues, fact)
		})
	}
	for _, fact := range f.AppendKeys() {
		paths := []BoundaryPath{fact.Array, fact.Key}
		if fact.HasTable {
			paths = append(paths, fact.Table)
		}
		indices := boundaryPathReturnIndices(paths...)
		if len(indices) == 0 {
			params.AppendKeys = append(params.AppendKeys, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendKeys = append(parts.AppendKeys, fact)
		})
	}
	for _, fact := range f.AppendHistoryBases() {
		indices := boundaryPathReturnIndices(fact.Array)
		if len(indices) == 0 {
			params.AppendBases = append(params.AppendBases, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendBases = append(parts.AppendBases, fact)
		})
	}
	for _, fact := range f.AppendHistoryEvents() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Key)
		if len(indices) == 0 {
			params.AppendEvents = append(params.AppendEvents, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendEvents = append(parts.AppendEvents, fact)
		})
	}
	for _, fact := range f.AppendHistoryCoverage() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Key, fact.Table)
		if len(indices) == 0 {
			params.AppendCoverage = append(params.AppendCoverage, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendCoverage = append(parts.AppendCoverage, fact)
		})
	}
	for _, fact := range f.AppendHistoryTableCoverage() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Table)
		if len(indices) == 0 {
			params.AppendTableCoverage = append(params.AppendTableCoverage, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendTableCoverage = append(parts.AppendTableCoverage, fact)
		})
	}
	for _, fact := range f.AppendElementFieldOrigins() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Source)
		if len(indices) == 0 {
			params.AppendOrigins = append(params.AppendOrigins, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.AppendOrigins = append(parts.AppendOrigins, fact)
		})
	}
	for _, fact := range f.LengthLowerBounds() {
		indices := boundaryPathReturnIndices(fact.Target)
		if len(indices) == 0 {
			params.LengthLower = append(params.LengthLower, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.LengthLower = append(parts.LengthLower, fact)
		})
	}
	for _, fact := range f.LengthUpperBounds() {
		indices := boundaryPathReturnIndices(fact.Target)
		if len(indices) == 0 {
			params.LengthUpper = append(params.LengthUpper, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.LengthUpper = append(parts.LengthUpper, fact)
		})
	}
	for _, fact := range f.LengthRelations() {
		indices := boundaryPathReturnIndices(fact.Target, fact.Source)
		if len(indices) == 0 {
			params.LengthRelations = append(params.LengthRelations, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.LengthRelations = append(parts.LengthRelations, fact)
		})
	}
	for _, fact := range f.IndexWrites() {
		paths := []BoundaryPath{fact.Table}
		if fact.HasKeyPath {
			paths = append(paths, fact.KeyPath)
		}
		if fact.HasValuePath {
			paths = append(paths, fact.ValuePath)
		}
		indices := boundaryPathReturnIndices(paths...)
		if len(indices) == 0 {
			params.IndexWrites = append(params.IndexWrites, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.IndexWrites = append(parts.IndexWrites, fact)
		})
	}
	for _, fact := range f.StaticMembers() {
		indices := boundaryPathReturnIndices(fact.Target)
		if len(indices) == 0 {
			params.StaticMembers = append(params.StaticMembers, fact)
			continue
		}
		addReturnFact(indices, func(parts *BoundaryFactParts) {
			parts.StaticMembers = append(parts.StaticMembers, fact)
		})
	}
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
	return BoundaryFacts{
		keyPresence:    intersectBoundaryKeyPresence(a.keyPresence, b.keyPresence),
		keyArrays:      intersectBoundaryKeyArrays(a.keyArrays, b.keyArrays),
		keyArrayValues: intersectBoundaryKeyArrayValues(a.keyArrayValues, b.keyArrayValues, widenPayload),
		appendKeys:     intersectBoundaryAppendKeys(a.appendKeys, b.appendKeys),
		appendBases:    intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
		appendEvents: intersectBoundaryAppendHistoryEventsWithBases(
			a.appendEvents,
			b.appendEvents,
			intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
		),
		appendCoverage: intersectBoundaryAppendHistoryCoverageWithBases(
			a.appendCoverage,
			b.appendCoverage,
			intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
			intersectBoundaryAppendHistoryEventsWithBases(
				a.appendEvents,
				b.appendEvents,
				intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
			),
			widenPayload,
		),
		appendTableCoverage: intersectBoundaryAppendHistoryTableCoverageWithBases(
			a.appendTableCoverage,
			b.appendTableCoverage,
			intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
			widenPayload,
		),
		appendOrigins: intersectBoundaryAppendElementFieldOriginsWithBases(
			a.appendOrigins,
			b.appendOrigins,
			intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases),
		),
		lenLower:      intersectBoundaryLengthLower(a.lenLower, b.lenLower),
		lenUpper:      intersectBoundaryLengthUpper(a.lenUpper, b.lenUpper),
		lenRelations:  intersectBoundaryLengthRelations(a.lenRelations, b.lenRelations),
		indexWrites:   intersectBoundaryIndexWrites(a.indexWrites, b.indexWrites, widenPayload),
		staticMembers: intersectBoundaryStaticMembers(a.staticMembers, b.staticMembers, widenPayload),
	}
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

func compactBoundaryKeyPresence(xs []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyPresenceFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Table) || !validBoundaryPath(fact.Key) {
			continue
		}
		out = append(out, cloneBoundaryKeyPresence(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyPresence)
	return slices.CompactFunc(out, func(a, b BoundaryKeyPresenceFact) bool {
		return compareBoundaryKeyPresence(a, b) == 0
	})
}

func compactBoundaryKeyArrays(xs []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryKeyArray(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArray)
	return slices.CompactFunc(out, func(a, b BoundaryKeyArrayFact) bool {
		return compareBoundaryKeyArray(a, b) == 0
	})
}

func compactBoundaryKeyArrayValues(xs []BoundaryKeyArrayValueFact) []BoundaryKeyArrayValueFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayValueFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryKeyArrayValue(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArrayValue)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryKeyArrayValue(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryKeyArrayValueFact(nil), dst...)
}

func compactBoundaryAppendKeys(xs []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendKeyFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) {
			continue
		}
		if fact.HasTable && !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryAppendKey(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendKey)
	return slices.CompactFunc(out, func(a, b BoundaryAppendKeyFact) bool {
		return compareBoundaryAppendKey(a, b) == 0
	})
}

func compactBoundaryAppendHistoryBases(xs []BoundaryAppendHistoryBaseFact) []BoundaryAppendHistoryBaseFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryBaseFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryBase(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryBase)
	return slices.CompactFunc(out, func(a, b BoundaryAppendHistoryBaseFact) bool {
		return compareBoundaryAppendHistoryBase(a, b) == 0
	})
}

func compactBoundaryAppendHistoryEvents(xs []BoundaryAppendHistoryEventFact) []BoundaryAppendHistoryEventFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryEventFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryEvent(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryEvent)
	return slices.CompactFunc(out, func(a, b BoundaryAppendHistoryEventFact) bool {
		return compareBoundaryAppendHistoryEvent(a, b) == 0
	})
}

func compactBoundaryAppendHistoryCoverage(xs []BoundaryAppendHistoryCoverageFact) []BoundaryAppendHistoryCoverageFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryCoverageFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryCoverage(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryCoverage)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryAppendHistoryCoverage(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryAppendHistoryCoverageFact(nil), dst...)
}

func compactBoundaryAppendHistoryTableCoverage(xs []BoundaryAppendHistoryTableCoverageFact) []BoundaryAppendHistoryTableCoverageFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryTableCoverageFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryTableCoverage(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryTableCoverage)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryAppendHistoryTableCoverage(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryAppendHistoryTableCoverageFact(nil), dst...)
}

func compactBoundaryAppendElementFieldOrigins(xs []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendElementFieldOriginFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || len(fact.Field) == 0 || !validBoundaryPath(fact.Source) {
			continue
		}
		out = append(out, cloneBoundaryAppendElementFieldOrigin(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendElementFieldOrigin)
	return slices.CompactFunc(out, func(a, b BoundaryAppendElementFieldOriginFact) bool {
		return compareBoundaryAppendElementFieldOrigin(a, b) == 0
	})
}

func compactBoundaryLengthLower(xs []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthLowerBound, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Lower <= 0 {
			continue
		}
		out = append(out, cloneBoundaryLengthLower(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthLower)
	return slices.CompactFunc(out, func(a, b BoundaryLengthLowerBound) bool {
		return compareBoundaryLengthLower(a, b) == 0
	})
}

func compactBoundaryLengthUpper(xs []BoundaryLengthUpperBound) []BoundaryLengthUpperBound {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthUpperBound, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Upper < 0 {
			continue
		}
		out = append(out, cloneBoundaryLengthUpper(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthUpper)
	return slices.CompactFunc(out, func(a, b BoundaryLengthUpperBound) bool {
		return compareBoundaryLengthUpper(a, b) == 0
	})
}

func compactBoundaryLengthRelations(xs []BoundaryLengthRelationFact) []BoundaryLengthRelationFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthRelationFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || !validBoundaryPath(fact.Source) {
			continue
		}
		out = append(out, cloneBoundaryLengthRelation(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthRelation)
	return slices.CompactFunc(out, func(a, b BoundaryLengthRelationFact) bool {
		return compareBoundaryLengthRelation(a, b) == 0
	})
}

func compactBoundaryIndexWrites(xs []BoundaryIndexWriteFact) []BoundaryIndexWriteFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryIndexWriteFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryIndexWrite(fact) {
			continue
		}
		out = append(out, cloneBoundaryIndexWrite(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryIndexWrite)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryIndexWrite(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].KeyValue = product.Domain.Join(dst[len(dst)-1].KeyValue, fact.KeyValue)
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryIndexWriteFact(nil), dst...)
}

func compactBoundaryStaticMembers(xs []BoundaryStaticMemberFact) []BoundaryStaticMemberFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryStaticMemberFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryStaticMember(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryStaticMember)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryStaticMember(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryStaticMemberFact(nil), dst...)
}

func validBoundaryPath(p BoundaryPath) bool {
	return (p.Kind == BoundaryPathParam || p.Kind == BoundaryPathReturn) && p.Index >= 0
}

func cloneBoundaryPath(p BoundaryPath) BoundaryPath {
	if len(p.Segments) == 0 {
		p.Segments = nil
		return p
	}
	p.Segments = append([]constraint.Segment(nil), p.Segments...)
	return p
}

func hashBoundaryPath(h uint64, path BoundaryPath) uint64 {
	h = internal.HashCombine(h, uint64(path.Kind))
	h = internal.HashCombine(h, uint64(path.Index+1))
	h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(path.Segments)))
	return h
}

func cloneBoundaryKeyPresence(f BoundaryKeyPresenceFact) BoundaryKeyPresenceFact {
	return BoundaryKeyPresenceFact{
		Table: cloneBoundaryPath(f.Table),
		Key:   cloneBoundaryPath(f.Key),
	}
}

func cloneBoundaryKeyArray(f BoundaryKeyArrayFact) BoundaryKeyArrayFact {
	return BoundaryKeyArrayFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
	}
}

func cloneBoundaryKeyArrayValue(f BoundaryKeyArrayValueFact) BoundaryKeyArrayValueFact {
	return BoundaryKeyArrayValueFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendKey(f BoundaryAppendKeyFact) BoundaryAppendKeyFact {
	return BoundaryAppendKeyFact{
		Array:    cloneBoundaryPath(f.Array),
		Key:      cloneBoundaryPath(f.Key),
		Table:    cloneBoundaryPath(f.Table),
		HasTable: f.HasTable,
	}
}

func cloneBoundaryAppendHistoryBase(f BoundaryAppendHistoryBaseFact) BoundaryAppendHistoryBaseFact {
	return BoundaryAppendHistoryBaseFact{Array: cloneBoundaryPath(f.Array)}
}

func cloneBoundaryAppendHistoryEvent(f BoundaryAppendHistoryEventFact) BoundaryAppendHistoryEventFact {
	return BoundaryAppendHistoryEventFact{
		Array: cloneBoundaryPath(f.Array),
		Key:   cloneBoundaryPath(f.Key),
	}
}

func cloneBoundaryAppendHistoryCoverage(f BoundaryAppendHistoryCoverageFact) BoundaryAppendHistoryCoverageFact {
	return BoundaryAppendHistoryCoverageFact{
		Array: cloneBoundaryPath(f.Array),
		Key:   cloneBoundaryPath(f.Key),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendHistoryTableCoverage(f BoundaryAppendHistoryTableCoverageFact) BoundaryAppendHistoryTableCoverageFact {
	return BoundaryAppendHistoryTableCoverageFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendElementFieldOrigin(f BoundaryAppendElementFieldOriginFact) BoundaryAppendElementFieldOriginFact {
	return BoundaryAppendElementFieldOriginFact{
		Array:       cloneBoundaryPath(f.Array),
		Field:       append([]constraint.Segment(nil), f.Field...),
		Source:      cloneBoundaryPath(f.Source),
		SourceField: append([]constraint.Segment(nil), f.SourceField...),
	}
}

func cloneBoundaryLengthLower(f BoundaryLengthLowerBound) BoundaryLengthLowerBound {
	return BoundaryLengthLowerBound{
		Target: cloneBoundaryPath(f.Target),
		Lower:  f.Lower,
	}
}

func cloneBoundaryLengthUpper(f BoundaryLengthUpperBound) BoundaryLengthUpperBound {
	return BoundaryLengthUpperBound{
		Target: cloneBoundaryPath(f.Target),
		Upper:  f.Upper,
	}
}

func cloneBoundaryLengthRelation(f BoundaryLengthRelationFact) BoundaryLengthRelationFact {
	return BoundaryLengthRelationFact{
		Target: cloneBoundaryPath(f.Target),
		Source: cloneBoundaryPath(f.Source),
	}
}

func cloneBoundaryIndexWrite(f BoundaryIndexWriteFact) BoundaryIndexWriteFact {
	return BoundaryIndexWriteFact{
		Table:        cloneBoundaryPath(f.Table),
		KeyPath:      cloneBoundaryPath(f.KeyPath),
		HasKeyPath:   f.HasKeyPath,
		KeyValue:     f.KeyValue,
		ValuePath:    cloneBoundaryPath(f.ValuePath),
		HasValuePath: f.HasValuePath,
		Value:        f.Value,
	}
}

func cloneBoundaryStaticMember(f BoundaryStaticMemberFact) BoundaryStaticMemberFact {
	return BoundaryStaticMemberFact{
		Target: cloneBoundaryPath(f.Target),
		Value:  f.Value,
	}
}

func compareBoundaryKeyPresence(a, b BoundaryKeyPresenceFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryKeyArray(a, b BoundaryKeyArrayFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryKeyArrayValue(a, b BoundaryKeyArrayValueFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendKey(a, b BoundaryAppendKeyFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Key, b.Key); c != 0 {
		return c
	}
	if a.HasTable != b.HasTable {
		if !a.HasTable {
			return -1
		}
		return 1
	}
	if !a.HasTable {
		return 0
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendHistoryBase(a, b BoundaryAppendHistoryBaseFact) int {
	return compareBoundaryPath(a.Array, b.Array)
}

func compareBoundaryAppendHistoryEvent(a, b BoundaryAppendHistoryEventFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryAppendHistoryCoverage(a, b BoundaryAppendHistoryCoverageFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Key, b.Key); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendHistoryTableCoverage(a, b BoundaryAppendHistoryTableCoverageFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendElementFieldOrigin(a, b BoundaryAppendElementFieldOriginFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareConstraintSegments(a.Field, b.Field); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Source, b.Source); c != 0 {
		return c
	}
	return compareConstraintSegments(a.SourceField, b.SourceField)
}

func compareBoundaryLengthLower(a, b BoundaryLengthLowerBound) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return cmp.Compare(a.Lower, b.Lower)
}

func compareBoundaryLengthUpper(a, b BoundaryLengthUpperBound) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return cmp.Compare(a.Upper, b.Upper)
}

func compareBoundaryLengthRelation(a, b BoundaryLengthRelationFact) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Source, b.Source)
}

func compareBoundaryIndexWrite(a, b BoundaryIndexWriteFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	if c := compareBoundaryIndexKey(a, b); c != 0 {
		return c
	}
	if c := compareBoundaryBool(a.HasValuePath, b.HasValuePath); c != 0 {
		return c
	}
	if a.HasValuePath {
		return compareBoundaryPath(a.ValuePath, b.ValuePath)
	}
	return 0
}

func compareBoundaryStaticMember(a, b BoundaryStaticMemberFact) int {
	return compareBoundaryPath(a.Target, b.Target)
}

func compareBoundaryPath(a, b BoundaryPath) int {
	if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Index, b.Index); c != 0 {
		return c
	}
	return compareConstraintSegments(a.Segments, b.Segments)
}

func compareBoundaryIndexKey(a, b BoundaryIndexWriteFact) int {
	if c := compareBoundaryBool(a.HasKeyPath, b.HasKeyPath); c != 0 {
		return c
	}
	if a.HasKeyPath {
		return compareBoundaryPath(a.KeyPath, b.KeyPath)
	}
	if c := cmp.Compare(a.KeyValue.Hash(), b.KeyValue.Hash()); c != 0 {
		return c
	}
	if product.Domain.Equal(a.KeyValue, b.KeyValue) {
		return 0
	}
	return cmp.Compare(product.ProjectValueOrUnknown(a.KeyValue).String(), product.ProjectValueOrUnknown(b.KeyValue).String())
}

func compareBoundaryBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}

func hashBoundaryIndexKey(h uint64, fact BoundaryIndexWriteFact) uint64 {
	if fact.HasKeyPath {
		h = internal.HashCombine(h, 1)
		return hashBoundaryPath(h, fact.KeyPath)
	}
	h = internal.HashCombine(h, 0)
	return internal.HashCombine(h, fact.KeyValue.Hash())
}

func validBoundaryIndexWrite(fact BoundaryIndexWriteFact) bool {
	if !validBoundaryPath(fact.Table) || fact.KeyValue.IsZero() || fact.Value.IsZero() {
		return false
	}
	if fact.HasKeyPath && !validBoundaryPath(fact.KeyPath) {
		return false
	}
	if fact.HasValuePath && !validBoundaryPath(fact.ValuePath) {
		return false
	}
	return true
}

var (
	boundaryKeyPresenceRowIdentity = orderedRowIdentity[BoundaryKeyPresenceFact]{
		less: func(a, b BoundaryKeyPresenceFact) bool { return compareBoundaryKeyPresence(a, b) < 0 },
		same: func(a, b BoundaryKeyPresenceFact) bool {
			return compareBoundaryKeyPresence(a, b) == 0
		},
	}
	boundaryKeyArrayRowIdentity = orderedRowIdentity[BoundaryKeyArrayFact]{
		less: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) == 0 },
	}
	boundaryKeyArrayValueRowIdentity = orderedRowIdentity[BoundaryKeyArrayValueFact]{
		less: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) == 0 },
	}
	boundaryAppendKeyRowIdentity = orderedRowIdentity[BoundaryAppendKeyFact]{
		less: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) < 0 },
		same: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) == 0 },
	}
	boundaryAppendHistoryBaseRowIdentity = orderedRowIdentity[BoundaryAppendHistoryBaseFact]{
		less: func(a, b BoundaryAppendHistoryBaseFact) bool {
			return compareBoundaryAppendHistoryBase(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryBaseFact) bool {
			return compareBoundaryAppendHistoryBase(a, b) == 0
		},
	}
	boundaryAppendHistoryEventRowIdentity = orderedRowIdentity[BoundaryAppendHistoryEventFact]{
		less: func(a, b BoundaryAppendHistoryEventFact) bool {
			return compareBoundaryAppendHistoryEvent(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryEventFact) bool {
			return compareBoundaryAppendHistoryEvent(a, b) == 0
		},
	}
	boundaryAppendHistoryCoverageRowIdentity = orderedRowIdentity[BoundaryAppendHistoryCoverageFact]{
		less: func(a, b BoundaryAppendHistoryCoverageFact) bool {
			return compareBoundaryAppendHistoryCoverage(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryCoverageFact) bool {
			return compareBoundaryAppendHistoryCoverage(a, b) == 0
		},
	}
	boundaryAppendHistoryTableCoverageRowIdentity = orderedRowIdentity[BoundaryAppendHistoryTableCoverageFact]{
		less: func(a, b BoundaryAppendHistoryTableCoverageFact) bool {
			return compareBoundaryAppendHistoryTableCoverage(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryTableCoverageFact) bool {
			return compareBoundaryAppendHistoryTableCoverage(a, b) == 0
		},
	}
	boundaryAppendElementFieldOriginRowIdentity = orderedRowIdentity[BoundaryAppendElementFieldOriginFact]{
		less: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) < 0
		},
		same: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) == 0
		},
	}
	boundaryLengthLowerRowIdentity = orderedRowIdentity[BoundaryLengthLowerBound]{
		less: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) < 0 },
		same: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) == 0 },
	}
	boundaryLengthUpperRowIdentity = orderedRowIdentity[BoundaryLengthUpperBound]{
		less: func(a, b BoundaryLengthUpperBound) bool { return compareBoundaryLengthUpper(a, b) < 0 },
		same: func(a, b BoundaryLengthUpperBound) bool { return compareBoundaryLengthUpper(a, b) == 0 },
	}
	boundaryLengthRelationRowIdentity = orderedRowIdentity[BoundaryLengthRelationFact]{
		less: func(a, b BoundaryLengthRelationFact) bool { return compareBoundaryLengthRelation(a, b) < 0 },
		same: func(a, b BoundaryLengthRelationFact) bool { return compareBoundaryLengthRelation(a, b) == 0 },
	}
	boundaryIndexWriteRowIdentity = orderedRowIdentity[BoundaryIndexWriteFact]{
		less: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) < 0 },
		same: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) == 0 },
	}
	boundaryStaticMemberRowIdentity = orderedRowIdentity[BoundaryStaticMemberFact]{
		less: func(a, b BoundaryStaticMemberFact) bool { return compareBoundaryStaticMember(a, b) < 0 },
		same: func(a, b BoundaryStaticMemberFact) bool { return compareBoundaryStaticMember(a, b) == 0 },
	}
)

func boundaryKeyPresenceContainAll(have, want []BoundaryKeyPresenceFact) bool {
	return boundaryKeyPresenceRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArraysContainAll(have, want []BoundaryKeyArrayFact) bool {
	return boundaryKeyArrayRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArrayValuesContainAll(have, want []BoundaryKeyArrayValueFact) bool {
	return boundaryKeyArrayValueRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryKeyArrayValueFact) bool {
		return compareBoundaryKeyArrayValue(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendKeysContainAll(have, want []BoundaryAppendKeyFact) bool {
	return boundaryAppendKeyRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryBasesContainAll(have, want []BoundaryAppendHistoryBaseFact) bool {
	return boundaryAppendHistoryBaseRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryEventsContainAll(have, want []BoundaryAppendHistoryEventFact) bool {
	return boundaryAppendHistoryEventRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryCoverageContainAll(have, want []BoundaryAppendHistoryCoverageFact) bool {
	return boundaryAppendHistoryCoverageRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryAppendHistoryCoverageFact) bool {
		return compareBoundaryAppendHistoryCoverage(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendHistoryTableCoverageContainAll(have, want []BoundaryAppendHistoryTableCoverageFact) bool {
	return boundaryAppendHistoryTableCoverageRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryAppendHistoryTableCoverageFact) bool {
		return compareBoundaryAppendHistoryTableCoverage(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendElementFieldOriginsContainAll(have, want []BoundaryAppendElementFieldOriginFact) bool {
	return boundaryAppendElementFieldOriginRowIdentity.ContainsAll(have, want)
}

func boundaryLengthLowerContainAll(have, want []BoundaryLengthLowerBound) bool {
	return boundaryLengthLowerRowIdentity.ContainsAll(have, want)
}

func boundaryLengthUpperContainAll(have, want []BoundaryLengthUpperBound) bool {
	return boundaryLengthUpperRowIdentity.ContainsAll(have, want)
}

func boundaryLengthRelationContainAll(have, want []BoundaryLengthRelationFact) bool {
	return boundaryLengthRelationRowIdentity.ContainsAll(have, want)
}

func boundaryIndexWritesContainAll(have, want []BoundaryIndexWriteFact) bool {
	return boundaryIndexWriteRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryIndexWriteFact) bool {
		return compareBoundaryIndexWrite(have, want) == 0 &&
			product.Domain.LessOrEq(have.KeyValue, want.KeyValue) &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryStaticMembersContainAll(have, want []BoundaryStaticMemberFact) bool {
	return boundaryStaticMemberRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryStaticMemberFact) bool {
		return compareBoundaryStaticMember(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func intersectBoundaryKeyPresence(a, b []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	return boundaryKeyPresenceRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyPresenceFact) (BoundaryKeyPresenceFact, bool) {
		return cloneBoundaryKeyPresence(left), true
	})
}

func intersectBoundaryKeyArrays(a, b []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	return boundaryKeyArrayRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyArrayFact) (BoundaryKeyArrayFact, bool) {
		return cloneBoundaryKeyArray(left), true
	})
}

func intersectBoundaryKeyArrayValues(a, b []BoundaryKeyArrayValueFact, widenPayload bool) []BoundaryKeyArrayValueFact {
	out := boundaryKeyArrayValueRowIdentity.MergeIntersect(a, b, func(left, right BoundaryKeyArrayValueFact) (BoundaryKeyArrayValueFact, bool) {
		fact := cloneBoundaryKeyArrayValue(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryKeyArrayValues(out)
}

func intersectBoundaryAppendKeys(a, b []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	return boundaryAppendKeyRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendKeyFact) (BoundaryAppendKeyFact, bool) {
		return cloneBoundaryAppendKey(left), true
	})
}

func intersectBoundaryAppendHistoryBases(a, b []BoundaryAppendHistoryBaseFact) []BoundaryAppendHistoryBaseFact {
	return boundaryAppendHistoryBaseRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendHistoryBaseFact) (BoundaryAppendHistoryBaseFact, bool) {
		return cloneBoundaryAppendHistoryBase(left), true
	})
}

func intersectBoundaryAppendHistoryEventsWithBases(
	a, b []BoundaryAppendHistoryEventFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryEventFact {
	out := intersectBoundaryAppendHistoryEvents(a, b)
	out = append(out, boundaryAppendHistoryEventsCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendHistoryEventsCoveredByBases(b, bases)...)
	return compactBoundaryAppendHistoryEvents(out)
}

func intersectBoundaryAppendHistoryEvents(a, b []BoundaryAppendHistoryEventFact) []BoundaryAppendHistoryEventFact {
	return boundaryAppendHistoryEventRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendHistoryEventFact) (BoundaryAppendHistoryEventFact, bool) {
		return cloneBoundaryAppendHistoryEvent(left), true
	})
}

func boundaryAppendHistoryEventsCoveredByBases(
	events []BoundaryAppendHistoryEventFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryEventFact {
	if len(events) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryEventFact
	for _, event := range events {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: event.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryEvent(event))
	}
	return out
}

func intersectBoundaryAppendHistoryCoverageWithBases(
	a, b []BoundaryAppendHistoryCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	events []BoundaryAppendHistoryEventFact,
	widenPayload bool,
) []BoundaryAppendHistoryCoverageFact {
	out := intersectBoundaryAppendHistoryCoverage(a, b, widenPayload)
	out = append(out, boundaryAppendHistoryCoverageCoveredByBases(a, bases, events)...)
	out = append(out, boundaryAppendHistoryCoverageCoveredByBases(b, bases, events)...)
	return compactBoundaryAppendHistoryCoverage(out)
}

func intersectBoundaryAppendHistoryCoverage(
	a, b []BoundaryAppendHistoryCoverageFact,
	widenPayload bool,
) []BoundaryAppendHistoryCoverageFact {
	out := boundaryAppendHistoryCoverageRowIdentity.MergeIntersect(a, b, func(left, right BoundaryAppendHistoryCoverageFact) (BoundaryAppendHistoryCoverageFact, bool) {
		fact := cloneBoundaryAppendHistoryCoverage(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryAppendHistoryCoverage(out)
}

func boundaryAppendHistoryCoverageCoveredByBases(
	coverage []BoundaryAppendHistoryCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	events []BoundaryAppendHistoryEventFact,
) []BoundaryAppendHistoryCoverageFact {
	if len(coverage) == 0 || len(bases) == 0 || len(events) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryCoverageFact
	for _, fact := range coverage {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: fact.Array}); !ok {
			continue
		}
		if _, ok := boundaryAppendHistoryEventRowIdentity.Find(events, BoundaryAppendHistoryEventFact{Array: fact.Array, Key: fact.Key}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryCoverage(fact))
	}
	return out
}

func intersectBoundaryAppendHistoryTableCoverageWithBases(
	a, b []BoundaryAppendHistoryTableCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	widenPayload bool,
) []BoundaryAppendHistoryTableCoverageFact {
	out := intersectBoundaryAppendHistoryTableCoverage(a, b, widenPayload)
	out = append(out, boundaryAppendHistoryTableCoverageCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendHistoryTableCoverageCoveredByBases(b, bases)...)
	return compactBoundaryAppendHistoryTableCoverage(out)
}

func intersectBoundaryAppendHistoryTableCoverage(
	a, b []BoundaryAppendHistoryTableCoverageFact,
	widenPayload bool,
) []BoundaryAppendHistoryTableCoverageFact {
	out := boundaryAppendHistoryTableCoverageRowIdentity.MergeIntersect(a, b, func(left, right BoundaryAppendHistoryTableCoverageFact) (BoundaryAppendHistoryTableCoverageFact, bool) {
		fact := cloneBoundaryAppendHistoryTableCoverage(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryAppendHistoryTableCoverage(out)
}

func boundaryAppendHistoryTableCoverageCoveredByBases(
	coverage []BoundaryAppendHistoryTableCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryTableCoverageFact {
	if len(coverage) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryTableCoverageFact
	for _, fact := range coverage {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: fact.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryTableCoverage(fact))
	}
	return out
}

func intersectBoundaryAppendElementFieldOriginsWithBases(
	a, b []BoundaryAppendElementFieldOriginFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendElementFieldOriginFact {
	out := intersectBoundaryAppendElementFieldOrigins(a, b)
	out = append(out, boundaryAppendElementFieldOriginsCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendElementFieldOriginsCoveredByBases(b, bases)...)
	return compactBoundaryAppendElementFieldOrigins(out)
}

func intersectBoundaryAppendElementFieldOrigins(a, b []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	return boundaryAppendElementFieldOriginRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendElementFieldOriginFact) (BoundaryAppendElementFieldOriginFact, bool) {
		return cloneBoundaryAppendElementFieldOrigin(left), true
	})
}

func boundaryAppendElementFieldOriginsCoveredByBases(
	origins []BoundaryAppendElementFieldOriginFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendElementFieldOriginFact {
	if len(origins) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendElementFieldOriginFact
	for _, origin := range origins {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: origin.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendElementFieldOrigin(origin))
	}
	return out
}

func intersectBoundaryLengthLower(a, b []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	return boundaryLengthLowerRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthLowerBound) (BoundaryLengthLowerBound, bool) {
		return cloneBoundaryLengthLower(left), true
	})
}

func intersectBoundaryLengthUpper(a, b []BoundaryLengthUpperBound) []BoundaryLengthUpperBound {
	return boundaryLengthUpperRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthUpperBound) (BoundaryLengthUpperBound, bool) {
		return cloneBoundaryLengthUpper(left), true
	})
}

func intersectBoundaryLengthRelations(a, b []BoundaryLengthRelationFact) []BoundaryLengthRelationFact {
	return boundaryLengthRelationRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthRelationFact) (BoundaryLengthRelationFact, bool) {
		return cloneBoundaryLengthRelation(left), true
	})
}

func intersectBoundaryIndexWrites(a, b []BoundaryIndexWriteFact, widenPayload bool) []BoundaryIndexWriteFact {
	out := boundaryIndexWriteRowIdentity.MergeIntersect(a, b, func(left, right BoundaryIndexWriteFact) (BoundaryIndexWriteFact, bool) {
		fact := cloneBoundaryIndexWrite(left)
		if widenPayload {
			fact.KeyValue = product.Domain.Widen(fact.KeyValue, right.KeyValue)
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.KeyValue = product.Domain.Join(fact.KeyValue, right.KeyValue)
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryIndexWrites(out)
}

func intersectBoundaryStaticMembers(a, b []BoundaryStaticMemberFact, widenPayload bool) []BoundaryStaticMemberFact {
	out := boundaryStaticMemberRowIdentity.MergeIntersect(a, b, func(left, right BoundaryStaticMemberFact) (BoundaryStaticMemberFact, bool) {
		fact := cloneBoundaryStaticMember(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryStaticMembers(out)
}
