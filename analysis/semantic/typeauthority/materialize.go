package typeauthority

import (
	"math"

	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/static"
)

// Materialize recovers existing typ semantics on the cold side of the
// authority boundary. It never inserts a typ.Type into a selector or product
// fact. Dependent static operators, open formals, and interface declarations
// deliberately report unsupported until their owning semantic Rules resolve
// them; treating any of those as typ.Any here would be an unsound shortcut.
func (a *Authority) Materialize(selector Selector) (typ.Type, bool) {
	if a == nil || selector == 0 || uint64(selector) > uint64(len(a.entries)) {
		return nil, false
	}
	// A public projection must never expose this Authority's memoized graph:
	// every typ composite has public mutable fields. Rebuild the exact Program
	// projection in an isolated, throwaway construction arena instead. This is
	// cold by contract and, unlike canonical structural decoding, preserves
	// aliases and the identity sharing between a generic and its formals.
	projection := &Authority{
		linkID:    a.linkID,
		entries:   a.entries,
		byRef:     a.byRef,
		states:    make([]materializationState, len(a.entries)),
		types:     make([]typ.Type, len(a.entries)),
		params:    make([]*typ.TypeParam, len(a.entries)),
		recursive: make([]*typ.Recursive, len(a.entries)),
	}
	value, ok := projection.materialize(selector)
	if !ok || value == nil || typ.ValidateStaticGenericRecurrence(value) != nil {
		return nil, false
	}
	return value, true
}

func (a *Authority) materialize(selector Selector) (typ.Type, bool) {
	if a == nil || selector == 0 || int(selector) > len(a.entries) {
		return nil, false
	}
	index := int(selector - 1)
	if a.states[index] == materializationReady {
		return a.types[index], a.types[index] != nil
	}
	if a.states[index] == materializationWorking {
		if value := a.types[index]; value != nil {
			return value, true
		}
		// A forward formal bound may be sitting on its one-step TypeRef while
		// prepareOwnerParams publishes the later formal. Its child has already
		// reached Ready under the same explicit postorder plan, so close this
		// transparent edge directly rather than re-entering Materialize.
		entry := a.entries[index]
		if _, target, _, isRef := entry.program.Static().References().Get(entry.ref.Root()); isRef {
			if targetSelector, found := a.lookupProgramTerm(entry.program, target); found {
				targetIndex := int(targetSelector - 1)
				if a.states[targetIndex] == materializationReady && a.types[targetIndex] != nil {
					a.types[index] = a.types[targetIndex]
					a.states[index] = materializationReady
					return a.types[index], true
				}
			}
		}
		return a.recursivePlaceholder(index)
	}

	// Program static terms form one finite sealed graph. Build its reachable
	// selector slice in postorder instead of using the Go call stack for every
	// child. A working backedge is represented by the existing explicit Mu
	// placeholder; no depth/fuel outcome is introduced.
	type frame struct {
		selector Selector
		exit     bool
	}
	work := []frame{{selector: selector}}
	started := make([]Selector, 0, 8)
	fail := func() (typ.Type, bool) {
		for _, startedSelector := range started {
			startedIndex := int(startedSelector - 1)
			if a.states[startedIndex] == materializationWorking {
				a.states[startedIndex] = materializationCold
				a.types[startedIndex] = nil
				a.recursive[startedIndex] = nil
			}
		}
		return nil, false
	}
	for len(work) != 0 {
		last := len(work) - 1
		current := work[last]
		work = work[:last]
		currentIndex := int(current.selector - 1)
		if current.exit {
			value, ok := a.materializeProgram(
				a.entries[currentIndex].program,
				a.entries[currentIndex].ref.Root(),
				current.selector,
			)
			if !ok || value == nil {
				return fail()
			}
			a.types[currentIndex] = value
			a.states[currentIndex] = materializationReady
			continue
		}
		switch a.states[currentIndex] {
		case materializationReady:
			continue
		case materializationWorking:
			if _, ok := a.recursivePlaceholder(currentIndex); !ok {
				return fail()
			}
			continue
		}
		a.states[currentIndex] = materializationWorking
		started = append(started, current.selector)
		children, ok := materializationChildren(a.entries[currentIndex].program, a.entries[currentIndex].ref.Root())
		if !ok {
			return fail()
		}
		work = append(work, frame{selector: current.selector, exit: true})
		for childIndex := len(children) - 1; childIndex >= 0; childIndex-- {
			child, found := a.lookupProgramTerm(a.entries[currentIndex].program, children[childIndex])
			if !found {
				return fail()
			}
			work = append(work, frame{selector: child})
		}
	}
	value := a.types[index]
	return value, value != nil && a.states[index] == materializationReady
}

