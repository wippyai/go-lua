package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (s *evalState) addSequence(from, to, owner keyspace.Term) error {
	fromEndpoint, fromOK := s.finishEndpoint(from)
	toEndpoint, toOK := s.entries.Entry(to)
	if !fromOK || !toOK {
		if !fromOK && s.evaluates(from) && !s.static(from) || !toOK && s.evaluates(to) && !s.static(to) {
			return fmt.Errorf("program/flow/causal: live evaluation endpoint %v -> %v has no typed port", from, to)
		}
		// Static/dead operands are not causal endpoints. The surrounding live
		// operation remains valid; its next live child is reached by the next
		// typed clause or by the enclosing structural route.
		return nil
	}
	if keyspace.TermFamily(fromEndpoint) == keyspace.FamilyCall {
		// A Call's normal arm belongs exclusively to CallBoundary. Incoming
		// evaluation routes into a Call are still local Edges.
		return nil
	}
	if fromEndpoint == toEndpoint {
		return nil
	}
	return s.appendEdge(fromEndpoint, toEndpoint, owner, 0, false, -1)
}

// addWriteCommit emits one already-proved assignment commit route and records
// only its existing Edge row for the later Successor projection. Evaluation
// routes into the same Write are deliberately not recorded: an indexed Write
// can have both an operand-evaluation predecessor and a reverse commit
// predecessor, so the commit role must be fixed at the owner emission site.
func (s *evalState) addWriteCommit(from, write, owner keyspace.Term) error {
	if keyspace.TermFamily(write) != keyspace.FamilyWrite || keyspace.TermOrdinal(write) == 0 ||
		uint64(keyspace.TermOrdinal(write)) >= uint64(len(s.writeCommitSet)) {
		return errors.New("program/flow/causal: assignment commit Write is unavailable")
	}
	before := len(s.edgeRows)
	if err := s.addSequence(from, write, owner); err != nil {
		return err
	}
	if len(s.edgeRows) == before {
		// Static/dead endpoints intentionally produce no causal route.
		return nil
	}
	if len(s.edgeRows) != before+1 {
		return errors.New("program/flow/causal: assignment commit emitted multiple Edges")
	}
	edgeIndex := uint32(before)
	expected, expectedOK := s.finishEndpoint(write)
	if !expectedOK || s.edgeRows[before].To != expected {
		return errors.New("program/flow/causal: assignment commit Edge does not terminate at Write Finish")
	}
	ordinal := keyspace.TermOrdinal(write)
	if s.writeCommitSet[ordinal] && s.writeCommitEdges[ordinal] != edgeIndex {
		return errors.New("program/flow/causal: duplicate assignment commit predecessor")
	}
	s.writeCommitEdges[ordinal] = edgeIndex
	s.writeCommitSet[ordinal] = true
	return nil
}

// tableFieldEntry keeps an unavailable exact-key normalization out of the
// evaluation plane.  Such a FieldExact already has its typed Field -> Throw
// operation outcome; entering its live Field term is the only route that can
// reach that failure.  Valid exact keys (including a runtime UnaryNeg) retain
// the ordinary key -> Values sequencing.
func (s *evalState) tableFieldEntry(field keyspace.Term) (keyspace.Term, bool) {
	_, _, _, _, ok := s.flow.Fields().Get(field)
	if !ok {
		return 0, false
	}
	if s.invalidExactField(field) {
		return field, true
	}
	return s.entries.Entry(field)
}

