package containment

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Static structural roles are permanent semantic vocabulary. They describe
// authored relation slots, never Term ordinals or pool offsets.
const structuralRoleImportCall uint32 = 20

// structuralRoleStaticScope labels a Static-owned expression endpoint which
// is forwarded to its lexical Body.  Unlike ordinary expression edges, this
// boundary has no Flow row describing a child slot; retaining the owner-issued
// role here lets downstream structural paths anchor the positionless subtree
// without manufacturing a Source position.
const structuralRoleStaticScope uint32 = 21

const (
	staticRoleOptional uint32 = iota + 32
	staticRoleUnion
	staticRoleIntersection
	staticRoleGenericBase
	staticRoleGenericArgument
	staticRoleArrayElement
	staticRoleMapKey
	staticRoleMapValue
	staticRoleFieldType
	staticRoleRecordField
	staticRoleAliasTarget
	staticRoleTypeParamConstraint
	staticRoleInterfaceExtend
	staticRoleInterfaceField
	staticRoleInterfaceMethod
	staticRoleDeclaredTarget
	staticRoleFunctionParameter
	staticRoleFunctionVariadic
	staticRoleFunctionReturn
	staticRoleAssertionNarrow
	staticRoleContractReturn
	staticRoleCallTypeArgument
	staticRoleKeyOfInner
	staticRoleIndexObject
	staticRoleIndexKey
	staticRoleConditionalCheck
	staticRoleConditionalExtends
	staticRoleConditionalThen
	staticRoleConditionalElse
	staticRoleClaimTarget
	staticRoleTypeValueTarget
	staticRolePublicationTarget
	staticRoleTypeParamOwner
	staticRoleFieldOwner
	staticRoleDeclaredCell
	staticRolePublicationAssign
	staticRoleAnnotationTarget
)

