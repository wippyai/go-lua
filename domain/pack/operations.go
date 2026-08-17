package pack

// This file contains Pack's small, structural list-language operations.  The
// operations never inspect Value/Heap/Call/Static facts: they consume sealed
// Pack terms and return terms in the same Schema owner.

// Term returns the complete Pack expression written at root by one non-extreme
// fact.  It is intentionally a read-only projection; callers must use Builder
// to publish a term at another root.
func (schema *Schema) Term(root Root, value Value) (Term, bool) {
	if schema == nil || schema.state == nil || !schema.Admit(root, value) || value.bottom || value.top {
		return Term{}, false
	}
	if root.schema != schema.state {
		return Term{}, false
	}
	port := schema.state.roots[root.index].port
	for _, current := range value.cases {
		if term, ok := casePortTerm(current, port); ok {
			return term, true
		}
	}
	return Term{}, false
}

// Terms exposes every antichain expression at root without exposing Cases or
// equation targets. The returned slice is detached so callers cannot mutate a
// recurrent fact through this observation.
func (schema *Schema) Terms(root Root, value Value) ([]Term, bool) {
	if schema == nil || schema.state == nil || !schema.Admit(root, value) {
		return nil, false
	}
	if value.bottom || value.top {
		return nil, true
	}
	port := schema.state.roots[root.index].port
	terms := make([]Term, 0, len(value.cases))
	for _, current := range value.cases {
		term, ok := casePortTerm(current, port)
		if !ok {
			return nil, false
		}
		terms = append(terms, term)
	}
	return terms, true
}

// Scalar projects one exact scalar Cell equation from a complete relation
// case. It is a read-only marginal, so callers cannot reassemble it into a
// new Pack Value without the output Builder's complete target vector.
func (schema *Schema) Scalar(root Root, value Value, endpoint Endpoint) (Scalar, bool) {
	if schema == nil || schema.state == nil || !schema.Admit(root, value) || !endpoint.valid() || endpoint.owner != schema.state.owner {
		return Scalar{}, false
	}
	if value.bottom || value.top {
		return Scalar{}, false
	}
	for _, current := range value.cases {
		if !current.valid() {
			return Scalar{}, false
		}
		for _, equation := range current.equations {
			if equation.kind == EquationScalar && equation.endpoint == endpoint {
				return equation.scalar, equation.scalar.valid()
			}
		}
	}
	return Scalar{}, false
}

// Pack publishes one exact term at root. It is the mirror of Term and keeps
// the complete target-vector admission in Builder rather than in a Rule.
func (builder Builder) PackTerm(term Term) (Value, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner {
		return Value{}, false
	}
	relationIndex, relationOK := builder.schema.state.relationIndex[builder.relation]
	if !relationOK || uint64(relationIndex) >= uint64(len(builder.schema.state.roots)) {
		return Value{}, false
	}
	port := builder.schema.state.roots[relationIndex].port
	equation, ok := builder.Pack(port, term)
	if !ok {
		return Value{}, false
	}
	caseValue, ok := builder.Case(equation)
	if !ok {
		return Value{}, false
	}
	return builder.Value(caseValue)
}

// ScalarAt applies Lua's zero-based Pack selection law. Closed terms nil-fill
// past their end; open terms preserve the exact shared tail subject and its
// adjusted offset. A dynamic/Any term remains class-unknown.
func (builder Builder) ScalarAt(term Term, index TableIndex) (Scalar, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || !index.valid() || index.offset.owner != term.owner {
		return Scalar{}, false
	}
	return projectTermTableIndex(term, index)
}

// ScalarAlternatives is the exact finite marginal for one table position.
// Unlike ScalarAt it preserves open-tail suffix branches rather than joining
// them into a class-only Scalar. Callers publish the returned alternatives as
// a Value union when the surrounding transformation can retain that union.
func (builder Builder) ScalarAlternatives(term Term, index TableIndex) ([]Scalar, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || !index.valid() || index.offset.owner != term.owner {
		return nil, false
	}
	return projectTermTableIndexAlternatives(term, index)
}