func (s *evalState) addTableRoute(from, field, owner keyspace.Term, entry bool) error {
	var fromEndpoint keyspace.Term
	var fromOK bool
	if entry {
		fromEndpoint, fromOK = s.entries.Entry(from)
	} else {
		fromEndpoint, fromOK = s.finishEndpoint(from)
	}
	toEndpoint, toOK := s.tableFieldEntry(field)
	if !fromOK || !toOK {
		if !fromOK && s.evaluates(from) && !s.static(from) || !toOK && s.evaluates(field) && !s.static(field) {
			return fmt.Errorf("program/flow/causal: live TableField endpoint %v -> %v has no typed port", from, field)
		}
		return nil
	}
	if keyspace.TermFamily(fromEndpoint) == keyspace.FamilyCall {
		return nil
	}
	if fromEndpoint == toEndpoint {
		return nil
	}
	return s.appendEdge(fromEndpoint, toEndpoint, owner, 0, false, -1)
}

// addFinish commits the next typed evaluation result. Entry is for an
// operand that still has to be evaluated; Finish is for the enclosing
// operation/Values/Lens/Return endpoint itself. Keeping the two formulas
// separate prevents a final child from re-entering the operation's Entry
// operand (for example, Binary right -> Binary left).
func (s *evalState) addFinish(from, to, owner keyspace.Term) error {
	fromEndpoint, fromOK := s.finishEndpoint(from)
	toEndpoint, toOK := s.finishEndpoint(to)
	if !fromOK || !toOK {
		if !fromOK && s.evaluates(from) && !s.static(from) || !toOK && s.evaluates(to) && !s.static(to) {
			return fmt.Errorf("program/flow/causal: live evaluation commit %v -> %v has no typed port", from, to)
		}
		return nil
	}
	if keyspace.TermFamily(fromEndpoint) == keyspace.FamilyCall {
		return nil
	}
	if fromEndpoint == toEndpoint {
		return nil
	}
	return s.appendEdge(fromEndpoint, toEndpoint, owner, 0, false, -1)
}

func (s *evalState) addGuard(from, to, owner, decision keyspace.Term, truth bool, arcIndex int) error {
	fromEndpoint, fromOK := s.finishEndpoint(from)
	toEndpoint, toOK := s.entries.Entry(to)
	if !fromOK || !toOK {
		if !fromOK && s.evaluates(from) && !s.static(from) || !toOK && s.evaluates(to) && !s.static(to) {
			return fmt.Errorf("program/flow/causal: live guarded endpoint %v -> %v has no typed port", from, to)
		}
		return nil
	}
	if keyspace.TermFamily(fromEndpoint) == keyspace.FamilyCall {
		return nil
	}
	if fromEndpoint == toEndpoint {
		return nil
	}
	return s.appendEdge(fromEndpoint, toEndpoint, owner, decision, truth, arcIndex)
}

func (s *evalState) addGuardFinish(from, to, owner, decision keyspace.Term, truth bool, arcIndex int) error {
	fromEndpoint, fromOK := s.finishEndpoint(from)
	toEndpoint, toOK := s.finishEndpoint(to)
	if !fromOK || !toOK {
		if !fromOK && s.evaluates(from) && !s.static(from) || !toOK && s.evaluates(to) && !s.static(to) {
			return fmt.Errorf("program/flow/causal: live guarded commit %v -> %v has no typed port", from, to)
		}
		return nil
	}
	if keyspace.TermFamily(fromEndpoint) == keyspace.FamilyCall {
		return nil
	}
	if fromEndpoint == toEndpoint {
		return nil
	}
	return s.appendEdge(fromEndpoint, toEndpoint, owner, decision, truth, arcIndex)
}

// planCallEntry/planCallFinish are the only normal-continuation writes for a
// Call found in an evaluation relation. They operate on the dense seal-local
// plan; the published CallBoundary is the sole retained representation.
func (s *evalState) planCallEntry(call, target keyspace.Term) error {
	if keyspace.TermFamily(call) != keyspace.FamilyCall || !s.evaluates(call) || s.tailPlans[keyspace.TermOrdinal(call)] != 0 {
		return nil
	}
	to, ok := s.entries.Entry(target)
	if !ok {
		return fmt.Errorf("program/flow/causal: live Call %v continuation target %v has no Entry port", call, target)
	}
	return s.setCallPlan(call, callNormalRoute{normal: to, mode: boundaryDirect})
}