func sealStaticStructuralRoles(result *Result, view static.View, moduleView imports.View) error {
	if result == nil || !result.available() || !view.Available() || view.ContentID() != result.staticID || moduleView.ContentID() != result.moduleID {
		return errors.New("program/flow/containment: Static semantic role owner mismatch")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if len(result.parents[family]) != 0 {
			result.roles[family] = make([]uint64, len(result.parents[family]))
		}
	}
	set := func(parent, child keyspace.Term, role, rank uint32) error {
		if parent == 0 || child == 0 || role == 0 || rank == 0 {
			return errors.New("program/flow/containment: invalid Static semantic role")
		}
		actual, parentOK := result.Parent(child)
		if !parentOK || actual != parent {
			return nil
		}
		family, ordinal, childOK := result.ordinal(child)
		if !childOK {
			return errors.New("program/flow/containment: Static semantic role child is unavailable")
		}
		packed := uint64(role)<<32 | uint64(rank)
		slot := &result.roles[family][ordinal-1]
		if *slot != 0 && *slot != packed {
			return errors.New("program/flow/containment: conflicting Static semantic roles")
		}
		*slot = packed
		return nil
	}
	for ordinal := uint32(1); ordinal <= uint32(moduleView.Count()); ordinal++ {
		child := keyspace.MakeTerm(keyspace.FamilyImport, ordinal)
		row, rowOK := moduleView.Import(child)
		if !rowOK || row.Term != child || set(row.Call, child, structuralRoleImportCall, 1) != nil {
			return errors.New("program/flow/containment: invalid Import semantic role")
		}
	}

	types := view.Types()
	optionals := types.Optionals()
	for index := 0; index < optionals.Count(); index++ {
		parent, ok := optionals.At(index)
		child, rowOK := optionals.Get(parent)
		if !ok || !rowOK || set(parent, child, staticRoleOptional, 1) != nil {
			return errors.New("program/flow/containment: invalid Optional semantic role")
		}
	}
	unions := types.Unions()
	for index := 0; index < unions.Count(); index++ {
		parent, ok := unions.At(index)
		count, rowOK := unions.MemberCount(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Union semantic role")
		}
		for member := 0; member < count; member++ {
			child, childOK := unions.MemberAt(parent, member)
			if !childOK || set(parent, child, staticRoleUnion, uint32(member+1)) != nil {
				return errors.New("program/flow/containment: invalid Union member semantic role")
			}
		}
	}
	intersections := types.Intersections()
	for index := 0; index < intersections.Count(); index++ {
		parent, ok := intersections.At(index)
		count, rowOK := intersections.MemberCount(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Intersection semantic role")
		}
		for member := 0; member < count; member++ {
			child, childOK := intersections.MemberAt(parent, member)
			if !childOK || set(parent, child, staticRoleIntersection, uint32(member+1)) != nil {
				return errors.New("program/flow/containment: invalid Intersection member semantic role")
			}
		}
	}
	generics := types.Generics()
	for index := 0; index < generics.Count(); index++ {
		parent, ok := generics.At(index)
		base, count, rowOK := generics.Get(parent)
		if !ok || !rowOK || set(parent, base, staticRoleGenericBase, 1) != nil {
			return errors.New("program/flow/containment: invalid Generic semantic role")
		}
		for argument := 0; argument < count; argument++ {
			child, childOK := generics.ArgAt(parent, argument)
			if !childOK || set(parent, child, staticRoleGenericArgument, uint32(argument+1)) != nil {
				return errors.New("program/flow/containment: invalid Generic argument semantic role")
			}
		}
	}
	arrays := types.Arrays()
	for index := 0; index < arrays.Count(); index++ {
		parent, ok := arrays.At(index)
		child, _, rowOK := arrays.Get(parent)
		if !ok || !rowOK || set(parent, child, staticRoleArrayElement, 1) != nil {
			return errors.New("program/flow/containment: invalid Array semantic role")
		}
	}
	maps := types.Maps()
	for index := 0; index < maps.Count(); index++ {
		parent, ok := maps.At(index)
		key, value, _, rowOK := maps.Get(parent)
		if !ok || !rowOK || set(parent, key, staticRoleMapKey, 1) != nil || set(parent, value, staticRoleMapValue, 1) != nil {
			return errors.New("program/flow/containment: invalid Map semantic role")
		}
	}
	fields := types.Fields()
	for index := 0; index < fields.Count(); index++ {
		parent, ok := fields.At(index)
		_, child, _, rowOK := fields.Get(parent)
		if !ok || !rowOK || set(parent, child, staticRoleFieldType, 1) != nil {
			return errors.New("program/flow/containment: invalid Field type semantic role")
		}
	}
	records := types.Records()
	for index := 0; index < records.Count(); index++ {
		parent, ok := records.At(index)
		_, count, rowOK := records.Get(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Record semantic role")
		}
		for field := 0; field < count; field++ {
			child, childOK := records.FieldAt(parent, field)
			if !childOK || set(parent, child, staticRoleRecordField, uint32(field+1)) != nil {
				return errors.New("program/flow/containment: invalid Record field semantic role")
			}
		}
	}

	declarations := view.Declarations()
	aliases := declarations.Aliases()
	for index := 0; index < aliases.Count(); index++ {
		parent, ok := aliases.At(index)
		_, child, _, _, rowOK := aliases.Get(parent)
		if !ok || !rowOK || set(parent, child, staticRoleAliasTarget, 1) != nil {
			return errors.New("program/flow/containment: invalid Alias semantic role")
		}
	}
	params := declarations.TypeParams()
	ownerRanks := make(map[keyspace.Term]uint32)
	for index := 0; index < params.Count(); index++ {
		term, ok := params.At(index)
		owner, _, constraint, rowOK := params.Get(term)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid TypeParam semantic role")
		}
		ownerRanks[owner]++
		if set(owner, term, staticRoleTypeParamOwner, ownerRanks[owner]) != nil {
			return errors.New("program/flow/containment: invalid TypeParam owner semantic role")
		}
		if constraint != 0 && set(term, constraint, staticRoleTypeParamConstraint, 1) != nil {
			return errors.New("program/flow/containment: invalid TypeParam constraint semantic role")
		}
	}
	interfaces := declarations.Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		parent, ok := interfaces.At(index)
		if !ok {
			return errors.New("program/flow/containment: invalid Interface semantic role")
		}
		extends, extendsOK := interfaces.ExtendCount(parent)
		if !extendsOK {
			return errors.New("program/flow/containment: invalid Interface extensions")
		}
		for extension := 0; extension < extends; extension++ {
			child, childOK := interfaces.ExtendAt(parent, extension)
			if !childOK || set(parent, child, staticRoleInterfaceExtend, uint32(extension+1)) != nil {
				return errors.New("program/flow/containment: invalid Interface extension semantic role")
			}
		}
		members, membersOK := interfaces.MemberCount(parent)
		if !membersOK {
			return errors.New("program/flow/containment: invalid Interface members")
		}
		fieldRank, methodRank := uint32(0), uint32(0)
		for member := 0; member < members; member++ {
			row, memberOK := interfaces.MemberAt(parent, member)
			if !memberOK {
				return errors.New("program/flow/containment: invalid Interface member")
			}
			switch row.Kind {
			case static.InterfaceField:
				fieldRank++
				if set(parent, row.Field, staticRoleInterfaceField, fieldRank) != nil {
					return errors.New("program/flow/containment: invalid Interface field semantic role")
				}
			case static.InterfaceMethod:
				methodRank++
				if set(parent, row.Signature, staticRoleInterfaceMethod, methodRank) != nil {
					return errors.New("program/flow/containment: invalid Interface method semantic role")
				}
			default:
				return errors.New("program/flow/containment: unknown Interface member role")
			}
		}
	}
	declared := declarations.DeclaredTypes()
	for index := 0; index < declared.Count(); index++ {
		term, ok := declared.At(index)
		cell, target, rowOK := declared.Get(term)
		if !ok || !rowOK || set(term, target, staticRoleDeclaredTarget, 1) != nil || set(cell, term, staticRoleDeclaredCell, 1) != nil {
			return errors.New("program/flow/containment: invalid DeclaredType semantic role")
		}
	}

	functions := view.Signatures().TypeFunctions()
	for index := 0; index < functions.Count(); index++ {
		parent, ok := functions.At(index)
		if !ok {
			return errors.New("program/flow/containment: invalid TypeFunction semantic role")
		}
		count, countOK := functions.ParameterCount(parent)
		if !countOK {
			return errors.New("program/flow/containment: invalid TypeFunction parameters")
		}
		for parameter := 0; parameter < count; parameter++ {
			row, rowOK := functions.ParameterAt(parent, parameter)
			if !rowOK || set(parent, row.Type, staticRoleFunctionParameter, uint32(parameter+1)) != nil {
				return errors.New("program/flow/containment: invalid TypeFunction parameter semantic role")
			}
		}
		_, variadic, _, _, rowOK := functions.Get(parent)
		if !rowOK {
			return errors.New("program/flow/containment: invalid TypeFunction row")
		}
		if variadic != 0 && set(parent, variadic, staticRoleFunctionVariadic, 1) != nil {
			return errors.New("program/flow/containment: invalid TypeFunction variadic semantic role")
		}
		returns, returnsOK := functions.ReturnCount(parent)
		if !returnsOK {
			return errors.New("program/flow/containment: invalid TypeFunction returns")
		}
		for resultIndex := 0; resultIndex < returns; resultIndex++ {
			child, childOK := functions.ReturnAt(parent, resultIndex)
			if !childOK || set(parent, child, staticRoleFunctionReturn, uint32(resultIndex+1)) != nil {
				return errors.New("program/flow/containment: invalid TypeFunction return semantic role")
			}
		}
	}
	assertions := view.Signatures().Assertions()
	for index := 0; index < assertions.Count(); index++ {
		parent, ok := assertions.At(index)
		_, _, _, _, narrow, rowOK := assertions.Get(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Assertion semantic role")
		}
		if narrow != 0 && set(parent, narrow, staticRoleAssertionNarrow, 1) != nil {
			return errors.New("program/flow/containment: invalid Assertion narrow semantic role")
		}
	}

	contracts := view.Contracts()
	functionContracts := contracts.Functions()
	for index := 0; index < functionContracts.Count(); index++ {
		parent, ok := functionContracts.At(index)
		count, rowOK := functionContracts.ReturnCount(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Function contract semantic role")
		}
		for resultIndex := 0; resultIndex < count; resultIndex++ {
			child, childOK := functionContracts.ReturnAt(parent, resultIndex)
			if !childOK || set(parent, child, staticRoleContractReturn, uint32(resultIndex+1)) != nil {
				return errors.New("program/flow/containment: invalid Function contract return semantic role")
			}
		}
	}
	callContracts := contracts.Calls()
	for index := 0; index < callContracts.Count(); index++ {
		parent, ok := callContracts.At(index)
		count, rowOK := callContracts.TypeArgumentCount(parent)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Call contract semantic role")
		}
		for argument := 0; argument < count; argument++ {
			child, childOK := callContracts.TypeArgumentAt(parent, argument)
			if !childOK || set(parent, child, staticRoleCallTypeArgument, uint32(argument+1)) != nil {
				return errors.New("program/flow/containment: invalid Call type argument semantic role")
			}
		}
	}

	operators := view.Operators()
	keyOfs := operators.KeyOfs()
	for index := 0; index < keyOfs.Count(); index++ {
		parent, ok := keyOfs.At(index)
		child, rowOK := keyOfs.Get(parent)
		if !ok || !rowOK || set(parent, child, staticRoleKeyOfInner, 1) != nil {
			return errors.New("program/flow/containment: invalid KeyOf semantic role")
		}
	}
	indexes := operators.IndexAccesses()
	for index := 0; index < indexes.Count(); index++ {
		parent, ok := indexes.At(index)
		object, key, rowOK := indexes.Get(parent)
		if !ok || !rowOK || set(parent, object, staticRoleIndexObject, 1) != nil || set(parent, key, staticRoleIndexKey, 1) != nil {
			return errors.New("program/flow/containment: invalid IndexAccess semantic role")
		}
	}
	conditionals := operators.Conditionals()
	for index := 0; index < conditionals.Count(); index++ {
		parent, ok := conditionals.At(index)
		check, extends, then, otherwise, rowOK := conditionals.Get(parent)
		if !ok || !rowOK || set(parent, check, staticRoleConditionalCheck, 1) != nil || set(parent, extends, staticRoleConditionalExtends, 1) != nil || set(parent, then, staticRoleConditionalThen, 1) != nil || set(parent, otherwise, staticRoleConditionalElse, 1) != nil {
			return errors.New("program/flow/containment: invalid Conditional semantic role")
		}
	}

	operands := view.Operands()
	claims := operands.Claims()
	for index := 0; index < claims.Count(); index++ {
		parent, ok := claims.At(index)
		child, rowOK := claims.Target(parent)
		if !ok || !rowOK || set(parent, child, staticRoleClaimTarget, 1) != nil {
			return errors.New("program/flow/containment: invalid Claim target semantic role")
		}
	}
	typeValues := operands.TypeValues()
	for index := 0; index < typeValues.Count(); index++ {
		parent, ok := typeValues.At(index)
		child, rowOK := typeValues.Target(parent)
		if !ok || !rowOK || set(parent, child, staticRoleTypeValueTarget, 1) != nil {
			return errors.New("program/flow/containment: invalid TypeValue target semantic role")
		}
	}
	annotations := operands.Annotations()
	annotationRanks := make(map[keyspace.Term]uint32)
	for index := 0; index < annotations.Count(); index++ {
		term, ok := annotations.At(index)
		row, rowOK := annotations.Get(term)
		if !ok || !rowOK {
			return errors.New("program/flow/containment: invalid Annotation semantic role")
		}
		annotationRanks[row.Target]++
		if set(row.Target, term, staticRoleAnnotationTarget, annotationRanks[row.Target]) != nil {
			return errors.New("program/flow/containment: invalid Annotation target semantic role")
		}
	}

	publications := view.Publications()
	for index := 0; index < publications.Count(); index++ {
		term, ok := publications.At(index)
		assign, pair, target, rowOK := publications.Get(term)
		if !ok || !rowOK || set(term, target, staticRolePublicationTarget, 1) != nil || pair == ^uint32(0) || set(assign, term, staticRolePublicationAssign, pair+1) != nil {
			return errors.New("program/flow/containment: invalid Publication semantic role")
		}
	}
	return nil
}

