package sourcevalue

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

const objectLiteralPlanFingerprintSeed uint64 = 0x6f626a6c6974706c // "objlitpl"

// ObjectLiteralPlanValue is one raw source value supplied to an
// ObjectLiteralPlan. Available distinguishes an unresolved source from a
// resolved product Bottom.
type ObjectLiteralPlanValue struct {
	Value     product.Value
	Available bool
}

// ObjectLiteralSourceObservation is the exact quotient of a raw product.Value
// observed by object-literal root construction. Differences outside this
// quotient cannot change constructor semantics and may be merged before a
// guarded n-ary Apply. The zero observation is invalid; unavailable is a valid
// semantic observation.
type ObjectLiteralSourceObservation struct {
	valid         bool
	available     bool
	entryType     typ.Type
	hasEntryType  bool
	untrustedTop  bool
	untrustedType typ.Type
	fingerprint   uint64
}

// ObserveObjectLiteralSourceCached reduces one raw source value through the
// exact proof/runtime-kind/variant-origin and untrusted-top observations used
// by object-literal construction.
func ObserveObjectLiteralSourceCached(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value, available bool) (ObjectLiteralSourceObservation, bool) {
	if reg == nil || available && !product.BelongsToRegistry(reg, value) {
		return ObjectLiteralSourceObservation{}, false
	}
	observation := ObjectLiteralSourceObservation{valid: true, available: available}
	if available {
		observation.entryType, observation.hasEntryType = ObjectLiteralEntryType(reg, typeValues, value)
		observation.untrustedTop = ObjectLiteralEntryHasUntrustedTopOrigin(reg, value)
		if observation.untrustedTop {
			observation.untrustedType = untrustedObjectLiteralEntryType(reg, typeValues, value)
		}
	}
	observation.fingerprint = objectLiteralSourceObservationFingerprint(observation)
	return observation, true
}

// Valid reports whether o was produced by ObserveObjectLiteralSourceCached.
func (o ObjectLiteralSourceObservation) Valid() bool { return o.valid }

// Available reports whether the source resolved on this semantic row.
func (o ObjectLiteralSourceObservation) Available() bool { return o.valid && o.available }

// Clone returns the immutable observation. Type nodes are themselves immutable.
func (o ObjectLiteralSourceObservation) Clone() ObjectLiteralSourceObservation { return o }

// Equal reports exact structural observation equality.
func (o ObjectLiteralSourceObservation) Equal(other ObjectLiteralSourceObservation) bool {
	return o.valid == other.valid && o.available == other.available &&
		o.hasEntryType == other.hasEntryType && o.untrustedTop == other.untrustedTop &&
		typesEqual(o.entryType, other.entryType) && typesEqual(o.untrustedType, other.untrustedType)
}

// Fingerprint returns a deterministic interning-bucket selector. Equal is the
// collision authority.
func (o ObjectLiteralSourceObservation) Fingerprint() uint64 {
	if !o.valid {
		return 0
	}
	return o.fingerprint
}

// ObjectLiteralPlan is the detached immutable constructor program for one Lua
// object literal. Sources are uniqued in first-use order; entries retain source
// order and their exact path namespace. The zero plan is invalid.
type ObjectLiteralPlan struct {
	valid           bool
	expectedType    typ.Type
	hasExpected     bool
	hasExpectedType bool
	identity        identity.ID
	hasIdentity     bool
	sources         []factflow.ValueSource
	entries         []objectLiteralPlanEntry
	fingerprint     uint64
}

type objectLiteralPlanEntry struct {
	path            []typetable.ConstructorKey
	segments        []segment.Segment
	sourceOrdinal   uint32
	expectedType    typ.Type
	hasExpected     bool
	hasExpectedType bool
}