// Splice implements the two Lua Values list modes. With final=false every
// expression is scalarized once (the ordinary non-final list rule). With
// final=true all preceding expressions are scalarized and the final
// expression contributes its complete Pack, including an open tail and
// end-relative suffix.
func (builder Builder) Splice(terms []Term, final bool) (Term, bool) {
	if !builder.valid() {
		return Term{}, false
	}
	if len(terms) == 0 {
		return builder.Closed()
	}
	for _, term := range terms {
		if !term.valid() || term.owner != builder.relation.owner {
			return Term{}, false
		}
	}
	if !final {
		zero, ok := builder.Zero()
		if !ok {
			return Term{}, false
		}
		prefix := make([]Scalar, 0, len(terms))
		for _, term := range terms {
			scalar, scalarOK := builder.ScalarAt(term, mustTableIndex(builder, zero))
			if !scalarOK {
				return Term{}, false
			}
			prefix = append(prefix, scalar)
		}
		return builder.Closed(prefix...)
	}

	zero, ok := builder.Zero()
	if !ok {
		return Term{}, false
	}
	prefix := make([]Scalar, 0, len(terms))
	for _, term := range terms[:len(terms)-1] {
		scalar, scalarOK := builder.ScalarAt(term, mustTableIndex(builder, zero))
		if !scalarOK {
			return Term{}, false
		}
		prefix = append(prefix, scalar)
	}
	last := terms[len(terms)-1]
	switch last.kind {
	case TermClosed:
		prefix = append(prefix, last.prefix...)
		return builder.Closed(prefix...)
	case TermOpen:
		prefix = append(prefix, last.prefix...)
		return builder.Open(prefix, last.rest, last.suffix)
	case TermAny:
		if len(prefix) == 0 {
			return builder.AnyPack()
		}
		rest, restOK := builder.AnyTail(builder.schema.state.owner.classes.AnyValue())
		if !restOK {
			return Term{}, false
		}
		return builder.Open(prefix, rest, nil)
	default:
		return Term{}, false
	}
}

// mustTableIndex turns a presealed zero Offset into the corresponding table
// selector. It is called only after Builder.Zero succeeded.
func mustTableIndex(builder Builder, offset Offset) TableIndex {
	index, _ := tableIndexForOffset(offset)
	return index
}

// Bind returns the fixed Cell-facing Pack and the residual Pack after a
// fixed-width Lua bind. The fixed side always has exactly width slots and
// nil-fills a closed input. For an open input the residual retains the same
// tail subject and the exact adjusted offset; no tail is scalarized away.
func (builder Builder) Bind(term Term, width int) (fixed Term, residual Term, ok bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || width < 0 {
		return Term{}, Term{}, false
	}
	alternatives, alternativesOK := builder.BindAlternatives(term, width)
	if !alternativesOK || len(alternatives) != 1 {
		return Term{}, Term{}, false
	}
	fixedScalars := alternatives[0].fixed
	fixed, fixedOK := builder.Closed(fixedScalars...)
	if !fixedOK {
		return Term{}, Term{}, false
	}
	return fixed, alternatives[0].residual, true
}

// BindAlternative keeps one exact correlation between the fixed scalar Cells
// and the residual Pack branch. It is the finite carrier needed when an open
// tail may end in a suffix; callers must publish each alternative separately,
// never cross-product independent scalar and residual lists.
type BindAlternative struct {
	fixed    []Scalar
	residual Term
}

func (alternative BindAlternative) FixedCount() int { return len(alternative.fixed) }
func (alternative BindAlternative) FixedAt(index int) (Scalar, bool) {
	if index < 0 || index >= len(alternative.fixed) {
		return Scalar{}, false
	}
	value := alternative.fixed[index]
	return value, value.valid()
}
func (alternative BindAlternative) Residual() (Term, bool) {
	return alternative.residual, alternative.residual.valid()
}

