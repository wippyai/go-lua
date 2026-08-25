package generated

import (
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// ReadPlan is one immutable ordered join row in a sealed generated program.
// The factor ordinal is deliberately independent from the output factor: the
// schema may bind heterogeneous reads and outputs without making the engine
// guess that they share a Go type or an axis.
//
// Form, Contract, Predicate, and Denominator are copied from the sealed
// rule-plan Join. They are part of the descriptor rather than being recovered
// from a runtime read capability. Summary and Complete are intentionally not
// executable by this first generated vertical; the seal rejects them instead
// of erasing their declaration metadata.
type ReadPlan struct {
	Input            uint32
	Factor           uint32
	Axis             uint32
	Sources          ruleplan.Span
	Relation         ruleplan.RelationAddr
	Key              ruleplan.ProjectionAddr
	Predicate        ruleplan.ProjectionAddr
	PredicatePresent bool
	// Parent is the sealed restatement of the join's Parent: the relation
	// whose candidate rows this read's relation nests under as a bounded,
	// ordinal-addressed member set. ParentPresent is explicit because the
	// zero relation address is a valid one.
	//
	// It is the addressing fact a Summary read over a self-provided member
	// set is admissible by, so it is carried here rather than recovered: the
	// seal below asks the declaration's own law which addressing a form
	// requires, and cannot ask without it.
	Parent        ruleplan.RelationAddr
	ParentPresent bool
	// KeyVector is the sealed restatement of the join's KeyVector: the
	// directory whose rows publish the ordered dense key vector this read is
	// taken over. KeyVectorPresent is explicit for the same reason
	// ParentPresent is - the zero relation address is a valid one - and the
	// two are alternatives, because a vector read has one denominator.
	KeyVector        ruleplan.RelationAddr
	KeyVectorPresent bool
	// Addressing is the sealed directory whose candidate ordinal indexes this
	// read: the relation that ISSUES the rows the read resolves against. It is
	// the rule's own candidate relation when the read borrows that directory,
	// and a foreign one the plan proved corresponds to it otherwise.
	//
	// AddressingPresent is a statement, not a nil guard. A selected read is
	// addressed by the selection its own family resolves and an issued
	// candidate is a Program row with no directory, so neither names one; every
	// candidate-addressed read of an axis-addressed rule does.
	//
	// It is carried because two directories addressed by one occurrence
	// enumerate their rows independently: the ordinal the rule resolved in its
	// own directory means nothing in a foreign one, and this is the address at
	// which it must be resolved again.
	Addressing        ruleplan.RelationAddr
	AddressingPresent bool
	// AddressIdentity is the owner-issued occurrence the corresponded foreign
	// directory above is enumerated under, projected from the rule's own
	// candidate row. Absent, the candidate's own occurrence is that address,
	// which is the ordinary case and the only one a directory of a single
	// occurrence family ever needs. Present, the candidate row NAMES a subject
	// rather than being one, and this is the identity it names.
	AddressIdentity        ruleplan.ProjectionAddr
	AddressIdentityPresent bool
	Form                   ruleprogram.ReadForm
	Contract               ruleplan.ReadContract
	Denominator            ruleplan.DenominatorAddr
	RowCapacity            uint16
	CellCapacity           uint16
	// PointBound is the authored disposition copied from the sealed Plan
	// Join: whether this Input slot's own predecessor topology point is
	// transported into the rule, or the read resolves through its Factor's
	// directory/route surface and shares the candidate's own point instead.
	PointBound ruleprogram.PointBoundDecl
}

// OutputPlan is one immutable output row. Exactly one output row is admitted
// per generated rule in this slice; its sealed mode may be Exact or Route.
// Structural output is rejected at seal and is never silently converted to an
// exact write. Exact and Strong remain capability evidence for the existing
// exact executor; Mode is the canonical publication disposition.
type OutputPlan struct {
	Factor      uint32
	Axis        uint32
	Address     ruleplan.OutputAddr
	Destination ruleplan.ProjectionAddr
	Mode        ruleprogram.OutputMode
	Slot        uint32
	// RouteJoin is the declaration-order join selected by a routed output.
	// RouteJoinPresent distinguishes the valid zero ordinal from an exact
	// output, which has no route at all.
	RouteJoin        uint32
	RouteJoinPresent bool
	Exact            bool
	Strong           bool
}

// CarryPlan is optional. Identity carry is retained in the descriptor. A
// transform is explicit metadata but remains a seal refusal until its
// owner-issued transform execution table exists; it is never dropped.
type CarryPlan struct {
	Input            uint32
	Factor           uint32
	Mode             ruleprogram.CarryMode
	Transform        ruleplan.CarryTransformAddr
	TransformPresent bool
	Identity         bool
}

// CompiledRule is the immutable, table-backed, type-neutral description of a
// generated Rule program. It is intentionally free of Read/Write
// capabilities and scratch storage.
//
// It does NOT carry its own Rule ordinal. A descriptor is a row of the sealed
// rule table, and its coordinate is its POSITION in that table: the seal
// assigns it once, the table answers it, and a copy stored in the row would be
// a second place the same number lives and a second thing to keep in agreement.
// A row of another table that has to name this one holds the ordinal as a
// typed foreign key - the member row and the plan row an execution family is
// handed both do - and nothing rebases or re-derives it.
//
// The admitted forms are structural, not a runtime enum: an ordered zero or
// more join table, one exact/route output row, and an optional identity carry.
// Read forms are Exact and Selected in this slice; Summary and Complete remain
// explicit seal refusals.
//
// All slices are seal-owned copies. No hot-path operation mutates or appends to
// them.
type CompiledRule struct {
	inputCount uint16
	reads      []ReadPlan
	outputs    []OutputPlan
	carry      *CarryPlan
	// transports is the sealed activation transport vector. It is the content
	// of a structural publication: the axes one candidate route instantiates
	// across its transition, each row carrying whether the mounted body carries
	// that axis back out. A rule that publishes a fact declares none.
	transports []ruleplan.Transport
	// branch is the sealed vocabulary one candidate branch is mounted by: the
	// owner-issued identities the construct plane keys and resolves an
	// activation member with. It stands under the same biconditional as the
	// vector above.
	branch        ruleplan.Activation
	branchPresent bool

	// planGeometry is set only by the schema-owned constructor below. A generic
	// invocation descriptor can omit Plan/member authority, but such a value
	// cannot serve as a SchemaBuilder descriptor. For the schema-owned form,
	// every axis field is already normalized to a runtime Factor ordinal; the
	// member/frame portion remains owner-local Plan geometry.
	planGeometry      bool
	axisCount         uint32
	candidateRelation ruleplan.RelationAddr
	issuedCandidate   bool
	reducer           ruleplan.ReducerAddr
}

// CompiledRuleSpec is the structural, Plan-derived half of one generated
// descriptor. Axis values must already be normalized to runtime Factor
// ordinals by the schema engine; member/frame values remain owner-local Plan
// coordinates. The address types stay distinct: a relation member cannot be
// laundered as a projection or reducer ordinal by sharing a fused shape tag.
// The schema engine is the only package that should issue this specification.
type CompiledRuleSpec struct {
	AxisCount  int
	InputCount int
	Candidate  ruleplan.RelationAddr
	// IssuedCandidate marks a rule whose candidate rows are Program rows. Its
	// Candidate address stays zero: an issued row has no Factor relation, and
	// the ordinal reaches the runtime on the mounted placement instead of
	// through an axis owner's directory.
	IssuedCandidate bool
	Reducer         ruleplan.ReducerAddr
	Reads           []ReadPlan
	Outputs         []OutputPlan
	Carry           *CarryPlan
	Transports      []ruleplan.Transport
	Activation      *ruleplan.Activation
}

// NewPlanCompiledRule seals a generated descriptor from one already compiled
// ruleplan.Plan projection whose axis values have already been normalized to
// runtime Factor ordinals. It is deliberately structural: every address is
// retained in its own nominal type, while the bounded scalar geometry remains
// available to the generic invocation owner. No domain value, callback, or
// runtime schema lookup enters this constructor.
func NewPlanCompiledRule(spec CompiledRuleSpec) (CompiledRule, bool) {
	if spec.AxisCount <= 0 || uint64(spec.AxisCount) > uint64(^uint32(0)) || spec.InputCount < 0 || spec.InputCount > int(^uint16(0)) {
		return CompiledRule{}, false
	}
	if spec.Reads == nil || spec.Outputs == nil {
		return CompiledRule{}, false
	}
	reads := spec.Reads
	outputs := spec.Outputs
	var carry *CarryPlan
	if spec.Carry != nil {
		copyCarry := *spec.Carry
		carry = &copyCarry
	}
	if len(outputs) != 1 {
		return CompiledRule{}, false
	}
	if !validInputPrefix(spec.InputCount, reads, carry) {
		return CompiledRule{}, false
	}
	readCopy := append([]ReadPlan(nil), reads...)
	outputCopy := append([]OutputPlan(nil), outputs...)
	for index := range readCopy {
		if !normalizeReadPlan(&readCopy[index], spec.IssuedCandidate) {
			return CompiledRule{}, false
		}
	}
	if !normalizeOutputPlan(&outputCopy[0], readCopy) {
		return CompiledRule{}, false
	}
	rule := CompiledRule{
		inputCount: uint16(spec.InputCount), reads: readCopy, outputs: outputCopy, carry: carry,
		transports:   append([]ruleplan.Transport(nil), spec.Transports...),
		planGeometry: true, axisCount: uint32(spec.AxisCount), candidateRelation: spec.Candidate, issuedCandidate: spec.IssuedCandidate, reducer: spec.Reducer,
	}
	if carry != nil && !normalizeCarryPlan(carry, spec.InputCount, spec.AxisCount, outputCopy[0].Factor) {
		return CompiledRule{}, false
	}
	if !validRelationAddr(spec.Candidate) || !validReducerAddr(spec.Reducer) || !addressAxesInRange(spec.AxisCount, spec.Candidate, spec.Reducer, outputCopy[0].Address, outputCopy[0].Destination) {
		return CompiledRule{}, false
	}
	if spec.Activation != nil {
		rule.branch, rule.branchPresent = *spec.Activation, true
	}
	if !validTransportVector(rule.transports, outputCopy[0].Mode, spec.AxisCount) {
		return CompiledRule{}, false
	}
	if !validActivationBranch(rule.branch, rule.branchPresent, len(rule.transports) != 0, spec.AxisCount) {
		return CompiledRule{}, false
	}
	for _, read := range readCopy {
		if !validReadPlan(read, spec.InputCount, spec.AxisCount, spec.IssuedCandidate) {
			return CompiledRule{}, false
		}
	}
	output := outputCopy[0]
	if !validOutputPlan(output, spec.InputCount, spec.AxisCount, spec.Candidate, spec.Reducer) {
		return CompiledRule{}, false
	}
	if output.Mode == ruleprogram.ModeStructural && output.Destination.Axis != spec.Candidate.Axis {
		return CompiledRule{}, false
	}
	if output.Mode == ruleprogram.ModeExact && output.Destination.Axis != spec.Candidate.Axis && output.Destination.Axis != output.Address.Axis {
		return CompiledRule{}, false
	}
	if output.Mode == ruleprogram.ModeRoute && output.Destination.Axis != readCopy[output.RouteJoin].Relation.Axis {
		return CompiledRule{}, false
	}
	if !rule.Available() {
		return CompiledRule{}, false
	}
	return rule, true
}

// validInputPrefix is the seal-time arity law. The executor never discovers
// missing ports by probing runtime state: every port in the rule-local prefix
// must be named by a read or the optional identity carry. A single input state
// may feed more than one declaration row.
func validInputPrefix(inputCount int, reads []ReadPlan, carry *CarryPlan) bool {
	if inputCount < 0 || inputCount > int(^uint16(0)) {
		return false
	}
	if inputCount == 0 {
		return len(reads) == 0 && carry == nil
	}
	seen := make([]bool, inputCount)
	for _, read := range reads {
		if read.Input >= uint32(inputCount) {
			return false
		}
		seen[read.Input] = true
	}
	if carry != nil {
		if carry.Input >= uint32(inputCount) {
			return false
		}
		seen[carry.Input] = true
	}
	for _, present := range seen {
		if !present {
			return false
		}
	}
	return true
}

func zeroDenominator(address ruleplan.DenominatorAddr) bool {
	return !address.Present && address.Ordinal == 0
}

// ReadFormAddressShape proves one sealed read's addressing metadata against
// the declaration law that decided it. The dense encoding is this package's
// own: a present address must be a valid one and an absent one must be zero.
// The zero address is a REAL address - relation 0 of axis 0 - so presence is
// declared and never inferred from it. Which addressing a form requires is
// not this package's to say - ruleprogram.ReadFormAddressing is the one
// statement of that, and it is asked here rather than spelled again.
//
// It is exported because the plan-shape fence in the schema engine holds
// sealed reads to the same proof.
func ReadFormAddressShape(form ruleprogram.ReadForm, predicate ruleplan.ProjectionAddr, predicatePresent bool, parent ruleplan.RelationAddr, parentPresent bool, keyVector ruleplan.RelationAddr, keyVectorPresent bool) bool {
	// Presence is the declaration's own statement and the address is checked
	// against it, exactly as ReadAddressingShape below states for the third
	// address. Reading presence off "the value is non-zero" instead would say
	// that relation 0 of axis 0 does not exist, which makes the FIRST relation
	// and the FIRST projection an axis declares unusable as a parent or a
	// predicate - and those are the ordinary ones, not edge cases.
	if predicatePresent {
		if !validProjectionAddr(predicate) {
			return false
		}
	} else if predicate != (ruleplan.ProjectionAddr{}) {
		return false
	}
	if parentPresent {
		if !validRelationAddr(parent) {
			return false
		}
	} else if parent != (ruleplan.RelationAddr{}) {
		return false
	}
	if keyVectorPresent {
		if !validRelationAddr(keyVector) {
			return false
		}
	} else if keyVector != (ruleplan.RelationAddr{}) {
		return false
	}
	return ruleprogram.ReadFormAddressing(form, predicatePresent, parentPresent, keyVectorPresent)
}

// ReadAddressingShape proves one sealed read's addressing directory against
// the declaration law that decided it.
//
// Which reads are indexed by the rule candidate's ordinal is not this
// package's to say - ruleprogram.ReadFormCandidateAddressed is the one
// statement of that - and whether the rule has a directory at all is the
// candidate arm's. Together they settle presence exactly, so an addressing
// directory is neither optional metadata nor derived from whether the value
// happens to be zero: the zero relation address is a real address, and a read
// that names none must carry it zero.
//
// It is exported because the plan-shape fence in the schema engine holds
// sealed reads to the same proof.
func ReadAddressingShape(form ruleprogram.ReadForm, candidateIssued bool, addressing ruleplan.RelationAddr, addressingPresent bool) bool {
	if addressingPresent != (ruleprogram.ReadFormCandidateAddressed(form) && !candidateIssued) {
		return false
	}
	if !addressingPresent {
		return addressing == ruleplan.RelationAddr{}
	}
	return validRelationAddr(addressing)
}

// normalizeReadPlan validates the complete sealed read metadata. There is no
// legacy exact default here: a descriptor without its form or contract is
// incomplete and is refused at the seal boundary.
func normalizeReadPlan(read *ReadPlan, candidateIssued bool) bool {
	if read == nil {
		return false
	}
	if !ReadAddressingShape(read.Form, candidateIssued, read.Addressing, read.AddressingPresent) {
		return false
	}
	contract := read.Contract
	if !contract.Order.Available() || !contract.Sparse.Available() || !contract.OnOpaque.Available() || !contract.Multiplicity.Available() {
		return false
	}
	if !ReadFormAddressShape(read.Form, read.Predicate, read.PredicatePresent, read.Parent, read.ParentPresent, read.KeyVector, read.KeyVectorPresent) {
		return false
	}
	if !read.PointBound.Available() {
		return false
	}
	if !zeroDenominator(read.Denominator) && !read.Denominator.Present {
		return false
	}
	if read.Denominator.Present && read.Denominator.Ordinal == ^uint32(0) {
		return false
	}
	if ruleprogram.RequiresFactorDenominator(read.Form, contract.Sparse, read.ParentPresent || read.KeyVectorPresent) && !read.Denominator.Present {
		return false
	}
	if read.Sources.Count == 0 {
		if read.Sources.Start != 0 {
			return false
		}
	} else if read.Sources.Start > ^uint32(0)-read.Sources.Count {
		return false
	}
	return true
}

// normalizeOutputPlan keeps Mode and RouteJoin as the canonical publication
// disposition. Exact/Strong remain capability evidence for the existing exact
// writer; they cannot synthesize an omitted output mode.
func normalizeOutputPlan(output *OutputPlan, reads []ReadPlan) bool {
	if output == nil {
		return false
	}
	switch output.Mode {
	case ruleprogram.ModeExact:
		// Exact and Strong are the exact writer's capability evidence.
		return output.Exact && output.Strong && !output.RouteJoinPresent && output.RouteJoin == 0
	case ruleprogram.ModeStructural:
		// A structural publication stages no fact, so it carries neither the
		// exact writer's evidence nor a route.
		return !output.Exact && !output.Strong && !output.RouteJoinPresent && output.RouteJoin == 0
	case ruleprogram.ModeRoute:
		// The route join must be the selected read the output publishes over.
		// How many OTHER selected reads the rule declares is not this law's
		// business: a route set computed from an earlier selection is the
		// ordinary dependent join, and counting selections instead of naming
		// the route one refuses it.
		return output.RouteJoinPresent && uint64(output.RouteJoin) < uint64(len(reads)) && reads[output.RouteJoin].Form == ruleprogram.Selected
	default:
		return false
	}
}

// validTransportVector states the biconditional between a structural
// publication and an activation transport vector. A structural publication
// with no vector instantiates nothing; a vector on a fact-writing rule is a
// transport no candidate route crosses. Neither is silently converted into the
// other, and every transported axis is fenced by the same sealed directory as
// the descriptor's other addresses.
func validTransportVector(transports []ruleplan.Transport, mode ruleprogram.OutputMode, axisCount int) bool {
	if (mode == ruleprogram.ModeStructural) != (len(transports) != 0) {
		return false
	}
	seen := make(map[uint32]struct{}, len(transports))
	for _, transport := range transports {
		if uint64(transport.Axis) >= uint64(axisCount) {
			return false
		}
		if _, duplicate := seen[transport.Axis]; duplicate {
			return false
		}
		seen[transport.Axis] = struct{}{}
	}
	return true
}

// validActivationBranch states the branch vocabulary against the vector it
// stands with. A transport vector says what one branch instantiates; the
// vocabulary says what that branch IS, and a descriptor carrying one without
// the other describes a mount nothing could address or an address nothing
// mounts.
//
// The branch relation is ENUMERATED and never read. Its members carry no fact
// any judgment consumes and have no coordinate of their own to be read at, so
// the descriptor names the relation and the issuance pass walks it through its
// owner - which is also why a structural rule declares one read, not two.
func validActivationBranch(branch ruleplan.Activation, present, transported bool, axisCount int) bool {
	if present != transported {
		return false
	}
	if !present {
		return branch == ruleplan.Activation{}
	}
	if !validRelationAddr(branch.Branch) || uint64(branch.Branch.Axis) >= uint64(axisCount) {
		return false
	}
	for _, address := range []ruleplan.ProjectionAddr{branch.Application, branch.Target, branch.Endpoint, branch.Mount, branch.Body} {
		if !validProjectionAddr(address) || uint64(address.Axis) >= uint64(axisCount) {
			return false
		}
	}
	return true
}

// ActivationBranch is the sealed vocabulary one candidate branch is mounted
// by, present exactly when this descriptor carries a transport vector.
func (rule CompiledRule) ActivationBranch() (ruleplan.Activation, bool) {
	if !rule.Available() {
		return ruleplan.Activation{}, false
	}
	return rule.branch, rule.branchPresent
}

func normalizeCarryPlan(carry *CarryPlan, inputCount, axisCount int, outputFactor uint32) bool {
	if carry == nil {
		return false
	}
	return validCarryPlan(*carry, inputCount, axisCount, outputFactor)
}

// validCarryPlan states the two sealed carry dispositions. Identity carries the
// prior output fact unchanged and names no transform. Transform names exactly
// one owner-issued transform member, addressed in the same axis directory as
// the rest of the descriptor; the transform is what makes the carry a form of
// its own, so a transformed carry that lost its address is not a carry at all.
func validCarryPlan(carry CarryPlan, inputCount, axisCount int, outputFactor uint32) bool {
	if inputCount < 0 || axisCount <= 0 || carry.Input >= uint32(inputCount) || carry.Factor == ^uint32(0) || carry.Factor != outputFactor {
		return false
	}
	switch carry.Mode {
	case ruleprogram.CarryIdentity:
		return carry.Identity && !carry.TransformPresent && carry.Transform == (ruleplan.CarryTransformAddr{})
	case ruleprogram.CarryTransform:
		return !carry.Identity && carry.TransformPresent &&
			carry.Transform.Axis != ^uint32(0) && carry.Transform.Member != ^uint32(0) &&
			uint64(carry.Transform.Axis) < uint64(axisCount)
	default:
		return false
	}
}

func validReadPlan(read ReadPlan, inputCount, axisCount int, candidateIssued bool) bool {
	if inputCount < 0 || axisCount <= 0 || read.Input >= uint32(inputCount) ||
		read.Factor == ^uint32(0) || read.Axis == ^uint32(0) ||
		uint64(read.Factor) >= uint64(axisCount) || uint64(read.Axis) >= uint64(axisCount) ||
		read.RowCapacity == 0 || read.CellCapacity == 0 ||
		!validRelationAddr(read.Relation) || !validProjectionAddr(read.Key) ||
		read.Relation.Axis != read.Key.Axis ||
		!addressAxesInRange(axisCount, read.Relation, read.Key, read.Predicate, read.Parent, read.Addressing) {
		return false
	}
	if !ReadAddressingShape(read.Form, candidateIssued, read.Addressing, read.AddressingPresent) {
		return false
	}
	if !read.Contract.Order.Available() || !read.Contract.Sparse.Available() || !read.Contract.OnOpaque.Available() || !read.Contract.Multiplicity.Available() {
		return false
	}
	if !ReadFormAddressShape(read.Form, read.Predicate, read.PredicatePresent, read.Parent, read.ParentPresent, read.KeyVector, read.KeyVectorPresent) {
		return false
	}
	if !read.PointBound.Available() {
		return false
	}
	if ruleprogram.RequiresFactorDenominator(read.Form, read.Contract.Sparse, read.ParentPresent || read.KeyVectorPresent) && !read.Denominator.Present {
		return false
	}
	if !zeroDenominator(read.Denominator) && !read.Denominator.Present || read.Denominator.Present && read.Denominator.Ordinal == ^uint32(0) {
		return false
	}
	if read.Sources.Count == 0 {
		return read.Sources.Start == 0
	}
	return read.Sources.Start <= ^uint32(0)-read.Sources.Count
}

func validOutputPlan(output OutputPlan, _ int, axisCount int, candidate ruleplan.RelationAddr, reducer ruleplan.ReducerAddr) bool {
	return output.Factor != ^uint32(0) && output.Axis != ^uint32(0) &&
		uint64(output.Factor) < uint64(axisCount) && uint64(output.Axis) < uint64(axisCount) &&
		validOutputAddr(output.Address) && validProjectionAddr(output.Destination) &&
		output.Address.Axis == output.Axis &&
		reducer.Axis == output.Axis && output.Slot == 0 &&
		output.Mode.Available() &&
		(output.Mode != ruleprogram.ModeExact || output.Exact && output.Strong) &&
		(output.Mode != ruleprogram.ModeStructural || !output.Exact && !output.Strong) &&
		(output.Mode == ruleprogram.ModeRoute) == output.RouteJoinPresent &&
		(output.RouteJoinPresent || output.RouteJoin == 0) &&
		addressAxesInRange(axisCount, output.Address, output.Destination)
}

func validRelationAddr(address ruleplan.RelationAddr) bool {
	return address.Axis != ^uint32(0) && address.Member != ^uint32(0)
}

func validProjectionAddr(address ruleplan.ProjectionAddr) bool {
	return address.Axis != ^uint32(0) && address.Member != ^uint32(0)
}

func validReducerAddr(address ruleplan.ReducerAddr) bool {
	return address.Axis != ^uint32(0) && address.Member != ^uint32(0)
}

func validOutputAddr(address ruleplan.OutputAddr) bool {
	return address.Axis != ^uint32(0) && address.Frame != ^uint32(0)
}

func addressAxesInRange(axisCount int, addresses ...interface{}) bool {
	for _, value := range addresses {
		var axis uint32
		switch address := value.(type) {
		case ruleplan.RelationAddr:
			axis = address.Axis
		case ruleplan.ProjectionAddr:
			axis = address.Axis
		case ruleplan.ReducerAddr:
			axis = address.Axis
		case ruleplan.OutputAddr:
			axis = address.Axis
		default:
			return false
		}
		if uint64(axis) >= uint64(axisCount) {
			return false
		}
	}
	return true
}

func (rule CompiledRule) Available() bool {
	if len(rule.outputs) != 1 || !rule.planGeometry {
		return false
	}
	if rule.axisCount == 0 || !validRelationAddr(rule.candidateRelation) || !validReducerAddr(rule.reducer) || !addressAxesInRange(int(rule.axisCount), rule.candidateRelation, rule.reducer) || !validInputPrefix(int(rule.inputCount), rule.reads, rule.carry) {
		return false
	}
	output := rule.outputs[0]
	if !validOutputPlan(output, int(rule.inputCount), int(rule.axisCount), rule.candidateRelation, rule.reducer) {
		return false
	}
	if output.Mode == ruleprogram.ModeRoute && (uint64(output.RouteJoin) >= uint64(len(rule.reads)) || rule.reads[output.RouteJoin].Form != ruleprogram.Selected) {
		return false
	}
	for _, read := range rule.reads {
		if !validReadPlan(read, int(rule.inputCount), int(rule.axisCount), rule.issuedCandidate) {
			return false
		}
	}
	if output.Mode == ruleprogram.ModeStructural && output.Destination.Axis != rule.candidateRelation.Axis {
		return false
	}
	if output.Mode == ruleprogram.ModeExact && output.Destination.Axis != rule.candidateRelation.Axis && output.Destination.Axis != output.Address.Axis {
		return false
	}
	if output.Mode == ruleprogram.ModeRoute && output.Destination.Axis != rule.reads[output.RouteJoin].Relation.Axis {
		return false
	}
	if !validTransportVector(rule.transports, output.Mode, int(rule.axisCount)) {
		return false
	}
	if !validActivationBranch(rule.branch, rule.branchPresent, len(rule.transports) != 0, int(rule.axisCount)) {
		return false
	}
	if rule.carry != nil {
		if !validCarryPlan(*rule.carry, int(rule.inputCount), int(rule.axisCount), output.Factor) {
			return false
		}
	}
	return true
}

func (rule CompiledRule) InputCount() int {
	if !rule.Available() {
		return 0
	}
	return int(rule.inputCount)
}

func (rule CompiledRule) OutputCount() int {
	if !rule.Available() {
		return 0
	}
	return len(rule.outputs)
}

// ReadCount returns the number of sealed exact join rows.
func (rule CompiledRule) ReadCount() int {
	if !rule.Available() {
		return 0
	}
	return len(rule.reads)
}

// ReadAt returns a value copy of one sealed read row.
func (rule CompiledRule) ReadAt(index int) (ReadPlan, bool) {
	if !rule.Available() || index < 0 || index >= len(rule.reads) {
		return ReadPlan{}, false
	}
	return rule.reads[index], true
}

// OutputAt returns a value copy of one sealed direct output row.
func (rule CompiledRule) OutputAt(index int) (OutputPlan, bool) {
	if !rule.Available() || index < 0 || index >= len(rule.outputs) {
		return OutputPlan{}, false
	}
	return rule.outputs[index], true
}

// TransportCount is the width of this descriptor's activation transport
// vector. It is zero for a rule that publishes a fact.
func (rule CompiledRule) TransportCount() int {
	if !rule.Available() {
		return 0
	}
	return len(rule.transports)
}

// TransportAt returns one sealed transport row by its declaration ordinal.
func (rule CompiledRule) TransportAt(index int) (ruleplan.Transport, bool) {
	if !rule.Available() || index < 0 || index >= len(rule.transports) {
		return ruleplan.Transport{}, false
	}
	return rule.transports[index], true
}

func (rule CompiledRule) ReadInput() int {
	if !rule.Available() || len(rule.reads) == 0 {
		return -1
	}
	return int(rule.reads[0].Input)
}

func (rule CompiledRule) ReadFactor() uint32 {
	if !rule.Available() {
		return 0
	}
	if len(rule.reads) == 0 {
		return 0
	}
	return rule.reads[0].Factor
}

func (rule CompiledRule) OutputFactor() uint32 {
	if !rule.Available() {
		return 0
	}
	return rule.outputs[0].Factor
}

func (rule CompiledRule) RowCapacity() int {
	if !rule.Available() {
		return 0
	}
	maximum := 0
	for _, read := range rule.reads {
		if int(read.RowCapacity) > maximum {
			maximum = int(read.RowCapacity)
		}
	}
	return maximum
}

func (rule CompiledRule) CellCapacity() int {
	if !rule.Available() {
		return 0
	}
	maximum := 0
	for _, read := range rule.reads {
		if int(read.CellCapacity) > maximum {
			maximum = int(read.CellCapacity)
		}
	}
	return maximum
}

// CandidateRelation returns the normalized candidate relation address. A zero
// value is returned for a descriptor that is unavailable or predates the
// schema-owned Plan projection; callers should check Available first.
func (rule CompiledRule) CandidateRelation() ruleplan.RelationAddr {
	if !rule.Available() || !rule.planGeometry {
		return ruleplan.RelationAddr{}
	}
	return rule.candidateRelation
}

// IssuedCandidate reports whether this rule's candidate rows are Program rows
// rather than rows of a Factor axis. CandidateRelation is zero when it is.
func (rule CompiledRule) IssuedCandidate() bool {
	return rule.Available() && rule.planGeometry && rule.issuedCandidate
}

// JoinRelation returns the first generated join's normalized relation address.
// Use ReadAt for the complete ordered join table.
func (rule CompiledRule) JoinRelation() ruleplan.RelationAddr {
	if !rule.Available() || !rule.planGeometry || len(rule.reads) == 0 {
		return ruleplan.RelationAddr{}
	}
	return rule.reads[0].Relation
}

// KeyProjection returns the generated join's normalized key projection address.
func (rule CompiledRule) KeyProjection() ruleplan.ProjectionAddr {
	if !rule.Available() || !rule.planGeometry {
		return ruleplan.ProjectionAddr{}
	}
	if len(rule.reads) == 0 {
		return ruleplan.ProjectionAddr{}
	}
	return rule.reads[0].Key
}

// ReadFormAt returns the sealed read form for one ordered join.
func (rule CompiledRule) ReadFormAt(index int) (ruleprogram.ReadForm, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleprogram.ReadFormInvalid, false
	}
	return read.Form, true
}

