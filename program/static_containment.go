package program

import "errors"

// staticSet is the sole transient/persisted classification plane. It is not a
// topology: each bit says only that a normal Term is exclusively static.
// Families allocate backing words lazily, so ordinary programs allocate none.
type staticSet [tagCount][]uint64

func (s *staticSet) add(term Term) {
	if term == 0 || term.tag() >= tagCount {
		return
	}
	word := int((term.index() - 1) >> 6)
	if word >= len(s[term.tag()]) {
		s[term.tag()] = append(s[term.tag()], make([]uint64, word+1-len(s[term.tag()]))...)
	}
	s[term.tag()][word] |= uint64(1) << uint((term.index()-1)&63)
}
func (s staticSet) has(term Term) bool {
	if term == 0 || term.tag() >= tagCount {
		return false
	}
	word := int((term.index() - 1) >> 6)
	return word < len(s[term.tag()]) && s[term.tag()][word]&(uint64(1)<<uint((term.index()-1)&63)) != 0
}

// typeOfRow is authored static type syntax. scope is deliberately the exact
// declaration term which owns the type tree; it is not a surrogate Body.
// operand is an ordinary expression occurrence whose containment is static.
type typeOfRow struct{ scope, operand Term }

// TypeOf records parser-reachable typeof(expr). The expression is built with
// the ordinary source expression relations, so it has its normal lexical Body
// owner and span. scope is the declaration/signature host for the type tree.
// This initial contract accepts the existing host terms that already carry an
// exact lexical Cell attachment host. Function is intentionally not accepted:
// its generic-constraint and after-formals phases are distinct source sites.
// Future declaration/signature rows add their exact host terms directly.
func (b *Builder) TypeOf(span Span, scope, operand Term) Term {
	if b == nil || !b.require(b.staticScopeBody(scope) != 0 && b.valueOccurrence(operand)) {
		return 0
	}
	if b.has(operand, tagFunction) {
		// A Function includes an executable Body authority today. Marking it
		// static would require binder evidence separating source-only function
		// literals from activated closures; do not approximate that distinction.
		b.poison = true
		return 0
	}
	b.typeOfs = append(b.typeOfs, typeOfRow{scope: scope, operand: operand})
	term := b.mint(tagTypeOf, span, b.familyIndex(len(b.typeOfs)))
	if term == 0 {
		b.typeOfs = b.typeOfs[:len(b.typeOfs)-1]
	}
	return term
}

func (b *Builder) staticScopeBody(scope Term) Term {
	for scope != 0 {
		switch {
		case b.has(scope, tagCell) && b.lexicalCell(scope):
			return b.cells[scope.index()-1].storage
		case b.has(scope, tagTypeAlias):
			body, _, ok := b.staticDeclarationScope(scope)
			if ok {
				return body
			}
			return 0
		case b.has(scope, tagTypeParam):
			scope = b.typeParams[scope.index()-1].owner
		case b.has(scope, tagTypeFunction):
			scope = b.signatures[scope.index()-1].scope
		default:
			return 0
		}
	}
	return 0
}

func (b *Builder) staticScopeValid(scope Term) bool {
	return b.staticScopeBody(scope) != 0
}

// markStaticTerms computes the transient static containment membership. It
// deliberately retains no topology in Program. Every expression descendant is
// marked so ordinary claim construction can enforce one parent and label the
// entire tree static.
func (b *Builder) markStaticTerms() (staticSet, error) {
	var marked staticSet
	stack := make([]Term, 0, len(b.typeOfs)*2)
	for i, row := range b.typeOfs {
		typeOf := makeTerm(tagTypeOf, uint32(i+1))
		marked.add(typeOf)
		if b.staticScopeBody(row.scope) == 0 || !b.valueOccurrence(row.operand) {
			return marked, errors.New("program: invalid typeof scope or operand")
		}
		stack = append(stack, row.operand)
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
		case tagNil, tagBool, tagInteger, tagFloat, tagString, tagKey:
		case tagVararg:
			return marked, errors.New("program: static vararg requires binder static identity evidence")
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
		case tagTable:
			r := b.tables[term.index()-1]
			for i := r.fields.start; i < r.fields.end; i++ {
				f := b.tableFields[i]
				if err := push(f.key); err != nil {
					return marked, err
				}
				if err := push(f.values); err != nil {
					return marked, err
				}
			}
		case tagFunction:
			return marked, errors.New("program: typeof function literal requires source-only function binder evidence")
		default:
			return marked, errors.New("program: unsupported static expression family")
		}
	}
	return marked, nil
}

