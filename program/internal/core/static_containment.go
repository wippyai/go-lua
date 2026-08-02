package core

import "errors"

// termSet is a compact membership plane, not a topology. Families allocate
// backing words lazily, so classifications that do not use a family allocate
// no storage for it.
type termSet [tagCount][]uint64

func (s *termSet) add(term Term) {
	if term == 0 || term.tag() >= tagCount {
		return
	}
	word := int((term.index() - 1) >> 6)
	if word >= len(s[term.tag()]) {
		s[term.tag()] = append(s[term.tag()], make([]uint64, word+1-len(s[term.tag()]))...)
	}
	s[term.tag()][word] |= uint64(1) << uint((term.index()-1)&63)
}
func (s termSet) has(term Term) bool {
	if term == 0 || term.tag() >= tagCount {
		return false
	}
	word := int((term.index() - 1) >> 6)
	return word < len(s[term.tag()]) && s[term.tag()][word]&(uint64(1)<<uint((term.index()-1)&63)) != 0
}

// sealStaticScopes memoizes the transient scope projections used throughout
// Seal. Static scope syntax is a forwarding forest (TypeParam, TypeFunction,
// and Annotation); caching both successful and rejected paths prevents a
// family of annotations from repeatedly walking the same deep scope chain.
// Nothing in this cache is persisted in Program.
type sealStaticScopes struct {
	b *Builder

	body      [tagCount][]Term
	bodyState [tagCount][]uint8

	annotationHost      []Term
	annotationHostState []uint8
}

func newSealStaticScopes(b *Builder) *sealStaticScopes {
	cache := &sealStaticScopes{b: b}
	if b == nil {
		return cache
	}
	cache.body[tagTypeParam] = make([]Term, len(b.typeParams))
	cache.bodyState[tagTypeParam] = make([]uint8, len(b.typeParams))
	cache.body[tagTypeFunction] = make([]Term, len(b.signatures))
	cache.bodyState[tagTypeFunction] = make([]uint8, len(b.signatures))
	cache.body[tagAnnotation] = make([]Term, len(b.annotations))
	cache.bodyState[tagAnnotation] = make([]uint8, len(b.annotations))
	cache.annotationHost = make([]Term, len(b.annotations))
	cache.annotationHostState = make([]uint8, len(b.annotations))
	return cache
}

func (c *sealStaticScopes) bodyFor(scope Term) Term {
	if c == nil || c.b == nil {
		return 0
	}
	b := c.b
	path := make([]Term, 0, 8)
	current := scope
	var body Term
	for current != 0 {
		tag, index := current.tag(), current.index()
		if tag < tagCount && index != 0 && int(index) <= len(c.bodyState[tag]) {
			switch c.bodyState[tag][index-1] {
			case 2:
				body = c.body[tag][index-1]
				current = 0
				continue
			case 1:
				// A malformed forwarding cycle is cached as rejected for every
				// node already visited on this resolution path.
				current = 0
				continue
			default:
				c.bodyState[tag][index-1] = 1
				path = append(path, current)
			}
		}
		switch {
		case b.has(current, tagCell) && b.lexicalCell(current):
			body = b.cells[current.index()-1].storage
			current = 0
		case b.has(current, tagTypeAlias):
			var ok bool
			body, ok = b.staticDeclarationScope(current)
			if !ok {
				body = 0
			}
			current = 0
		case b.has(current, tagTypeInterface):
			var ok bool
			body, ok = b.staticInterfaceScope(current)
			if !ok {
				body = 0
			}
			current = 0
		case b.has(current, tagTypeParam):
			owner := b.typeParams[current.index()-1].owner
			if b.has(owner, tagFunction) {
				// Function generics are authored before the Function's
				// formals. Do not forward into the direct Function case below:
				// that frontier is post-formals and belongs to the child Body.
				body = b.functions[owner.index()-1].owner
				current = 0
			} else {
				current = owner
			}
		case b.has(current, tagTypeFunction):
			current = b.signatures[current.index()-1].scope
		case b.has(current, tagValueClaim):
			body = b.valueClaims[current.index()-1].owner
			current = 0
		case b.has(current, tagCall):
			body = b.calls[current.index()-1].owner
			current = 0
		case b.has(current, tagAnnotation):
			current = b.annotations[current.index()-1].scope
		case b.has(current, tagFunction):
			// Only a direct Function static host reaches this case. Return
			// types and their annotations are post-formals, at its child Body.
			body = b.functions[current.index()-1].body
			current = 0
		default:
			body = 0
			current = 0
		}
	}
	for _, term := range path {
		c.body[term.tag()][term.index()-1] = body
		c.bodyState[term.tag()][term.index()-1] = 2
	}
	return body
}

