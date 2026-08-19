package typeauthority

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// ArtifactAuthority is the detached type-graph constructor. It admits only
// ProgramArtifact rows and retains no Link, Program, Flow, or authored Term.
// It is intentionally separate from the legacy Link constructor while the
// Static evaluator migration is completed.
type ArtifactAuthority struct {
	rows map[identity.ContentID]programartifact.StaticTypeNodeRow
}

// artifactResolver is deliberately transaction-local. Artifact rows are
// immutable after sealing; semantic graphs are rebuilt per Resolve so a caller
// can never mutate a cached graph and affect a later projection.
//
// An entry is installed before descending into its children, but it is not
// given a recursive placeholder up front. A placeholder is created only when
// an active entry is actually reached again. This distinction matters for
// ordinary zero-parameter aliases and interfaces: an unused forward shell
// would turn an acyclic declaration into a spurious mu type.
type artifactResolution struct {
	value    typ.Type
	active   bool
	backedge bool
}

type artifactResolver struct {
	rows    map[identity.ContentID]programartifact.StaticTypeNodeRow
	entries map[identity.ContentID]*artifactResolution
}

func SealArtifacts(artifacts []*programartifact.Artifact) (*ArtifactAuthority, error) {
	if len(artifacts) == 0 {
		return nil, errors.New("typeauthority: no artifacts")
	}
	a := &ArtifactAuthority{rows: make(map[identity.ContentID]programartifact.StaticTypeNodeRow)}
	for _, artifact := range artifacts {
		if artifact == nil || !artifact.Available() {
			return nil, errors.New("typeauthority: unavailable artifact")
		}
		owner := artifact.CompileKey().ProgramID()
		for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
			row, ok := artifact.StaticTypeNodeAt(index)
			if !ok || !row.Available() || row.Owner() != owner {
				return nil, errors.New("typeauthority: malformed artifact type row")
			}
			if _, duplicate := a.rows[row.ID()]; duplicate {
				return nil, errors.New("typeauthority: duplicate artifact type row")
			}
			a.rows[row.ID()] = row
		}
	}
	return a, nil
}

func (a *ArtifactAuthority) Row(id identity.ContentID) (programartifact.StaticTypeNodeRow, bool) {
	if a == nil {
		return programartifact.StaticTypeNodeRow{}, false
	}
	row, ok := a.rows[id]
	return row, ok
}
func (a *ArtifactAuthority) Resolve(id identity.ContentID) (typ.Type, bool) {
	if a == nil || !id.Available() {
		return nil, false
	}
	resolver := &artifactResolver{
		rows:    a.rows,
		entries: make(map[identity.ContentID]*artifactResolution),
	}
	value, ok := resolver.resolve(id)
	if !ok || value == nil {
		return nil, false
	}
	return value, true
}

