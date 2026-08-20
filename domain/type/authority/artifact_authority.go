package typeauthority

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// ArtifactAuthority is the detached type-graph constructor. It admits only
// canonical Program rows and retains no Link, Artifact, Flow, or authored Term.
// It is intentionally separate from the legacy Link constructor while the
// Static evaluator migration is completed.
//
// The rows form a content-addressed graph. Sealing decomposes that graph into
// strongly connected components once, so every later materialization knows
// which rows lie on a cycle before it builds anything.
type ArtifactAuthority struct {
	programs  []programschema.Program
	component map[identity.ContentID]rowComponent
}

// artifactResolver is transaction-local. Artifact rows are immutable after
// sealing; semantic graphs are rebuilt per Resolve so a caller can never
// mutate a materialized graph and affect a later projection.
//
// Materialization follows the row dependency graph. A row outside every cycle
// has one closed value, built once and shared for the whole resolution. A row
// on a cycle carries a binder positioned at the node the descent entered the
// component through, so its value is only closed for that entry: the values of
// such a component are built inside a scope, and only the entry value leaves
// it. A value that still names an open binder is therefore never handed to an
// unrelated caller.
type artifactResolver struct {
	authority *ArtifactAuthority
	built     map[identity.ContentID]typ.Type
	scope     *componentScope
}

func (a *artifactResolver) programIndex(row programschema.StaticTypeNode) (programschema.Program, int, bool) {
	if a == nil || a.authority == nil {
		return programschema.Program{}, 0, false
	}
	return a.authority.programFor(row.ID())
}

// children is a transient reconstruction of the historical semantic child
// order from the canonical Program families.
func (a *artifactResolver) children(row programschema.StaticTypeNode) ([]identity.ContentID, bool) {
	program, index, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	return canonicalProgramStaticNodeChildren(program, index, row)
}

// componentScope is one descent into a cyclic component. local holds the
// member values of this descent; holes holds the binder installed for each
// member the descent re-enters.
type componentScope struct {
	component int
	active    map[identity.ContentID]bool
	local     map[identity.ContentID]typ.Type
	holes     map[identity.ContentID]*typ.Recursive
	outer     *componentScope
}

func SealPrograms(programs []programschema.Program) (*ArtifactAuthority, error) {
	if len(programs) == 0 {
		return nil, errors.New("typeauthority: no programs")
	}
	locations := make(map[identity.ContentID]canonicalRowLocation)
	for _, program := range programs {
		if !program.Available() {
			return nil, errors.New("typeauthority: unavailable program")
		}
		owner := program.ProgramID
		count, countOK := program.StaticTypeNodeCount()
		if !countOK {
			return nil, errors.New("typeauthority: unavailable program static graph")
		}
		for index := 0; index < count; index++ {
			row, ok := program.StaticTypeNodeAt(index)
			if !ok || !row.Available() || row.Owner() != owner {
				return nil, errors.New("typeauthority: malformed program type row")
			}
			if _, duplicate := locations[row.ID()]; duplicate {
				return nil, errors.New("typeauthority: duplicate program type row")
			}
			locations[row.ID()] = canonicalRowLocation{program: program, index: index}
		}
	}
	components, componentsOK := componentsOfRows(locations)
	if !componentsOK {
		return nil, errors.New("typeauthority: malformed canonical static graph")
	}
	a := &ArtifactAuthority{programs: append([]programschema.Program(nil), programs...), component: components}
	return a, nil
}

func (a *ArtifactAuthority) Row(id identity.ContentID) (programschema.StaticTypeNode, bool) {
	if a == nil {
		return programschema.StaticTypeNode{}, false
	}
	for _, program := range a.programs {
		count, countOK := program.StaticTypeNodeCount()
		if !countOK {
			continue
		}
		for index := 0; index < count; index++ {
			row, ok := program.StaticTypeNodeAt(index)
			if ok && row.ID() == id {
				return row, true
			}
		}
	}
	return programschema.StaticTypeNode{}, false
}

func (a *ArtifactAuthority) programFor(id identity.ContentID) (programschema.Program, int, bool) {
	for _, program := range a.programs {
		count, countOK := program.StaticTypeNodeCount()
		if !countOK {
			continue
		}
		for index := 0; index < count; index++ {
			row, ok := program.StaticTypeNodeAt(index)
			if ok && row.ID() == id {
				return program, index, true
			}
		}
	}
	return programschema.Program{}, 0, false
}
func (a *ArtifactAuthority) Resolve(id identity.ContentID) (typ.Type, bool) {
	if a == nil || !id.Available() {
		return nil, false
	}
	resolver := &artifactResolver{authority: a, built: make(map[identity.ContentID]typ.Type)}
	value, ok := resolver.resolve(id)
	if !ok || value == nil {
		return nil, false
	}
	return value, true
}