func (a *Authority) materializeProgram(p *program.Program, term keyspace.Term, selector Selector) (typ.Type, bool) {
	if p == nil {
		return nil, false
	}
	view := p.Static()
	declarations := view.Declarations()
	types := view.Types()
	if _, target, name, _, ok := declarations.Aliases().Get(term); ok {
		return a.materializeAlias(p, term, selector, target, name)
	}
	if _, _, _, ok := declarations.TypeParams().Get(term); ok {
		return a.materializeTypeParam(p, term)
	}
	if kind, ok := types.Primitives().Get(term); ok {
		return primitive(kind)
	}
	if kind, key, bits, ok := types.Literals().Get(term); ok {
		return literalType(p, kind, key, bits)
	}
	if inner, ok := types.Optionals().Get(term); ok {
		value, ok := a.materializeTerm(p, inner)
		if !ok {
			return nil, false
		}
		return typeexpr.Optional(value), true
	}
	if count, ok := types.Unions().MemberCount(term); ok {
		members, ok := a.materializeMembers(p, term, count, types.Unions().MemberAt)
		if !ok {
			return nil, false
		}
		return typeexpr.Union(members...), true
	}
	if count, ok := types.Intersections().MemberCount(term); ok {
		members, ok := a.materializeMembers(p, term, count, types.Intersections().MemberAt)
		if !ok {
			return nil, false
		}
		return typeexpr.Intersection(members...), true
	}
	if resolution, target, _, ok := view.References().Get(term); ok {
		if resolution != static.TypeRefDeclaration || target == 0 {
			return nil, false
		}
		return a.materializeTerm(p, target)
	}
	if base, _, ok := types.Generics().Get(term); ok {
		return a.materializeGeneric(p, base, term)
	}
	if element, readonly, ok := types.Arrays().Get(term); ok {
		value, ok := a.materializeTerm(p, element)
		if !ok {
			return nil, false
		}
		if readonly {
			// typ deliberately represents the covariant readonly array view as
			// its already-proven integer-keyed readonly-map semantics. This is
			// a semantic projection, not a new authority type form.
			return typ.NewReadonlyMap(typ.Integer, value), true
		}
		return typ.NewArray(value), true
	}
	if key, value, readonly, ok := types.Maps().Get(term); ok {
		keyType, keyOK := a.materializeTerm(p, key)
		valueType, valueOK := a.materializeTerm(p, value)
		if !keyOK || !valueOK {
			return nil, false
		}
		if readonly {
			return typ.NewReadonlyMap(keyType, valueType), true
		}
		return typ.NewMap(keyType, valueType), true
	}
	if readonly, count, ok := types.Records().Get(term); ok {
		return a.materializeRecord(p, term, readonly, count)
	}
	if _, variadic, _, _, ok := view.Signatures().TypeFunctions().Get(term); ok {
		return a.materializeSignature(p, term, variadic)
	}
	if _, _, _, ok := declarations.Interfaces().Get(term); ok {
		return a.materializeInterface(p, term, selector)
	}
	// typeof, keyof, index access, conditional, and assertion nodes remain
	// exact selectors but require their owning static semantic Rule. They must
	// not be eagerly approximated.
	return nil, false
}