// annotationOwnerHost removes only Annotation forwarding. Other scope terms
// are exact static ownership identities and must continue to compare by Term.
func (c *sealStaticScopes) annotationOwnerHost(scope Term) Term {
	if c == nil || c.b == nil {
		return 0
	}
	b := c.b
	path := make([]Term, 0, 8)
	current := scope
	var host Term
	for current != 0 {
		if !b.has(current, tagAnnotation) {
			if b.valid(current) {
				host = current
			}
			break
		}
		index := current.index() - 1
		switch c.annotationHostState[index] {
		case 2:
			host = c.annotationHost[index]
			current = 0
			continue
		case 1:
			current = 0
			continue
		default:
			c.annotationHostState[index] = 1
			path = append(path, current)
			current = b.annotations[index].scope
		}
	}
	for _, term := range path {
		c.annotationHost[term.index()-1] = host
		c.annotationHostState[term.index()-1] = 2
	}
	return host
}

// typeOfRow is authored static type syntax. scope is deliberately the exact
// declaration term which owns the type tree; it is not a surrogate Body.
// operand is an ordinary expression occurrence whose containment is static.
type typeOfRow struct{ scope, operand Term }

// TypeOf records parser-reachable typeof(expr). The expression is built with
// the ordinary source expression relations, so it has its normal lexical Body
// owner and span. scope is the declaration/signature host for the type tree.
// Runtime Function is a precise host too: a direct Function scope is
// post-formals, while a TypeParam it owns is pre-formals.
func (b *Builder) TypeOf(span Span, scope, operand Term) Term {
	if b == nil || !b.require(b.staticScopeHandle(scope) && b.valueOccurrence(operand)) {
		return 0
	}
	b.typeOfs = append(b.typeOfs, typeOfRow{scope: scope, operand: operand})
	term := b.mint(tagTypeOf, span, b.familyIndex(len(b.typeOfs)))
	if term == 0 {
		b.typeOfs = b.typeOfs[:len(b.typeOfs)-1]
	}
	return term
}

// staticScopeHandle is the O(1) construction-time shape check. Forwarding
// chains are intentionally not chased while lowering; Seal resolves and
// validates every chain once through sealStaticScopes.
func (b *Builder) staticScopeHandle(scope Term) bool {
	switch {
	case b.has(scope, tagCell):
		return b.lexicalCell(scope)
	case b.has(scope, tagTypeAlias), b.has(scope, tagTypeInterface),
		b.has(scope, tagTypeParam), b.has(scope, tagTypeFunction),
		b.has(scope, tagValueClaim), b.has(scope, tagCall),
		b.has(scope, tagAnnotation), b.has(scope, tagFunction):
		return true
	default:
		return false
	}
}