func (s *evalState) planCallFinish(call, term keyspace.Term) error {
	if keyspace.TermFamily(call) != keyspace.FamilyCall || !s.evaluates(call) || s.tailPlans[keyspace.TermOrdinal(call)] != 0 {
		return nil
	}
	to, ok := s.finishEndpoint(term)
	if !ok {
		return fmt.Errorf("program/flow/causal: live Call %v continuation term %v has no Finish port", call, term)
	}
	return s.setCallPlan(call, callNormalRoute{normal: to, mode: boundaryDirect})
}

func (s *evalState) planCallSelect(call, selectTerm, right keyspace.Term, operation kind.SelectOp) error {
	if keyspace.TermFamily(call) != keyspace.FamilyCall || !s.evaluates(call) || s.tailPlans[keyspace.TermOrdinal(call)] != 0 {
		return nil
	}
	other, ok := s.entries.Entry(right)
	if !ok {
		if s.static(right) || !s.evaluates(right) {
			// A static/dead right operand contributes no runtime endpoint. Both
			// short-circuit arms still remain represented in the closed
			// Boundary plane, with the right arm collapsing to Select Finish.
			other = selectTerm
		} else {
			return fmt.Errorf("program/flow/causal: Select Call %v right operand has no Entry port", call)
		}
	}
	mode := boundarySelectAnd
	if operation == kind.SelectOr {
		mode = boundarySelectOr
	}
	return s.setCallPlan(call, callNormalRoute{normal: selectTerm, other: other, mode: mode})
}