func (a *Authority) materializeAlias(p *program.Program, alias keyspace.Term, selector Selector, target keyspace.Term, name keyspace.Key) (typ.Type, bool) {
	nameText, ok := keyText(p, name)
	if !ok {
		return nil, false
	}
	paramCount, ok := p.Static().Declarations().Aliases().ParamCount(alias)
	if !ok {
		return nil, false
	}
	if paramCount != 0 {
		params, ok := a.prepareOwnerParams(p, alias)
		if !ok || len(params) != paramCount {
			return nil, false
		}
		generic := typ.NewGeneric(nameText, params, nil)
		a.types[int(selector-1)] = generic
		body, ok := a.materializeTerm(p, target)
		if !ok {
			return nil, false
		}
		generic.SetBody(body)
		return generic, true
	}
	index := int(selector - 1)
	targetType, ok := a.materializeTerm(p, target)
	if !ok {
		return nil, false
	}
	if placeholder := a.recursive[index]; placeholder != nil {
		placeholder.SetBody(targetType)
		return placeholder, true
	}
	return typ.NewAlias(nameText, targetType), true
}

// materializeTypeParam gives one Program-owned formal its unique existing typ
// identity. The identity is shared by its owning alias/signature and every
// TypeRef that resolves to it; names are never used as the key.
func (a *Authority) materializeTypeParam(p *program.Program, term keyspace.Term) (typ.Type, bool) {
	owner, _, _, ok := p.Static().Declarations().TypeParams().Get(term)
	if !ok {
		return nil, false
	}
	if _, ok := a.prepareOwnerParams(p, owner); !ok {
		return nil, false
	}
	selector, ok := a.lookupProgramTerm(p, term)
	if !ok {
		return nil, false
	}
	param := a.params[int(selector-1)]
	return param, param != nil
}