// markStaticTerms computes the transient static containment membership. Static
// query syntax uses the ordinary Program relations, including Function/Body;
// this walk follows that same topology and merely classifies it as unreachable
// executable syntax. No parallel expression or control representation exists.
func (b *Builder) markStaticTerms(scopes *sealStaticScopes) (termSet, error) {
	var marked termSet
	stack := make([]Term, 0, (len(b.typeOfs)+len(b.annotations))*2)
	for i, row := range b.typeOfs {
		typeOf := makeTerm(tagTypeOf, uint32(i+1))
		marked.add(typeOf)
		if scopes.bodyFor(row.scope) == 0 || !b.valueOccurrence(row.operand) {
			return marked, errors.New("program: invalid typeof scope or operand")
		}
		stack = append(stack, row.operand)
	}
	for i, row := range b.annotations {
		annotation := makeTerm(tagAnnotation, uint32(i+1))
		marked.add(annotation)
		if scopes.bodyFor(row.scope) == 0 || !row.filled || !b.has(row.values, tagValues) {
			return marked, errors.New("program: invalid annotation scope or arguments")
		}
		stack = append(stack, row.values)
	}
	push := func(term Term) error {
		if !b.valid(term) {
			return errors.New("program: invalid static expression child")
		}
		stack = append(stack, term)
		return nil
	}
	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if marked.has(term) {
			continue
		}
		marked.add(term)
		switch term.tag() {
		case tagNil, tagBool, tagInteger, tagFloat, tagString, tagKey,
			tagBreak, tagLabel, tagGoto, tagControlFault:
		case tagBody:
			row := b.bodies[term.index()-1]
			for at := row.source.start; at < row.source.end; at++ {
				source := b.sourceTerms[at]
				if !b.statementRoot(source) {
					continue
				}
				if err := push(source); err != nil {
					return marked, err
				}
			}
		case tagCell:
		case tagTypeValue:
			// TypeValue is an atomic dynamic occurrence here. Its authored
			// target is already owned by the static type forest, not the
			// typeof expression tree.
		case tagVararg:
			// The occurrence is static; its Cell remains the owning runtime
			// function's exact vararg identity and is validated at the query
			// frontier rather than copied into a static storage plane.
		case tagRead:
			source := b.reads[term.index()-1].source
			if !b.has(source, tagCell) {
				if err := push(source); err != nil {
					return marked, err
				}
			}
		case tagUnary:
			if err := push(b.unaries[term.index()-1].operand); err != nil {
				return marked, err
			}
		case tagBinary:
			r := b.binaries[term.index()-1]
			if err := push(r.left); err != nil {
				return marked, err
			}
			if err := push(r.right); err != nil {
				return marked, err
			}
		case tagSelect:
			r := b.selects[term.index()-1]
			if err := push(r.left); err != nil {
				return marked, err
			}
			if err := push(r.right); err != nil {
				return marked, err
			}
		case tagValueClaim:
			// Only the dynamic operand belongs to typeof expression
			// containment. The target remains in the static type forest.
			if err := push(b.valueClaims[term.index()-1].operand); err != nil {
				return marked, err
			}
		case tagValues:
			r := b.values[term.index()-1]
			for i := r.fixed.start; i < r.fixed.end; i++ {
				if err := push(b.valueTerms[i]); err != nil {
					return marked, err
				}
			}
			if r.tail != 0 {
				if err := push(r.tail); err != nil {
					return marked, err
				}
			}
		case tagLensExact:
			r := b.lensExact[term.index()-1]
			if err := push(r.base); err != nil {
				return marked, err
			}
			if err := push(r.source); err != nil {
				return marked, err
			}
		case tagLensKey:
			r := b.lensKeys[term.index()-1]
			if err := push(r.base); err != nil {
				return marked, err
			}
			if err := push(r.key); err != nil {
				return marked, err
			}
		case tagCall:
			r := b.calls[term.index()-1]
			if err := push(r.callee); err != nil {
				return marked, err
			}
			if err := push(r.actuals); err != nil {
				return marked, err
			}
		case tagReturn:
			if err := push(b.returns[term.index()-1].values); err != nil {
				return marked, err
			}
		case tagBind:
			row := b.binds[term.index()-1]
			for at := row.cells.start; at < row.cells.end; at++ {
				marked.add(b.bindTerms[at])
			}
			if err := push(row.values); err != nil {
				return marked, err
			}
		case tagAssign:
			row := b.assigns[term.index()-1]
			for at := row.writes.start; at < row.writes.end; at++ {
				write := makeTerm(tagWrite, at)
				target := b.writes[write.index()-1].target
				if b.has(target, tagLensExact) || b.has(target, tagLensKey) {
					if err := push(target); err != nil {
						return marked, err
					}
				}
			}
			if err := push(row.values); err != nil {
				return marked, err
			}
		case tagFunction:
			row := b.functions[term.index()-1]
			for at := row.formals.start; at < row.formals.end; at++ {
				marked.add(b.formalTerms[at])
			}
			for at := row.captures.start; at < row.captures.end; at++ {
				marked.add(b.captures[at].inner)
			}
			if row.vararg != 0 {
				marked.add(row.vararg)
			}
			if err := push(row.body); err != nil {
				return marked, err
			}
		case tagBranch:
			row := b.branches[term.index()-1]
			if err := push(row.condition); err != nil {
				return marked, err
			}
			if err := push(row.whenTrue); err != nil {
				return marked, err
			}
			if err := push(row.whenFalse); err != nil {
				return marked, err
			}
		case tagLoop:
			index := term.index() - 1
			for at := b.loopCellRanges[index].start; at < b.loopCellRanges[index].end; at++ {
				marked.add(b.loopCells[at])
			}
			if err := push(b.loopControls[index]); err != nil {
				return marked, err
			}
			if err := push(b.loopBodies[index]); err != nil {
				return marked, err
			}
		case tagTable:
			r := b.tables[term.index()-1]
			for i := r.fields.start; i < r.fields.end; i++ {
				field := b.tableFieldTerms[i]
				if !b.has(field, tagTableField) {
					return marked, errors.New("program: invalid Table field")
				}
				f := b.tableFields[field.index()-1]
				if err := push(f.key); err != nil {
					return marked, err
				}
				if err := push(f.values); err != nil {
					return marked, err
				}
			}
		default:
			return marked, errors.New("program: unsupported static expression family")
		}
	}
	// Labels are zero-width Body cursor metadata rather than roots. A static
	// Function Body therefore reaches them through ownership, not through the
	// root walk above.
	for index, owner := range b.labelOwners {
		if marked.has(owner) {
			marked.add(makeTerm(tagLabel, uint32(index+1)))
		}
	}
	for index, row := range b.controlFaults {
		if marked.has(row.owner) {
			marked.add(makeTerm(tagControlFault, uint32(index+1)))
		}
	}
	return marked, nil
}