func (s *evalState) emitEvaluation() error {
	values := s.flow.Values()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyValues]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyValues, ordinal)
		if !s.evaluates(term) {
			continue
		}
		length, ok := values.Len(term)
		if !ok || length < 0 {
			return errors.New("program/flow/causal: Values range is unavailable")
		}
		owner, tail, ok := values.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Values owner")
		}
		previous := keyspace.Term(0)
		for index := 0; index < length; index++ {
			member, memberOK := values.Member(term, index)
			if !memberOK {
				return errors.New("program/flow/causal: Values member is unavailable")
			}
			if !s.evaluates(member) {
				continue
			}
			if previous != 0 {
				if err := s.planCallEntry(previous, member); err != nil {
					return err
				}
				if err := s.addSequence(previous, member, owner); err != nil {
					return err
				}
			}
			previous = member
		}
		if tail != 0 && s.evaluates(tail) {
			if previous != 0 {
				if err := s.planCallEntry(previous, tail); err != nil {
					return err
				}
				if err := s.addSequence(previous, tail, owner); err != nil {
					return err
				}
			}
			previous = tail
		}
		if previous != 0 {
			if keyspace.TermFamily(previous) == keyspace.FamilyCall {
				if err := s.planCallFinish(previous, term); err != nil {
					return err
				}
			}
			if err := s.addFinish(previous, term, owner); err != nil {
				return err
			}
		}
	}

	binds := s.flow.Storage().Binds()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyBind]; ordinal++ {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, ordinal)
		if !s.evaluates(bind) {
			continue
		}
		owner, values, ok := binds.Get(bind)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Bind")
		}
		if err := s.planCallFinish(values, bind); err != nil {
			return err
		}
		if err := s.addFinish(values, bind, owner); err != nil {
			return err
		}
	}

	exact := s.flow.Access().Exact()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyLensExact]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLensExact, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, base, sourceTerm, fieldKind, ok := exact.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed exact Lens")
		}
		if fieldKind == kind.FieldExact && sourceTerm != 0 && s.evaluates(sourceTerm) {
			if err := s.planCallEntry(base, sourceTerm); err != nil {
				return err
			}
			if err := s.planCallFinish(sourceTerm, term); err != nil {
				return err
			}
			if err := s.addSequence(base, sourceTerm, owner); err != nil {
				return err
			}
			if err := s.addFinish(sourceTerm, term, owner); err != nil {
				return err
			}
		} else {
			if err := s.planCallFinish(base, term); err != nil {
				return err
			}
			if err := s.addFinish(base, term, owner); err != nil {
				return err
			}
		}
	}

	dynamic := s.flow.Access().Dynamic()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyLensKey]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLensKey, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, base, key, ok := dynamic.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed dynamic Lens")
		}
		if s.evaluates(key) {
			if err := s.planCallEntry(base, key); err != nil {
				return err
			}
			if err := s.planCallFinish(key, term); err != nil {
				return err
			}
			if err := s.addSequence(base, key, owner); err != nil {
				return err
			}
			if err := s.addFinish(key, term, owner); err != nil {
				return err
			}
		} else {
			if err := s.planCallFinish(base, term); err != nil {
				return err
			}
			if err := s.addFinish(base, term, owner); err != nil {
				return err
			}
		}
	}

	reads := s.flow.Storage().Reads()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyRead]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyRead, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, sourceTerm, _, ok := reads.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Read")
		}
		if keyspace.TermFamily(sourceTerm) != keyspace.FamilyCell {
			if err := s.planCallFinish(sourceTerm, term); err != nil {
				return err
			}
			if err := s.addFinish(sourceTerm, term, owner); err != nil {
				return err
			}
		}
	}

	unaries := s.flow.Operators().Unaries()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyUnary]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyUnary, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, _, operand, ok := unaries.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Unary")
		}
		if s.invalidExactFieldKey(term) {
			continue
		}
		if err := s.planCallFinish(operand, term); err != nil {
			return err
		}
		if err := s.addFinish(operand, term, owner); err != nil {
			return err
		}
	}

	binaries := s.flow.Operators().Binaries()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyBinary]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBinary, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, _, left, right, ok := binaries.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Binary")
		}
		if s.evaluates(right) {
			if err := s.planCallEntry(left, right); err != nil {
				return err
			}
			if err := s.planCallFinish(right, term); err != nil {
				return err
			}
			if err := s.addSequence(left, right, owner); err != nil {
				return err
			}
		} else {
			if err := s.planCallFinish(left, term); err != nil {
				return err
			}
		}
		if s.evaluates(right) {
			if err := s.addFinish(right, term, owner); err != nil {
				return err
			}
		} else if err := s.addFinish(left, term, owner); err != nil {
			return err
		}
	}

	selects := s.flow.Operators().Selects()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilySelect]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilySelect, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, operation, left, right, ok := selects.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Select")
		}
		leftEndpoint, leftOK := s.finishEndpoint(left)
		if leftOK && keyspace.TermFamily(leftEndpoint) != keyspace.FamilyCall {
			if operation == kind.SelectAnd {
				if s.evaluates(right) {
					if err := s.addGuard(left, right, owner, term, true, -1); err != nil {
						return err
					}
				} else if err := s.addGuardFinish(left, term, owner, term, true, -1); err != nil {
					return err
				}
				if err := s.addGuardFinish(left, term, owner, term, false, -1); err != nil {
					return err
				}
			} else {
				if err := s.addGuardFinish(left, term, owner, term, true, -1); err != nil {
					return err
				}
				if s.evaluates(right) {
					if err := s.addGuard(left, right, owner, term, false, -1); err != nil {
						return err
					}
				} else if err := s.addGuardFinish(left, term, owner, term, false, -1); err != nil {
					return err
				}
			}
		} else if leftOK && keyspace.TermFamily(leftEndpoint) == keyspace.FamilyCall {
			if err := s.planCallSelect(leftEndpoint, term, right, operation); err != nil {
				return err
			}
		}
		if err := s.planCallFinish(right, term); err != nil {
			return err
		}
		if err := s.addFinish(right, term, owner); err != nil {
			return err
		}
	}

	claims := s.flow.Claims()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyValueClaim]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyValueClaim, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, operand, _, ok := claims.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed ValueClaim")
		}
		if err := s.planCallFinish(operand, term); err != nil {
			return err
		}
		if err := s.addFinish(operand, term, owner); err != nil {
			return err
		}
	}

	if err := s.emitAssigns(); err != nil {
		return err
	}
	if err := s.emitCalls(); err != nil {
		return err
	}
	if err := s.emitTables(); err != nil {
		return err
	}
	if err := s.emitReturns(); err != nil {
		return err
	}
	return nil
}

