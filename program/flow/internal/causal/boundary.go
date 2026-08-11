package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func (s *boundaryState) appendBoundary(boundary CallBoundary, owner keyspace.Term) error {
	if keyspace.TermFamily(boundary.Call) != keyspace.FamilyCall || !s.live(boundary.Call) ||
		!validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return errors.New("program/flow/causal: malformed CallBoundary owner")
	}
	if boundary.Normal == 0 && boundary.TailReturn == 0 {
		return errors.New("program/flow/causal: live CallBoundary has neither normal nor tail disposition")
	}
	switch boundary.mode {
	case boundaryDirect:
		if boundary.Normal == 0 || boundary.Other != 0 || boundary.TailReturn != 0 {
			return errors.New("program/flow/causal: malformed direct CallBoundary normal disposition")
		}
	case boundarySelectAnd, boundarySelectOr:
		if keyspace.TermFamily(boundary.Normal) != keyspace.FamilySelect || !s.live(boundary.Normal) || boundary.Other == 0 || boundary.TailReturn != 0 {
			return errors.New("program/flow/causal: malformed Select CallBoundary normal split")
		}
	case boundaryTail:
		if boundary.Normal != 0 || boundary.Other != 0 || boundary.TailReturn == 0 {
			return errors.New("program/flow/causal: malformed tail CallBoundary disposition")
		}
	default:
		return errors.New("program/flow/causal: CallBoundary normal disposition is unspecified")
	}
	if boundary.Normal != 0 && !s.live(boundary.Normal) && !isOutcome(boundary.Normal) {
		return fmt.Errorf("program/flow/causal: CallBoundary normal resume %v is not executable", boundary.Normal)
	}
	if boundary.Other != 0 && !s.live(boundary.Other) && !isOutcome(boundary.Other) {
		return fmt.Errorf("program/flow/causal: CallBoundary alternate resume %v is not executable", boundary.Other)
	}
	if boundary.TailReturn != 0 && !s.validOutcome(boundary.TailReturn) {
		return errors.New("program/flow/causal: CallBoundary tail disposition is not an Outcome")
	}
	for _, arm := range [...]keyspace.Term{boundary.Throw, boundary.Yield, boundary.Cancel} {
		if !s.validOutcome(arm) {
			return errors.New("program/flow/causal: CallBoundary exceptional arm is not an Outcome")
		}
	}
	if uint64(len(s.boundaryRows)) >= uint64(^uint32(0)) {
		return errors.New("program/flow/causal: CallBoundary denominator overflows")
	}
	s.boundaryRows = append(s.boundaryRows, boundaryRow{CallBoundary: boundary})
	s.boundaryOwners = append(s.boundaryOwners, owner)
	return nil
}

func (s *boundaryState) validOutcome(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyOutcome &&
		keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= s.counts[keyspace.FamilyOutcome]
}