// prepareOwnerParams constructs an owner binder once, in dependency order.
// typ.TypeParam is immutable, so a cyclic constraint graph has no lawful
// projection into the existing typ representation and remains symbolic rather
// than receiving an invented approximation. This is distinct from recursive
// aliases, which typ represents with explicit Mu nodes.
func (a *Authority) prepareOwnerParams(p *program.Program, owner keyspace.Term) ([]*typ.TypeParam, bool) {
	terms, ok := ownerTypeParams(p, owner)
	if !ok {
		return nil, false
	}
	if len(terms) == 0 {
		return nil, true
	}
	params := make([]*typ.TypeParam, len(terms))
	remaining := make([]bool, len(terms))
	positions := make(map[keyspace.Term]int, len(terms))
	selectors := make([]Selector, len(terms))
	for index, term := range terms {
		positions[term] = index
		selector, found := a.lookupProgramTerm(p, term)
		if !found {
			return nil, false
		}
		selectors[index] = selector
		if a.params[int(selector-1)] != nil {
			params[index] = a.params[int(selector-1)]
			continue
		}
		remaining[index] = true
	}
	dependencies := make([]map[int]struct{}, len(terms))
	for index, term := range terms {
		_, _, constraint, valid := p.Static().Declarations().TypeParams().Get(term)
		if !valid {
			return nil, false
		}
		if constraint == 0 {
			dependencies[index] = map[int]struct{}{}
			continue
		}
		dependencyTerms, ok := formalDependencies(p, constraint, owner)
		if !ok {
			return nil, false
		}
		dependencies[index] = make(map[int]struct{}, len(dependencyTerms))
		for dependency := range dependencyTerms {
			position, local := positions[dependency]
			if !local {
				return nil, false
			}
			dependencies[index][position] = struct{}{}
		}
	}
	for {
		progress := false
		for index, term := range terms {
			if !remaining[index] {
				continue
			}
			ready := true
			for dependency := range dependencies[index] {
				if a.params[int(selectors[dependency]-1)] == nil {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			_, name, constraint, valid := p.Static().Declarations().TypeParams().Get(term)
			if !valid {
				return nil, false
			}
			nameText, named := keyText(p, name)
			if !named {
				return nil, false
			}
			var bound typ.Type
			if constraint != 0 {
				var bounded bool
				bound, bounded = a.materializeTerm(p, constraint)
				if !bounded {
					continue
				}
			}
			param := typ.NewTypeParam(nameText, bound)
			selector := selectors[index]
			a.params[int(selector-1)] = param
			a.types[int(selector-1)] = param
			a.states[int(selector-1)] = materializationReady
			params[index] = param
			remaining[index] = false
			progress = true
		}
		if !progress {
			break
		}
	}
	for index := range remaining {
		if remaining[index] {
			return nil, false
		}
	}
	return params, true
}

func ownerTypeParams(p *program.Program, owner keyspace.Term) ([]keyspace.Term, bool) {
	if p == nil {
		return nil, false
	}
	declarations := p.Static().Declarations()
	signatures := p.Static().Signatures().TypeFunctions()
	contracts := p.Static().Contracts().Functions()
	if count, ok := declarations.Aliases().ParamCount(owner); ok {
		return ownerParamTerms(count, func(index int) (keyspace.Term, bool) {
			return declarations.Aliases().ParamAt(owner, index)
		})
	}
	if count, ok := signatures.TypeParamCount(owner); ok {
		return ownerParamTerms(count, func(index int) (keyspace.Term, bool) {
			return signatures.TypeParamAt(owner, index)
		})
	}
	if count, ok := contracts.TypeParamCount(owner); ok {
		return ownerParamTerms(count, func(index int) (keyspace.Term, bool) {
			return contracts.TypeParamAt(owner, index)
		})
	}
	return nil, false
}

func ownerParamTerms(count int, at func(int) (keyspace.Term, bool)) ([]keyspace.Term, bool) {
	if count < 0 {
		return nil, false
	}
	terms := make([]keyspace.Term, count)
	for index := range terms {
		term, ok := at(index)
		if !ok {
			return nil, false
		}
		terms[index] = term
	}
	return terms, true
}

func (a *Authority) materializeTerm(p *program.Program, term keyspace.Term) (typ.Type, bool) {
	if p == nil {
		return nil, false
	}
	selector, ok := a.lookupProgramTerm(p, term)
	if !ok {
		return nil, false
	}
	index := int(selector - 1)
	if a.states[index] == materializationReady && a.types[index] != nil {
		return a.types[index], true
	}
	// A same-owner formal may already be published while this transparent
	// TypeRef has not itself entered the postorder work stack. Close that exact
	// one-step edge without recursively materializing anything. This is the
	// backward-bound case in declarations such as <T, U: T>.
	if _, target, _, isRef := p.Static().References().Get(term); isRef && target != 0 {
		if targetSelector, found := a.lookupProgramTerm(p, target); found {
			targetIndex := int(targetSelector - 1)
			if a.states[targetIndex] == materializationReady && a.types[targetIndex] != nil {
				a.types[index] = a.types[targetIndex]
				a.states[index] = materializationReady
				return a.types[index], true
			}
		}
	}
	if a.states[index] == materializationWorking {
		if value := a.types[index]; value != nil {
			return value, true
		}
		entry := a.entries[index]
		if _, target, _, isRef := entry.program.Static().References().Get(entry.ref.Root()); isRef {
			if targetSelector, found := a.lookupProgramTerm(entry.program, target); found {
				targetIndex := int(targetSelector - 1)
				if a.states[targetIndex] == materializationReady && a.types[targetIndex] != nil {
					a.types[index] = a.types[targetIndex]
					a.states[index] = materializationReady
					return a.types[index], true
				}
			}
		}
		return a.recursivePlaceholder(index)
	}
	// materialize's postorder planner must have scheduled every direct sealed
	// child before a parent builder runs. Re-entering it here would recreate
	// Go-stack semantic descent and incorrectly make a missed dependency look
	// like a valid alternate path.
	return nil, false
}

// lookupProgramTerm mints the only portable static reference through the
// sealed Program before admitting it to this Link-local selector authority.
// Callers never reconstruct an owner/root pair themselves.
func (a *Authority) lookupProgramTerm(p *program.Program, term keyspace.Term) (Selector, bool) {
	if a == nil || p == nil {
		return 0, false
	}
	if _, ok := p.Static().StaticTypes().Ref(term); !ok {
		return 0, false
	}
	return a.Lookup(StaticTypeRef{owner: p.ContentID(), root: term})
}

// materializationChildren returns the direct Program static-type operands of a
// materializable term in source order. It is intentionally a single-step
// decoder: Materialize owns graph traversal with its explicit work stack.
func materializationChildren(p *program.Program, term keyspace.Term) ([]keyspace.Term, bool) {
	if p == nil || term == 0 {
		return nil, false
	}
	view := p.Static()
	declarations := view.Declarations()
	types := view.Types()
	signatures := view.Signatures().TypeFunctions()
	if _, target, _, _, ok := declarations.Aliases().Get(term); ok {
		count, counted := declarations.Aliases().ParamCount(term)
		if !counted {
			return nil, false
		}
		out := make([]keyspace.Term, 0, count+1)
		for index := 0; index < count; index++ {
			param, ok := declarations.Aliases().ParamAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, param)
		}
		return append(out, target), true
	}
	if _, _, constraint, ok := declarations.TypeParams().Get(term); ok {
		if constraint == 0 {
			return nil, true
		}
		return []keyspace.Term{constraint}, true
	}
	if _, ok := types.Primitives().Get(term); ok {
		return nil, true
	}
	if _, _, _, ok := types.Literals().Get(term); ok {
		return nil, true
	}
	if inner, ok := types.Optionals().Get(term); ok {
		return []keyspace.Term{inner}, true
	}
	if count, ok := types.Unions().MemberCount(term); ok {
		return materializationMemberChildren(term, count, types.Unions().MemberAt)
	}
	if count, ok := types.Intersections().MemberCount(term); ok {
		return materializationMemberChildren(term, count, types.Intersections().MemberAt)
	}
	if _, target, _, ok := view.References().Get(term); ok {
		if target == 0 {
			return nil, false
		}
		return []keyspace.Term{target}, true
	}
	if base, count, ok := types.Generics().Get(term); ok {
		out := make([]keyspace.Term, 0, count+1)
		out = append(out, base)
		for index := 0; index < count; index++ {
			arg, ok := types.Generics().ArgAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, arg)
		}
		return out, true
	}
	if element, _, ok := types.Arrays().Get(term); ok {
		return []keyspace.Term{element}, true
	}
	if key, value, _, ok := types.Maps().Get(term); ok {
		return []keyspace.Term{key, value}, true
	}
	if _, count, ok := types.Records().Get(term); ok {
		out := make([]keyspace.Term, 0, count)
		for index := 0; index < count; index++ {
			field, ok := types.Records().FieldAt(term, index)
			if !ok {
				return nil, false
			}
			_, value, _, ok := types.Fields().Get(field)
			if !ok {
				return nil, false
			}
			out = append(out, value)
		}
		return out, true
	}
	if _, variadic, _, _, ok := signatures.Get(term); ok {
		genericCount, ok := signatures.TypeParamCount(term)
		if !ok {
			return nil, false
		}
		paramCount, ok := signatures.ParameterCount(term)
		if !ok {
			return nil, false
		}
		returnCount, ok := signatures.ReturnCount(term)
		if !ok {
			return nil, false
		}
		out := make([]keyspace.Term, 0, genericCount+paramCount+returnCount+1)
		for index := 0; index < genericCount; index++ {
			param, ok := signatures.TypeParamAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, param)
		}
		for index := 0; index < paramCount; index++ {
			param, ok := signatures.ParameterAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, param.Type)
		}
		if variadic != 0 {
			out = append(out, variadic)
		}
		for index := 0; index < returnCount; index++ {
			result, ok := signatures.ReturnAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, result)
		}
		return out, true
	}
	if _, _, _, ok := declarations.Interfaces().Get(term); ok {
		extendCount, ok := declarations.Interfaces().ExtendCount(term)
		if !ok {
			return nil, false
		}
		memberCount, ok := declarations.Interfaces().MemberCount(term)
		if !ok {
			return nil, false
		}
		out := make([]keyspace.Term, 0, extendCount+memberCount)
		for index := 0; index < extendCount; index++ {
			extend, ok := declarations.Interfaces().ExtendAt(term, index)
			if !ok {
				return nil, false
			}
			out = append(out, extend)
		}
		for index := 0; index < memberCount; index++ {
			member, ok := declarations.Interfaces().MemberAt(term, index)
			if !ok {
				return nil, false
			}
			switch member.Kind {
			case static.InterfaceField:
				_, value, _, ok := types.Fields().Get(member.Field)
				if !ok {
					return nil, false
				}
				out = append(out, value)
			case static.InterfaceMethod:
				out = append(out, member.Signature)
			default:
				return nil, false
			}
		}
		return out, true
	}
	// Rule-owned static operators are deliberately not materialized. Treat
	// them as closed leaves so the planner reaches the same explicit failure
	// from materializeProgram without inventing a projection dependency.
	return nil, true
}