func (s *evalState) emitAssigns() error {
	assigns := s.flow.Storage().Assigns()
	writes := s.flow.Storage().Writes()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyAssign]; ordinal++ {
		assign := keyspace.MakeTerm(keyspace.FamilyAssign, ordinal)
		if !s.evaluates(assign) {
			continue
		}
		owner, values, ok := assigns.Get(assign)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Assign")
		}
		count, countOK := assigns.WriteCount(assign)
		if !countOK || count < 0 {
			return errors.New("program/flow/causal: Assign write range is unavailable")
		}
		nonCellTargets := make([]keyspace.Term, 0, count)
		// This temporary sequence is Seal-local. It is deliberately not
		// retained as a second Write relation.
		for index := 0; index < count; index++ {
			write, writeOK := assigns.WriteAt(assign, index)
			if !writeOK {
				return errors.New("program/flow/causal: Assign Write is unavailable")
			}
			parent, target, targetOK := writes.Get(write)
			if !targetOK || parent != assign {
				return errors.New("program/flow/causal: Write parent disagrees with Assign")
			}
			if keyspace.TermFamily(target) != keyspace.FamilyCell && s.evaluates(target) {
				nonCellTargets = append(nonCellTargets, target)
			}
		}
		for index, target := range nonCellTargets {
			next := values
			if index+1 < len(nonCellTargets) {
				next = nonCellTargets[index+1]
			}
			if err := s.planCallEntry(target, next); err != nil {
				return err
			}
			if err := s.addSequence(target, next, owner); err != nil {
				return err
			}
		}
		// The RHS pack releases the last authored Write, then commits in
		// reverse order. Cell targets have no evaluation edge but remain in
		// this exact reverse commit chain.
		if count > 0 {
			lastWrite, lastOK := assigns.WriteAt(assign, count-1)
			if !lastOK {
				return errors.New("program/flow/causal: Assign final Write is unavailable")
			}
			if err := s.addWriteCommit(values, lastWrite, owner); err != nil {
				return err
			}
			for index := count - 1; index > 0; index-- {
				current, currentOK := assigns.WriteAt(assign, index)
				previous, previousOK := assigns.WriteAt(assign, index-1)
				if !currentOK || !previousOK {
					return errors.New("program/flow/causal: Assign reverse Write chain is unavailable")
				}
				if !s.evaluates(current) || !s.evaluates(previous) {
					continue
				}
				if err := s.addWriteCommit(current, previous, owner); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *evalState) emitCalls() error {
	calls := s.flow.Calls()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		if !s.evaluates(call) {
			continue
		}
		owner, callee, _, actuals, ok := calls.Get(call)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Call")
		}
		if err := s.planCallEntry(callee, actuals); err != nil {
			return err
		}
		if err := s.addSequence(callee, actuals, owner); err != nil {
			return err
		}
		actualsFinish, actualsOK := s.finishEndpoint(actuals)
		if actualsOK && s.evaluates(call) {
			if keyspace.TermFamily(actualsFinish) != keyspace.FamilyCall {
				if err := s.appendEdge(actualsFinish, call, owner, 0, false, -1); err != nil {
					return err
				}
			} else {
				// The outer Call's callee and actual Values have already
				// completed. A nested actual Call therefore resumes at the
				// outer invocation term itself, not at Ports.Entry(outerCall)
				// (which would evaluate its callee a second time).
				if err := s.setCallPlan(actualsFinish, callNormalRoute{normal: call, mode: boundaryDirect}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *evalState) emitTables() error {
	tables := s.flow.Tables()
	fields := s.flow.Fields()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTable]; ordinal++ {
		table := keyspace.MakeTerm(keyspace.FamilyTable, ordinal)
		if !s.evaluates(table) {
			continue
		}
		owner, ok := tables.Get(table)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Table")
		}
		count, countOK := tables.FieldCount(table)
		if !countOK || count < 0 {
			return errors.New("program/flow/causal: Table field range is unavailable")
		}
		previous := table
		for index := 0; index < count; index++ {
			field, fieldOK := tables.FieldAt(table, index)
			if !fieldOK {
				return errors.New("program/flow/causal: TableField order is unavailable")
			}
			if !s.evaluates(field) {
				continue
			}
			if previous == table {
				if err := s.addTableRoute(table, field, owner, true); err != nil {
					return err
				}
			} else if !s.invalidExactField(previous) {
				if err := s.addTableRoute(previous, field, owner, false); err != nil {
					return err
				}
			}
			previous = field
		}
		_ = fields
	}

	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTableField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		if !s.evaluates(field) {
			continue
		}
		table, key, values, _, ok := fields.Get(field)
		if !ok {
			return errors.New("program/flow/causal: malformed TableField")
		}
		owner, ownerOK := s.bodyOf(table)
		if !ownerOK || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: TableField owner is unavailable")
		}
		keyUsable := !s.invalidExactField(field)
		if keyUsable && s.evaluates(key) {
			if err := s.planCallEntry(key, values); err != nil {
				return err
			}
			if err := s.addSequence(key, values, owner); err != nil {
				return err
			}
		}
		if err := s.planCallFinish(values, field); err != nil {
			return err
		}
		if err := s.addFinish(values, field, owner); err != nil {
			return err
		}
	}
	return nil
}