// validateStaticTypeOf derives the lexical Body and declaration frontier from
// the authored host Term. Nothing about that derivation is persisted: static
// containment is a Seal proof, not a second program topology.
func (b *Builder) validateStaticTypeOf(claims [tagCount][]sealClaimSlot, static staticSet) error {
	if len(b.typeOfs) == 0 {
		return nil
	}
	type frontier struct {
		body         Term
		cursor       int
		afterFormals bool
	}
	cellFrontier := make([]frontier, len(b.cells)+1)
	cellRole := make([]uint8, len(b.cells)+1) // 1 Bind, 2 formal, 3 vararg; others are not source hosts here.
	bindCursor := make([]int, len(b.binds))
	for i := range bindCursor {
		bindCursor[i] = -1
	}
	for bodyIndex, row := range b.bodies {
		for cursor, at := 0, row.roots.start; at < row.roots.end; cursor, at = cursor+1, at+1 {
			root := b.bodyTerms[at]
			if b.has(root, tagBind) {
				bindCursor[root.index()-1] = cursor
				if b.binds[root.index()-1].owner != makeTerm(tagBody, uint32(bodyIndex+1)) {
					return errors.New("program: Bind root owner mismatch")
				}
			}
		}
	}
	for bindIndex, row := range b.binds {
		cursor := bindCursor[bindIndex]
		if cursor < 0 {
			return errors.New("program: invalid Cell declaration root")
		}
		for at := row.cells.start; at < row.cells.end; at++ {
			cell := b.bindTerms[at]
			if !b.lexicalCell(cell) || cellFrontier[cell.index()].body != 0 {
				return errors.New("program: ambiguous Cell declaration")
			}
			cellFrontier[cell.index()] = frontier{body: row.owner, cursor: cursor}
			cellRole[cell.index()] = 1
		}
	}
	for _, row := range b.functions {
		for at := row.formals.start; at < row.formals.end; at++ {
			cell := b.formalTerms[at]
			if cellFrontier[cell.index()].body != 0 {
				return errors.New("program: ambiguous formal Cell declaration")
			}
			cellFrontier[cell.index()] = frontier{body: row.body, cursor: -1}
			cellRole[cell.index()] = 2
		}
		if row.vararg != 0 {
			if cellFrontier[row.vararg.index()].body != 0 {
				return errors.New("program: ambiguous vararg Cell declaration")
			}
			cellFrontier[row.vararg.index()] = frontier{body: row.body, cursor: -1}
			cellRole[row.vararg.index()] = 3
		}
	}
	for loopIndex, r := range b.loopCellRanges {
		body := b.loopBodies[loopIndex]
		for at := r.start; at < r.end; at++ {
			cell := b.loopCells[at]
			if cellFrontier[cell.index()].body != 0 {
				return errors.New("program: ambiguous Loop Cell declaration")
			}
			cellFrontier[cell.index()] = frontier{body: body, cursor: -1}
		}
	}
	scopeFrontier := func(scope Term) (frontier, bool) {
		for scope != 0 {
			switch {
			case b.has(scope, tagCell):
				if _, ok := b.globalCellKey(scope); ok {
					return frontier{}, false
				}
				f := cellFrontier[scope.index()]
				if cellRole[scope.index()] == 0 {
					return frontier{}, false
				}
				f.afterFormals = cellRole[scope.index()] == 2 || cellRole[scope.index()] == 3
				if f.afterFormals {
					f.cursor = 0
				}
				return f, f.body != 0
			case b.has(scope, tagTypeAlias):
				body, gap, ok := b.staticDeclarationScope(scope)
				return frontier{body: body, cursor: int(gap)}, ok
			case b.has(scope, tagTypeParam):
				scope = b.typeParams[scope.index()-1].owner
			case b.has(scope, tagTypeFunction):
				scope = b.signatures[scope.index()-1].scope
			default:
				return frontier{}, false
			}
		}
		return frontier{}, false
	}
	visible := func(at frontier, cell Term) bool {
		if _, ok := b.globalCellKey(cell); ok {
			return true
		}
		if !b.lexicalCell(cell) {
			return false
		}
		decl := cellFrontier[cell.index()]
		if decl.body == 0 {
			return false
		}
		if decl.body == at.body {
			return (at.afterFormals && (cellRole[cell.index()] == 2 || cellRole[cell.index()] == 3)) || decl.cursor < at.cursor
		}
		return false
	}
	typeOfFrontier := make([]frontier, len(b.typeOfs))
	for i, row := range b.typeOfs {
		typeOf := makeTerm(tagTypeOf, uint32(i+1))
		if !static.has(typeOf) || claims[tagTypeOf][i].parent != typeOf {
			return errors.New("program: invalid typeof containment")
		}
		at, ok := scopeFrontier(row.scope)
		if !ok || b.staticScopeBody(row.scope) != at.body {
			return errors.New("program: invalid typeof scope frontier")
		}
		typeOfFrontier[typeOf.index()-1] = at
	}
	// Each static Read is classified once. staticContainmentRoot path-compresses
	// through the transient ledger, making this linear in the static forest.
	for readIndex, read := range b.reads {
		if !static.has(makeTerm(tagRead, uint32(readIndex+1))) {
			continue
		}
		root, err := staticContainmentRoot(claims, static, makeTerm(tagRead, uint32(readIndex+1)))
		if err != nil {
			return err
		}
		if !b.has(root, tagTypeOf) {
			return errors.New("program: static Read has non-typeof root")
		}
		at := typeOfFrontier[root.index()-1]
		if at.body == 0 || read.owner != at.body || (b.lexicalCell(read.source) && !visible(at, read.source)) {
			return errors.New("program: static Read is not visible at typeof scope")
		}
	}
	for _, read := range b.implicitReads {
		if b.has(read, tagRead) && static.has(read) {
			return errors.New("program: static Read cannot be implicit-global evidence")
		}
	}
	return nil
}

func staticContainmentRoot(claims [tagCount][]sealClaimSlot, static staticSet, start Term) (Term, error) {
	current := start
	for {
		if current == 0 || current.tag() >= tagCount || int(current.index()) > len(claims[current.tag()]) {
			return 0, errors.New("program: invalid static containment parent")
		}
		slot := claims[current.tag()][current.index()-1]
		if !static.has(current) || slot.parent == 0 {
			return 0, errors.New("program: source term has no static root")
		}
		if slot.parent == current {
			root := current
			for current = start; current != root; {
				parent := claims[current.tag()][current.index()-1].parent
				claims[current.tag()][current.index()-1].parent = root
				current = parent
			}
			return root, nil
		}
		current = slot.parent
	}
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