func (a *artifactResolver) resolve(id identity.ContentID) (typ.Type, bool) {
	row, ok := a.authority.Row(id)
	if !ok {
		return nil, false
	}
	if row.Kind() == programschema.StaticNodeReference {
		return a.resolveReference(row)
	}
	// A member of the component under descent is materialized by that descent
	// even when an earlier descent already published it: its published value
	// closes on a binder at the earlier entry, and reusing it here would cut
	// the cycle this entry has to close.
	component := a.authority.component[id]
	if a.scope != nil && a.scope.component == component.id {
		return a.member(a.scope, id, row)
	}
	if value, built := a.built[id]; built {
		return value, true
	}
	if !component.cyclic {
		value, ok := a.construct(id, row, nil)
		if !ok {
			return nil, false
		}
		a.built[id] = value
		return value, true
	}
	scope := &componentScope{
		component: component.id,
		active:    make(map[identity.ContentID]bool),
		local:     make(map[identity.ContentID]typ.Type),
		holes:     make(map[identity.ContentID]*typ.Recursive),
		outer:     a.scope,
	}
	a.scope = scope
	value, ok := a.member(scope, id, row)
	a.scope = scope.outer
	if !ok {
		return nil, false
	}
	a.built[id] = value
	return value, true
}

// resolveReference materializes a TypeRef edge. A resolved reference is a
// transparent edge of the type graph: it owns no value of its own and never
// carries a binder, so resolution entered at a reference and resolution
// entered at the declaration it names are one type. An unresolved reference is
// a complete, targetless leaf: Unknown is the Static carrier for missing
// information and stays distinct from Any. A reference chain that closes on
// itself names no declaration and issues no value.
func (a *artifactResolver) resolveReference(row programschema.StaticTypeNode) (typ.Type, bool) {
	var seen map[identity.ContentID]bool
	for {
		children, childrenOK := a.children(row)
		if !childrenOK {
			return nil, false
		}
		unresolved, shapeOK := staticReferenceResolutionShape(staticrefs.Resolution(row.Resolution()), len(children))
		if !shapeOK {
			return nil, false
		}
		if unresolved {
			return typ.Unknown, true
		}
		target := children[0]
		targetOK := target.Available()
		if !targetOK {
			return nil, false
		}
		next, ok := a.authority.Row(target)
		if !ok {
			return nil, false
		}
		if next.Kind() != programschema.StaticNodeReference {
			return a.resolve(target)
		}
		if seen == nil {
			seen = map[identity.ContentID]bool{row.ID(): true}
		}
		if seen[target] {
			return nil, false
		}
		seen[target] = true
		row = next
	}
}

// member materializes one row of the cyclic component currently under descent.
// A member re-entered while it is still under construction installs the binder
// for the cycle it closes; every other member is an ordinary node of the
// entry's graph.
func (a *artifactResolver) member(scope *componentScope, id identity.ContentID, row programschema.StaticTypeNode) (typ.Type, bool) {
	if value, ok := scope.local[id]; ok {
		return value, true
	}
	if scope.active[id] {
		hole, ok := scope.holes[id]
		if !ok {
			hole = typ.NewRecursivePlaceholder(row.Name())
			scope.holes[id] = hole
		}
		return hole, true
	}
	scope.active[id] = true
	value, ok := a.construct(id, row, scope)
	delete(scope.active, id)
	if !ok {
		return nil, false
	}
	if hole, bound := scope.holes[id]; bound {
		// Alias identity is transparent only at the current declaration
		// boundary. Strip this declaration's compatibility wrapper, but keep
		// an inner authored alias (for example A = B; B = A) in the mu body.
		// Flattening the whole chain here loses that presentation and changes
		// the productive mutual graph.
		_, aliasParamCount, aliasSpanOK := row.AliasParameterSpan()
		if row.Kind() == programschema.StaticNodeAlias && aliasSpanOK && aliasParamCount == 0 {
			value = withoutArtifactAliasWrapper(value)
		}
		hole.SetBody(value)
		value = hole
	}
	scope.local[id] = value
	return value, true
}