// ReadContractAt returns the complete sealed contract for one ordered join.
// The contract contains cardinality and opaque/sparsity outcome policy; the
// denominator address is returned separately by ReadDenominatorAt.
func (rule CompiledRule) ReadContractAt(index int) (ruleplan.ReadContract, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.ReadContract{}, false
	}
	return read.Contract, true
}

// ReadDenominatorAt returns the sealed complete-denominator address for one
// ordered join. Present distinguishes the valid zero ordinal from absence.
func (rule CompiledRule) ReadDenominatorAt(index int) (ruleplan.DenominatorAddr, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.DenominatorAddr{}, false
	}
	return read.Denominator, true
}

// ReadPredicateAt returns the optional owner-qualified predicate projection
// for one ordered join.
func (rule CompiledRule) ReadPredicateAt(index int) (ruleplan.ProjectionAddr, bool, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.ProjectionAddr{}, false, false
	}
	return read.Predicate, read.PredicatePresent, true
}

// ReadParentAt returns the sealed parent relation address and its presence for
// one ordered join: the relation whose candidate rows this read's relation
// nests under. A read over a relation that is not a nested member set names
// none.
func (rule CompiledRule) ReadParentAt(index int) (ruleplan.RelationAddr, bool, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.RelationAddr{}, false, false
	}
	return read.Parent, read.ParentPresent, true
}