func materializationMemberChildren(term keyspace.Term, count int, at func(keyspace.Term, int) (keyspace.Term, bool)) ([]keyspace.Term, bool) {
	if count < 2 {
		return nil, false
	}
	out := make([]keyspace.Term, count)
	for index := range out {
		member, ok := at(term, index)
		if !ok {
			return nil, false
		}
		out[index] = member
	}
	return out, true
}

func (a *Authority) materializeMembers(
	p *program.Program,
	term keyspace.Term,
	count int,
	at func(keyspace.Term, int) (keyspace.Term, bool),
) ([]typ.Type, bool) {
	if count < 2 {
		return nil, false
	}
	members := make([]typ.Type, count)
	for index := range members {
		member, ok := at(term, index)
		if !ok {
			return nil, false
		}
		members[index], ok = a.materializeTerm(p, member)
		if !ok {
			return nil, false
		}
	}
	return members, true
}

func (a *Authority) materializeRecord(p *program.Program, term keyspace.Term, readonly bool, count int) (typ.Type, bool) {
	types := p.Static().Types()
	fields := make([]typ.Field, count)
	for index := range fields {
		fieldTerm, ok := types.Records().FieldAt(term, index)
		if !ok {
			return nil, false
		}
		key, fieldType, optional, ok := types.Fields().Get(fieldTerm)
		if !ok {
			return nil, false
		}
		name, ok := keyText(p, key)
		if !ok {
			return nil, false
		}
		value, ok := a.materializeTerm(p, fieldType)
		if !ok {
			return nil, false
		}
		fields[index] = typ.Field{Name: name, Type: value, Optional: optional, Readonly: readonly}
	}
	return typ.RebuildRecord(typ.RecordParts{Fields: fields}), true
}

