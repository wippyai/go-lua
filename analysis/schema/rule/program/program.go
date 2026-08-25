// Package program owns the callback-free, domain-neutral cold declaration
// attached to a Rule. A Program is a candidate relation, an ordered uniform
// join list, and one reducer/output declaration. The five census forms are
// sealed normal forms of those data; they are never runtime opcodes or shape
// types.
package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	contentDomain = "wippy.analysis/schema/rule/program"
	// Version 7 carries the explicit route-producing JoinRef on routed output
	// rows. Runtime binding must resolve this row from the sealed vocabulary;
	// callers never infer it from selected-join cardinality.
	//
	// Version 8 carries each read's authored PointBoundDecl. The predecessor
	// topology width a rule expects is no longer re-derived from Form at
	// construction time; it is this declared fact.
	//
	// Version 9 carries each join's declared Parent: the restated
	// MemberParent/MemberOrdinal fact that licenses a Summary read over a
	// self-provided nested member set to declare no Predicate.
	contentVersion = 9
)

const (
	contentRecordProgram uint64 = 1
	// contentRecordCandidate is written only by a Program whose candidate is an
	// issued Program row. The axis-relation arm emits the exact member
	// reference it emitted before the choice existed, so stating the choice
	// remints no program that keeps the arm it already had.
	contentRecordCandidate uint64 = 2
	contentRecordJoin      uint64 = 3
	contentRecordSource    uint64 = 4
	contentRecordRead      uint64 = 5
	contentRecordFold      uint64 = 6
	contentRecordInput     uint64 = 7
	contentRecordOutput    uint64 = 8
	contentRecordReference uint64 = 9
	contentRecordCarry     uint64 = 10
	contentRecordOperand   uint64 = 11
	// contentRecordTransport is written only by a Program that declares an
	// activation transport vector. A rule that carries none emits the exact
	// stream it emitted before the vector existed, so adding it remints no
	// program that does not use it.
	contentRecordTransport uint64 = 12
	// contentRecordActivation is written only by a Program that declares the
	// branch identities of a structural publication.
	contentRecordActivation uint64 = 13
)

// TransportDecl is one axis carried across an activation edge. The row's
// existence is the import direction - the trigger seeds the mounted body's
// entry Points on this axis - and Exported is the return direction, the body's
// exit Points carried back to that same trigger.
//
// One row is one axis, so a Factor named on both sides is one bidirectional
// transport rather than two authorities, and an export whose axis has no
// import cannot be written down: the symmetry the issuer used to seal over two
// lists is a property of this shape.
type TransportDecl struct {
	Axis     AxisRef
	Exported bool
}

func (transport TransportDecl) Available() bool { return transport.Axis.Available() }

// ActivationDecl is the branch vocabulary of a structural publication: the
// owner-issued identities the construct plane mounts one activation member AS.
//
// They are declared and not derived because the analyzer minted none of them.
// A module, a body path, the semantic axis a role is issued under - each names
// a subject outside this analyzer, so no dense coordinate carries one and no
// engine rule could reconstruct one from the row's address. Every field is
// therefore an owner-issued projection in the member.Identity role.
//
// The execution CONTEXT a branch runs on is deliberately absent. Which
// Contexts two modules are connected by is the Link's own sealed directory,
// which the issuance pass already holds; a rule's axis restating it would be a
// second authority over the Link's relation.
type ActivationDecl struct {
	// Branch is the relation whose members are the candidate branches: a
	// nested member set hanging off the rule's own candidate row.
	//
	// It is ENUMERATED and never read. A branch's fact is not part of any
	// judgment - the trigger's own value and the branch's identity settle it -
	// and a branch has no coordinate of its own to be read at, so a read here
	// would deliver the trigger's cell once per branch. The owner publishes
	// this set through MemberCount/MemberAt, and the issuance pass walks it
	// directly.
	Branch member.RelationRef
	// Transport is the ordered vector of axes one activation branch carries
	// across its edge. The vector belongs to the activation vocabulary because
	// it is instantiated by each branch; keeping it here prevents a second
	// Program-level authority from drifting away from the branch set.
	Transport []TransportDecl
	// Application is the identity the trigger row is applied under, projected
	// from the rule's own candidate row rather than from a branch: every
	// branch of one trigger is an alternative of the same application.
	Application member.ProjectionRef
	// Target and Endpoint are the two semantic axes the transition one branch
	// runs on connects, projected from the branch row.
	Target   member.ProjectionRef
	Endpoint member.ProjectionRef
	// Mount and Body name the module and the body path the branch lands in.
	// They are what resolves the branch's entry and exit Points in the
	// constructed point plane.
	Mount member.ProjectionRef
	Body  member.ProjectionRef
}

