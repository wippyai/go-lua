package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeSatisfaction is the ambient call-boundary relation used for ordinary
// structural leaves. Call diagnostics pass their memoized call consistency
// relation; tests and domain callers may use the subtype package fallback.
type TypeSatisfaction func(actual, expected typ.Type) bool

// ContractProof supplies path-sensitive proof evidence for ParamContract leaves
// that cannot be represented faithfully by the structural type projection alone.
type ContractProof interface {
	PathSatisfies(segments []constraint.Segment, contract ParamContract, expected typ.Type) bool
	ElementFieldSatisfies(array []constraint.Segment, field []constraint.Segment, contract ParamContract, expected typ.Type) bool
}

// ObligationContract returns the native parameter-contract witness preserved on
// body-summary call obligations.
func ObligationContract(obligation callobligation.Obligation) (ParamContract, bool) {
	return obligationParamContract(obligation)
}

// CallArgObligationSatisfied proves that actual satisfies obligation. Body
// obligations with a ParamContract witness are checked in the obligation domain;
// signature-only obligations use the supplied type relation directly.
func CallArgObligationSatisfied(actual typ.Type, obligation callobligation.Obligation, satisfies TypeSatisfaction) bool {
	if actual == nil || !obligation.Informative() {
		return false
	}
	if contract, ok := ObligationContract(obligation); ok {
		return ContractSatisfiedByType(contract, actual, satisfies)
	}
	return satisfiesType(actual, obligation.Type, satisfies)
}

// ContractSatisfiedByType proves a native parameter contract against a concrete
// caller type without first collapsing capabilities into broad structural union
// projections.
func ContractSatisfiedByType(contract ParamContract, actual typ.Type, satisfies TypeSatisfaction) bool {
	return contractSatisfiedByType(contract, actual, satisfies, make(map[typ.Type]bool))
}

// ContractSatisfiedByTypeOrProof checks a contract against both the structural
// type surface and a path-sensitive proof surface. It preserves ownership of the
// ParamContract algebra in this package while allowing canonical observation to
// answer facts such as append-element field origins.
func ContractSatisfiedByTypeOrProof(contract ParamContract, actual typ.Type, satisfies TypeSatisfaction, proof ContractProof) bool {
	return contractSatisfiedByTypeOrProofAt(contract, actual, satisfies, proof, nil, make(map[typ.Type]bool))
}

func contractSatisfiedByTypeOrProofAt(
	contract ParamContract,
	actual typ.Type,
	satisfies TypeSatisfaction,
	proof ContractProof,
	segments []constraint.Segment,
	seen map[typ.Type]bool,
) bool {
	if contractSatisfiedByType(contract, actual, satisfies, seen) {
		return true
	}
	expected := contract.ProjectValue()
	if proof != nil && proof.PathSatisfies(segments, contract, expected) {
		return true
	}
	if actual == nil {
		actual = typ.Unknown
	}
	if required := contractValueType(contract.value); required != nil && !satisfiesType(actual, required, satisfies) {
		if proof == nil || !proof.PathSatisfies(segments, contract, required) {
			return false
		}
	}
	if !capabilitySetSatisfiedByType(contract.caps, actual) {
		return false
	}
	for name, child := range contract.fields {
		childSegs := appendContractSegment(segments, constraint.Segment{Kind: constraint.SegmentField, Name: name})
		fieldType, _ := contractFieldType(actual, name, seen)
		if !contractSatisfiedByTypeOrProofAt(child, fieldType, satisfies, proof, childSegs, seen) {
			return false
		}
	}
	if contract.element != nil {
		elemType, _ := contractElementType(actual, seen)
		if !contractSatisfiedByTypeOrProofAt(*contract.element, elemType, satisfies, proof, segments, seen) {
			if proof == nil || !elementFieldsSatisfiedByProof(segments, *contract.element, proof, satisfies, seen) {
				return false
			}
		}
	}
	if contract.mapKey != nil || contract.mapValue != nil {
		if !contractMapSatisfiedByType(actual, contract.mapKey, contract.mapValue, satisfies, seen) {
			return false
		}
	}
	return true
}

