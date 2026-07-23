package sourcevalue

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefinement "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/type/indexproof"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ResolveDynamicRead is the sole value-level dynamic-read algebra. Both the
// concrete State adapter and guarded coordinate evaluator obtain evidence from
// ProductDomain's same finite demand binder and call this function; no second
// concrete or symbolic read implementation exists.
func ResolveDynamicRead(request state.DynamicReadQuery, evidence state.DynamicReadEvidence) (product.Value, bool) {
	domain := evidence.Domain()
	reg := domain.Registry()
	keys := request.KeySpace
	typeValues := request.TypeValues
	if !domain.Valid() || keys == nil || reg == nil || !evidence.MatchesQuery(request) ||
		!product.BelongsToRegistry(reg, request.TableValue) || !product.BelongsToRegistry(reg, request.KeyValue) {
		return product.Value{}, false
	}
	if product.Equal(reg, request.TableValue, product.Bottom(reg)) || product.Equal(reg, request.KeyValue, product.Bottom(reg)) {
		if evidence.KeyMembershipProven {
			// A path-backed key-membership theorem proves this read executes and
			// selects an existing value even when the sparse scalar projection has
			// no axis payload for one operand. Bottom here is omitted scalar
			// information, not permission to erase the structural theorem.
			return product.NewWithPresence(reg, product.ShapeTop, presence.Present()), true
		}
		return product.Bottom(reg), true
	}
	table := request.TableValue
	allowUnconstrained := !request.ProjectPath
	ownerVisible := false
	if request.ProjectPath && request.HasOwnerValue {
		if !product.BelongsToRegistry(reg, request.OwnerValue) ||
			product.Equal(reg, product.Meet(reg, request.OwnerValue, table), product.Bottom(reg)) {
			return product.Value{}, false
		}
		ownerVisible = true
	}
	if request.ProjectPath && request.OwnerPath.Kind != keyspace.KindInvalid {
		if owner, ok := evidence.PathValue(request.OwnerPath); ok && !product.Equal(reg, owner, product.Bottom(reg)) {
			if product.Equal(reg, product.Meet(reg, owner, table), product.Bottom(reg)) {
				return product.Value{}, false
			}
			ownerVisible = true
		}
	}
	if request.ProjectPath {
		if request.TablePath.Kind == keyspace.KindInvalid {
			return product.Value{}, false
		}
		projected, pathOK := evidence.PathValue(request.TablePath)
		pathOK = pathOK && !product.Equal(reg, projected, product.Bottom(reg))
		if pathOK {
			table = projected
		} else {
			segments, segmentsOK := keys.SegmentsView(request.TablePath)
			if !segmentsOK {
				return product.Value{}, false
			}
			start := 0
			if evidence.ProjectedSegments != 0 {
				table, start = evidence.ProjectedTable, evidence.ProjectedSegments
			}
			for _, suffix := range segments[start:] {
				keyValue, scalarOK := scalarSegmentValue(reg, suffix)
				if !scalarOK {
					return product.Value{}, false
				}
				var projectedOK bool
				table, projectedOK = runtimeDynamicIndexValue(reg, typeValues, table, keyValue, ownerVisible || start != 0)
				if !projectedOK {
					return product.Value{}, false
				}
				start++
			}
		}
		allowUnconstrained = true
	}
	if request.ProjectPath && !ownerVisible {
		if _, identified := product.Get(reg, request.TableValue, identity.Key).ID(); !identified && request.OwnerPath.Kind != keyspace.KindInvalid {
			allowUnconstrained = false
		}
	}

	// An exact path coordinate is the freshest flow-sensitive authority.
	if request.TablePath.Kind != keyspace.KindInvalid {
		if keySegment, exact := typevalue.ExactScalarKeySegment(reg, typeValues, request.KeyValue); exact {
			if member, ok := keys.AppendPathSegment(request.TablePath, keySegment); ok {
				candidates := []keyspace.Key{member}
				if canonical, canonicalOK := keys.FieldCanonical(member); canonicalOK && canonical != member {
					candidates = append(candidates, canonical)
				}
				for _, candidate := range candidates {
					if value, readable := evidence.PathValue(candidate); readable && !product.Equal(reg, value, product.Bottom(reg)) {
						return finalizeDynamicReadPresence(request, evidence, value), true
					}
				}
			}
		}
	}

	if evidence.HasValue {
		// A finite write fact is evidence for one producer, not a certificate
		// that the selected table/key relation is closed.  Preserve the
		// canonical runtime-index upper bound before using the fact.  In
		// particular, an unconstrained table has Top as its exact read bound;
		// one admitted fact cannot soundly narrow that result.  Typed tables
		// continue below, where their declared index contract bounds and
		// enriches the observed fact.
		if upper, bounded := runtimeDynamicIndexValue(reg, typeValues, table, request.KeyValue, allowUnconstrained); bounded &&
			product.Equal(reg, upper, product.Top()) {
			return finalizeDynamicReadPresence(request, evidence, upper), true
		}
		value := evidence.Value
		if evidence.KeyMembershipProven {
			value = membershipCertifiedDynamicReadValue(reg, typeValues, table, request.KeyValue, value)
		}
		value = WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
		return finalizeDynamicReadPresence(request, evidence, value), true
	}
	if evidence.HasHeapObject {
		if value, resolved := resolveDynamicReadHeapObject(reg, keys, typeValues, table, request.KeyValue, evidence.HeapObject); resolved {
			if evidence.KeyMembershipProven {
				value = membershipCertifiedDynamicReadValue(reg, typeValues, table, request.KeyValue, value)
			}
			return finalizeDynamicReadPresence(request, evidence, value), true
		}
	}
	if value, ok := rangeCertifiedStaticIndexValue(request, evidence, table); ok {
		return finalizeDynamicReadPresence(request, evidence, InheritTopOriginEvidence(reg, value, table)), true
	}

	value, resolved := runtimeDynamicIndexValue(reg, typeValues, table, request.KeyValue, allowUnconstrained)
	if !resolved && typevalue.HasIntegerType(reg, request.KeyValue) {
		integerKey := typevalue.FromType(reg, typ.Integer)
		if typeValues != nil {
			integerKey = typeValues.FromTypeWithWitness(reg, typ.Integer)
		}
		value, resolved = runtimeDynamicIndexValue(reg, typeValues, table, integerKey, allowUnconstrained)
	}
	if !resolved {
		return product.Value{}, false
	}
	if evidence.KeyMembershipProven {
		value = membershipCertifiedDynamicReadValue(reg, typeValues, table, request.KeyValue, value)
	}
	return finalizeDynamicReadPresence(request, evidence, InheritTopOriginEvidence(reg, value, table)), true
}

