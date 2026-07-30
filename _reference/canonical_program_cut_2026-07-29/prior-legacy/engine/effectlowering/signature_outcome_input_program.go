package effectlowering

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

type signatureOutcomeInputProgramSeal struct{}

// SignatureOutcomeInputProgram is the closed registered-query vocabulary for
// signature lowering. Call operands are already-evaluated canonical ValueTerm
// roots owned by CallOutcomeInput; this program grants only the additional
// non-scalar observations explicitly requested by a signature producer.
//
// Programs are immutable and union by capability. An evaluator cannot perform
// an undeclared heap or path query: the corresponding method fails closed.
type SignatureOutcomeInputProgram struct {
	domain state.ProductDomain
	keys   *keyspace.KeySpace

	heap state.ProductLane
	path state.CoordinateFamily

	hasHeap bool
	hasPath bool
	seal    *signatureOutcomeInputProgramSeal
}

// SealSignatureOutcomeOperands seals the operand-only program. It observes no
// State lane; Callee, Receiver and Arguments come from the canonical call
// operand tuple evaluated before provider execution.
func SealSignatureOutcomeOperands(domain state.ProductDomain, keys *keyspace.KeySpace) (SignatureOutcomeInputProgram, error) {
	if !domain.Valid() || keys == nil || !keys.Valid() {
		return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature outcome input program is unowned")
	}
	return SignatureOutcomeInputProgram{domain: domain, keys: keys, seal: &signatureOutcomeInputProgramSeal{}}, nil
}

// WithHeapMemberQuery derives the heap dependency from the registered product
// domain. No caller spells the HeapTableIdentity lane by name.
func (p SignatureOutcomeInputProgram) WithHeapMemberQuery() (SignatureOutcomeInputProgram, error) {
	if !p.Valid() {
		return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: invalid signature outcome input program")
	}
	lane, ok := p.domain.ProductLane(state.LaneHeapTableIdentity)
	if !ok {
		return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature heap-member query is not registered")
	}
	p.heap, p.hasHeap = lane, true
	return p, nil
}

// WithPathValueQuery derives the unique path-value coordinate family from the
// registered product domain. It is intended for typed extensions such as
// contextual argument typing; ordinary call operands must stay in ValueTerms.
func (p SignatureOutcomeInputProgram) WithPathValueQuery() (SignatureOutcomeInputProgram, error) {
	if !p.Valid() {
		return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: invalid signature outcome input program")
	}
	family, ok := p.domain.PathValueFamily()
	if !ok {
		return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature path-value query is not registered")
	}
	p.path, p.hasPath = family, true
	return p, nil
}

// UnionSignatureOutcomeInputPrograms composes typed extension requirements.
// Programs from another product domain or keyspace are rejected rather than
// widened into an inventory scan.
func UnionSignatureOutcomeInputPrograms(programs ...SignatureOutcomeInputProgram) (SignatureOutcomeInputProgram, error) {
	var out SignatureOutcomeInputProgram
	for _, program := range programs {
		if !program.Valid() {
			continue
		}
		if !out.Valid() {
			out = program
			continue
		}
		if out.keys != program.keys || !sameSignatureOutcomeDomain(out.domain, program.domain) {
			return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature outcome input programs have different owners")
		}
		if program.hasHeap {
			if out.hasHeap && out.heap.ID() != program.heap.ID() {
				return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature heap query capabilities differ")
			}
			out.heap, out.hasHeap = program.heap, true
		}
		if program.hasPath {
			if out.hasPath && out.path.Lane().ID() != program.path.Lane().ID() {
				return SignatureOutcomeInputProgram{}, fmt.Errorf("effectlowering: signature path query capabilities differ")
			}
			out.path, out.hasPath = program.path, true
		}
	}
	return out, nil
}

func sameSignatureOutcomeDomain(left, right state.ProductDomain) bool {
	if !left.Valid() || !right.Valid() || left.Registry() != right.Registry() {
		return false
	}
	leftLanes, rightLanes := left.Lanes().IDs(), right.Lanes().IDs()
	if len(leftLanes) != len(rightLanes) {
		return false
	}
	for index := range leftLanes {
		if leftLanes[index] != rightLanes[index] {
			return false
		}
	}
	return true
}

func (p SignatureOutcomeInputProgram) Valid() bool {
	return p.seal != nil && p.domain.Valid() && p.keys != nil && p.keys.Valid()
}