func elementFieldsSatisfiedByProof(
	array []constraint.Segment,
	contract ParamContract,
	proof ContractProof,
	satisfies TypeSatisfaction,
	seen map[typ.Type]bool,
) bool {
	if proof == nil {
		return false
	}
	for name, child := range contract.fields {
		field := []constraint.Segment{{Kind: constraint.SegmentField, Name: name}}
		if !proof.ElementFieldSatisfies(array, field, child, child.ProjectValue()) {
			return false
		}
	}
	if contract.element != nil || contract.mapKey != nil || contract.mapValue != nil {
		return contractSatisfiedByTypeOrProofAt(contract, typ.Unknown, satisfies, proof, array, seen)
	}
	return len(contract.fields) > 0
}

func appendContractSegment(segments []constraint.Segment, seg constraint.Segment) []constraint.Segment {
	out := make([]constraint.Segment, 0, len(segments)+1)
	out = append(out, segments...)
	out = append(out, seg)
	return out
}

func contractSatisfiedByType(contract ParamContract, actual typ.Type, satisfies TypeSatisfaction, seen map[typ.Type]bool) bool {
	if ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
		return true
	}
	if contract.top {
		return actual != nil && typ.IsNever(actual)
	}
	if actual == nil || typ.IsUnknown(actual) {
		return false
	}
	if seen[actual] {
		return true
	}
	seen[actual] = true
	defer delete(seen, actual)

	if required := contractValueType(contract.value); required != nil && !satisfiesType(actual, required, satisfies) {
		return false
	}
	if !capabilitySetSatisfiedByType(contract.caps, actual) {
		return false
	}
	for name, child := range contract.fields {
		fieldType, ok := contractFieldType(actual, name, seen)
		if !ok || !contractSatisfiedByType(child, fieldType, satisfies, seen) {
			return false
		}
	}
	if contract.element != nil {
		elemType, ok := contractElementType(actual, seen)
		if !ok || !contractSatisfiedByType(*contract.element, elemType, satisfies, seen) {
			return false
		}
	}
	if contract.mapKey != nil || contract.mapValue != nil {
		if !contractMapSatisfiedByType(actual, contract.mapKey, contract.mapValue, satisfies, seen) {
			return false
		}
	}
	return true
}

func satisfiesType(actual, expected typ.Type, satisfies TypeSatisfaction) bool {
	if actual == nil || expected == nil {
		return false
	}
	if satisfies != nil {
		return satisfies(actual, expected)
	}
	return subtype.Consistent(actual, expected)
}

func capabilitySetSatisfiedByType(caps capabilitySet, actual typ.Type) bool {
	if caps == 0 {
		return true
	}
	for _, item := range []struct {
		bit capabilitySet
		cap Capability
	}{
		{capLength, CapabilityLength},
		{capStringable, CapabilityStringable},
		{capOrderable, CapabilityOrderable},
	} {
		if caps&item.bit == 0 {
			continue
		}
		if !capabilityCoversType(actual, item.cap) {
			return false
		}
	}
	return true
}

func contractFieldType(actual typ.Type, name string, seen map[typ.Type]bool) (typ.Type, bool) {
	if actual == nil || name == "" {
		return nil, false
	}
	switch v := typ.UnwrapAnnotated(actual).(type) {
	case *typ.Optional:
		return nil, false
	case *typ.Union:
		fields := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			field, ok := contractFieldType(member, name, seen)
			if !ok {
				return nil, false
			}
			fields = append(fields, field)
		}
		return joinContractMemberTypes(fields), len(fields) > 0
	case *typ.Intersection:
		fields := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			field, ok := contractFieldType(member, name, seen)
			if ok {
				fields = append(fields, field)
			}
		}
		return intersectContractMemberTypes(fields), len(fields) > 0
	case *typ.Alias:
		return contractFieldType(v.Target, name, seen)
	case *typ.Recursive:
		return contractFieldType(v.Body, name, seen)
	case *typ.Record:
		if field := v.GetField(name); field != nil {
			return field.Type, field.Type != nil
		}
		if v.HasMapComponent() && subtype.IsSubtype(typ.LiteralString(name), v.MapKey) {
			return v.MapValue, v.MapValue != nil
		}
		return nil, false
	case *typ.Map:
		if subtype.IsSubtype(typ.LiteralString(name), v.Key) {
			return v.Value, v.Value != nil
		}
		return nil, false
	case *typ.ReadonlyMap:
		if subtype.IsSubtype(typ.LiteralString(name), v.Key) {
			return v.Value, v.Value != nil
		}
		return nil, false
	default:
		return nil, false
	}
}