// Available reports whether every identity the mounted branch is keyed or
// resolved by is declared. The row is whole or absent: a branch missing any
// one of them is a member the construct plane could not address.
func (activation ActivationDecl) Available() bool {
	return activation.Branch.Available() && activation.Application.Available() && activation.Target.Available() &&
		activation.Endpoint.Available() && activation.Mount.Available() && activation.Body.Available()
}

// projections is the branch vocabulary in declaration order. One order serves
// the digest and the reference list, so the two can never disagree about which
// identities a branch carries.
func (activation ActivationDecl) projections() []member.ProjectionRef {
	return []member.ProjectionRef{
		activation.Application, activation.Target,
		activation.Endpoint, activation.Mount, activation.Body,
	}
}

func (activation ActivationDecl) references() schema.EntryReferences {
	references := make(schema.EntryReferences, 0, 6+len(activation.Transport))
	if activation.Branch.Declared() {
		references = append(references, activation.Branch.EntryReference())
	}
	for _, transport := range activation.Transport {
		if transport.Axis.Declared() {
			references = append(references, transport.Axis.EntryReference())
		}
	}
	for _, projection := range activation.projections() {
		if projection.Declared() {
			references = append(references, projection.EntryReference())
		}
	}
	return references
}

// Program is the complete Rule-owned cold declaration. The zero value is the
// explicit migration ratchet for families that have not yet crossed from the
// legacy catalog; every present declaration must carry a Candidate and Fold.
type Program struct {
	OperandRole schema.Key
	Candidate   member.CandidateRef
	Joins       []JoinDecl
	Fold        FoldDecl
	Carry       *CarryDecl
	// ActivationRole is the semantic role of the activation family this rule's
	// candidate branches are grouped under. It is the structural sibling of
	// OperandRole: the declaration names a role, and the composition's role
	// vocabulary resolves it to the one semantic identity the cold row claims.
	//
	// A rule that publishes a fact declares none, and a rule that transports a
	// vector across an activation edge declares one. That biconditional is
	// checked below, so a transport vector can never reach a cold row that has
	// no family to be admitted under.
	ActivationRole schema.Key
	// Activation is the branch vocabulary a structural publication mounts its
	// candidate branches as. It stands or falls with the transport vector and
	// the family, under the same biconditional.
	Activation *ActivationDecl
}

func (program Program) Available() bool {
	return program.OperandRole.Available() || program.Candidate.Declared() || len(program.Joins) != 0 ||
		program.Fold.Reducer.Declared() || len(program.Fold.Inputs) != 0 ||
		len(program.Fold.Outputs) != 0 || program.Carry != nil || program.transportCount() != 0 ||
		program.ActivationRole.Available() || program.Activation != nil
}

func (program Program) transportCount() int {
	if program.Activation == nil {
		return 0
	}
	return len(program.Activation.Transport)
}

func (program Program) JoinCount() int { return len(program.Joins) }

func (program Program) JoinAt(index int) (JoinDecl, bool) {
	if index < 0 || index >= len(program.Joins) {
		return JoinDecl{}, false
	}
	return cloneJoin(program.Joins[index]), true
}

