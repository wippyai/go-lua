package program

// Capture is one exact lexical closure edge.
type Capture struct{ Inner, Outer Term }

type captureRange struct{ start, end uint32 }

// functionRow is the sole executable callable relation. Its declaration is
// minted at the outer frontier; FillFunction later installs the child Body,
// cells, and captures after generic constraints have been lowered.
type functionRow struct {
	owner, body, vararg Term
	formals             termRange
	captures            captureRange
	typeParams          termRange
	returns             termRange
	outerGap            uint32
	outerGapSet         bool
	typeParamsSet       bool
	returnsSet          bool
	returnsKnown        bool
}

func (b *Builder) DeclareFunction(span Span, owner Term) Term {
	if !b.require(b.has(owner, tagBody)) {
		return 0
	}
	b.functions = append(b.functions, functionRow{owner: owner})
	term := b.mint(tagFunction, span, b.familyIndex(len(b.functions)))
	if term == 0 {
		b.functions = b.functions[:len(b.functions)-1]
	}
	return term
}

// SetFunctionOuterGap retains the parent Body cursor at which a function
// literal was declared. It is only the pre-formal generic-constraint frontier;
// ordinary Function header types use the filled child Body instead.
func (b *Builder) SetFunctionOuterGap(function Term, gap int) bool {
	if !b.has(function, tagFunction) || gap < 0 || uint64(gap) > uint64(^uint32(0)) {
		b.poison = true
		return false
	}
	r := &b.functions[function.index()-1]
	if r.outerGapSet || r.body != 0 {
		b.poison = true
		return false
	}
	r.outerGap, r.outerGapSet = uint32(gap), true
	return true
}

func (b *Builder) FillFunction(function, body Term, formals []Term, vararg Term, captures []Capture) bool {
	if !b.has(function, tagFunction) || !b.require(b.has(body, tagBody) && b.functions[function.index()-1].owner != body) {
		return false
	}
	row := &b.functions[function.index()-1]
	if row.body != 0 || row.outerGapSet != row.typeParamsSet {
		b.poison = true
		return false
	}
	if vararg != 0 && !b.require(b.lexicalCell(vararg)) {
		return false
	}
	for _, formal := range formals {
		if !b.require(b.lexicalCell(formal)) {
			return false
		}
	}
	for _, capture := range captures {
		if !b.require(b.lexicalCell(capture.Inner) && b.lexicalCell(capture.Outer)) {
			return false
		}
	}
	formalsRange, ok := b.appendPool(&b.formalTerms, formals)
	if !ok {
		return false
	}
	captureRange, ok := b.appendCaptures(captures)
	if !ok {
		b.formalTerms = b.formalTerms[:formalsRange.start]
		return false
	}
	row.body, row.vararg, row.formals, row.captures = body, vararg, formalsRange, captureRange
	return true
}

func (b *Builder) SetFunctionGenerics(function Term, params []Term) bool {
	if !b.has(function, tagFunction) {
		b.poison = true
		return false
	}
	r := &b.functions[function.index()-1]
	if r.typeParamsSet || r.body != 0 || !r.outerGapSet {
		b.poison = true
		return false
	}
	for _, param := range params {
		if !b.has(param, tagTypeParam) || b.typeParams[param.index()-1].owner != function {
			b.poison = true
			return false
		}
	}
	range_, ok := b.appendPool(&b.typeParamTerms, params)
	if !ok {
		return false
	}
	r.typeParams, r.typeParamsSet = range_, true
	return true
}

func (b *Builder) SetFunctionReturns(function Term, returnsKnown bool, returns []Term) bool {
	if !b.has(function, tagFunction) {
		b.poison = true
		return false
	}
	r := &b.functions[function.index()-1]
	if r.returnsSet || r.body == 0 || (!returnsKnown && len(returns) != 0) {
		b.poison = true
		return false
	}
	for _, result := range returns {
		if !b.staticTypeNode(result) {
			b.poison = true
			return false
		}
	}
	range_, ok := b.appendPool(&b.staticTypeTerms, returns)
	if !ok {
		return false
	}
	r.returns, r.returnsKnown, r.returnsSet = range_, returnsKnown, true
	return true
}

func (p *Program) Function(term Term) (owner, body, vararg Term, ok bool) {
	if !p.has(term, tagFunction) {
		return 0, 0, 0, false
	}
	r := p.functions[term.index()-1]
	return r.owner, r.body, r.vararg, true
}

func (p *Program) FormalLen(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].formals
	return int(r.end - r.start), true
}

func (p *Program) FormalAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagFunction) || index < 0 {
		return 0, false
	}
	r := p.functions[term.index()-1].formals
	at := r.start + uint32(index)
	if at >= r.end {
		return 0, false
	}
	return p.formalTerms[at], true
}

func (p *Program) FunctionTypeParamCount(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].typeParams
	return int(r.end - r.start), true
}

func (p *Program) FunctionTypeParamAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagFunction) || index < 0 {
		return 0, false
	}
	r := p.functions[term.index()-1].typeParams
	at := r.start + uint32(index)
	if at >= r.end {
		return 0, false
	}
	return p.typeParamTerms[at], true
}

func (p *Program) FunctionReturnsKnown(term Term) (bool, bool) {
	if !p.has(term, tagFunction) {
		return false, false
	}
	return p.functions[term.index()-1].returnsKnown, true
}

func (p *Program) FunctionReturnCount(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].returns
	return int(r.end - r.start), true
}

func (p *Program) FunctionReturnAt(term Term, index int) (Term, bool) {
	if !p.has(term, tagFunction) || index < 0 {
		return 0, false
	}
	r := p.functions[term.index()-1].returns
	at := r.start + uint32(index)
	if at >= r.end {
		return 0, false
	}
	return p.staticTypeTerms[at], true
}

func (p *Program) FunctionCaptureCount(term Term) (int, bool) {
	if !p.has(term, tagFunction) {
		return 0, false
	}
	r := p.functions[term.index()-1].captures
	return int(r.end - r.start), true
}

func (p *Program) FunctionCapture(term Term, index int) (inner, outer Term, ok bool) {
	if !p.has(term, tagFunction) || index < 0 {
		return 0, 0, false
	}
	r := p.functions[term.index()-1].captures
	at := r.start + uint32(index)
	if at >= r.end {
		return 0, 0, false
	}
	row := p.captures[at]
	return row.inner, row.outer, true
}
