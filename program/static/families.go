package static

import "github.com/wippyai/go-lua/program/keyspace"

// staticInputRelationFamilies is the closed denominator for every dense
// authored Static input relation. A family not listed here is either owned by
// another Program component or is a sparse cross-owner anchor, and must not
// acquire an accidental Static cardinality rule.
//
// This is an explicit semantic inventory, not a runtime registry or a generic
// IR. matchingCounts consumes these closed lists, so adding a Static family
// requires declaring its sole input relation here before Build can accept it.
var staticInputRelationFamilies = [...]keyspace.Family{
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
	keyspace.FamilyTypeField,
	keyspace.FamilyTypeAlias,
	keyspace.FamilyTypeParam,
	keyspace.FamilyTypeInterface,
	keyspace.FamilyDeclaredType,
	keyspace.FamilyTypeFunction,
	keyspace.FamilyTypeAsserts,
	keyspace.FamilyFunction,
	keyspace.FamilyCall,
	keyspace.FamilyTypePublication,
	keyspace.FamilyTypeValue,
	keyspace.FamilyAnnotation,
	keyspace.FamilyTypeOf,
	keyspace.FamilyTypeKeyOf,
	keyspace.FamilyTypeIndexAccess,
	keyspace.FamilyTypeConditional,
}

// staticSparseInputRelationFamilies contains cross-owner sparse relations.
// Their authored rows are optional, but every row must still fit within the
// external family census supplied by the owning component.
var staticSparseInputRelationFamilies = [...]keyspace.Family{
	keyspace.FamilyValueClaim,
}

func matchingCounts(input Input) bool {
	for _, family := range staticInputRelationFamilies {
		length, ok := staticFamilyInputCount(input, family)
		if !ok || !countEquals(input.Counts[family], length) {
			return false
		}
	}
	for _, family := range staticSparseInputRelationFamilies {
		length, ok := staticFamilyInputCount(input, family)
		if !ok || length < 0 || uint64(length) > uint64(input.Counts[family]) {
			return false
		}
	}
	return true
}

// staticFamilyInputCount is intentionally a closed switch rather than
// reflection. Every case names its typed relation, preserving the owner and
// avoiding a universal node/container vocabulary at this semantic boundary.
func staticFamilyInputCount(input Input, family keyspace.Family) (int, bool) {
	switch family {
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
	case keyspace.FamilyTypeField:
		return len(input.Types.Field), true
	case keyspace.FamilyTypeAlias:
		return len(input.Declarations.Alias), true
	case keyspace.FamilyTypeParam:
		return len(input.Declarations.TypeParam), true
	case keyspace.FamilyTypeInterface:
		return len(input.Declarations.Interface), true
	case keyspace.FamilyDeclaredType:
		return len(input.Declarations.DeclaredType), true
	case keyspace.FamilyTypeFunction:
		return len(input.Signatures.TypeFunction), true
	case keyspace.FamilyTypeAsserts:
		return len(input.Signatures.TypeAsserts), true
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
	case keyspace.FamilyTypeOf:
		return len(input.Operators.TypeOf), true
	case keyspace.FamilyTypeKeyOf:
		return len(input.Operators.KeyOf), true
	case keyspace.FamilyTypeIndexAccess:
		return len(input.Operators.IndexAccess), true
	case keyspace.FamilyTypeConditional:
		return len(input.Operators.Conditional), true
	case keyspace.FamilyValueClaim:
		return len(input.Operands.Claim), true
	default:
		return 0, false
	}
}