// MemberAddressed reports whether this read's cells are addressed one at a
// time - by a nested member set, or by a span its candidate published - rather
// than delivered through the Factor's own summary cursor. It is the ONE
// statement of that distinction, because the cold row kind and the bound
// read's kind are checked against each other and would drift apart the moment
// each decided it for itself.
func (read ReadPlan) MemberAddressed() bool {
	return read.ParentPresent || read.KeyVectorPresent
}

// ReadKeyVectorAt returns the sealed directory whose rows publish the key
// vector one ordered join is taken over, and its presence. A read spanned by a
// predicate or a member set names none.
func (rule CompiledRule) ReadKeyVectorAt(index int) (ruleplan.RelationAddr, bool, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.RelationAddr{}, false, false
	}
	return read.KeyVector, read.KeyVectorPresent, true
}

// ReadAddressingAt returns the sealed addressing directory and its presence
// for one ordered join: the relation whose candidate ordinal indexes this
// read. A read the rule's candidate ordinal does not index names none.
func (rule CompiledRule) ReadAddressingAt(index int) (ruleplan.RelationAddr, bool, bool) {
	read, ok := rule.ReadAt(index)
	if !ok {
		return ruleplan.RelationAddr{}, false, false
	}
	return read.Addressing, read.AddressingPresent, true
}

