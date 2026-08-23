package typeauthority

import (
	"context"
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programstaticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// artifactAuthority is the detached type-graph constructor. It admits only
// canonical Program rows and retains no Link, Artifact, Flow, or authored Term.
// It is intentionally separate from the legacy Link constructor while the
// Static evaluator migration is completed.
//
// The rows form a content-addressed graph. Sealing decomposes that graph into
// strongly connected components once, so every later materialization knows
// which rows lie on a cycle before it builds anything.
type artifactAuthority struct {
	views      []programstaticnode.View
	location   map[identity.ContentID]canonicalRowLocation
	component  map[identity.ContentID]rowComponent
	projection map[identity.ContentID]artifactReferenceProjection
	// qualified is the name-to-type directory a canonical reference resolves
	// through. It is the Link's target vocabulary, handed in already read.
	qualified qualifiedDirectory
	// refusal is the first reason a reference named something the authority
	// could not reach. A skipped projection is how an open or unresolved row
	// leaves the directory, so a reference that names a missing declaration
	// must state its reason here instead of disappearing into that same path.
	refusal string
}

// refuse records the first refusal reason. Later reasons are consequences of
// the first, so the seal reports the one that started it.
func (a *artifactAuthority) refuse(reason string) {
	if a != nil && a.refusal == "" {
		a.refusal = reason
	}
}

// artifactReferenceProjection is the seal-local image from which the Link
// authority mints its public scalar projection. A closed row carries the one
// owner-issued graph receipt used to mint RuntimeInput; it is released after
// the Link projection directory is bound.
type artifactReferenceProjection struct {
	semantic identity.ContentID
	root     kind.Kind
	may      runtimekind.Set
	name     string
	open     bool
	graph    typ.CanonicalGraphReceipt
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
	authority *artifactAuthority
	built     map[identity.ContentID]typ.Type
	scope     *componentScope
}

func (a *artifactResolver) programIndex(row programstaticnode.StaticTypeNode) (programstaticnode.View, int, bool) {
	if a == nil || a.authority == nil {
		return programstaticnode.View{}, 0, false
	}
	return a.authority.programFor(row.ID())
}

// children is a transient reconstruction of the historical semantic child
// order from the canonical Program families.
func (a *artifactResolver) children(row programstaticnode.StaticTypeNode) ([]identity.ContentID, bool) {
	view, index, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	return view.StaticTypeNodeChildren(index, row, false)
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

func sealPrograms(programs []programschema.Program, retainValues bool, qualified []QualifiedType) (*artifactAuthority, error) {
	if len(programs) == 0 {
		return nil, errors.New("typeauthority: no programs")
	}
	directory, directoryErr := newQualifiedDirectory(qualified)
	if directoryErr != nil {
		return nil, directoryErr
	}
	locations := make(map[identity.ContentID]canonicalRowLocation)
	views := make([]programstaticnode.View, 0, len(programs))
	for _, program := range programs {
		if !program.Available() {
			return nil, errors.New("typeauthority: unavailable program")
		}
		state, stateOK := program.ColdState()
		if !stateOK {
			return nil, errors.New("typeauthority: unavailable program cold state")
		}
		view, viewOK := programstaticnode.NewView(state)
		if !viewOK {
			return nil, errors.New("typeauthority: unavailable program static view")
		}
		owner := program.ProgramID
		count, countOK := view.StaticTypeNodeCount()
		if !countOK {
			return nil, errors.New("typeauthority: unavailable program static graph")
		}
		for index := 0; index < count; index++ {
			row, ok := view.StaticTypeNodeAt(index)
			if !ok || !row.Available() || row.Owner() != owner {
				return nil, errors.New("typeauthority: malformed program type row")
			}
			if _, duplicate := locations[row.ID()]; duplicate {
				return nil, errors.New("typeauthority: duplicate program type row")
			}
			locations[row.ID()] = canonicalRowLocation{view: view, index: index}
		}
		views = append(views, view)
	}
	components, componentsOK := componentsOfRows(locations)
	if !componentsOK {
		return nil, errors.New("typeauthority: malformed canonical static graph")
	}
	a := &artifactAuthority{
		views:      views,
		location:   locations,
		component:  components,
		projection: make(map[identity.ContentID]artifactReferenceProjection, len(locations)),
		qualified:  directory,
	}
	resolver := &artifactResolver{authority: a, built: make(map[identity.ContentID]typ.Type)}
	for _, view := range views {
		count, countOK := view.StaticTypeNodeCount()
		if !countOK {
			return nil, errors.New("typeauthority: unavailable program static graph")
		}
		for index := 0; index < count; index++ {
			row, rowOK := view.StaticTypeNodeAt(index)
			if !rowOK {
				return nil, errors.New("typeauthority: malformed program type row")
			}
			value, valueOK := resolver.resolve(row.ID())
			if !valueOK {
				continue
			}
			graph, graphErr := typ.EncodeCanonicalGraph(context.Background(), value)
			if graphErr != nil || !graph.Valid() {
				continue
			}
			digest, digestOK := graph.Digest()
			if !digestOK {
				continue
			}
			root, rootOK := graph.Root()
			if !rootOK {
				continue
			}
			projection := artifactReferenceProjection{
				semantic: identity.ContentID(digest),
				root:     root.Kind,
				may:      typ.MayRuntimeKinds(value),
				name:     referenceTypeName(value),
				open:     !root.Closed,
			}
			if retainValues && !projection.open {
				projection.graph = graph
			}
			a.projection[row.ID()] = projection
		}
	}
	if a.refusal != "" {
		return nil, errors.New(a.refusal)
	}
	return a, nil
}

func (a *artifactAuthority) referenceProjection(id identity.ContentID) (artifactReferenceProjection, bool) {
	if a == nil || !id.Available() {
		return artifactReferenceProjection{}, false
	}
	projection, ok := a.projection[id]
	return projection, ok && projection.semantic.Available() && projection.may.Valid()
}

func (a *artifactAuthority) releaseProjectionGraphs() {
	if a == nil {
		return
	}
	for id, projection := range a.projection {
		projection.graph = typ.CanonicalGraphReceipt{}
		a.projection[id] = projection
	}
}

func referenceTypeName(value typ.Type) string {
	switch named := value.(type) {
	case *typ.Alias:
		return named.Name
	case *typ.Interface:
		return named.Name
	case *typ.Generic:
		return named.Name
	case *typ.Recursive:
		return named.Name
	}
	return ""
}

func (a *artifactAuthority) row(id identity.ContentID) (programstaticnode.StaticTypeNode, bool) {
	if a == nil || !id.Available() {
		return programstaticnode.StaticTypeNode{}, false
	}
	location, ok := a.location[id]
	if !ok {
		return programstaticnode.StaticTypeNode{}, false
	}
	row, ok := location.view.StaticTypeNodeAt(location.index)
	return row, ok && row.ID() == id
}

func (a *artifactAuthority) programFor(id identity.ContentID) (programstaticnode.View, int, bool) {
	if a == nil || !id.Available() {
		return programstaticnode.View{}, 0, false
	}
	location, ok := a.location[id]
	if !ok {
		return programstaticnode.View{}, 0, false
	}
	return location.view, location.index, true
}
func (a *artifactResolver) resolve(id identity.ContentID) (typ.Type, bool) {
	row, ok := a.authority.row(id)
	if !ok {
		return nil, false
	}
	if row.Kind() == programstaticnode.StaticNodeReference {
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

// resolveReference materializes a TypeRef edge. A declaration reference is a
// transparent edge of the type graph: it owns no value of its own and never
// carries a binder, so resolution entered at a reference and resolution
// entered at the declaration it names are one type. An unresolved reference is
// a complete, targetless leaf: Unknown is the Static carrier for missing
// information and stays distinct from Any. A canonical reference is resolved
// too, but the declaration it names is outside this Program, so it is read by
// path from the qualified type directory the authority was sealed with. A
// reference chain that closes on itself names no declaration and issues no
// value.
func (a *artifactResolver) resolveReference(row programstaticnode.StaticTypeNode) (typ.Type, bool) {
	var seen map[identity.ContentID]bool
	for {
		children, childrenOK := a.children(row)
		if !childrenOK {
			return nil, false
		}
		switch staticReferenceResolutionShape(staticrefs.Resolution(row.Resolution()), len(children)) {
		case staticReferenceUnresolved:
			return typ.Unknown, true
		case staticReferenceCanonical:
			return a.resolveCanonical(row)
		case staticReferenceDeclaration:
		default:
			return nil, false
		}
		target := children[0]
		targetOK := target.Available()
		if !targetOK {
			return nil, false
		}
		next, ok := a.authority.row(target)
		if !ok {
			return nil, false
		}
		if next.Kind() != programstaticnode.StaticNodeReference {
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
func (a *artifactResolver) member(scope *componentScope, id identity.ContentID, row programstaticnode.StaticTypeNode) (typ.Type, bool) {
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
		if row.Kind() == programstaticnode.StaticNodeAlias && aliasSpanOK && aliasParamCount == 0 {
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
func (a *artifactResolver) construct(id identity.ContentID, row programstaticnode.StaticTypeNode, scope *componentScope) (typ.Type, bool) {
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
	case programstaticnode.StaticNodePrimitive:
		value, ok = primitiveKind(row.LiteralKind())
	case programstaticnode.StaticNodeLiteral:
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
	case programstaticnode.StaticNodeOptional:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			value = typeexpr.Optional(inner)
		}
	case programstaticnode.StaticNodeUnion, programstaticnode.StaticNodeIntersection:
		members := make([]typ.Type, len(children))
		for i := range members {
			members[i], ok = child(i)
			if !ok {
				break
			}
		}
		if ok {
			if row.Kind() == programstaticnode.StaticNodeUnion {
				value = typeexpr.Union(members...)
			} else {
				value = typeexpr.Intersection(members...)
			}
		}
	case programstaticnode.StaticNodeArray:
		var inner typ.Type
		inner, ok = child(0)
		if ok {
			if row.Flag() {
				value = typ.NewReadonlyMap(typ.Integer, inner)
			} else {
				value = typ.NewArray(inner)
			}
		}
	case programstaticnode.StaticNodeMap:
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
	case programstaticnode.StaticNodeRecord:
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
	case programstaticnode.StaticNodeAlias:
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
	case programstaticnode.StaticNodeGeneric:
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
	case programstaticnode.StaticNodeInterface:
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
	case programstaticnode.StaticNodeTypeFunction:
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
	case programstaticnode.StaticNodeTypeParam:
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

// staticReferenceEdge is the edge one TypeRef row resolves through. Exactly
// one target edge belongs to a declaration reference: an unresolved reference
// has nothing to name, and a canonical reference names a declaration outside
// this Program by path, which is a name rather than an edge.
type staticReferenceEdge uint8

const (
	staticReferenceInvalid staticReferenceEdge = iota
	staticReferenceUnresolved
	staticReferenceDeclaration
	staticReferenceCanonical
)

// staticReferenceResolutionShape is the sole TypeRef edge-cardinality gate
// used by artifactAuthority. Keeping resolution and arity in one predicate
// prevents an unknown enum or an extra target edge from being accepted merely
// because ChildAt(0) happens to resolve.
func staticReferenceResolutionShape(resolution staticrefs.Resolution, childCount int) staticReferenceEdge {
	switch resolution {
	case staticrefs.Unresolved:
		if childCount != 0 {
			return staticReferenceInvalid
		}
		return staticReferenceUnresolved
	case staticrefs.Declaration:
		if childCount != 1 {
			return staticReferenceInvalid
		}
		return staticReferenceDeclaration
	case staticrefs.CanonicalPath:
		if childCount != 0 {
			return staticReferenceInvalid
		}
		return staticReferenceCanonical
	default:
		return staticReferenceInvalid
	}
}

func withoutArtifactAliasWrapper(value typ.Type) typ.Type {
	if alias, ok := value.(*typ.Alias); ok && alias != nil {
		return alias.Target
	}
	return value
}

func childAliasParam(a *artifactResolver, row programstaticnode.StaticTypeNode, index int) (typ.Type, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := view.StaticTypeNodeAliasParameterFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}
func childInterfaceExtend(a *artifactResolver, row programstaticnode.StaticTypeNode, index int) (typ.Type, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := view.StaticTypeNodeInterfaceExtendFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}
func childInterfaceMember(a *artifactResolver, row programstaticnode.StaticTypeNode, index int) (typ.Type, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return nil, false
	}
	child, ok := view.StaticTypeNodeInterfaceMemberFor(parent, index)
	if !ok {
		return nil, false
	}
	return a.resolve(child.ChildID())
}

func (a *artifactResolver) recordField(row programstaticnode.StaticTypeNode, index int) (programstaticnode.StaticTypeNodeRecordField, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return programstaticnode.StaticTypeNodeRecordField{}, false
	}
	return view.StaticTypeNodeRecordFieldFor(parent, index)
}
func (a *artifactResolver) interfaceMember(row programstaticnode.StaticTypeNode, index int) (programstaticnode.StaticTypeNodeInterfaceMember, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return programstaticnode.StaticTypeNodeInterfaceMember{}, false
	}
	return view.StaticTypeNodeInterfaceMemberFor(parent, index)
}
func (a *artifactResolver) functionTypeParameter(row programstaticnode.StaticTypeNode, index int) (programstaticnode.StaticTypeNodeTypeFunctionTypeParameter, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return programstaticnode.StaticTypeNodeTypeFunctionTypeParameter{}, false
	}
	return view.StaticTypeNodeTypeFunctionTypeParameterFor(parent, index)
}
func (a *artifactResolver) functionParameter(row programstaticnode.StaticTypeNode, index int) (programstaticnode.StaticTypeNodeTypeFunctionParameter, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return programstaticnode.StaticTypeNodeTypeFunctionParameter{}, false
	}
	return view.StaticTypeNodeTypeFunctionParameterFor(parent, index)
}
func (a *artifactResolver) functionReturn(row programstaticnode.StaticTypeNode, index int) (programstaticnode.StaticTypeNodeTypeFunctionReturn, bool) {
	view, parent, ok := a.programIndex(row)
	if !ok {
		return programstaticnode.StaticTypeNodeTypeFunctionReturn{}, false
	}
	return view.StaticTypeNodeTypeFunctionReturnFor(parent, index)
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