func (builder Builder) BindAlternatives(term Term, width int) ([]BindAlternative, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || width < 0 {
		return nil, false
	}
	if term.kind == TermClosed || term.kind == TermAny {
		fixed := make([]Scalar, width)
		for index := range fixed {
			table, tableOK := builder.schema.TableIndex(int64(index))
			scalar, scalarOK := builder.ScalarAt(term, table)
			if !tableOK || !scalarOK {
				return nil, false
			}
			fixed[index] = scalar
		}
		var residual Term
		var residualOK bool
		if term.kind == TermClosed {
			residual, residualOK = builder.Closed()
		} else {
			residual, residualOK = builder.AnyPack()
		}
		if !residualOK {
			return nil, false
		}
		return []BindAlternative{{fixed: fixed, residual: residual}}, true
	}
	if term.kind != TermOpen {
		return nil, false
	}
	if width < len(term.prefix) {
		residual, residualOK := builder.Open(term.prefix[width:], term.rest, term.suffix)
		if !residualOK {
			return nil, false
		}
		return []BindAlternative{{fixed: append([]Scalar(nil), term.prefix[:width]...), residual: residual}}, true
	}
	delta := width - len(term.prefix)
	longResiduals, residualOK := builder.DropAlternatives(term, width)
	if !residualOK || len(longResiduals) == 0 {
		return nil, false
	}
	alternatives := make([]BindAlternative, 0, len(longResiduals))
	appendAlternative := func(fixed []Scalar, residual Term) {
		for _, existing := range alternatives {
			if len(existing.fixed) != len(fixed) || !existing.residual.Equal(residual) {
				continue
			}
			equalFixed := true
			for index := range fixed {
				if !equalScalar(existing.fixed[index], fixed[index]) {
					equalFixed = false
					break
				}
			}
			if equalFixed {
				return
			}
		}
		alternatives = append(alternatives, BindAlternative{fixed: fixed, residual: residual})
	}
	longFixed, fixedOK := builder.openBranchScalars(term, delta, -1)
	if !fixedOK {
		return nil, false
	}
	appendAlternative(longFixed, longResiduals[0])
	// DropAlternatives names each short branch by the number of known
	// suffix slots skipped after the symbolic middle ended.  Keep that same
	// branch coordinate here: the fixed Cells and residual Pack are one
	// correlated alternative, never independent scalar/residual products.
	for start := 1; start <= delta; start++ {
		middleLength := delta - start
		fixed, fixedOK := builder.openBranchScalars(term, delta, middleLength)
		if !fixedOK || start >= len(longResiduals) {
			return nil, false
		}
		appendAlternative(fixed, longResiduals[start])
	}
	return alternatives, true
}

// openBranchScalars returns the fixed prefix plus one exact scalar for each
// consumed middle position. middleLength=-1 denotes the long residual branch
// whose shared tail still supplies every consumed position.
func (builder Builder) openBranchScalars(term Term, delta, middleLength int) ([]Scalar, bool) {
	if term.kind != TermOpen || delta < 0 || (middleLength < -1 || middleLength > delta) {
		return nil, false
	}
	fixed := append([]Scalar(nil), term.prefix...)
	for offset := 0; offset < delta; offset++ {
		if middleLength < 0 || offset < middleLength {
			scalar, scalarOK := builder.openHeadScalar(term, offset)
			if !scalarOK {
				return nil, false
			}
			fixed = append(fixed, scalar)
			continue
		}
		suffixIndex := offset - middleLength
		if suffixIndex < len(term.suffix) {
			scalar := term.suffix[suffixIndex]
			if !scalar.valid() {
				return nil, false
			}
			fixed = append(fixed, scalar)
			continue
		}
		nilScalar, nilOK := anyScalar(term.owner, term.owner.classes.Nil())
		if !nilOK {
			return nil, false
		}
		fixed = append(fixed, nilScalar)
	}
	return fixed, true
}

