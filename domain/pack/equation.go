package pack

import (
	"sort"
)

// EquationKind keeps scalar and whole-Pack conclusions disjoint. A scalar
// marginal may be read from a Pack term, but it can never be promoted back to
// one; the distinct equation kinds enforce that law at carrier admission.
type EquationKind uint8

const (
	EquationInvalid EquationKind = iota
	EquationScalar
	EquationPack
)

// Equation is one owned output equation in a Pack case. Endpoint and Port are
// schema-issued handles; no raw Program term, Link ordinal, or solver key can
// enter a recurrent Pack value.
type Equation struct {
	owner    *algebra
	kind     EquationKind
	endpoint Endpoint
	port     Port
	scalar   Scalar
	term     Term
	sealed   bool
}

func scalarEquation(endpoint Endpoint, value Scalar) (Equation, bool) {
	if !endpoint.valid() || !value.valid() || endpoint.owner != value.owner {
		return Equation{}, false
	}
	equation := Equation{owner: endpoint.owner, kind: EquationScalar, endpoint: endpoint, scalar: value, sealed: true}
	return equation, equation.valid()
}

func packEquation(port Port, value Term) (Equation, bool) {
	if !port.valid() || !value.valid() || port.owner != value.owner {
		return Equation{}, false
	}
	equation := Equation{owner: port.owner, kind: EquationPack, port: port, term: value, sealed: true}
	return equation, equation.valid()
}

func (equation Equation) valid() bool {
	return equation.sealed && equation.owner != nil
}

func (equation Equation) Kind() EquationKind {
	if !equation.valid() {
		return EquationInvalid
	}
	return equation.kind
}

// Case is one finite conjunction of Pack output equations. Bound tails are
// existential only within this conjunction and are alpha-normalized on every
// constructor path. Case has no guard, control outcome, or engine identity;
// those remain in the typed Rule plane.
type Case struct {
	owner      *algebra
	relation   *relation
	equations  []Equation
	top        bool
	sealed     bool
	hash       uint64
	shapeRank  uint64
	syntaxRank uint64
	classRank  uint64
}

func exactCase(relation *relation, equations []Equation) (Case, bool) {
	if !relation.valid() {
		return Case{}, false
	}
	owner := relation.owner
	copyOf := append([]Equation(nil), equations...)
	// The empty conjunction denotes every valuation. Canonicalize it directly
	// to the only top Case; retaining a separate empty non-top case would make
	// one semantic element have two carrier representations.
	if len(copyOf) == 0 {
		return topCase(owner)
	}
	for _, equation := range copyOf {
		if !equation.valid() || equation.owner != owner {
			return Case{}, false
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return compareEquation(copyOf[left], copyOf[right]) < 0 })
	for index := 1; index < len(copyOf); index++ {
		if compareEquationTarget(copyOf[index-1], copyOf[index]) == 0 {
			return Case{}, false
		}
	}
	caseValue := Case{owner: owner, relation: relation, equations: copyOf}
	return alphaCase(caseValue)
}

func topCase(owner *algebra) (Case, bool) {
	if owner == nil || !owner.valid() {
		return Case{}, false
	}
	return finishCase(Case{owner: owner, top: true})
}

func (value Case) valid() bool {
	if !value.sealed || value.owner == nil || !value.owner.valid() {
		return false
	}
	if value.top {
		return value.relation == nil && len(value.equations) == 0
	}
	return value.relation != nil && value.relation.owner == value.owner && value.relation.valid()
}

func compareEquation(left, right Equation) int {
	if target := compareEquationTarget(left, right); target != 0 {
		return target
	}
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	return 0
}

func compareEquationTarget(left, right Equation) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	switch left.kind {
	case EquationScalar:
		if left.endpoint.index < right.endpoint.index {
			return -1
		}
		if left.endpoint.index > right.endpoint.index {
			return 1
		}
	case EquationPack:
		if left.port.index < right.port.index {
			return -1
		}
		if left.port.index > right.port.index {
			return 1
		}
	}
	return 0
}

type alphaState struct {
	owner  *algebra
	next   uint32
	bound  map[uint32]TailRef
	failed bool
}

func alphaCase(value Case) (Case, bool) {
	if value.owner == nil || !value.owner.valid() {
		return Case{}, false
	}
	if value.top {
		return topCase(value.owner)
	}
	state := alphaState{owner: value.owner, bound: make(map[uint32]TailRef)}
	equations := make([]Equation, len(value.equations))
	for index, equation := range value.equations {
		var valid bool
		equations[index], valid = state.equation(equation)
		if !valid {
			return Case{}, false
		}
	}
	result := Case{owner: value.owner, relation: value.relation, equations: equations}
	if state.failed || !caseAlphaNormalized(result) {
		return Case{}, false
	}
	return finishCase(result)
}

