// Package role owns Static's closed operand-role vocabulary.
//
// The predicates expose no generic node or relation representation.  They
// validate only a counted canonical Term's family role; row-dependent
// contracts and cross-owner closure remain in Static's owning verticals.
package role

import "github.com/wippyai/go-lua/program/keyspace"

// Node is the closed authored static-type occurrence family set. TypeOf is a
// type node here; the TypeOf-specific exception is only its Operand, which is
// a cross-owner handle and must not be validated with this predicate.
func Node(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	if !validTerm(counts, term) {
		return false
	}
	return NodeFamily(keyspace.TermFamily(term))
}

// NodeFamily is the family-only half of Node. It is used by artifact
// decoders, which can reject a foreign family before the owning Build checks
// its dense ordinal against the injected denominators.
func NodeFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral, keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion, keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray, keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord, keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf, keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional:
		return true
	default:
		return false
	}
}

// ScopeHandle is the exact authored scope-handle family set shared by
// signatures and TypeOf.  Lexical resolution and forwarding-chain closure
// are proven later by Source/Flow and the joint seal.
func ScopeHandle(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && ScopeHandleFamily(keyspace.TermFamily(term))
}

// ScopeHandleFamily is the family-only half of ScopeHandle.
func ScopeHandleFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyCell, keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam, keyspace.FamilyTypeFunction, keyspace.FamilyValueClaim,
		keyspace.FamilyCall, keyspace.FamilyAnnotation, keyspace.FamilyFunction:
		return true
	default:
		return false
	}
}

// TypeReferenceTarget is the binder-produced declaration target set for a
// resolved TypeRef.  A TypeRef itself is not a target and canonical/unresolved
// paths carry no target term.
func TypeReferenceTarget(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && TypeReferenceTargetFamily(keyspace.TermFamily(term))
}

// TypeReferenceTargetFamily is the family-only half of
// TypeReferenceTarget.
func TypeReferenceTargetFamily(family keyspace.Family) bool {
	return family == keyspace.FamilyTypeAlias || family == keyspace.FamilyTypeInterface || family == keyspace.FamilyTypeParam
}

// TypeParameterOwner is the complete owner family set for a TypeParam row.
// The exact row-to-owner and one-claim laws remain in Static's declaration,
// signature, and contract seal.
func TypeParameterOwner(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && TypeParameterOwnerFamily(keyspace.TermFamily(term))
}

// TypeParameterOwnerFamily is the family-only half of TypeParameterOwner.
func TypeParameterOwnerFamily(family keyspace.Family) bool {
	return family == keyspace.FamilyTypeAlias || family == keyspace.FamilyTypeFunction || family == keyspace.FamilyFunction
}

// AnnotationTarget is the closed authored target set for Annotation rows:
// any static type occurrence or a declared TypeField. Name/value/containment
// relations remain Annotation-row and joint-seal obligations.
func AnnotationTarget(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return validTerm(counts, term) && AnnotationTargetFamily(keyspace.TermFamily(term))
}

// AnnotationTargetFamily is the family-only half of AnnotationTarget.
func AnnotationTargetFamily(family keyspace.Family) bool {
	return NodeFamily(family) || family == keyspace.FamilyTypeField
}

func validTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		ordinal != 0 && ordinal <= counts[family]
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}