// CompileObjectLiteralPlanCached detaches the immutable semantic inputs needed
// to compose lit. Expected product contracts are reduced through the exact
// proof.ValueType observation and canonically rematerialized, so no
// registry-owned product or caller-owned mutable type graph is retained.
// Malformed constructor paths retain the historical behavior of being ignored.
func CompileObjectLiteralPlanCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView) (ObjectLiteralPlan, bool) {
	if reg == nil {
		return ObjectLiteralPlan{}, false
	}
	plan := ObjectLiteralPlan{valid: true}
	proofs := proof.New(reg, typeValues)
	if expected, ok := lit.Expected(); ok {
		if !product.BelongsToRegistry(reg, expected) {
			return ObjectLiteralPlan{}, false
		}
		plan.hasExpected = true
		if expectedType, typed := proofs.ValueType(expected); typed {
			sealed, sealOK := sealObjectLiteralPlanType(expectedType)
			if !sealOK {
				return ObjectLiteralPlan{}, false
			}
			plan.expectedType, plan.hasExpectedType = sealed, true
		}
	}
	plan.identity, plan.hasIdentity = lit.Identity()
	ordinals := make(map[factflow.ValueSource]uint32, lit.EntryCount())
	valid := true
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		path, ok := constructorPathFromEntry(entry)
		if !ok {
			return true
		}
		source := entry.Source()
		ordinal, seen := ordinals[source]
		if !seen {
			ordinal = uint32(len(plan.sources))
			ordinals[source] = ordinal
			plan.sources = append(plan.sources, source)
		}
		var expectedType typ.Type
		hasExpectedType := false
		_, hasExpected := entry.Expected()
		if expected, ok := entry.Expected(); ok {
			if !product.BelongsToRegistry(reg, expected) {
				valid = false
				return false
			}
			if observed, typed := proofs.ValueType(expected); typed {
				sealed, sealOK := sealObjectLiteralPlanType(observed)
				if !sealOK {
					valid = false
					return false
				}
				expectedType, hasExpectedType = sealed, true
			}
		}
		plan.entries = append(plan.entries, objectLiteralPlanEntry{
			path:            append([]typetable.ConstructorKey(nil), path...),
			segments:        append([]segment.Segment(nil), entry.SuffixSegmentsView()...),
			sourceOrdinal:   ordinal,
			expectedType:    expectedType,
			hasExpected:     hasExpected,
			hasExpectedType: hasExpectedType,
		})
		return true
	})
	if !valid {
		return ObjectLiteralPlan{}, false
	}
	plan.fingerprint, valid = objectLiteralPlanFingerprint(plan)
	if !valid {
		return ObjectLiteralPlan{}, false
	}
	return plan, true
}

// Valid reports whether p was produced by CompileObjectLiteralPlan.
func (p ObjectLiteralPlan) Valid() bool { return p.valid }

// Identity returns the literal's exact lexical identity when the frozen
// constructor declares one. It is producer topology used by symbolic
// coordinate-footprint closure; runtime evaluation does not rediscover it.
func (p ObjectLiteralPlan) Identity() (identity.ID, bool) {
	return p.identity, p.valid && p.hasIdentity && p.identity != (identity.ID{})
}

// Clone returns a detached immutable copy.
func (p ObjectLiteralPlan) Clone() ObjectLiteralPlan {
	if !p.valid {
		return ObjectLiteralPlan{}
	}
	out := p
	out.sources = append([]factflow.ValueSource(nil), p.sources...)
	out.entries = make([]objectLiteralPlanEntry, len(p.entries))
	for i := range p.entries {
		out.entries[i] = p.entries[i]
		out.entries[i].path = append([]typetable.ConstructorKey(nil), p.entries[i].path...)
		out.entries[i].segments = append([]segment.Segment(nil), p.entries[i].segments...)
	}
	return out
}

// Equal reports exact structural equality. It is the authority paired with
// Fingerprint; fingerprints alone never establish equality.
func (p ObjectLiteralPlan) Equal(other ObjectLiteralPlan) bool {
	if p.valid != other.valid {
		return false
	}
	if !p.valid {
		return true
	}
	if p.hasExpected != other.hasExpected || p.hasExpectedType != other.hasExpectedType || !typesEqual(p.expectedType, other.expectedType) ||
		p.identity != other.identity || p.hasIdentity != other.hasIdentity ||
		len(p.sources) != len(other.sources) || len(p.entries) != len(other.entries) {
		return false
	}
	for i := range p.sources {
		if !factflow.ValueSourceEqual(p.sources[i], other.sources[i]) {
			return false
		}
	}
	for i := range p.entries {
		left, right := p.entries[i], other.entries[i]
		if left.sourceOrdinal != right.sourceOrdinal || left.hasExpected != right.hasExpected || left.hasExpectedType != right.hasExpectedType ||
			!typesEqual(left.expectedType, right.expectedType) ||
			len(left.path) != len(right.path) || len(left.segments) != len(right.segments) {
			return false
		}
		for j := range left.path {
			if left.path[j] != right.path[j] {
				return false
			}
		}
		for j := range left.segments {
			if left.segments[j] != right.segments[j] {
				return false
			}
		}
	}
	return true
}