// Lanes is the exact registered non-scalar footprint. Operand ValueTerm lanes
// are intentionally absent: their access is already sealed by the relation.
func (p SignatureOutcomeInputProgram) Lanes() state.LaneSet {
	if !p.Valid() {
		return state.LaneSet{}
	}
	lanes := state.NewLaneSet()
	if p.hasHeap {
		lanes = lanes.With(p.heap.ID())
	}
	if p.hasPath {
		lanes = lanes.With(p.path.Lane().ID())
	}
	return lanes
}

// Bind closes one dense call input under this program. No concrete State or
// read(point) callback is retained.
func (p SignatureOutcomeInputProgram) Bind(input callpayload.CallOutcomeInput) (SignatureOutcomeInput, error) {
	if !p.Valid() || !sameSignatureOutcomeDomain(input.Domain(), p.domain) {
		return SignatureOutcomeInput{}, fmt.Errorf("effectlowering: signature outcome input has a foreign domain")
	}
	primary := input.Primary()
	for _, lane := range p.Lanes().IDs() {
		if _, ok := primary.Factor(lane); !ok {
			return SignatureOutcomeInput{}, fmt.Errorf("effectlowering: signature outcome input is missing lane %q", lane)
		}
	}
	return SignatureOutcomeInput{program: p, input: input}, nil
}

// SignatureOutcomeInput is the only semantic input accepted by factor-native
// signature lowering.
type SignatureOutcomeInput struct {
	program SignatureOutcomeInputProgram
	input   callpayload.CallOutcomeInput
}

func (i SignatureOutcomeInput) Callee() (product.Value, bool)   { return i.input.Callee() }
func (i SignatureOutcomeInput) Receiver() (product.Value, bool) { return i.input.Receiver() }
func (i SignatureOutcomeInput) Argument(index int) (product.Value, bool) {
	return i.input.Argument(index)
}
func (i SignatureOutcomeInput) ArgumentCount() int { return i.input.ArgumentCount() }

// HeapMember applies the one carrier-neutral static-member law to the exact
// registered heap factor. It never reconstructs State.
func (i SignatureOutcomeInput) HeapMember(value product.Value, suffix []segment.Segment) (product.Value, bool, error) {
	if !i.program.Valid() || !i.program.hasHeap {
		return product.Value{}, false, fmt.Errorf("effectlowering: undeclared signature heap-member query")
	}
	id, ok := identityvalue.ExactID(i.program.domain.Registry(), value)
	if !ok {
		return product.Value{}, false, nil
	}
	factor, ok := i.input.Primary().Factor(i.program.heap.ID())
	if !ok {
		return product.Value{}, false, fmt.Errorf("effectlowering: signature heap-member factor is absent")
	}
	object, err := i.program.domain.ReadHeapTableObjectTermFactor(factor, identity.ConcreteTerm(id))
	if err != nil {
		return product.Value{}, false, err
	}
	member, present := sourcevalue.HeapMemberFromObject(i.program.domain.Registry(), i.program.keys, object, value, suffix)
	return member, present, nil
}

// PathValue observes one structural path through the unique registered path
// coordinate family. Dynamic source expressions remain operand ValueTerms and
// never enter this extension-only query.
func (i SignatureOutcomeInput) PathValue(path pathdom.Path) (product.Value, bool, error) {
	if !i.program.Valid() || !i.program.hasPath || path.IsEmpty() {
		return product.Value{}, false, fmt.Errorf("effectlowering: undeclared or empty signature path-value query")
	}
	key := i.program.keys.FromPath(path)
	if key.Kind == keyspace.KindInvalid {
		return product.Value{}, false, fmt.Errorf("effectlowering: signature path-value query is not internable")
	}
	factor, ok := i.input.Primary().Factor(i.program.path.Lane().ID())
	if !ok {
		return product.Value{}, false, fmt.Errorf("effectlowering: signature path-value factor is absent")
	}
	return i.program.domain.ReadPathValueFactor(factor, i.program.keys, key)
}

// signatureOutcomeIntrinsicInputProgram derives the exact extra query owned by
// canonical signature lowering. All ordinary argument/receiver reads are
// operands. Only a preserved param-member return consults heap identity.
func signatureOutcomeIntrinsicInputProgram(base SignatureOutcomeInputProgram, sig signature.Function) (SignatureOutcomeInputProgram, error) {
	if !base.Valid() {
		return SignatureOutcomeInputProgram{}, nil
	}
	if effects := sig.OperationalEffects; effects != nil {
		for _, flow := range effects.ReturnFlows {
			if flow.Kind == signature.ReturnFlowParamMember {
				return base.WithHeapMemberQuery()
			}
		}
	}
	return base, nil
}