// membershipCertifiedDynamicReadValue interprets the complete key-membership
// theorem: the selected value exists, while the table's declared index
// relation bounds which value can exist. A stable heap's absent-value result
// is negative shape evidence, so it cannot survive a positive membership
// premise; replace that arm with the declared contract instead of producing
// the contradictory nil-and-present product bottom.
func membershipCertifiedDynamicReadValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	table, key, observed product.Value,
) product.Value {
	contract, hasContract := product.Value{}, false
	if typeValues != nil {
		contract, hasContract = typeValues.RuntimeIndex(reg, table, key)
	} else {
		contract, hasContract = typevalue.RuntimeIndex(reg, table, key)
	}
	if hasContract {
		if product.Equal(reg, observed, product.Bottom(reg)) || typevalue.HasOnlyNilType(reg, observed) {
			observed = contract
		} else {
			observed = valuerefinement.MergeDeclaredContract(reg, observed, contract)
		}
	} else if product.Equal(reg, observed, product.Bottom(reg)) || typevalue.HasOnlyNilType(reg, observed) {
		// Membership proves existence even when no type axis can bound the
		// selected value. The exact abstract result is therefore an otherwise
		// unconstrained present value, not the contradictory nil-and-present
		// product bottom.
		observed = product.Top()
	}
	certified := WithoutNilRuntimeKind(reg, product.WithPresence(reg, observed, presence.Present()))
	if product.Equal(reg, certified, product.Bottom(reg)) {
		return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	}
	return certified
}

func rangeCertifiedStaticIndexValue(request state.DynamicReadQuery, evidence state.DynamicReadEvidence, table product.Value) (product.Value, bool) {
	if !request.HasRange || !evidence.InRangeIndexEvidence() {
		return product.Value{}, false
	}
	constant, ok := request.Range.Shape.Constant()
	if !ok {
		return product.Value{}, false
	}
	reg := evidence.Domain().Registry()
	var tableType typ.Type
	if request.TypeValues != nil {
		tableType, ok = request.TypeValues.TypeOf(reg, table)
	} else {
		tableType, ok = typevalue.TypeOf(reg, table)
	}
	if !ok {
		return product.Value{}, false
	}
	selected, ok := indexproof.StaticIndexTypeUnderLengthFloor(tableType, constant, constant)
	if !ok {
		return product.Value{}, false
	}
	if request.TypeValues != nil {
		return request.TypeValues.FromTypeWithWitness(reg, selected), true
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, selected), selected), true
}