// buildTailPlans derives every terminal Call disposition in one typed Return
// pass. The dense plan is seal-local and is later discarded with the other
// seal-local scratch; boundary sealing never searches Return rows for a
// Call. Outcome propagation validation is memoized so shared chains remain
// linear in the Outcome denominator.
func (s *boundaryState) buildTailPlans() error {
	returns := s.flow.Control().Returns()
	valuesView := s.flow.Values()
	calls := s.flow.Calls()
	chainState := make([]uint8, s.counts[keyspace.FamilyOutcome]+1)
	chainValid := make([]bool, s.counts[keyspace.FamilyOutcome]+1)
	chainPath := make([]uint32, 0, s.counts[keyspace.FamilyOutcome])
	validateChain := func(exit keyspace.Term) error {
		chainPath = chainPath[:0]
		current := exit
		for {
			ordinal := keyspace.TermOrdinal(current)
			if keyspace.TermFamily(current) != keyspace.FamilyOutcome || ordinal == 0 || ordinal > s.counts[keyspace.FamilyOutcome] {
				for _, pathOrdinal := range chainPath {
					chainState[pathOrdinal], chainValid[pathOrdinal] = 2, false
				}
				return errors.New("program/flow/causal: tail Return Outcome is unavailable")
			}
			if chainState[ordinal] == 2 {
				if !chainValid[ordinal] {
					for _, pathOrdinal := range chainPath {
						chainState[pathOrdinal], chainValid[pathOrdinal] = 2, false
					}
					return errors.New("program/flow/causal: tail Return Outcome chain is malformed")
				}
				for _, pathOrdinal := range chainPath {
					chainState[pathOrdinal], chainValid[pathOrdinal] = 2, true
				}
				return nil
			}
			if chainState[ordinal] == 1 {
				for _, pathOrdinal := range chainPath {
					chainState[pathOrdinal], chainValid[pathOrdinal] = 2, false
				}
				return errors.New("program/flow/causal: tail Return Outcome chain is cyclic")
			}
			chainState[ordinal] = 1
			chainPath = append(chainPath, ordinal)
			_, outcomeKind, target, ok := s.outs.Get(current)
			if !ok || outcomeKind != kind.OutcomeReturn || target != 0 {
				for _, pathOrdinal := range chainPath {
					chainState[pathOrdinal], chainValid[pathOrdinal] = 2, false
				}
				return errors.New("program/flow/causal: tail Return Outcome chain is malformed")
			}
			next, propagated := s.outs.Propagation(current)
			if !propagated {
				for _, pathOrdinal := range chainPath {
					chainState[pathOrdinal], chainValid[pathOrdinal] = 2, true
				}
				return nil
			}
			current = next
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyReturn]; ordinal++ {
		ret := keyspace.MakeTerm(keyspace.FamilyReturn, ordinal)
		// A dead or static Return cannot be a live tail disposition. Its
		// authored Values may still carry a stale Tail witness, but that
		// witness is outside the executable causal plane and must not be
		// admitted or rejected as a live Call topology.
		if !s.live(ret) || s.static(ret) {
			continue
		}
		owner, values, ok := returns.Get(ret)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: Return row is unavailable")
		}
		length, lengthOK := valuesView.Len(values)
		valuesOwner, tail, valuesOK := valuesView.Get(values)
		if !lengthOK || !valuesOK || length < 0 || valuesOwner != owner {
			return errors.New("program/flow/causal: malformed Return Values ownership")
		}
		if length != 0 || tail == 0 {
			continue
		}
		// An open Return Values row has two distinct producers. A Call tail
		// forwards the callee's complete result list and therefore needs a
		// causal tail boundary. A Vararg tail forwards the current function's
		// own vararg cell and has no dynamic Call topology to publish. The
		// latter is authored semantic data, not a malformed Call witness.
		if keyspace.TermFamily(tail) == keyspace.FamilyVararg {
			varargOwner, _, varargOK := s.flow.Storage().Varargs().Get(tail)
			if !varargOK || varargOwner != owner || !s.live(tail) {
				return errors.New("program/flow/causal: malformed Vararg tail ownership")
			}
			if keyspace.TermOrdinal(tail) == 0 || keyspace.TermOrdinal(tail) > s.counts[keyspace.FamilyVararg] {
				return errors.New("program/flow/causal: tail Values term is not a Vararg")
			}
			continue
		}
		if keyspace.TermFamily(tail) != keyspace.FamilyCall || keyspace.TermOrdinal(tail) == 0 ||
			keyspace.TermOrdinal(tail) > s.counts[keyspace.FamilyCall] {
			return errors.New("program/flow/causal: tail Values term is not a Call")
		}
		callOwner, _, _, actuals, callOK := calls.Get(tail)
		actualsOwner, _, actualsOK := valuesView.Get(actuals)
		callOrdinal := keyspace.TermOrdinal(tail)
		valuesOrdinal := keyspace.TermOrdinal(values)
		if !callOK || callOwner != owner || actuals == 0 || !actualsOK || actualsOwner != owner ||
			valuesOrdinal == 0 || uint64(valuesOrdinal) >= uint64(len(s.valueParent)) || s.valueParent[valuesOrdinal] != ret ||
			keyspace.TermOrdinal(actuals) == 0 || uint64(keyspace.TermOrdinal(actuals)) >= uint64(len(s.valueParent)) ||
			s.valueParent[keyspace.TermOrdinal(actuals)] != tail || !s.live(values) || !s.live(actuals) ||
			!s.live(tail) || !s.live(ret) {
			return errors.New("program/flow/causal: malformed tail Call/Values/Return ownership")
		}
		if s.tailPlans[callOrdinal] != 0 {
			return errors.New("program/flow/causal: Call has multiple tail Return destinations")
		}
		exit, exitOK := s.outs.ReturnExit(ret)
		if !exitOK {
			return errors.New("program/flow/causal: tail Return Outcome is unavailable")
		}
		if err := validateChain(exit); err != nil {
			return err
		}
		s.tailPlans[callOrdinal] = exit
	}
	return nil
}

func (s *callState) setCallPlan(call keyspace.Term, route callNormalRoute) error {
	if keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 ||
		uint64(keyspace.TermOrdinal(call)) >= uint64(len(s.callPlans)) {
		return errors.New("program/flow/causal: Call normal plan key is invalid")
	}
	if !s.live(call) {
		return nil
	}
	ordinal := keyspace.TermOrdinal(call)
	if s.tailPlans[ordinal] != 0 {
		return fmt.Errorf("program/flow/causal: tail Call %v received a normal plan", call)
	}
	if route.normal == 0 || route.mode == 0 {
		return errors.New("program/flow/causal: Call normal plan is empty")
	}
	if s.callPlanSet[ordinal] {
		previous := s.callPlans[ordinal]
		if previous.normal != route.normal || previous.other != route.other || previous.mode != route.mode {
			return fmt.Errorf("program/flow/causal: Call %v receives conflicting normal plans", call)
		}
		return nil
	}
	s.callPlans[ordinal] = route
	s.callPlanSet[ordinal] = true
	return nil
}