// Clone makes the immutable declaration copy stored by Rule.Template.
func (program Program) Clone() Program {
	joins := append([]JoinDecl(nil), program.Joins...)
	program.Joins = make([]JoinDecl, len(joins))
	for index, join := range joins {
		program.Joins[index] = cloneJoin(join)
	}
	program.Fold = cloneFold(program.Fold)
	if program.Activation != nil {
		activation := *program.Activation
		activation.Transport = append([]TransportDecl(nil), program.Activation.Transport...)
		program.Activation = &activation
	}
	if program.Carry != nil {
		carry := *program.Carry
		program.Carry = &carry
	}
	return program
}

// Problem identifies a data-local structural failure. Cross-surface target
// resolution remains the responsibility of analysis/schema/seal.
type Problem struct {
	Join   int
	Input  int
	Output int
	Kind   ProblemKind
}

type ProblemKind uint8

const (
	ProblemNone ProblemKind = iota
	ProblemOperand
	ProblemCandidate
	ProblemJoin
	ProblemInput
	ProblemOutput
	ProblemFold
	ProblemCarry
	ProblemTransport
	ProblemActivation
)

func (problem Problem) Available() bool { return problem.Kind != ProblemNone }

// Check seals source ordering, the five normal forms, fold connectivity, and
// output arity without reopening any owner schema.
func (program Program) Check() (Problem, bool) {
	if !program.Available() {
		return Problem{}, true
	}
	if !program.OperandRole.Available() {
		return Problem{Kind: ProblemOperand}, false
	}
	if !program.Candidate.Available() {
		return Problem{Kind: ProblemCandidate}, false
	}
	for index, join := range program.Joins {
		if !join.normalForm(index) {
			return Problem{Join: index, Kind: ProblemJoin}, false
		}
	}
	if program.Carry != nil && !program.Carry.Available() {
		return Problem{Kind: ProblemCarry}, false
	}
	if !program.checkTransport() {
		return Problem{Kind: ProblemTransport}, false
	}
	if !program.checkActivation() {
		return Problem{Kind: ProblemActivation}, false
	}
	if _, contiguous := program.inputCount(); !contiguous {
		return Problem{Kind: ProblemInput}, false
	}
	if problem := program.Fold.check(len(program.Joins)); problem != foldProblemNone {
		switch problem {
		case foldProblemInputs:
			return Problem{Kind: ProblemInput}, false
		case foldProblemOutputs:
			return Problem{Kind: ProblemOutput}, false
		default:
			return Problem{Kind: ProblemFold}, false
		}
	}
	if problem, valid := program.checkRoutes(); !valid {
		return problem, false
	}
	if problem, valid := program.checkReachability(); !valid {
		return problem, false
	}
	return Problem{}, true
}

// CheckAgainst is Check plus the reducer-shape agreement, which Check alone
// cannot decide.
//
// A fold's call shape is the OWNER's statement: how many arguments it takes,
// and in which form each arrives. Check sees only the Program, so a
// declaration that passes the reducer a join it does not consume is
// well-formed to Check and wrong against the row it names. That is a gate a
// declaration package can close before any schema exists, because the reducer
// it names belongs to the axis it writes and it holds that catalog.
//
// A Program that passes this is not thereby admitted: which carrier each join
// yields is the joined axis's statement and stays with the Plan compiler,
// which applies these same clauses through one implementation.
func (program Program) CheckAgainst(reducer member.Reducer) (Problem, bool) {
	if problem, valid := program.Check(); !valid {
		return problem, false
	}
	if !program.Available() {
		return Problem{}, true
	}
	switch program.Fold.checkAgainst(program.Joins, reducer) {
	case foldProblemNone:
		return Problem{}, true
	case foldProblemInputs:
		return Problem{Kind: ProblemInput}, false
	case foldProblemOutputs:
		return Problem{Kind: ProblemOutput}, false
	default:
		return Problem{Kind: ProblemFold}, false
	}
}

