package pack

import "github.com/wippyai/go-lua/domain/static"

// Builder is the sole domain-local construction capability for one sealed Pack
// relation. It cannot mint a root, endpoint, port, class, or offset: every
// handle it accepts or returns is already fenced by Schema. A future typed Rule
// supplies the Link-selected equations, while Builder enforces that they form
// the complete output relation for exactly this root.
type Builder struct {
	schema   *Schema
	relation *relation
}

// Builder returns the complete construction capability for root.
func (schema *Schema) Builder(root Root) (Builder, bool) {
	relation, ok := schema.relation(root)
	if !ok {
		return Builder{}, false
	}
	return Builder{schema: schema, relation: relation}, true
}

func (builder Builder) valid() bool {
	return builder.schema != nil && builder.schema.state != nil && builder.relation != nil &&
		builder.relation.valid() && builder.relation.owner == builder.schema.state.owner
}

// Endpoint names an existing scalar source expression.
func (builder Builder) Endpoint(endpoint Endpoint) (Scalar, bool) {
	if !builder.valid() || !endpoint.valid() || endpoint.owner != builder.relation.owner {
		return Scalar{}, false
	}
	return endpointScalar(endpoint)
}

// FreeTail names an existing whole-Pack source expression.
func (builder Builder) FreeTail(port Port) (TailRef, bool) {
	if !builder.valid() || !port.valid() || port.owner != builder.relation.owner {
		return TailRef{}, false
	}
	return freeTail(port)
}

// Zero returns the canonical zero adjustment. Nonzero adjustments must arrive
// as their own sealed typed Link operand; Builder intentionally does not
// reinterpret a raw integer as a Pack offset.
func (builder Builder) Zero() (Offset, bool) {
	if !builder.valid() {
		return Offset{}, false
	}
	return zeroOffset(builder.relation.owner)
}

func (builder Builder) Tail(tail TailRef, offset Offset) (Rest, bool) {
	if !builder.valid() || !tail.valid() || tail.owner != builder.relation.owner || !offset.valid() || offset.owner != builder.relation.owner {
		return Rest{}, false
	}
	return tailRest(tail, offset)
}

func (builder Builder) AnyTail(class static.Class) (Rest, bool) {
	if !builder.valid() || !builder.relation.owner.admits(class) {
		return Rest{}, false
	}
	return anyRest(builder.relation.owner, class)
}

func (builder Builder) Closed(values ...Scalar) (Term, bool) {
	if !builder.valid() {
		return Term{}, false
	}
	return closedTerm(builder.relation.owner, values)
}

func (builder Builder) Open(prefix []Scalar, middle Rest, suffix []Scalar) (Term, bool) {
	if !builder.valid() || !middle.valid() || middle.owner != builder.relation.owner {
		return Term{}, false
	}
	return openTerm(builder.relation.owner, prefix, middle, suffix)
}

func (builder Builder) AnyPack() (Term, bool) {
	if !builder.valid() {
		return Term{}, false
	}
	return anyTerm(builder.relation.owner)
}

// Pack writes one existing whole-Pack target.
func (builder Builder) Pack(target Port, value Term) (Equation, bool) {
	if !builder.valid() || !target.valid() || target.owner != builder.relation.owner || !value.valid() || value.owner != builder.relation.owner {
		return Equation{}, false
	}
	if !builder.relation.hasTarget(EquationPack, target.index) {
		return Equation{}, false
	}
	return packEquation(target, value)
}

func (builder Builder) Case(equations ...Equation) (Case, bool) {
	if !builder.valid() {
		return Case{}, false
	}
	return exactCase(builder.relation, equations)
}

// Value normalizes cases into the relation's antichain carrier. This is the
// only public non-extreme admission path; the relation fence is rechecked by
// valueFromCases even when cases were built elsewhere in the same schema.
func (builder Builder) Value(cases ...Case) (Value, bool) {
	if !builder.valid() {
		return Value{}, false
	}
	return valueFromCases(builder.relation, cases)
}