// construct builds the value of one row from its already-materialized edges.
// scope is the descent this row belongs to, or nil for a row outside every
// cycle; a generic declaration publishes its binder into that scope before
// walking its body, which is the one owner binder a member of the same
// component may observe.
func (a *artifactResolver) construct(id identity.ContentID, row programschema.StaticTypeNode, scope *componentScope) (typ.Type, bool) {
	ok := true
	children, childrenOK := a.children(row)
	child := func(index int) (typ.Type, bool) {
		if !childrenOK || index < 0 || index >= len(children) {
			return nil, false
		}
		return a.resolve(children[index])
	}
	var value typ.Type
	switch row.Kind() {
	case programschema.StaticNodePrimitive:
		value, ok = primitiveKind(row.LiteralKind())
	case programschema.StaticNodeLiteral:
		exact := row.Exact()
		switch exact.Kind {
		case keyspace.LiteralBool:
			value = typ.LiteralBool(exact.Bool)
		case keyspace.LiteralInteger:
			value = typ.LiteralInt(exact.Integer)
		case keyspace.LiteralFloat:
			value = typ.LiteralNumber(math.Float64frombits(exact.FloatBits))
		case keyspace.LiteralString:
			value = typ.LiteralString(exact.String)
		default:
			ok = false
		}
	case programschema.StaticNodeOptional:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			value = typeexpr.Optional(inner)
		}
	case programschema.StaticNodeUnion, programschema.StaticNodeIntersection:
		members := make([]typ.Type, len(children))
		for i := range members {
			members[i], ok = child(i)
			if !ok {
				break
			}
		}
		if ok {
			if row.Kind() == programschema.StaticNodeUnion {
				value = typeexpr.Union(members...)
			} else {
				value = typeexpr.Intersection(members...)
			}
		}
	case programschema.StaticNodeArray:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			if row.Flag() {
				value = typ.NewReadonlyMap(typ.Integer, inner)
			} else {
				value = typ.NewArray(inner)
			}
		}
	case programschema.StaticNodeMap:
		var keyType, valueType typ.Type
		keyType, ok = child(0)
		if ok {
			valueType, ok = child(1)
		}
		if ok {
			if row.Flag() {
				value = typ.NewReadonlyMap(keyType, valueType)
			} else {
				value = typ.NewMap(keyType, valueType)
			}
		}
	case programschema.StaticNodeRecord:
		fields := make([]typ.Field, len(children))
		for index := range fields {
			var fieldType typ.Type
			fieldType, ok = child(index)
			if !ok {
				break
			}
			field, fieldOK := a.recordField(row, index)
			name, nameOK := field.Text(), field.Available()
			optionalOK := fieldOK
			optional := field.Optional()
			if !fieldOK || !nameOK || name == "" || !optionalOK {
				ok = false
				break
			}
			readonly, readonlyOK := field.Readonly(), fieldOK
			if !readonlyOK {
				ok = false
				break
			}
			fields[index] = typ.Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
		}
		if ok {
			value = typ.RebuildRecord(typ.RecordParts{Fields: fields})
		}
	case programschema.StaticNodeAlias:
		var target typ.Type
		_, aliasParamCount, aliasSpanOK := row.AliasParameterSpan()
		if !aliasSpanOK {
			ok = false
			break
		}
		if aliasParamCount != 0 {
			params := make([]*typ.TypeParam, aliasParamCount)
			for index := range params {
				param, paramOK := childAliasParam(a, row, index)
				if !paramOK {
					ok = false
					break
				}
				var paramType *typ.TypeParam
				paramType, paramOK = param.(*typ.TypeParam)
				if !paramOK {
					ok = false
					break
				}
				params[index] = paramType
			}
			if ok {
				generic := typ.NewGeneric(row.Name(), params, nil)
				if scope != nil {
					scope.local[id] = generic
				}
				target, ok = child(0)
				if !ok {
					break
				}
				generic.SetBody(target)
				value = generic
			}
		} else {
			target, ok = child(0)
			if !ok {
				break
			}
			value = typ.NewAlias(row.Name(), target)
		}
	case programschema.StaticNodeGeneric:
		var base typ.Type
		base, ok = child(0)
		if !ok {
			break
		}
		args := make([]typ.Type, 0, len(children)-1)
		for index := 1; index < len(children) && ok; index++ {
			var arg typ.Type
			arg, ok = child(index)
			if ok {
				args = append(args, arg)
			}
		}
		if !ok {
			break
		}
		// An application of a targetless reference is itself complete and
		// targetless: the artifact holds no declaration to bind, and Unknown
		// is the carrier for that missing information. A generic binder is
		// never fabricated to stand in for one.
		if base == typ.Unknown {
			value = typ.Unknown
			break
		}
		generic, genericOK := base.(*typ.Generic)
		if !genericOK {
			ok = false
			break
		}
		value = typ.Instantiate(generic, args...)
	case programschema.StaticNodeInterface:
		if row.SegmentCount() < 2 {
			ok = false
			break
		}
		extends, _ := row.SegmentAt(0)
		members, _ := row.SegmentAt(1)
		requirements := make(map[string]interfaceRequirement)
		membersValue := make([]typ.Type, 0, members+1)
		for index := 0; index < int(extends) && ok; index++ {
			var extended typ.Type
			extended, ok = childInterfaceExtend(a, row, index)
			if ok {
				if !addInterfaceRequirements(extended, requirements, make(map[typ.Type]bool)) {
					ok = false
					break
				}
				membersValue = append(membersValue, extended)
			}
		}
		methods := make([]typ.Method, 0, members)
		fields := make([]typ.Field, 0, members)
		for index := 0; index < int(members) && ok; index++ {
			member, memberOK := a.interfaceMember(row, index)
			memberKind, kindOK := member.KindCode(), member.Available()
			name, nameOK := member.Text(), member.Available()
			if !memberOK || !kindOK || !nameOK {
				ok = false
				break
			}
			memberType, memberTypeOK := childInterfaceMember(a, row, index)
			ok = memberTypeOK
			if ok && memberKind == 1 {
				optional, optionalOK := member.Optional(), member.Available()
				if !optionalOK {
					ok = false
					break
				}
				readonly, readonlyOK := member.Readonly(), member.Available()
				if !readonlyOK {
					ok = false
					break
				}
				if !addInterfaceRequirement(requirements, name, interfaceRequirement{field: true, typ: memberType, optional: optional, readonly: readonly}) {
					ok = false
					break
				}
				fields = append(fields, typ.Field{Name: name, Type: memberType, Optional: optional, Readonly: readonly})
			} else if ok && memberKind == 2 {
				function, functionOK := memberType.(*typ.Function)
				if functionOK {
					if !addInterfaceRequirement(requirements, name, interfaceRequirement{typ: function}) {
						ok = false
						break
					}
					methods = append(methods, typ.Method{Name: name, Type: function})
				} else {
					ok = false
				}
			}
		}
		if ok {
			if len(fields) != 0 {
				membersValue = append(membersValue, typ.RebuildRecord(typ.RecordParts{Fields: fields}))
			}
			membersValue = append(membersValue, typ.NewInterface(row.Name(), methods))
			value = typeexpr.Intersection(membersValue...)
		}
	case programschema.StaticNodeTypeFunction:
		// The return-clause header is omitted from the row when ReturnsKnown is
		// false: the valid shape is [variadic, type params, params]. An
		// authored empty return clause retains its fourth, zero-valued segment.
		segmentCount := row.SegmentCount()
		if (row.ReturnsKnown() && segmentCount != 4) || (!row.ReturnsKnown() && segmentCount != 3) {
			ok = false
			break
		}
		variadicCount, _ := row.SegmentAt(0)
		typeParamCount, _ := row.SegmentAt(1)
		paramCount, _ := row.SegmentAt(2)
		var returnCount uint32
		if row.ReturnsKnown() {
			returnCount, _ = row.SegmentAt(3)
		}
		childIndex := 0
		builder := typ.Func()
		if variadicCount != 0 {
			var variadicType typ.Type
			variadicType, ok = child(childIndex)
			childIndex++
			if ok {
				builder.Variadic(variadicType)
			}
		}
		for index := 0; index < int(typeParamCount) && ok; index++ {
			paramRow, paramOK := a.functionTypeParameter(row, index)
			paramID := paramRow.ChildID()
			if !paramOK || !paramRow.Available() {
				ok = false
				break
			}
			paramType, typeOK := a.resolve(paramID)
			formal, formalOK := paramType.(*typ.TypeParam)
			if !typeOK || !formalOK {
				ok = false
				break
			}
			builder.TypeParamRef(formal)
		}
		for index := 0; index < int(paramCount) && ok; index++ {
			var parameterType typ.Type
			parameterRow, parameterOK := a.functionParameter(row, index)
			parameterID := parameterRow.ChildID()
			if !parameterOK || !parameterRow.Available() {
				ok = false
				break
			}
			parameterType, ok = a.resolve(parameterID)
			name, nameOK := parameterRow.Text(), parameterRow.Available()
			if !nameOK {
				ok = false
				break
			}
			if ok {
				builder.Param(name, parameterType)
			}
		}
		returns := make([]typ.Type, 0, returnCount)
		for index := 0; index < int(returnCount) && ok; index++ {
			var resultType typ.Type
			resultRow, resultOK := a.functionReturn(row, index)
			resultID := resultRow.ChildID()
			if !resultOK || !resultRow.Available() {
				ok = false
				break
			}
			resultType, ok = a.resolve(resultID)
			if ok {
				returns = append(returns, resultType)
			}
		}
		if ok && row.ReturnsKnown() {
			builder.Returns(returns...)
		}
		if ok {
			value = builder.Build()
		}
	case programschema.StaticNodeTypeParam:
		var constraint typ.Type
		if len(children) > 0 {
			constraint, ok = child(0)
		}
		if ok {
			value = typ.NewTypeParam(row.Name(), constraint)
		}
	default:
		ok = false
	}
	if !ok || value == nil {
		return nil, false
	}
	return value, true
}