// Fingerprint returns a deterministic compact interning-bucket selector.
// Equal is always required to resolve collisions.
func (p ObjectLiteralPlan) Fingerprint() uint64 {
	if !p.valid {
		return 0
	}
	return p.fingerprint
}

// ValueSourceCount returns the exact number of unique raw source operands.
func (p ObjectLiteralPlan) ValueSourceCount() int {
	if !p.valid {
		return 0
	}
	return len(p.sources)
}

// ValueSourceAt returns one unique raw source operand by dense ordinal.
func (p ObjectLiteralPlan) ValueSourceAt(index int) (factflow.ValueSource, bool) {
	if !p.valid || index < 0 || index >= len(p.sources) {
		return factflow.ValueSource{}, false
	}
	return p.sources[index], true
}

// ComposeObjectLiteralPlanCached executes the canonical object-literal law
// over one correlated row of raw source values. Callers with guarded operands
// lift this pure operation pointwise over their decision algebra.
func ComposeObjectLiteralPlanCached(reg *axis.Registry, typeValues *typevalue.Cache, plan ObjectLiteralPlan, values []ObjectLiteralPlanValue) (product.Value, bool) {
	if reg == nil || !plan.valid || len(values) != len(plan.sources) {
		return product.Value{}, false
	}
	observations := make([]ObjectLiteralSourceObservation, len(values))
	for i := range values {
		observation, ok := ObserveObjectLiteralSourceCached(reg, typeValues, values[i].Value, values[i].Available)
		if !ok {
			return product.Value{}, false
		}
		observations[i] = observation
	}
	return ComposeObjectLiteralPlanFromObservationsCached(reg, typeValues, plan, observations)
}

// ComposeObjectLiteralPlanFromObservationsCached executes the canonical
// constructor law from exact source observations. Guarded evaluators should
// intern observations first and n-ary Apply this pure operation pointwise.
func ComposeObjectLiteralPlanFromObservationsCached(reg *axis.Registry, typeValues *typevalue.Cache, plan ObjectLiteralPlan, observations []ObjectLiteralSourceObservation) (product.Value, bool) {
	t, ok := composeObjectLiteralPlanTypeFromObservationsCached(reg, typeValues, plan, observations)
	if !ok {
		return product.Value{}, false
	}
	return objectLiteralPlanValueFromTypeCached(reg, typeValues, plan, t), true
}

func composeObjectLiteralPlanTypeFromObservationsCached(reg *axis.Registry, typeValues *typevalue.Cache, plan ObjectLiteralPlan, observations []ObjectLiteralSourceObservation) (typ.Type, bool) {
	if reg == nil || !plan.valid || len(observations) != len(plan.sources) {
		return nil, false
	}
	for i := range observations {
		if !observations[i].valid {
			return nil, false
		}
	}
	builder := typetable.NewConstructorBuilder()
	queries := objectLiteralPlanQueries{
		plan:         plan,
		observations: observations,
		types:        make([]resolvedObjectLiteralType, len(plan.sources)),
		typesKnown:   make([]bool, len(plan.sources)),
	}
	var expectedType typ.Type
	if plan.hasExpectedType {
		expectedType = plan.expectedType
	}
	expected, hasExpected := luatypeprojection.ExpectedObjectLiteralRecordCached(typeValues, expectedType, queries.dotFieldType)
	seen := false
	for i := range plan.entries {
		entry := plan.entries[i]
		observation, observed := queries.observation(entry)
		if !observed || !observation.available {
			if hasExpected {
				if filled, ok := expectedPlanEntryField(expected, entry); ok {
					if !builder.Add(entry.path, filled) {
						return nil, false
					}
					seen = true
					continue
				}
			}
			if entry.hasExpectedType {
				if !builder.Add(entry.path, entry.expectedType) {
					return nil, false
				}
				seen = true
				continue
			}
			if !builder.Add(entry.path, typ.Unknown) {
				return nil, false
			}
			seen = true
			continue
		}
		t, typed := queries.entryType(entry)
		if !typed {
			if observation.untrustedTop {
				if !builder.Add(entry.path, observation.untrustedType) {
					return nil, false
				}
				seen = true
				continue
			}
			if hasExpected {
				if filled, ok := expectedPlanEntryField(expected, entry); ok {
					if !builder.Add(entry.path, filled) {
						return nil, false
					}
					seen = true
					continue
				}
			}
			if entry.hasExpectedType {
				if !builder.Add(entry.path, entry.expectedType) {
					return nil, false
				}
				seen = true
				continue
			}
			if !builder.Add(entry.path, typ.Unknown) {
				return nil, false
			}
			seen = true
			continue
		}
		if hasExpected {
			if adopted, ok := adoptExpectedPlanEntryFieldType(typeValues, expected, entry, t); ok {
				if !builder.AddSealed(entry.path, adopted) {
					return nil, false
				}
				seen = true
				continue
			}
		}
		if !builder.Add(entry.path, t) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		empty := typetable.NewRecord().Build()
		if expectedType != nil && typeValues.IsFreshAssignable(empty, expectedType) {
			return expectedType, true
		}
		return empty, true
	}
	return builder.Build()
}