const (
	frontierScope uint8 = iota + 1
	frontierOccurrence
)

type frontierRef struct {
	term Term
	kind uint8
}

type frontierCacheEntry struct {
	value frontierPoint
	state uint8 // 1 resolving, 2 valid, 3 rejected
}

type frontierPathEntry struct {
	ref         frontierRef
	environment lexicalEnvRef
}

// sealFrontiers is Seal's sole lexical-position projection. It derives every
// occurrence coordinate from the already-proved typed claim forest and Body
// roots, then resolves that coordinate through staticVisibility's one shared
// environment authority. No builder caller can forge or retain a frontier.
type sealFrontiers struct {
	b          *Builder
	claims     [tagCount][]sealClaimSlot
	visibility *staticVisibility
	scopes     *sealStaticScopes
	sources    *sealSourceIndex

	cache map[frontierRef]frontierCacheEntry
}

func newSealFrontiers(
	b *Builder,
	claims [tagCount][]sealClaimSlot,
	visibility *staticVisibility,
	scopes *sealStaticScopes,
	sources *sealSourceIndex,
) (*sealFrontiers, error) {
	if b == nil || visibility == nil || sources == nil {
		return nil, errors.New("program: invalid source frontier index")
	}
	f := &sealFrontiers{
		b: b, claims: claims, visibility: visibility, scopes: scopes,
		sources: sources,
		cache:   make(map[frontierRef]frontierCacheEntry),
	}
	return f, nil
}

func (f *sealFrontiers) scopeFrontier(scope Term) (staticFrontier, bool) {
	point, ok := f.scopePoint(scope)
	if !ok {
		return staticFrontier{}, false
	}
	return f.visibility.frontier(point)
}

func (f *sealFrontiers) occurrence(term Term) (staticFrontier, bool) {
	point, ok := f.occurrencePoint(term)
	if !ok {
		return staticFrontier{}, false
	}
	return f.visibility.frontier(point)
}

func (f *sealFrontiers) scopePoint(scope Term) (frontierPoint, bool) {
	return f.resolvePoint(frontierRef{term: scope, kind: frontierScope})
}

func (f *sealFrontiers) occurrencePoint(term Term) (frontierPoint, bool) {
	return f.resolvePoint(frontierRef{term: term, kind: frontierOccurrence})
}

func (f *sealFrontiers) resolvePoint(start frontierRef) (frontierPoint, bool) {
	if f == nil || f.b == nil {
		return frontierPoint{}, false
	}
	path := make([]frontierPathEntry, 0, 8)
	current := start
	var result frontierPoint
	valid := false
	for {
		if !f.b.valid(current.term) || current.term.tag() >= tagCount {
			break
		}
		entry := f.cache[current]
		switch entry.state {
		case 2:
			result, valid = entry.value, true
			goto resolved
		case 3, 1:
			goto resolved
		}
		f.cache[current] = frontierCacheEntry{state: 1}
		next, direct, environment, directOK, terminal := f.next(current)
		path = append(path, frontierPathEntry{
			ref: current, environment: environment,
		})
		if terminal {
			result, valid = direct, directOK
			break
		}
		current = next
	}

resolved:
	stateValue := uint8(3)
	if valid {
		stateValue = 2
	}
	for index := len(path) - 1; index >= 0; index-- {
		entry := path[index]
		if valid && entry.environment.kind != 0 {
			result.environment = entry.environment
		}
		f.cache[entry.ref] = frontierCacheEntry{
			value: result, state: stateValue,
		}
	}
	return result, valid
}

