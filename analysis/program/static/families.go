package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// staticFamilyInventory is the one closed denominator for every canonical
// family Static measures. A family absent here is either owned by another
// Program component or is a sparse cross-owner anchor, and must not acquire an
// accidental Static cardinality rule. This is an explicit semantic inventory,
// not a runtime registry or a generic IR: admitting a new Static family
// requires placing it here and sealing its authored relation in staticCensus.
//
// The order is load-bearing and cuts the inventory into three segments:
//
//	[0, staticTypeFamilyCount)                       the static type forest, in
//	                                                 its stable query order
//	[staticTypeFamilyCount, staticDenseFamilyCount)  the remaining dense
//	                                                 authored relations
//	[staticDenseFamilyCount, len)                    cross-owner sparse anchors
//
// Inside the forest segment the three declaration-owned roots come first, so
// the tail from staticNodeFamilyOffset is exactly the authored static type
// occurrence vocabulary that role.NodeFamily admits.
var staticFamilyInventory = [...]keyspace.Family{
	keyspace.FamilyTypeAlias,
	keyspace.FamilyTypeInterface,
	keyspace.FamilyTypeParam,

	keyspace.FamilyTypePrimitive,
	keyspace.FamilyTypeLiteral,
	keyspace.FamilyTypeOptional,
	keyspace.FamilyTypeUnion,
	keyspace.FamilyTypeIntersection,
	keyspace.FamilyTypeRef,
	keyspace.FamilyTypeGeneric,
	keyspace.FamilyTypeArray,
	keyspace.FamilyTypeMap,
	keyspace.FamilyTypeRecord,
	keyspace.FamilyTypeFunction,
	keyspace.FamilyTypeAsserts,
	keyspace.FamilyTypeOf,
	keyspace.FamilyTypeKeyOf,
	keyspace.FamilyTypeIndexAccess,
	keyspace.FamilyTypeConditional,

	keyspace.FamilyTypeField,
	keyspace.FamilyDeclaredType,
	keyspace.FamilyFunction,
	keyspace.FamilyCall,
	keyspace.FamilyTypePublication,
	keyspace.FamilyTypeValue,
	keyspace.FamilyAnnotation,

	keyspace.FamilyValueClaim,
}

const (
	// staticNodeFamilyOffset skips the three declaration-owned type roots.
	staticNodeFamilyOffset = 3
	// staticTypeFamilyCount ends the static type forest segment.
	staticTypeFamilyCount = 19
	// staticDenseFamilyCount ends the dense authored relation segment. Every
	// family beyond it is sparse within an external owner's census.
	staticDenseFamilyCount = 26
)

// staticTypeFamilies is the complete authored static-type authority: the
// declaration-owned type roots followed by the typed expression relations.
// DeclaredType bindings, fields, annotations, and runtime operands are
// intentionally not static type roots.
//
// Keep the inventory order stable behind this window: it is a derived query
// order and is used to make source-to-term enumeration deterministic. It is
// not part of Static ContentID and does not create another semantic identity.
var staticTypeFamilies = staticFamilyInventory[:staticTypeFamilyCount]

// staticNodeFamilies is the authored static type occurrence window: the forest
// without its three declaration-owned roots. It enumerates, in canonical
// order, the same vocabulary role.NodeFamily admits by family.
var staticNodeFamilies = staticFamilyInventory[staticNodeFamilyOffset:staticTypeFamilyCount]

// staticCensus seals Static's one cardinality column. A dense family's count
// is its authored relation length; a sparse family keeps the owning
// component's external denominator, which its optional rows must fit within.
// Build assigns the column once and it is thereafter the only cardinality
// authority in this package: no later stage recounts a compacted store to
// rediscover a number already sealed here.
func staticCensus(input Input) (census [keyspace.FamilyCount]uint32, ok bool) {
	census = input.Counts
	for _, family := range staticFamilyInventory[:staticDenseFamilyCount] {
		length, known := staticFamilyInputCount(input, family)
		if !known || !countEquals(census[family], length) {
			return [keyspace.FamilyCount]uint32{}, false
		}
	}
	for _, family := range staticFamilyInventory[staticDenseFamilyCount:] {
		length, known := staticFamilyInputCount(input, family)
		if !known || !countWithin(census[family], length) {
			return [keyspace.FamilyCount]uint32{}, false
		}
	}
	return census, true
}

// staticFamilyInputCount is the one family-to-authored-relation mapping. It is
// intentionally a closed switch rather than reflection: every case names its
// typed relation, preserving the owner and avoiding a universal node/container
// vocabulary at this semantic boundary.
func staticFamilyInputCount(input Input, family keyspace.Family) (int, bool) {
	switch family {
	case keyspace.FamilyTypeAlias:
		return len(input.Declarations.Alias), true
	case keyspace.FamilyTypeInterface:
		return len(input.Declarations.Interface), true
	case keyspace.FamilyTypeParam:
		return len(input.Declarations.TypeParam), true
	case keyspace.FamilyTypePrimitive:
		return len(input.Types.Primitive), true
	case keyspace.FamilyTypeLiteral:
		return len(input.Types.Literal), true
	case keyspace.FamilyTypeOptional:
		return len(input.Types.Optional), true
	case keyspace.FamilyTypeUnion:
		return len(input.Types.Union), true
	case keyspace.FamilyTypeIntersection:
		return len(input.Types.Intersection), true
	case keyspace.FamilyTypeRef:
		return len(input.References.TypeRef), true
	case keyspace.FamilyTypeGeneric:
		return len(input.Types.Generic), true
	case keyspace.FamilyTypeArray:
		return len(input.Types.Array), true
	case keyspace.FamilyTypeMap:
		return len(input.Types.Map), true
	case keyspace.FamilyTypeRecord:
		return len(input.Types.Record), true
	case keyspace.FamilyTypeFunction:
		return len(input.Signatures.TypeFunction), true
	case keyspace.FamilyTypeAsserts:
		return len(input.Signatures.TypeAsserts), true
	case keyspace.FamilyTypeOf:
		return len(input.Operators.TypeOf), true
	case keyspace.FamilyTypeKeyOf:
		return len(input.Operators.KeyOf), true
	case keyspace.FamilyTypeIndexAccess:
		return len(input.Operators.IndexAccess), true
	case keyspace.FamilyTypeConditional:
		return len(input.Operators.Conditional), true
	case keyspace.FamilyTypeField:
		return len(input.Types.Field), true
	case keyspace.FamilyDeclaredType:
		return len(input.Declarations.DeclaredType), true
	case keyspace.FamilyFunction:
		return len(input.Contracts.Function), true
	case keyspace.FamilyCall:
		return len(input.Contracts.Call), true
	case keyspace.FamilyTypePublication:
		return len(input.Publications.Type), true
	case keyspace.FamilyTypeValue:
		return len(input.Operands.TypeValue), true
	case keyspace.FamilyAnnotation:
		return len(input.Operands.Annotation), true
	case keyspace.FamilyValueClaim:
		return len(input.Operands.Claim), true
	default:
		return 0, false
	}
}

func countWithin(count uint32, length int) bool {
	return length >= 0 && uint64(length) <= uint64(count)
}

func countEquals(count uint32, length int) bool {
	return length >= 0 && uint64(length) <= uint64(keyspace.MaxTermOrdinal) && uint64(count) == uint64(length)
}