// finalizeDynamicReadPresence is the sole in-range/non-nil refinement law.
// The paired proof and lower bound come from ProductDomain's exact evidence;
// no caller may independently strengthen a resolved dynamic-read value.
func finalizeDynamicReadPresence(request state.DynamicReadQuery, evidence state.DynamicReadEvidence, value product.Value) product.Value {
	if !request.HasRange {
		return value
	}
	if !evidence.InRangeIndexEvidence() {
		return value
	}
	container := request.TableValue
	if request.HasRangeContainer {
		container = request.RangeContainer
	}
	var tableType typ.Type
	var typeOK bool
	if request.TypeValues != nil {
		tableType, typeOK = request.TypeValues.TypeOf(evidence.Domain().Registry(), container)
	} else {
		tableType, typeOK = typevalue.TypeOf(evidence.Domain().Registry(), container)
	}
	if !typeOK {
		return value
	}
	nonNil := typevalue.InRangeIndexExcludesNil(tableType)
	if constant, ok := request.Range.Shape.Constant(); ok {
		nonNil = indexproof.StaticIndexExcludesNilUnderLengthFloor(tableType, constant, constant)
	}
	if !nonNil {
		return value
	}
	reg := evidence.Domain().Registry()
	return WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
}

func resolveDynamicReadHeapObject(reg *axis.Registry, keys *keyspace.KeySpace, typeValues *typevalue.Cache, table, key product.Value, object heapidentity.TableObject) (product.Value, bool) {
	id, identified := product.Get(reg, table, identity.Key).ID()
	rootID, rootIdentified := product.Get(reg, object.Root(), identity.Key).ID()
	if !identified || !rootIdentified || id != rootID || !heapRootValueCompatible(reg, object.Root(), table) || object.IsBottom() {
		return product.Value{}, false
	}
	keySegment, exact := typevalue.ExactScalarKeySegment(reg, typeValues, key)
	if !exact && !object.StableShape() {
		return product.Value{}, false
	}
	if exact {
		suffix, ok := keys.FromRootlessSuffix([]segment.Segment{keySegment})
		if ok {
			if value, found := readStaticMemberWithFieldCanonicalAlias(keys, object, suffix, []segment.Segment{keySegment}); found {
				return value, true
			}
		}
	}
	if object.DynamicIndexFactsTop() {
		return product.Value{}, false
	}
	domain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	if !exact {
		members := object.StaticMembers()
		memberKeys := make([]keyspace.Key, 0, len(members))
		for member := range members {
			memberKeys = append(memberKeys, member)
		}
		sort.Slice(memberKeys, func(i, j int) bool { return keys.Less(memberKeys[i], memberKeys[j]) })
		for _, member := range memberKeys {
			segments, ok := keys.SuffixSegmentsView(member)
			if !ok || len(segments) != 1 {
				continue
			}
			candidate, ok := scalarSegmentValue(reg, segments[0])
			if !ok || domain.Equal(domain.Meet(candidate, key), domain.Bottom()) {
				continue
			}
			value := members[member]
			if !domain.Equal(value, domain.Bottom()) {
				if !found {
					joined, found = value, true
				} else {
					joined = domain.Join(joined, value)
				}
			}
		}
	}
	facts := object.DynamicIndexFacts()
	factKeys := make([]dynamicindex.Key, 0, len(facts))
	for factKey := range facts {
		factKeys = append(factKeys, factKey)
	}
	sort.Slice(factKeys, func(i, j int) bool {
		if factKeys[i].Table != factKeys[j].Table {
			return keys.Less(factKeys[i].Table, factKeys[j].Table)
		}
		return factKeys[i].Site < factKeys[j].Site
	})
	for _, factKey := range factKeys {
		fact := facts[factKey]
		if fact.Admission == dynamicindex.AdmissionRejected || domain.Equal(fact.KeyValue, domain.Bottom()) ||
			domain.Equal(fact.Value, domain.Bottom()) || domain.Equal(domain.Meet(fact.KeyValue, key), domain.Bottom()) {
			continue
		}
		if !found {
			joined, found = fact.Value, true
		} else {
			joined = domain.Join(joined, fact.Value)
		}
	}
	if found {
		if !exact {
			joined = domain.Join(joined, typevalue.Nil(reg))
		}
		return joined, true
	}
	if object.StableShape() {
		return typevalue.Nil(reg), true
	}
	return product.Value{}, false
}