// sealEmittedStructuralRoles publishes the semantic labels carried by owner
// emissions after the containment kernel has accepted their canonical Parent
// rows.  The role column already belongs to Result; this pass only fills the
// labels for edges whose owner has supplied one (currently Static scope
// forwarding). It retains no alternate relation or lookup table.
func sealEmittedStructuralRoles(result *Result, edges []kernelEdge) error {
	if result == nil || !result.available() {
		return errors.New("program/flow/containment: semantic role owner is unavailable")
	}
	for _, edge := range edges {
		if edge.role == 0 && edge.rank == 0 {
			continue
		}
		if edge.role == 0 || edge.rank == 0 {
			return errors.New("program/flow/containment: incomplete emitted semantic role")
		}
		parent, ok := result.Parent(edge.child)
		if !ok || parent != edge.parent {
			return errors.New("program/flow/containment: emitted semantic role disagrees with Parent")
		}
		family, ordinal, ok := result.ordinal(edge.child)
		if !ok || uint64(ordinal) > uint64(len(result.roles[family])) {
			return errors.New("program/flow/containment: emitted semantic role child is unavailable")
		}
		packed := uint64(edge.role)<<32 | uint64(edge.rank)
		slot := &result.roles[family][ordinal-1]
		if *slot != 0 && *slot != packed {
			return errors.New("program/flow/containment: conflicting emitted semantic roles")
		}
		*slot = packed
	}
	return nil
}