func (s *evalState) emitReturns() error {
	returns := s.flow.Control().Returns()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyReturn]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyReturn, ordinal)
		if !s.evaluates(term) {
			continue
		}
		owner, values, ok := returns.Get(term)
		if !ok || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Return")
		}
		if err := s.planCallFinish(values, term); err != nil {
			return err
		}
		if err := s.addFinish(values, term, owner); err != nil {
			return err
		}
	}
	return nil
}

func rootKind(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall, keyspace.FamilyReturn,
		keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyBody:
		return true
	default:
		return false
	}
}

func (s *evalState) nextRoot(bodyTerm keyspace.Term, cursor int) (keyspace.Term, bool) {
	count, ok := s.bodies.RootCount(bodyTerm)
	if !ok || cursor < 0 || cursor >= count {
		return 0, false
	}
	if cursor+1 < count {
		root, rootOK := s.bodies.RootAt(bodyTerm, cursor+1)
		return root, rootOK
	}
	return bodyTerm, true
}

func (s *evalState) nextLiveRoot(bodyTerm keyspace.Term, cursor int) (keyspace.Term, bool) {
	count, ok := s.bodies.RootCount(bodyTerm)
	if !ok || cursor < 0 || cursor >= count {
		return 0, false
	}
	for at := cursor + 1; at < count; at++ {
		root, rootOK := s.bodies.RootAt(bodyTerm, at)
		if !rootOK {
			return 0, false
		}
		if !rootKind(keyspace.TermFamily(root)) || s.static(root) || !s.evaluates(root) {
			continue
		}
		return root, true
	}
	return bodyTerm, true
}

func (s *evalState) bodyNormal(bodyTerm keyspace.Term) (keyspace.Term, bool) {
	return s.outs.BodyExit(bodyTerm, kind.OutcomeNormal)
}