// Reducer returns the normalized reducer address.
func (rule CompiledRule) Reducer() ruleplan.ReducerAddr {
	if !rule.Available() || !rule.planGeometry {
		return ruleplan.ReducerAddr{}
	}
	return rule.reducer
}

// OutputAddress returns the normalized dense output-frame address.
func (rule CompiledRule) OutputAddress() ruleplan.OutputAddr {
	if !rule.Available() || !rule.planGeometry || len(rule.outputs) == 0 {
		return ruleplan.OutputAddr{}
	}
	return rule.outputs[0].Address
}

// DestinationProjection returns the normalized output destination projection address.
func (rule CompiledRule) DestinationProjection() ruleplan.ProjectionAddr {
	if !rule.Available() || !rule.planGeometry || len(rule.outputs) == 0 {
		return ruleplan.ProjectionAddr{}
	}
	return rule.outputs[0].Destination
}

// OutputMode returns the canonical disposition of the one sealed output row.
func (rule CompiledRule) OutputMode() (ruleprogram.OutputMode, bool) {
	output, ok := rule.OutputAt(0)
	if !ok {
		return ruleprogram.ModeInvalid, false
	}
	return output.Mode, true
}

// ReadAxis returns the runtime Factor ordinal of the generated read.
func (rule CompiledRule) ReadAxis() uint32 {
	if !rule.Available() {
		return 0
	}
	if rule.planGeometry {
		if len(rule.reads) == 0 {
			return 0
		}
		return rule.reads[0].Axis
	}
	if len(rule.reads) == 0 {
		return 0
	}
	return rule.reads[0].Factor
}