func (a *Authority) materializeGeneric(p *program.Program, base, term keyspace.Term) (typ.Type, bool) {
	definition, ok := a.materializeTerm(p, base)
	if !ok {
		return nil, false
	}
	generic, ok := definition.(*typ.Generic)
	if !ok || generic == nil {
		return nil, false
	}
	_, count, ok := p.Static().Types().Generics().Get(term)
	if !ok || count != len(generic.TypeParams) {
		return nil, false
	}
	args := make([]typ.Type, count)
	for index := range args {
		arg, ok := p.Static().Types().Generics().ArgAt(term, index)
		if !ok {
			return nil, false
		}
		args[index], ok = a.materializeTerm(p, arg)
		if !ok {
			return nil, false
		}
		bound := generic.TypeParams[index].Constraint
		if bound != nil {
			// Constraints may depend on earlier declaration formals (for
			// example U: T). Validate this application after substituting the
			// exact supplied arguments; checking the raw formal would reject
			// valid dependent bounds.
			bound = subst.Params(bound, generic.TypeParams, args)
		}
		if bound != nil && !subtype.IsSubtype(args[index], bound) {
			// Generic applications are closed only after their exact existing
			// arguments satisfy the declaration-owned bound.  Open formals can
			// still flow through this path when their own constraint proves the
			// bound; an unproved or mismatched argument remains unavailable to
			// Static rather than being laundered into any.
			return nil, false
		}
	}
	return typ.Instantiate(generic, args...), true
}