// checkReachability states that every declared join is reached from the fold's
// arguments through the source graph: a join nothing depends on and nothing
// folds is a row the Program does not use.
func (program Program) checkReachability() (Problem, bool) {
	if len(program.Joins) != 0 {
		reachable := make(map[uint64]struct{}, len(program.Joins))
		pending := make([]uint64, 0, len(program.Fold.Inputs))
		for _, input := range program.Fold.Inputs {
			pending = append(pending, uint64(input))
		}
		for len(pending) != 0 {
			last := len(pending) - 1
			position := pending[last]
			pending = pending[:last]
			if _, seen := reachable[position]; seen {
				continue
			}
			reachable[position] = struct{}{}
			for _, source := range program.Joins[position].Sources {
				if !source.Candidate {
					pending = append(pending, source.Position)
				}
			}
		}
		for index := range program.Joins {
			if _, used := reachable[uint64(index)]; !used {
				return Problem{Join: index, Kind: ProblemJoin}, false
			}
		}
	}
	return Problem{}, true
}

// checkActivation seals the branch vocabulary against the declaration that
// needs it.
//
// The vocabulary and the transport vector are one statement, on the same
// biconditional the family is held to: a structural publication that names no
// branch identities has not said what the construct plane would mount, and a
// fact-writing rule that names them has declared a vocabulary nothing reads.
//
// The branch relation is named directly rather than through a join, because
// the set is enumerated and never read: the owner publishes it under the
// trigger's candidate row and the issuance pass walks it there. Whether it is
// a nested member set of this rule's own candidate is the catalog's answer,
// checked where the catalog is in scope.
func (program Program) checkActivation() bool {
	if (program.Activation != nil) != (len(program.Transport) != 0) {
		return false
	}
	if program.Activation == nil {
		return true
	}
	return program.Activation.Available()
}

// checkTransport seals the activation transport vector: every row names a
// declared axis, and one axis crosses the edge exactly once. Two rows for one
// axis would be two authorities for one crossing, which is the defect the
// single-list shape exists to make unwritable.
func (program Program) checkTransport() bool {
	// The family and the vector are one declaration. A vector with no family
	// names candidate branches nothing groups; a family with no vector groups
	// branches that instantiate nothing.
	if (len(program.Transport) != 0) != program.ActivationRole.Available() {
		return false
	}
	if len(program.Transport) == 0 {
		return true
	}
	seen := make(map[schema.EntryReference]struct{}, len(program.Transport))
	for _, transport := range program.Transport {
		if !transport.Available() {
			return false
		}
		reference := transport.Axis.EntryReference()
		if _, duplicate := seen[reference]; duplicate {
			return false
		}
		seen[reference] = struct{}{}
	}
	return true
}

// checkRoutes seals the route-specific part of a Fold without reopening any
// owner schema. Each route is a bounded publication row: it is backed by a
// selected source with finite read multiplicity and a declared denominator.
// Route destinations are unique even though ordinary exact outputs may
// intentionally share a destination projection.
func (program Program) checkRoutes() (Problem, bool) {
	seenDestinations := make(map[member.ProjectionRef]struct{})
	for outputIndex, output := range program.Fold.Outputs {
		if output.Mode != ModeRoute && output.RouteJoinPresent {
			return Problem{Output: outputIndex, Kind: ProblemOutput}, false
		}
		if output.Mode != ModeRoute {
			continue
		}
		if !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(len(program.Joins)) {
			return Problem{Output: outputIndex, Kind: ProblemOutput}, false
		}
		routeJoin := program.Joins[output.RouteJoin]
		if routeJoin.Read.Form != Selected || !routeJoin.Read.Contract.DenominatorRef.Available() || routeJoin.Read.Contract.Multiplicity == MultiplicityMany {
			return Problem{Join: int(output.RouteJoin), Output: outputIndex, Kind: ProblemJoin}, false
		}
		foldInput := false
		for _, input := range program.Fold.Inputs {
			if input == output.RouteJoin {
				foldInput = true
				break
			}
		}
		if !foldInput {
			return Problem{Join: int(output.RouteJoin), Output: outputIndex, Kind: ProblemOutput}, false
		}
		if _, duplicate := seenDestinations[output.Destination]; duplicate {
			return Problem{Output: outputIndex, Kind: ProblemOutput}, false
		}
		seenDestinations[output.Destination] = struct{}{}
	}
	return Problem{}, true
}

func (program Program) Valid() bool {
	_, valid := program.Check()
	return valid
}