// OutputAxis returns the runtime Factor ordinal of the generated output.
func (rule CompiledRule) OutputAxis() uint32 {
	if !rule.Available() || len(rule.outputs) == 0 {
		return 0
	}
	return rule.outputs[0].Axis
}

// CarryIdentity reports whether the sealed carry hands the prior output fact
// on unchanged. A transformed carry answers false and names its transform
// through CarryTransform.
func (rule CompiledRule) CarryIdentity() bool {
	return rule.Available() && rule.carry != nil && rule.carry.Identity && rule.planGeometry
}

// CarryInput returns the carried input port.  It is -1 for an unavailable
// descriptor; zero is a valid input only after Available has been checked.
func (rule CompiledRule) CarryInput() int {
	if !rule.Available() || !rule.planGeometry || rule.carry == nil {
		return -1
	}
	return int(rule.carry.Input)
}

// CarryMode returns the sealed carry disposition.
func (rule CompiledRule) CarryMode() (ruleprogram.CarryMode, bool) {
	if !rule.Available() || rule.carry == nil {
		return ruleprogram.CarryModeInvalid, false
	}
	return rule.carry.Mode, true
}

// CarryTransform returns the owner-qualified transform address when present.
// It is the one authority for which transform a carried fact passes through;
// no consumer may derive a member from the carry input or the output factor.
func (rule CompiledRule) CarryTransform() (ruleplan.CarryTransformAddr, bool) {
	if !rule.Available() || rule.carry == nil || !rule.carry.TransformPresent {
		return ruleplan.CarryTransformAddr{}, false
	}
	return rule.carry.Transform, true
}