func (f *sealFrontiers) next(ref frontierRef) (
	next frontierRef,
	direct frontierPoint,
	environment lexicalEnvRef,
	ok bool,
	terminal bool,
) {
	b := f.b
	if ref.kind == frontierOccurrence {
		if body, _, cursor, direct := f.sources.direct(ref.term); direct {
			at := bodyGapPoint(body, int(cursor))
			if b.has(ref.term, tagLoop) &&
				b.loopKinds[ref.term.index()-1] == LoopRepeat {
				at.body = b.loopBodies[ref.term.index()-1]
				roots, ok := f.sources.bodyRoots(at.body)
				if !ok {
					return frontierRef{}, frontierPoint{},
						lexicalEnvRef{}, false, true
				}
				at.cursor = len(roots)
				at.environment = lexicalEnvRef{
					kind: envBodyGap, term: at.body, cursor: at.cursor,
				}
			}
			return frontierRef{}, at, lexicalEnvRef{}, true, true
		}
		if ref.term.tag() >= tagCount ||
			int(ref.term.index()) > len(f.claims[ref.term.tag()]) {
			return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
		}
		parent := f.claims[ref.term.tag()][ref.term.index()-1].parent
		if parent != 0 && parent != ref.term {
			return frontierRef{term: parent, kind: frontierOccurrence},
				frontierPoint{}, lexicalEnvRef{}, false, false
		}
		switch ref.term.tag() {
		case tagTypeOf:
			return frontierRef{
					term: b.typeOfs[ref.term.index()-1].scope, kind: frontierScope,
				},
				frontierPoint{}, lexicalEnvRef{}, false, false
		case tagAnnotation:
			return frontierRef{
					term: b.annotations[ref.term.index()-1].scope, kind: frontierScope,
				},
				frontierPoint{}, lexicalEnvRef{}, false, false
		default:
			return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
		}
	}

	scope := ref.term
	switch {
	case b.has(scope, tagCell):
		if _, global := b.globalCellKey(scope); global {
			return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
		}
		at := f.visibility.cellPoint[scope.index()]
		role := f.visibility.cellRole[scope.index()]
		if !role.valid() || role == CellGlobal || at.body == 0 {
			return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
		}
		return frontierRef{}, at, lexicalEnvRef{}, true, true
	case b.has(scope, tagTypeAlias):
		body, _, cursor, valid := f.sources.direct(scope)
		owner, declared := b.staticDeclarationScope(scope)
		valid = valid && declared && body == owner
		return frontierRef{}, queryPoint(
			envAliasQuery, scope, body, int(cursor),
		), lexicalEnvRef{}, valid, true
	case b.has(scope, tagTypeInterface):
		body, _, cursor, valid := f.sources.direct(scope)
		owner, declared := b.staticInterfaceScope(scope)
		valid = valid && declared && body == owner
		return frontierRef{}, queryPoint(
			envInterfaceQuery, scope, body, int(cursor),
		), lexicalEnvRef{}, valid, true
	case b.has(scope, tagTypeParam):
		owner := b.typeParams[scope.index()-1].owner
		kind := frontierScope
		var environment lexicalEnvRef
		switch owner.tag() {
		case tagTypeAlias:
			environment = lexicalEnvRef{kind: envAliasQuery, term: owner}
		case tagTypeFunction:
			environment = lexicalEnvRef{kind: envSignatureQuery, term: owner}
		case tagFunction:
			environment = lexicalEnvRef{kind: envFunctionGeneric, term: owner}
		default:
			return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
		}
		if b.has(owner, tagFunction) {
			kind = frontierOccurrence
		}
		return frontierRef{term: owner, kind: kind}, frontierPoint{},
			environment, false, false
	case b.has(scope, tagTypeFunction):
		return frontierRef{
				term: b.signatures[scope.index()-1].scope, kind: frontierScope,
			},
			frontierPoint{},
			lexicalEnvRef{kind: envSignatureQuery, term: scope},
			false, false
	case b.has(scope, tagValueClaim), b.has(scope, tagCall):
		return frontierRef{term: scope, kind: frontierOccurrence},
			frontierPoint{}, lexicalEnvRef{}, false, false
	case b.has(scope, tagAnnotation):
		return frontierRef{term: b.annotations[scope.index()-1].scope, kind: frontierScope},
			frontierPoint{}, lexicalEnvRef{}, false, false
	case b.has(scope, tagFunction):
		return frontierRef{}, functionHeaderPoint(
			scope, b.functions[scope.index()-1].body,
		), lexicalEnvRef{}, true, true
	default:
		return frontierRef{}, frontierPoint{}, lexicalEnvRef{}, false, true
	}
}