// References returns all owner-issued references in declaration order. The
// seal subsystem snapshots this provider and validates upward targets only
// after the complete surface catalog is published.
func (program Program) References() schema.EntryReferences {
	var references schema.EntryReferences
	references = append(references, program.Candidate.References()...)
	for _, join := range program.Joins {
		references = append(references, join.References()...)
	}
	if program.Carry != nil {
		references = append(references, program.Carry.References()...)
	}
	for _, transport := range program.Transport {
		if transport.Axis.Declared() {
			references = append(references, transport.Axis.EntryReference())
		}
	}
	if program.Activation != nil {
		references = append(references, program.Activation.references()...)
	}
	return append(references, program.Fold.References()...)
}

// InputCount returns the dense input-port prefix used by all reads and the
// optional carry. An invalid/holey declaration returns zero; Check is the
// authoritative validity result.
func (program Program) InputCount() int {
	count, contiguous := program.inputCount()
	if !contiguous || count > uint64(^uint(0)>>1) {
		return 0
	}
	return int(count)
}

func (program Program) inputCount() (uint64, bool) {
	used := make(map[uint64]struct{}, len(program.Joins)+1)
	var maximum uint64
	var present bool
	for _, join := range program.Joins {
		port := uint64(join.Read.Input)
		used[port] = struct{}{}
		if !present || port > maximum {
			maximum, present = port, true
		}
	}
	if program.Carry != nil {
		port := uint64(program.Carry.Input)
		used[port] = struct{}{}
		if !present || port > maximum {
			maximum, present = port, true
		}
	}
	if !present {
		return 0, true
	}
	for port := uint64(0); ; port++ {
		if _, ok := used[port]; !ok {
			return 0, false
		}
		if port == maximum {
			break
		}
	}
	return maximum + 1, true
}