func (builder Builder) openHeadScalar(term Term, offset int) (Scalar, bool) {
	if term.kind != TermOpen || offset < 0 {
		return Scalar{}, false
	}
	switch term.rest.kind {
	case RestTail:
		delta, deltaOK := offsetForUint64(term.owner, uint64(offset))
		adjusted, adjustedOK := addOffsets(term.rest.offset, delta)
		if !deltaOK || !adjustedOK {
			return Scalar{}, false
		}
		return headScalar(term.rest.tail, adjusted)
	case RestAny:
		class, classOK := term.owner.joinClass(term.rest.class, term.owner.classes.Nil())
		if !classOK {
			return Scalar{}, false
		}
		return anyScalar(term.owner, class)
	default:
		return Scalar{}, false
	}
}

// Take returns the first count adjusted slots, preserving nil-fill and
// widening a dynamic term only when the source term itself is dynamic.
func (builder Builder) Take(term Term, count int) (Term, bool) {
	fixed, _, ok := builder.Bind(term, count)
	return fixed, ok
}

// Drop returns the exact residual Pack after count slots. Closed terms become
// an empty Closed term; open tails retain their original subject and offset.
func (builder Builder) Drop(term Term, count int) (Term, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || count < 0 {
		return Term{}, false
	}
	alternatives, ok := builder.DropAlternatives(term, count)
	if !ok || len(alternatives) == 0 {
		return Term{}, false
	}
	if len(alternatives) == 1 {
		return alternatives[0], true
	}
	// A single Term cannot encode the finite union of long-middle and
	// short-suffix residuals without overapproximating it. Return the exact
	// alternatives through DropAlternatives; callers that require one result
	// must handle the rejected union explicitly rather than silently widening.
	return Term{}, false
}

// DropAlternatives retains the finite suffix alternatives which a symbolic
// open middle can expose when the requested position is beyond its actual
// length. The long-middle branch remains one exact open term; short-middle
// branches are exact closed suffixes in their causal order.
func (builder Builder) DropAlternatives(term Term, count int) ([]Term, bool) {
	if !builder.valid() || !term.valid() || term.owner != builder.relation.owner || count < 0 {
		return nil, false
	}
	switch term.kind {
	case TermClosed:
		if count >= len(term.prefix) {
			closed, ok := builder.Closed()
			return []Term{closed}, ok
		}
		closed, ok := builder.Closed(term.prefix[count:]...)
		return []Term{closed}, ok
	case TermAny:
		any, ok := builder.AnyPack()
		return []Term{any}, ok
	case TermOpen:
		return dropOpenAlternatives(builder, term, count)
	default:
		return nil, false
	}
}

func dropOpenAlternatives(builder Builder, term Term, count int) ([]Term, bool) {
	if term.kind != TermOpen || count < 0 {
		return nil, false
	}
	if count < len(term.prefix) {
		residual, ok := builder.Open(term.prefix[count:], term.rest, term.suffix)
		return []Term{residual}, ok
	}
	deltaValue := count - len(term.prefix)
	delta, ok := builder.schema.TableIndex(int64(deltaValue))
	if !ok {
		return nil, false
	}
	var longRest Rest
	switch term.rest.kind {
	case RestAny:
		var restOK bool
		longRest, restOK = builder.AnyTail(term.rest.class)
		if !restOK {
			return nil, false
		}
	case RestTail:
		offset, offsetOK := addOffsets(term.rest.offset, delta.offset)
		if !offsetOK {
			return nil, false
		}
		tail := term.rest.tail
		tailOK := tail.valid() && tail.owner == builder.relation.owner
		if term.rest.tail.kind == TailFree {
			tail, tailOK = builder.FreeTail(term.rest.tail.port)
		}
		if !tailOK {
			return nil, false
		}
		var restOK bool
		longRest, restOK = builder.Tail(tail, offset)
		if !restOK {
			return nil, false
		}
	default:
		return nil, false
	}
	long, longOK := builder.Open(nil, longRest, term.suffix)
	if !longOK {
		return nil, false
	}
	alternatives := []Term{long}
	for start := 1; start <= deltaValue; start++ {
		suffix := term.suffix
		if start < len(suffix) {
			suffix = suffix[start:]
		} else {
			suffix = nil
		}
		short, shortOK := builder.Closed(suffix...)
		if !shortOK {
			return nil, false
		}
		alternatives = append(alternatives, short)
	}
	return alternatives, true
}