// validateStaticQueries derives lexical frontiers for TypeOf and Annotation
// roots. Nested Function bodies retain ordinary lexical control validation;
// only roots with no executable Body cursor are checked here.
func (b *Builder) validateStaticQueries(
	claims [tagCount][]sealClaimSlot,
	static termSet,
	visibility staticVisibility,
	scopes *sealStaticScopes,
	frontiers *sealFrontiers,
	bindings sealFunctionBindings,
) error {
	if len(b.typeOfs) == 0 && len(b.annotations) == 0 {
		return nil
	}
	for i, row := range b.typeOfs {
		typeOf := makeTerm(tagTypeOf, uint32(i+1))
		if !static.has(typeOf) || claims[tagTypeOf][i].parent != typeOf {
			return errors.New("program: invalid typeof containment")
		}
		at, ok := frontiers.scopeFrontier(row.scope)
		if !ok || scopes.bodyFor(row.scope) != at.body {
			return errors.New("program: invalid typeof scope frontier")
		}
	}
	for i, row := range b.annotations {
		annotation := makeTerm(tagAnnotation, uint32(i+1))
		if !static.has(annotation) || claims[tagAnnotation][i].parent != annotation {
			return errors.New("program: invalid annotation containment")
		}
		at, ok := frontiers.scopeFrontier(row.scope)
		if !ok || scopes.bodyFor(row.scope) != at.body {
			return errors.New("program: invalid annotation scope frontier")
		}
	}
	// Each static source relation resolves through the same occurrence
	// projection used by Call, ValueClaim, Function, and TypePublication.
	for readIndex, read := range b.reads {
		term := makeTerm(tagRead, uint32(readIndex+1))
		if !static.has(term) {
			continue
		}
		at, found := frontiers.occurrence(term)
		if !found {
			return errors.New("program: static Read has no lexical frontier")
		}
		if at.body == 0 || read.owner != at.body || (b.lexicalCell(read.source) && !visibility.visible(at, read.source)) {
			return errors.New("program: static Read is not visible at query scope")
		}
	}
	for _, read := range b.implicitReads {
		if b.has(read, tagRead) && static.has(read) {
			return errors.New("program: static Read cannot be implicit-global evidence")
		}
	}
	for index, row := range b.varargs {
		vararg := makeTerm(tagVararg, uint32(index+1))
		if !static.has(vararg) {
			continue
		}
		at, found := frontiers.occurrence(vararg)
		if !found {
			return errors.New("program: static Vararg has no lexical frontier")
		}
		if row.owner != at.body || !visibility.visible(at, row.cell) {
			return errors.New("program: static Vararg is not visible at query scope")
		}
	}
	if len(bindings.cellForFunction) != len(b.functions) {
		return errors.New("program: invalid static Function binding projection")
	}
	for index, row := range b.functions {
		function := makeTerm(tagFunction, uint32(index+1))
		if !static.has(function) || row.captures.start == row.captures.end {
			continue
		}
		at, found := frontiers.occurrence(function)
		if !found {
			return errors.New("program: static Function has no lexical frontier")
		}
		for capture := row.captures.start; capture < row.captures.end; capture++ {
			outer := b.captures[capture].outer
			if !visibility.visible(at, outer) && outer != bindings.cellForFunction[index] {
				return errors.New("program: static Function capture is not visible at query scope")
			}
		}
	}
	return nil
}

// TypeOf reports the exact static host and ordinary expression operand.
func (p *Program) TypeOf(term Term) (scope, operand Term, ok bool) {
	if !p.has(term, tagTypeOf) {
		return 0, 0, false
	}
	r := p.typeOfs[term.index()-1]
	return r.scope, r.operand, true
}

func (p *Program) TypeOfCount() int {
	if p == nil {
		return 0
	}
	return len(p.typeOfs)
}
func (p *Program) TypeOfAt(index int) (Term, bool) {
	return familyTerm(tagTypeOf, p.TypeOfCount(), index)
}