// WriteContent emits the canonical framed cold declaration stream.
func (program Program) WriteContent(content *framing.Writer) error {
	if err := content.Record(contentRecordProgram); err != nil {
		return err
	}
	if err := content.Record(contentRecordOperand); err != nil {
		return err
	}
	if err := content.String(string(program.OperandRole)); err != nil {
		return err
	}
	if program.Candidate.Issued() {
		if err := content.Record(contentRecordCandidate); err != nil {
			return err
		}
		if err := content.String(string(program.Candidate.IssuedRow)); err != nil {
			return err
		}
	} else if err := writeMemberReference(content, program.Candidate.AxisRelation.Axis, program.Candidate.AxisRelation.Member); err != nil {
		return err
	}
	if err := content.Count(uint64(len(program.Joins))); err != nil {
		return err
	}
	for _, join := range program.Joins {
		if err := content.Record(contentRecordJoin); err != nil {
			return err
		}
		if err := content.Count(uint64(len(join.Sources))); err != nil {
			return err
		}
		for _, source := range join.Sources {
			if err := content.Record(contentRecordSource); err != nil {
				return err
			}
			if err := content.Bool(source.Candidate); err != nil {
				return err
			}
			if err := content.Uint(source.Position); err != nil {
				return err
			}
		}
		if err := writeMemberReference(content, join.Relation.Axis, join.Relation.Member); err != nil {
			return err
		}
		if err := writeMemberReference(content, join.Key.Axis, join.Key.Member); err != nil {
			return err
		}
		if err := writeMemberReference(content, join.Predicate.Axis, join.Predicate.Member); err != nil {
			return err
		}
		if err := writeMemberReference(content, join.Parent.Axis, join.Parent.Member); err != nil {
			return err
		}
		if err := writeReadContent(content, join.Read); err != nil {
			return err
		}
	}
	if err := content.Bool(program.Carry != nil); err != nil {
		return err
	}
	if program.Carry != nil {
		if err := content.Record(contentRecordCarry); err != nil {
			return err
		}
		if err := content.Uint(uint64(program.Carry.Input)); err != nil {
			return err
		}
		if err := content.Uint(uint64(program.Carry.Mode)); err != nil {
			return err
		}
		if err := writeMemberReference(content, program.Carry.Transform.Axis, program.Carry.Transform.Member); err != nil {
			return err
		}
	}
	if len(program.Transport) != 0 {
		if err := content.Record(contentRecordTransport); err != nil {
			return err
		}
		if err := content.Count(uint64(len(program.Transport))); err != nil {
			return err
		}
		for _, transport := range program.Transport {
			if err := writeReference(content, transport.Axis.EntryReference()); err != nil {
				return err
			}
			if err := content.Bool(transport.Exported); err != nil {
				return err
			}
		}
		if err := content.String(string(program.ActivationRole)); err != nil {
			return err
		}
	}
	if program.Activation != nil {
		if err := content.Record(contentRecordActivation); err != nil {
			return err
		}
		if err := writeMemberReference(content, program.Activation.Branch.Axis, program.Activation.Branch.Member); err != nil {
			return err
		}
		for _, reference := range program.Activation.projections() {
			if err := writeMemberReference(content, reference.Axis, reference.Member); err != nil {
				return err
			}
		}
	}
	if err := content.Record(contentRecordFold); err != nil {
		return err
	}
	if err := writeMemberReference(content, program.Fold.Reducer.Axis, program.Fold.Reducer.Member); err != nil {
		return err
	}
	if err := content.Record(contentRecordInput); err != nil {
		return err
	}
	if err := content.Count(uint64(len(program.Fold.Inputs))); err != nil {
		return err
	}
	for _, input := range program.Fold.Inputs {
		if err := content.Uint(uint64(input)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(program.Fold.Outputs))); err != nil {
		return err
	}
	for _, output := range program.Fold.Outputs {
		if err := content.Record(contentRecordOutput); err != nil {
			return err
		}
		if err := writeOutputReference(content, output.Column); err != nil {
			return err
		}
		if err := writeMemberReference(content, output.Destination.Axis, output.Destination.Member); err != nil {
			return err
		}
		if err := content.Uint(uint64(output.Mode)); err != nil {
			return err
		}
		if err := content.Uint(uint64(output.ValueSlot)); err != nil {
			return err
		}
		if err := content.Bool(output.RouteJoinPresent); err != nil {
			return err
		}
		if err := content.Uint(uint64(output.RouteJoin)); err != nil {
			return err
		}
	}
	return nil
}

func writeReadContent(content *framing.Writer, read ReadDecl) error {
	if err := content.Record(contentRecordRead); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Input)); err != nil {
		return err
	}
	if err := writeReference(content, read.Axis.EntryReference()); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Form)); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Contract.Order)); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Contract.Sparse)); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Contract.OnOpaque)); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.Contract.Multiplicity)); err != nil {
		return err
	}
	if err := content.Uint(uint64(read.PointBound)); err != nil {
		return err
	}
	return writeReference(content, read.Contract.DenominatorRef.EntryReference())
}

func writeReference(content *framing.Writer, reference schema.EntryReference) error {
	if err := content.Record(contentRecordReference); err != nil {
		return err
	}
	if err := content.Uint(uint64(reference.Surface)); err != nil {
		return err
	}
	return content.String(string(reference.Key))
}

func writeMemberReference(content *framing.Writer, axis schema.EntryReference, member schema.Key) error {
	if err := content.Record(contentRecordReference); err != nil {
		return err
	}
	if err := content.Uint(uint64(axis.Surface)); err != nil {
		return err
	}
	if err := content.String(string(axis.Key)); err != nil {
		return err
	}
	return content.String(string(member))
}

func writeOutputReference(content *framing.Writer, output axis.OutputRef) error {
	return writeMemberReference(content, output.Axis, output.Key)
}

// Digest derives the versioned identity of this Program's canonical bytes.
func (program Program) Digest() identity.ContentID {
	if !program.Valid() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var content framing.Writer
	if content.Reset(hash, contentDomain, contentVersion) != nil || program.WriteContent(&content) != nil || content.Finish() != nil {
		return identity.ContentID{}
	}
	var digest identity.ContentID
	copy(digest[:], hash.Sum(nil))
	return digest
}