func (s *boundaryState) directRootCall(call keyspace.Term) (keyspace.Term, int, bool) {
	ordinal := keyspace.TermOrdinal(call)
	if keyspace.TermFamily(call) != keyspace.FamilyCall || ordinal == 0 || uint64(ordinal) >= uint64(len(s.directCallSet)) || !s.directCallSet[ordinal] {
		return 0, -1, false
	}
	return s.directCallOwner[ordinal], s.directCallCursor[ordinal], true
}

func (s *structureState) prepareDirectCall(owner, root keyspace.Term, cursor int) error {
	if keyspace.TermFamily(root) != keyspace.FamilyCall || keyspace.TermOrdinal(root) == 0 ||
		uint64(keyspace.TermOrdinal(root)) >= uint64(len(s.directCallSet)) {
		return errors.New("program/flow/causal: direct Call root is invalid")
	}
	target, raw, routeOK := s.structuralRoute(owner, cursor)
	if !routeOK {
		return errors.New("program/flow/causal: direct Call structural route is unavailable")
	}
	to, toOK := s.causalTarget(target)
	if !toOK {
		if s.live(target) && !s.static(target) {
			return errors.New("program/flow/causal: direct Call target has no Entry port")
		}
		return errors.New("program/flow/causal: direct Call target is unavailable")
	}
	ordinal := keyspace.TermOrdinal(root)
	s.directCallOwner[ordinal] = owner
	s.directCallCursor[ordinal] = cursor
	s.directCallRaw[ordinal] = raw
	s.directCallSet[ordinal] = true
	if s.tailPlans[ordinal] != 0 {
		return nil
	}
	return s.setCallPlan(root, callNormalRoute{normal: to, mode: boundaryDirect})
}

func (s *boundaryState) claimDirectCallArc(call keyspace.Term, disposition arcDisposition) error {
	owner, cursor, ok := s.directRootCall(call)
	if !ok {
		return nil
	}
	_ = cursor
	raw := s.directCallRaw[keyspace.TermOrdinal(call)]
	if raw == 0 {
		return errors.New("program/flow/causal: direct Call structural target is unavailable")
	}
	rawLive := raw == owner || (rootKind(keyspace.TermFamily(raw)) && !s.static(raw) && s.live(raw))
	if !rawLive {
		disposition = arcLivenessOnly
	}
	if _, claimed := s.claimArc(call, raw, 0, false, disposition); !claimed {
		return errors.New("program/flow/causal: direct Call structural Arc is unavailable")
	}
	return nil
}

type callNormalRoute struct {
	normal keyspace.Term
	other  keyspace.Term
	mode   boundaryMode
}

func (s *callState) callNormal(call keyspace.Term) (callNormalRoute, bool) {
	ordinal := keyspace.TermOrdinal(call)
	if keyspace.TermFamily(call) != keyspace.FamilyCall || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(s.callPlans)) || !s.callPlanSet[ordinal] {
		return callNormalRoute{}, false
	}
	return s.callPlans[ordinal], true
}

func (s *boundaryState) emitBoundaries() error {
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		if !s.live(call) {
			continue
		}
		owner, _, _, _, ok := s.flow.Calls().Get(call)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: Call owner is unavailable")
		}
		boundary := CallBoundary{Call: call}
		tail := s.tailPlans[ordinal]
		if tail != 0 {
			if err := s.claimDirectCallArc(call, arcLivenessOnly); err != nil {
				return err
			}
			boundary.TailReturn = tail
			boundary.mode = boundaryTail
		} else {
			route, normalOK := s.callNormal(call)
			if !normalOK || route.normal == 0 {
				return errors.New("program/flow/causal: live Call has no typed normal continuation")
			}
			if err := s.claimDirectCallArc(call, arcBoundaryNormal); err != nil {
				return err
			}
			boundary.Normal = route.normal
			boundary.Other = route.other
			boundary.mode = route.mode
		}
		for _, item := range []struct {
			kind kind.OutcomeKind
			dst  *keyspace.Term
		}{
			{kind.OutcomeThrow, &boundary.Throw},
			{kind.OutcomeYield, &boundary.Yield},
			{kind.OutcomeCancel, &boundary.Cancel},
		} {
			exit, exitOK := s.outs.BodyExit(owner, item.kind)
			if !exitOK {
				return errors.New("program/flow/causal: Call owning Body Outcome is unavailable")
			}
			*item.dst = exit
		}
		if err := s.appendBoundary(boundary, owner); err != nil {
			return err
		}
	}
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyCall]; ordinal++ {
		call := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		if !s.live(call) {
			continue
		}
		if s.tailPlans[ordinal] == 0 && !s.callPlanSet[ordinal] {
			return fmt.Errorf("program/flow/causal: live Call %v has no sealed normal disposition", call)
		}
	}
	return nil
}