func (a *artifactResolver) resolve(id identity.ContentID) (typ.Type, bool) {
	row, ok := a.rows[id]
	if !ok {
		return nil, false
	}
	entry := a.entries[id]
	if entry != nil {
		if entry.value != nil {
			if entry.active {
				// A generic alias publishes its Generic shell before walking
				// the body; that is an intentional owner binder, not a mu
				// backedge. Only an installed recursive placeholder records
				// the latter.
				_, isPlaceholder := entry.value.(*typ.Recursive)
				if isPlaceholder {
					entry.backedge = true
				}
			}
			return entry.value, true
		}
		if entry.active {
			return a.closeCycle(row, entry)
		}
	} else {
		entry = &artifactResolution{}
		a.entries[id] = entry
	}
	entry.active = true
	defer func() { entry.active = false }()
	child := func(index int) (typ.Type, bool) {
		childID, ok := row.ChildAt(index)
		if !ok {
			return nil, false
		}
		return a.resolve(childID)
	}
	var value typ.Type
	switch row.Kind() {
	case programartifact.StaticNodePrimitive:
		value, ok = primitiveKind(row.LiteralKind())
	case programartifact.StaticNodeLiteral:
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
	case programartifact.StaticNodeOptional:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			value = typeexpr.Optional(inner)
		}
	case programartifact.StaticNodeUnion, programartifact.StaticNodeIntersection:
		members := make([]typ.Type, row.ChildCount())
		for i := range members {
			members[i], ok = child(i)
			if !ok {
				break
			}
		}
		if ok {
			if row.Kind() == programartifact.StaticNodeUnion {
				value = typeexpr.Union(members...)
			} else {
				value = typeexpr.Intersection(members...)
			}
		}
	case programartifact.StaticNodeArray:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			if row.Flag() {
				value = typ.NewReadonlyMap(typ.Integer, inner)
			} else {
				value = typ.NewArray(inner)
			}
		}
	case programartifact.StaticNodeMap:
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
	case programartifact.StaticNodeRecord:
		fields := make([]typ.Field, row.ChildCount())
		for index := range fields {
			var fieldType typ.Type
			fieldType, ok = child(index)
			if !ok {
				break
			}
			name, nameOK := row.TextAt(index)
			optional, optionalOK := row.OptionalAt(index)
			if !nameOK || name == "" || !optionalOK {
				ok = false
				break
			}
			readonly, readonlyOK := row.FieldReadonlyAt(index)
			if !readonlyOK {
				ok = false
				break
			}
			fields[index] = typ.Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
		}
		if ok {
			value = typ.RebuildRecord(typ.RecordParts{Fields: fields})
		}
	case programartifact.StaticNodeAlias:
		var target typ.Type
		if row.AliasParamCount() != 0 {
			params := make([]*typ.TypeParam, row.AliasParamCount())
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
				entry.value = generic
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
	case programartifact.StaticNodeGeneric:
		var base typ.Type
		base, ok = child(0)
		if !ok {
			break
		}
		args := make([]typ.Type, 0, row.ChildCount()-1)
		for index := 1; index < row.ChildCount() && ok; index++ {
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
	case programartifact.StaticNodeReference:
		// A resolved declaration or canonical reference is an exact graph edge
		// in the artifact. An unresolved reference is instead a complete,
		// targetless leaf: Unknown is the existing Static carrier for missing
		// information and remains distinct from Any. Every resolved form owns
		// exactly one authenticated target child; extra children are not an
		// ignorable extension of that proof.
		unresolved, shapeOK := staticReferenceResolutionShape(
			staticrefs.Resolution(row.Resolution()),
			row.ChildCount(),
		)
		if !shapeOK {
			ok = false
			break
		}
		if unresolved {
			value, ok = typ.Unknown, true
		} else {
			value, ok = child(0)
		}
	case programartifact.StaticNodeInterface:
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
			memberKind, kindOK := row.MemberKindAt(index)
			name, nameOK := row.TextAt(index)
			if !kindOK || !nameOK {
				ok = false
				break
			}
			memberType, memberOK := childInterfaceMember(a, row, index)
			ok = memberOK
			if ok && memberKind == 1 {
				optional, optionalOK := row.OptionalAt(index)
				if !optionalOK {
					ok = false
					break
				}
				readonly, readonlyOK := row.FieldReadonlyAt(index)
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
	case programartifact.StaticNodeTypeFunction:
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
			paramID, paramOK := row.TypeFunctionTypeParamAt(index)
			if !paramOK {
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
			parameterID, parameterOK := row.TypeFunctionParamAt(index)
			if !parameterOK {
				ok = false
				break
			}
			parameterType, ok = a.resolve(parameterID)
			name, nameOK := row.TextAt(index)
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
			resultID, resultOK := row.TypeFunctionReturnAt(index)
			if !resultOK {
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
	case programartifact.StaticNodeTypeParam:
		var constraint typ.Type
		if row.ChildCount() > 0 {
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
	if entry.backedge {
		recursive, recursiveOK := entry.value.(*typ.Recursive)
		if !recursiveOK || recursive == nil {
			return nil, false
		}
		// Alias identity is transparent only at the current declaration
		// boundary. Strip this declaration's compatibility wrapper, but keep
		// an inner authored alias (for example A = B; B = A) in the mu body.
		// Flattening the whole chain here loses that presentation and changes
		// the productive mutual graph.
		if row.Kind() == programartifact.StaticNodeAlias && row.AliasParamCount() == 0 {
			value = withoutArtifactAliasWrapper(value)
		}
		recursive.SetBody(value)
		value = recursive
	}
	entry.value = value
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

// closeCycle binds the fixed point of a cycle. A static coordinate addresses
// one artifact node, so resolution legitimately starts at an interior node of
// a recursive declaration and the binder belongs to the node re-entered: only
// that position recurs. The name carried by that node is the binder's
// presentation and nothing else; a fixed point reached from two entry points
// spells its binder twice and stays one type, because the canonical identity
// of a binder is its body and its edges. A resolved reference is the one
// transparent edge - the declaration it names owns the binder and is still
// under construction, so an instantiation observes its generic instead of an
// opaque placeholder.
func (a *artifactResolver) closeCycle(row programartifact.StaticTypeNodeRow, entry *artifactResolution) (typ.Type, bool) {
	if row.Kind() == programartifact.StaticNodeReference {
		if targetID, targetOK := row.ChildAt(0); targetOK {
			if target, rowOK := a.rows[targetID]; rowOK && target.Kind() != programartifact.StaticNodeReference {
				return a.resolve(targetID)
			}
		}
	}
	placeholder := typ.NewRecursivePlaceholder(row.Name())
	entry.value = placeholder
	entry.backedge = true
	return placeholder, true
}

func withoutArtifactAliasWrapper(value typ.Type) typ.Type {
	if alias, ok := value.(*typ.Alias); ok && alias != nil {
		return alias.Target
	}
	return value
}

func childAliasParam(a *artifactResolver, row programartifact.StaticTypeNodeRow, index int) (typ.Type, bool) {
	id, ok := row.AliasParamAt(index)
	if !ok {
		return nil, false
	}
	return a.resolve(id)
}
func childInterfaceExtend(a *artifactResolver, row programartifact.StaticTypeNodeRow, index int) (typ.Type, bool) {
	id, ok := row.InterfaceExtendAt(index)
	if !ok {
		return nil, false
	}
	return a.resolve(id)
}
func childInterfaceMember(a *artifactResolver, row programartifact.StaticTypeNodeRow, index int) (typ.Type, bool) {
	id, ok := row.InterfaceMemberTypeAt(index)
	if !ok {
		return nil, false
	}
	return a.resolve(id)
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