// materializeInterface proves the only existing typ projection that preserves
// all context-independent interface behavior: inherited requirements remain
// explicit intersection members, fields remain an exact record, and methods
// remain an Interface. A duplicate member spelling cannot be represented by
// typ's single-name record/method lookup, so it remains symbolic for the
// static interface Rule instead of silently choosing a member.
func (a *Authority) materializeInterface(p *program.Program, term keyspace.Term, selector Selector) (typ.Type, bool) {
	declarations := p.Static().Declarations()
	types := p.Static().Types()
	_, name, _, ok := declarations.Interfaces().Get(term)
	if !ok {
		return nil, false
	}
	nameText, ok := keyText(p, name)
	if !ok {
		return nil, false
	}
	index := int(selector - 1)

	members := make([]typ.Type, 0, 3)
	requirements := make(map[string]interfaceRequirement)
	extendCount, ok := declarations.Interfaces().ExtendCount(term)
	if !ok {
		return nil, false
	}
	for extendIndex := 0; extendIndex < extendCount; extendIndex++ {
		extend, ok := declarations.Interfaces().ExtendAt(term, extendIndex)
		if !ok {
			return nil, false
		}
		value, ok := a.materializeTerm(p, extend)
		if !ok {
			return nil, false
		}
		if !addInterfaceRequirements(value, requirements, make(map[typ.Type]bool)) {
			return nil, false
		}
		members = append(members, value)
	}

	memberCount, ok := declarations.Interfaces().MemberCount(term)
	if !ok {
		return nil, false
	}
	fields := make([]typ.Field, 0, memberCount)
	methods := make([]typ.Method, 0, memberCount)
	for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
		member, ok := declarations.Interfaces().MemberAt(term, memberIndex)
		if !ok {
			return nil, false
		}
		switch member.Kind {
		case static.InterfaceField:
			key, fieldType, optional, ok := types.Fields().Get(member.Field)
			if !ok {
				return nil, false
			}
			fieldName, ok := keyText(p, key)
			if !ok {
				return nil, false
			}
			value, ok := a.materializeTerm(p, fieldType)
			if !ok {
				return nil, false
			}
			if !addInterfaceRequirement(requirements, fieldName, interfaceRequirement{
				field: true, typ: value, optional: optional,
			}) {
				return nil, false
			}
			fields = append(fields, typ.Field{Name: fieldName, Type: value, Optional: optional})
		case static.InterfaceMethod:
			methodName, ok := keyText(p, member.Name)
			if !ok {
				return nil, false
			}
			value, ok := a.materializeTerm(p, member.Signature)
			if !ok {
				return nil, false
			}
			function, ok := value.(*typ.Function)
			if !ok {
				return nil, false
			}
			if !addInterfaceRequirement(requirements, methodName, interfaceRequirement{typ: function}) {
				return nil, false
			}
			methods = append(methods, typ.Method{Name: methodName, Type: function})
		default:
			return nil, false
		}
	}
	if len(fields) != 0 {
		members = append(members, typ.RebuildRecord(typ.RecordParts{Fields: fields}))
	}
	// Preserve a named empty interface as an existing diagnostic identity;
	// subtype deliberately treats its empty method set as structural top.
	members = append(members, typ.NewInterface(nameText, methods))
	value := typeexpr.Intersection(members...)
	if placeholder := a.recursive[index]; placeholder != nil {
		placeholder.SetBody(value)
		return placeholder, true
	}
	return value, true
}

// recursivePlaceholder creates Mu only at an actual working backedge. The
// ordinary nonrecursive alias/interface path allocates no unused recursive
// node and retains no construction residue.
func (a *Authority) recursivePlaceholder(index int) (typ.Type, bool) {
	if index < 0 || index >= len(a.entries) {
		return nil, false
	}
	if placeholder := a.recursive[index]; placeholder != nil {
		return placeholder, true
	}
	entry := a.entries[index]
	var name keyspace.Key
	if _, _, aliasName, _, ok := entry.program.Static().Declarations().Aliases().Get(entry.ref.Root()); ok {
		name = aliasName
	} else if _, interfaceName, _, ok := entry.program.Static().Declarations().Interfaces().Get(entry.ref.Root()); ok {
		name = interfaceName
	} else {
		return nil, false
	}
	nameText, ok := keyText(entry.program, name)
	if !ok {
		return nil, false
	}
	placeholder := typ.NewRecursivePlaceholder(nameText)
	a.recursive[index] = placeholder
	return placeholder, true
}