func contractElementType(actual typ.Type, seen map[typ.Type]bool) (typ.Type, bool) {
	if actual == nil {
		return nil, false
	}
	switch v := typ.UnwrapAnnotated(actual).(type) {
	case *typ.Optional:
		return nil, false
	case *typ.Union:
		elems := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			elem, ok := contractElementType(member, seen)
			if !ok {
				return nil, false
			}
			elems = append(elems, elem)
		}
		return joinContractMemberTypes(elems), len(elems) > 0
	case *typ.Intersection:
		elems := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			elem, ok := contractElementType(member, seen)
			if ok {
				elems = append(elems, elem)
			}
		}
		return intersectContractMemberTypes(elems), len(elems) > 0
	case *typ.Alias:
		return contractElementType(v.Target, seen)
	case *typ.Recursive:
		return contractElementType(v.Body, seen)
	case *typ.Array:
		return v.Element, v.Element != nil
	case *typ.Tuple:
		if len(v.Elements) == 0 {
			return typ.Never, true
		}
		return joinContractMemberTypes(v.Elements), true
	default:
		return nil, false
	}
}

func contractMapSatisfiedByType(
	actual typ.Type,
	keyContract *ParamContract,
	valueContract *ParamContract,
	satisfies TypeSatisfaction,
	seen map[typ.Type]bool,
) bool {
	if actual == nil {
		return false
	}
	switch v := typ.UnwrapAnnotated(actual).(type) {
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !contractMapSatisfiedByType(member, keyContract, valueContract, satisfies, seen) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, member := range v.Members {
			if contractMapSatisfiedByType(member, keyContract, valueContract, satisfies, seen) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return contractMapSatisfiedByType(v.Target, keyContract, valueContract, satisfies, seen)
	case *typ.Recursive:
		return contractMapSatisfiedByType(v.Body, keyContract, valueContract, satisfies, seen)
	case *typ.Map:
		return contractEntrySatisfied(v.Key, v.Value, keyContract, valueContract, satisfies, seen)
	case *typ.ReadonlyMap:
		return contractEntrySatisfied(v.Key, v.Value, keyContract, valueContract, satisfies, seen)
	case *typ.Array:
		return contractEntrySatisfied(typ.Integer, v.Element, keyContract, valueContract, satisfies, seen)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if !contractEntrySatisfied(typ.Integer, elem, keyContract, valueContract, satisfies, seen) {
				return false
			}
		}
		return true
	case *typ.Record:
		return contractRecordEntriesSatisfied(v, keyContract, valueContract, satisfies, seen)
	default:
		return false
	}
}

func contractRecordEntriesSatisfied(
	record *typ.Record,
	keyContract *ParamContract,
	valueContract *ParamContract,
	satisfies TypeSatisfaction,
	seen map[typ.Type]bool,
) bool {
	if record == nil {
		return false
	}
	for _, field := range record.Fields {
		if !contractEntrySatisfied(typ.LiteralString(field.Name), field.Type, keyContract, valueContract, satisfies, seen) {
			return false
		}
	}
	for _, member := range record.StaticMembers {
		key := staticMemberKeyType(member)
		if key == nil || !contractEntrySatisfied(key, member.Type, keyContract, valueContract, satisfies, seen) {
			return false
		}
	}
	if record.HasMapComponent() {
		return contractEntrySatisfied(record.MapKey, record.MapValue, keyContract, valueContract, satisfies, seen)
	}
	return !record.Open
}

func contractEntrySatisfied(
	key typ.Type,
	value typ.Type,
	keyContract *ParamContract,
	valueContract *ParamContract,
	satisfies TypeSatisfaction,
	seen map[typ.Type]bool,
) bool {
	if keyContract != nil && !contractSatisfiedByType(*keyContract, key, satisfies, seen) {
		return false
	}
	if valueContract != nil && !contractSatisfiedByType(*valueContract, value, satisfies, seen) {
		return false
	}
	return true
}

func staticMemberKeyType(member typ.StaticMember) typ.Type {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return typ.LiteralString(member.Name)
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index)
	default:
		return nil
	}
}

func joinContractMemberTypes(members []typ.Type) typ.Type {
	switch len(members) {
	case 0:
		return nil
	case 1:
		return members[0]
	default:
		return typ.NewUnion(members...)
	}
}

func intersectContractMemberTypes(members []typ.Type) typ.Type {
	switch len(members) {
	case 0:
		return nil
	case 1:
		return members[0]
	default:
		return typ.NewIntersection(members...)
	}
}