type objectLiteralPlanQueries struct {
	plan         ObjectLiteralPlan
	observations []ObjectLiteralSourceObservation
	types        []resolvedObjectLiteralType
	typesKnown   []bool
}

type resolvedObjectLiteralType struct {
	t  typ.Type
	ok bool
}

func (q *objectLiteralPlanQueries) observation(entry objectLiteralPlanEntry) (ObjectLiteralSourceObservation, bool) {
	index := int(entry.sourceOrdinal)
	if index < 0 || index >= len(q.observations) || !q.observations[index].valid {
		return ObjectLiteralSourceObservation{}, false
	}
	return q.observations[index], true
}

func (q *objectLiteralPlanQueries) entryType(entry objectLiteralPlanEntry) (typ.Type, bool) {
	index := int(entry.sourceOrdinal)
	if index < 0 || index >= len(q.types) {
		return nil, false
	}
	if q.typesKnown[index] {
		cached := q.types[index]
		return cached.t, cached.ok
	}
	observation, ok := q.observation(entry)
	if ok && observation.available {
		t, typed := observation.entryType, observation.hasEntryType
		if !typed && entry.hasExpectedType {
			t, typed = entry.expectedType, true
		}
		q.types[index] = resolvedObjectLiteralType{t: t, ok: typed}
	} else {
		q.types[index] = resolvedObjectLiteralType{}
	}
	q.typesKnown[index] = true
	cached := q.types[index]
	return cached.t, cached.ok
}

func (q *objectLiteralPlanQueries) dotFieldType(name string) (typ.Type, bool) {
	for i := range q.plan.entries {
		entry := q.plan.entries[i]
		if len(entry.segments) != 1 || entry.segments[0].Kind != segment.SegmentField || entry.segments[0].Name != name {
			continue
		}
		return q.entryType(entry)
	}
	return nil, false
}

func expectedPlanEntryField(rec *typ.Record, entry objectLiteralPlanEntry) (typ.Type, bool) {
	if len(entry.segments) != 1 {
		return nil, false
	}
	return luatypeprojection.ExpectedRecordSegment(rec, entry.segments[0])
}

func adoptExpectedPlanEntryFieldType(typeValues *typevalue.Cache, rec *typ.Record, entry objectLiteralPlanEntry, inferred typ.Type) (typ.Type, bool) {
	if len(entry.segments) != 1 {
		return nil, false
	}
	return luatypeprojection.AdoptExpectedSegmentTypeCached(typeValues, rec, entry.segments[0], inferred)
}

func objectLiteralPlanValueFromTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, plan ObjectLiteralPlan, t typ.Type) product.Value {
	value := typeValues.FromTypeWithWitness(reg, t)
	ed := product.Edit(reg, value)
	if plan.hasIdentity {
		product.EditSet(&ed, identity.Key, identity.Singleton(plan.identity))
	}
	product.EditSet(&ed, escape.Key, escape.Fresh())
	return ed.Done()
}

