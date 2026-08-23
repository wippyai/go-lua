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
	contentVersion = 7
)

const (
	contentRecordProgram   uint64 = 1
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
)

// Program is the complete Rule-owned cold declaration. The zero value is the
// explicit migration ratchet for families that have not yet crossed from the
// legacy catalog; every present declaration must carry a Candidate and Fold.
type Program struct {
	OperandRole schema.Key
	Candidate   member.RelationRef
	Joins       []JoinDecl
	Fold        FoldDecl
	Carry       *CarryDecl
}

func (program Program) Available() bool {
	return program.OperandRole.Available() || program.Candidate.Declared() || len(program.Joins) != 0 ||
		program.Fold.Reducer.Declared() || len(program.Fold.Inputs) != 0 ||
		len(program.Fold.Outputs) != 0 || program.Carry != nil
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
		if !join.normalFormFor(index, program.routeAllowsOptionalPredicate(index)) {
			return Problem{Join: index, Kind: ProblemJoin}, false
		}
	}
	if program.Carry != nil && !program.Carry.Available() {
		return Problem{Kind: ProblemCarry}, false
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

// routeAllowsOptionalPredicate reports whether this join is explicitly named
// by a routed output. The route row remains the single source of this
// allowance; selected joins are not inferred from their count or position.
func (program Program) routeAllowsOptionalPredicate(index int) bool {
	if index < 0 || index >= len(program.Joins) {
		return false
	}
	for _, output := range program.Fold.Outputs {
		if output.Mode == ModeRoute && output.RouteJoinPresent && int(output.RouteJoin) == index {
			return program.Joins[index].Read.Form == Selected
		}
	}
	return false
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
	if program.Candidate.Declared() {
		references = append(references, program.Candidate.EntryReference())
	}
	for _, join := range program.Joins {
		references = append(references, join.References()...)
	}
	if program.Carry != nil {
		references = append(references, program.Carry.References()...)
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
	if err := writeMemberReference(content, program.Candidate.Axis, program.Candidate.Member); err != nil {
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