func (a *Authority) materializeSignature(p *program.Program, term, variadic keyspace.Term) (typ.Type, bool) {
	signatures := p.Static().Signatures().TypeFunctions()
	genericCount, ok := signatures.TypeParamCount(term)
	if !ok {
		return nil, false
	}
	paramCount, ok := signatures.ParameterCount(term)
	if !ok {
		return nil, false
	}
	builder := typ.Func().ReserveParams(paramCount)
	if genericCount != 0 {
		formals, ok := a.prepareOwnerParams(p, term)
		if !ok || len(formals) != genericCount {
			return nil, false
		}
		for _, formal := range formals {
			builder.TypeParamRef(formal)
		}
	}
	for index := 0; index < paramCount; index++ {
		parameter, ok := signatures.ParameterAt(term, index)
		if !ok {
			return nil, false
		}
		value, ok := a.materializeTerm(p, parameter.Type)
		if !ok {
			return nil, false
		}
		name := ""
		if parameter.Name != 0 {
			name, ok = keyText(p, parameter.Name)
			if !ok {
				return nil, false
			}
		}
		builder.Param(name, value)
	}
	if variadic != 0 {
		value, ok := a.materializeTerm(p, variadic)
		if !ok {
			return nil, false
		}
		builder.Variadic(value)
	}
	returnCount, ok := signatures.ReturnCount(term)
	if !ok {
		return nil, false
	}
	returns := make([]typ.Type, returnCount)
	for index := range returns {
		returnTerm, ok := signatures.ReturnAt(term, index)
		if !ok {
			return nil, false
		}
		returns[index], ok = a.materializeTerm(p, returnTerm)
		if !ok {
			return nil, false
		}
	}
	return builder.Returns(returns...).Build(), true
}

func primitive(kind static.PrimitiveKind) (typ.Type, bool) {
	switch kind {
	case static.PrimitiveNil:
		return typ.Nil, true
	case static.PrimitiveBoolean:
		return typ.Boolean, true
	case static.PrimitiveNumber:
		return typ.Number, true
	case static.PrimitiveInteger:
		return typ.Integer, true
	case static.PrimitiveString:
		return typ.String, true
	case static.PrimitiveFunction:
		// Preserve typ's established primitive `function` meaning.  A function
		// primitive is not the particular callable signature
		// fun(...any): any: that signature falsely requires one result and
		// changes ordinary function subtyping.  Exact callable signatures stay
		// in Program's Signature/Function terms.
		return typ.BuiltinPrimitiveType("function")
	case static.PrimitiveAny:
		return typ.Any, true
	case static.PrimitiveUnknown:
		return typ.Unknown, true
	case static.PrimitiveNever:
		return typ.Never, true
	case static.PrimitiveSelf:
		return typ.Self, true
	default:
		return nil, false
	}
}

func literalType(p *program.Program, kind keyspace.LiteralKind, key keyspace.Key, bits uint64) (typ.Type, bool) {
	var value keyspace.LiteralValue
	if kind == keyspace.LiteralFloat {
		value = keyspace.LiteralValue{Kind: kind, FloatBits: bits}
	} else {
		var ok bool
		value, ok = p.Source().Keys().Exact(key)
		if !ok || value.Kind != kind {
			return nil, false
		}
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return typ.LiteralBool(value.Bool), true
	case keyspace.LiteralInteger:
		return typ.LiteralInt(value.Integer), true
	case keyspace.LiteralFloat:
		return typ.LiteralNumber(math.Float64frombits(value.FloatBits)), true
	case keyspace.LiteralString:
		return typ.LiteralString(value.String), true
	default:
		return nil, false
	}
}

func keyText(p *program.Program, key keyspace.Key) (string, bool) {
	literal, ok := p.Source().Keys().Exact(key)
	return literal.String, ok && literal.Kind == keyspace.LiteralString
}