// staticReferenceResolutionShape is the sole TypeRef edge-cardinality gate
// used by ArtifactAuthority. Keeping resolution and arity in one predicate
// prevents an unknown enum or an extra target edge from being accepted merely
// because ChildAt(0) happens to resolve.
func staticReferenceResolutionShape(resolution staticrefs.Resolution, childCount int) (unresolved bool, ok bool) {
	switch resolution {
	case staticrefs.Unresolved:
		if childCount != 0 {
			return false, false
		}
		return true, true
	case staticrefs.Declaration, staticrefs.CanonicalPath:
		return false, childCount == 1
	default:
		return false, false
	}
}

func withoutArtifactAliasWrapper(value typ.Type) typ.Type {
	if alias, ok := value.(*typ.Alias); ok && alias != nil {
		return alias.Target
	}
	return value
}

func childAliasParam(a *artifactResolver, row programschema.StaticTypeNode, index int) (typ.Type, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := program.StaticTypeNodeAliasParameterFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}
func childInterfaceExtend(a *artifactResolver, row programschema.StaticTypeNode, index int) (typ.Type, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := program.StaticTypeNodeInterfaceExtendFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}
func childInterfaceMember(a *artifactResolver, row programschema.StaticTypeNode, index int) (typ.Type, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := program.StaticTypeNodeInterfaceMemberFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}