func resolveObjectLiteralPlanObservations(reg *axis.Registry, typeValues *typevalue.Cache, plan ObjectLiteralPlan, resolver factflow.ValueSourceResolver) ([]ObjectLiteralSourceObservation, bool) {
	observations := make([]ObjectLiteralSourceObservation, len(plan.sources))
	for i, source := range plan.sources {
		var value product.Value
		available := false
		if resolver != nil {
			value, available = resolver.ResolveValueSource(source)
		}
		observation, ok := ObserveObjectLiteralSourceCached(reg, typeValues, value, available)
		if !ok {
			return nil, false
		}
		observations[i] = observation
	}
	return observations, true
}

func objectLiteralSourceObservationFingerprint(observation ObjectLiteralSourceObservation) uint64 {
	h := internalhash.MixHash(objectLiteralPlanFingerprintSeed, 0x6f627365727665) // "observe"
	h = internalhash.MixHash(h, boolHash(observation.available))
	h = internalhash.MixHash(h, boolHash(observation.hasEntryType))
	h = internalhash.MixHash(h, boolHash(observation.untrustedTop))
	if observation.entryType != nil {
		h = internalhash.MixHash(h, typ.EqualityHash(observation.entryType))
	}
	if observation.untrustedType != nil {
		h = internalhash.MixHash(h, typ.EqualityHash(observation.untrustedType))
	}
	return h
}

func typesEqual(left, right typ.Type) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return typ.TypeEquals(left, right)
}

func sealObjectLiteralPlanType(value typ.Type) (typ.Type, bool) {
	if value == nil {
		return nil, false
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return nil, false
	}
	sealed, err := typ.DecodeCanonical(context.Background(), encoded)
	if errors.Is(err, typ.ErrCanonicalRecursiveIdentityUnavailable) {
		// Recursive type witnesses carry declaration identity in addition to their
		// coinductive shape. Canonical bytes validate and fingerprint that shape,
		// but deliberately cannot mint a second owner of the declaration identity.
		// Type graphs crossing factflow are immutable after binding, so retain the
		// unique recursive authority instead of either rejecting it or fabricating
		// a parallel identity. Nonrecursive graphs continue to be rematerialized
		// below and therefore own detached aggregate storage.
		return value, true
	}
	if err != nil || !typ.TypeEquals(value, sealed) {
		return nil, false
	}
	return sealed, true
}

func objectLiteralPlanFingerprint(plan ObjectLiteralPlan) (uint64, bool) {
	h := internalhash.MixHash(objectLiteralPlanFingerprintSeed, boolHash(plan.hasExpected))
	h = internalhash.MixHash(h, boolHash(plan.hasExpectedType))
	if plan.hasExpectedType {
		h = internalhash.MixHash(h, typ.EqualityHash(plan.expectedType))
	}
	h = internalhash.MixHash(h, boolHash(plan.hasIdentity))
	if plan.hasIdentity {
		h = internalhash.MixHash(h, internalhash.FnvString(plan.identity.Kind))
		h = internalhash.MixHash(h, internalhash.FnvString(plan.identity.Site))
		h = internalhash.MixHash(h, plan.identity.Index)
	}
	h = internalhash.MixHash(h, uint64(len(plan.sources)))
	for _, source := range plan.sources {
		id, err := factflow.CanonicalValueSourceContentID(context.Background(), source)
		if err != nil {
			return 0, false
		}
		for offset := 0; offset < len(id); offset += 8 {
			h = internalhash.MixHash(h, binary.BigEndian.Uint64(id[offset:offset+8]))
		}
	}
	h = internalhash.MixHash(h, uint64(len(plan.entries)))
	for _, entry := range plan.entries {
		h = internalhash.MixHash(h, uint64(entry.sourceOrdinal))
		h = internalhash.MixHash(h, boolHash(entry.hasExpected))
		h = internalhash.MixHash(h, boolHash(entry.hasExpectedType))
		if entry.hasExpectedType {
			h = internalhash.MixHash(h, typ.EqualityHash(entry.expectedType))
		}
		h = internalhash.MixHash(h, uint64(len(entry.path)))
		for _, key := range entry.path {
			h = internalhash.MixHash(h, uint64(key.Kind))
			h = internalhash.MixHash(h, internalhash.FnvString(key.Name))
			h = internalhash.MixHash(h, uint64(key.Index))
		}
	}
	return h, true
}

func boolHash(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