// finishCase is the only final admission path. It performs the deep structural
// validation and alpha check once, then records all hot metadata. Callers of
// Value equality/order/join never revalidate an immutable Case graph.
func finishCase(value Case) (Case, bool) {
	if value.owner == nil || !value.owner.valid() {
		return Case{}, false
	}
	if value.top {
		if value.relation != nil || len(value.equations) != 0 {
			return Case{}, false
		}
	} else {
		if value.relation == nil || !value.relation.valid() || value.relation.owner != value.owner || len(value.equations) == 0 {
			return Case{}, false
		}
		for index, equation := range value.equations {
			if !equation.valid() || equation.owner != value.owner || (index > 0 && compareEquation(value.equations[index-1], equation) >= 0) {
				return Case{}, false
			}
		}
		if !caseAlphaNormalized(value) {
			return Case{}, false
		}
		if !value.relation.matches(value.equations) {
			return Case{}, false
		}
	}
	value.sealed = true
	value.shapeRank = rawCaseShapeRank(value)
	value.syntaxRank = rawCaseSyntaxRank(value)
	value.classRank = rawCaseClassRank(value)
	h := newFNV64()
	h.write([]byte("wippy.analysis.pack/case\x00\x02"))
	writeCaseHash(&h, value)
	value.hash = uint64(h)
	return value, true
}

func (state *alphaState) equation(value Equation) (Equation, bool) {
	if state == nil || state.owner == nil || !value.valid() || value.owner != state.owner {
		return Equation{}, false
	}
	switch value.kind {
	case EquationScalar:
		scalar, ok := state.scalar(value.scalar)
		if !ok {
			return Equation{}, false
		}
		return scalarEquation(value.endpoint, scalar)
	case EquationPack:
		term, ok := state.term(value.term)
		if !ok {
			return Equation{}, false
		}
		return packEquation(value.port, term)
	default:
		return Equation{}, false
	}
}

func (state *alphaState) scalar(value Scalar) (Scalar, bool) {
	if state == nil || !value.valid() || value.owner != state.owner {
		return Scalar{}, false
	}
	if value.kind != ScalarHead {
		return value, true
	}
	tail, ok := state.tail(value.tail)
	if !ok {
		return Scalar{}, false
	}
	return headScalar(tail, value.offset)
}

func (state *alphaState) rest(value Rest) (Rest, bool) {
	if state == nil || !value.valid() || value.owner != state.owner {
		return Rest{}, false
	}
	if value.kind != RestTail {
		return value, true
	}
	tail, ok := state.tail(value.tail)
	if !ok {
		return Rest{}, false
	}
	return tailRest(tail, value.offset)
}

func (state *alphaState) term(value Term) (Term, bool) {
	if state == nil || !value.valid() || value.owner != state.owner {
		return Term{}, false
	}
	prefix := make([]Scalar, len(value.prefix))
	for index, scalar := range value.prefix {
		var ok bool
		prefix[index], ok = state.scalar(scalar)
		if !ok {
			return Term{}, false
		}
	}
	switch value.kind {
	case TermClosed:
		return closedTerm(state.owner, prefix)
	case TermOpen:
		rest, ok := state.rest(value.rest)
		if !ok {
			return Term{}, false
		}
		suffix := make([]Scalar, len(value.suffix))
		for index, scalar := range value.suffix {
			suffix[index], ok = state.scalar(scalar)
			if !ok {
				return Term{}, false
			}
		}
		return openTerm(state.owner, prefix, rest, suffix)
	case TermAny:
		return anyTerm(state.owner)
	default:
		return Term{}, false
	}
}

func (state *alphaState) tail(value TailRef) (TailRef, bool) {
	if state == nil || !value.valid() || value.owner != state.owner {
		return TailRef{}, false
	}
	if value.kind == TailFree {
		return value, true
	}
	if prior, exists := state.bound[value.index]; exists {
		if !state.owner.equalClass(prior.class, value.class) {
			state.failed = true
			return TailRef{}, false
		}
		return prior, true
	}
	state.next++
	normalized, ok := boundTail(state.owner, state.next, value.class)
	if !ok {
		state.failed = true
		return TailRef{}, false
	}
	state.bound[value.index] = normalized
	return normalized, true
}

func caseAlphaNormalized(value Case) bool {
	if value.top {
		return len(value.equations) == 0
	}
	seen := make(map[uint32]struct{})
	next := uint32(1)
	validTail := func(tail TailRef) bool {
		if tail.kind != TailBound {
			return true
		}
		if _, exists := seen[tail.index]; exists {
			return true
		}
		if tail.index != next {
			return false
		}
		seen[tail.index] = struct{}{}
		next++
		return true
	}
	var scalar func(Scalar) bool
	scalar = func(value Scalar) bool {
		return value.kind != ScalarHead || validTail(value.tail)
	}
	var term func(Term) bool
	term = func(value Term) bool {
		for _, scalarValue := range value.prefix {
			if !scalar(scalarValue) {
				return false
			}
		}
		if value.kind == TermOpen && value.rest.kind == RestTail && !validTail(value.rest.tail) {
			return false
		}
		for _, scalarValue := range value.suffix {
			if !scalar(scalarValue) {
				return false
			}
		}
		return true
	}
	for _, equation := range value.equations {
		switch equation.kind {
		case EquationScalar:
			if !scalar(equation.scalar) {
				return false
			}
		case EquationPack:
			if !term(equation.term) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