func (a *artifactResolver) recordField(row programschema.StaticTypeNode, index int) (programschema.StaticTypeNodeRecordField, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return programschema.StaticTypeNodeRecordField{}, false
	}
	return program.StaticTypeNodeRecordFieldFor(parent, index)
}
func (a *artifactResolver) interfaceMember(row programschema.StaticTypeNode, index int) (programschema.StaticTypeNodeInterfaceMember, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return programschema.StaticTypeNodeInterfaceMember{}, false
	}
	return program.StaticTypeNodeInterfaceMemberFor(parent, index)
}
func (a *artifactResolver) functionTypeParameter(row programschema.StaticTypeNode, index int) (programschema.StaticTypeNodeTypeFunctionTypeParameter, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return programschema.StaticTypeNodeTypeFunctionTypeParameter{}, false
	}
	return program.StaticTypeNodeTypeFunctionTypeParameterFor(parent, index)
}
func (a *artifactResolver) functionParameter(row programschema.StaticTypeNode, index int) (programschema.StaticTypeNodeTypeFunctionParameter, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return programschema.StaticTypeNodeTypeFunctionParameter{}, false
	}
	return program.StaticTypeNodeTypeFunctionParameterFor(parent, index)
}
func (a *artifactResolver) functionReturn(row programschema.StaticTypeNode, index int) (programschema.StaticTypeNodeTypeFunctionReturn, bool) {
	program, parent, ok := a.programIndex(row)
	if !ok {
		return programschema.StaticTypeNodeTypeFunctionReturn{}, false
	}
	return program.StaticTypeNodeTypeFunctionReturnFor(parent, index)
}

func primitiveKind(raw uint8) (typ.Type, bool) {
	var name string
	switch statictypes.PrimitiveKind(raw) {
	case statictypes.PrimitiveNil:
		name = "nil"
	case statictypes.PrimitiveBoolean:
		name = "boolean"
	case statictypes.PrimitiveNumber:
		name = "number"
	case statictypes.PrimitiveInteger:
		name = "integer"
	case statictypes.PrimitiveString:
		name = "string"
	case statictypes.PrimitiveFunction:
		name = "function"
	case statictypes.PrimitiveAny:
		name = "any"
	case statictypes.PrimitiveUnknown:
		name = "unknown"
	case statictypes.PrimitiveNever:
		name = "never"
	case statictypes.PrimitiveSelf:
		name = "self"
	default:
		return nil, false
	}
	return typ.BuiltinPrimitiveType(name)
}
