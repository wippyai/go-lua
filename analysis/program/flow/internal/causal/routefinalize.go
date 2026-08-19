package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/recurrence"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// finalizeBinding is the sole Causal consumer of recurrence's ordinal-aligned
// certificate. It copies no SCC topology: each claim only decorates the
// already-materialized final row with its Arc-owned Mu/reset witness.
func (s *sealState) finalizeBinding(plan *routeplan.Plan, binding *recurrence.Binding) error {
	if s == nil || s.plan == nil || s.plan.plan != plan || s.plan.nextOrdinal != plan.Count() ||
		len(s.edges.edgeRows) != len(s.edges.edgeOwners) || len(s.edges.edgeRows) != len(s.edges.planOrdinals) ||
		len(s.rows.boundaryRows) != len(s.rows.boundaryOwners) || len(s.rows.boundaryRows) != len(s.rows.planOrdinals) {
		return errors.New("program/flow/causal: route-plan scratch is malformed")
	}
	if err := s.validateArcPlanBijection(plan); err != nil {
		return err
	}
	s.pub.result.boundRouteRows = make([]boundRouteRow, plan.Count())
	for index := range s.edges.edgeRows {
		ordinal := s.edges.planOrdinals[index]
		if !matchesPlanRoute(plan, ordinal, s.edges.edgeRows[index].From, s.edges.edgeRows[index].To, s.edges.edgeRows[index].Decision, s.edges.edgeRows[index].Truth, routeplan.ArmLocal) {
			return fmt.Errorf("program/flow/causal: local route %d declaration disagrees with final row", index)
		}
		bound, ok := binding.Claim(plan, ordinal)
		if !ok || !bound.Valid() {
			return fmt.Errorf("program/flow/causal: local route %d has no recurrence certificate", index)
		}
		if err := s.applyLocalBinding(&s.edges.edgeRows[index], bound); err != nil {
			return err
		}
		fromPath, fromOK := bound.FromPath()
		toPath, toOK := bound.ToPath()
		if !fromOK || !toOK {
			return fmt.Errorf("program/flow/causal: local route %d lacks endpoint path views", index)
		}
		diagnostic, diagnosticOK := s.routeDiagnostic(plan, ordinal)
		if !diagnosticOK {
			return fmt.Errorf("program/flow/causal: local route %d diagnostic phase row is unavailable", index)
		}
		s.pub.result.boundRouteRows[ordinal] = boundRouteRow{fromPath: fromPath, toPath: toPath, diagnostic: diagnostic}
	}
	for index := range s.rows.boundaryRows {
		row := &s.rows.boundaryRows[index]
		planned := s.rows.planOrdinals[index]
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if !boundaryArmPresent(*row, arm) {
				if planned.present[arm] {
					return errors.New("program/flow/causal: absent CallBoundary arm was planned")
				}
				continue
			}
			if !planned.present[arm] {
				return errors.New("program/flow/causal: present CallBoundary arm has no plan ordinal")
			}
			to, decision, truth, present := boundarySuccessor(row.CallBoundary, arm)
			if !present || !matchesPlanRoute(plan, planned.ordinals[arm], row.Call, to, decision, truth, routePlanArm(arm)) {
				return fmt.Errorf("program/flow/causal: CallBoundary %d arm %d declaration disagrees with final row", index, arm)
			}
			bound, ok := binding.Claim(plan, planned.ordinals[arm])
			if !ok || !bound.Valid() {
				return fmt.Errorf("program/flow/causal: CallBoundary %d arm %d has no recurrence certificate", index, arm)
			}
			component, member := bound.Member()
			if !canonicalComponent(component, member) {
				return errors.New("program/flow/causal: CallBoundary component certificate is malformed")
			}
			row.components[arm] = component
			fromPath, fromOK := bound.FromPath()
			toPath, toOK := bound.ToPath()
			if !fromOK || !toOK {
				return fmt.Errorf("program/flow/causal: CallBoundary %d arm %d lacks endpoint path views", index, arm)
			}
			diagnostic, diagnosticOK := s.routeDiagnostic(plan, planned.ordinals[arm])
			if !diagnosticOK {
				return fmt.Errorf("program/flow/causal: CallBoundary %d arm %d diagnostic phase row is unavailable", index, arm)
			}
			s.pub.result.boundRouteRows[planned.ordinals[arm]] = boundRouteRow{fromPath: fromPath, toPath: toPath, diagnostic: diagnostic}
			head, first, past, hasMu := bound.Mu()
			if !hasMu {
				if head != 0 || first != 0 || past != 0 {
					return errors.New("program/flow/causal: Mu-less CallBoundary arm carried a reset range")
				}
				continue
			}
			if !member || head == 0 || past < first || !canonicalComponent(head, true) {
				return errors.New("program/flow/causal: CallBoundary Mu/reset disagrees with component")
			}
			streamCount, streamOK := s.proof.recur.DecisionCount(head)
			if !streamOK || streamCount < 0 || uint64(past) > uint64(streamCount) {
				return errors.New("program/flow/causal: CallBoundary reset range is unavailable")
			}
			// A nested head can be a local interval over its component's
			// primary stream. Issue that owner first so route order cannot
			// accidentally claim the same decision under the nested view.
			if component != head {
				if err := s.reset.ensureMuStream(component); err != nil {
					return err
				}
			}
			if err := s.reset.ensureMuStream(head); err != nil {
				return err
			}
			row.proofs[arm] = boundaryRecurrenceProof{mu: head, resetStart: first, resetPast: past}
		}
	}
	return nil
}

// routeDiagnostic classifies a route while the exact Plan and outcome
// owner are still live. It stores no Plan/term capability; publication clears
// the resulting scratch after hierarchy row attachment succeeds.
func (s *sealState) routeDiagnostic(plan *routeplan.Plan, ordinal int) (routeDiagnostic, bool) {
	if s == nil || s.proof == nil || s.proof.outs == nil {
		return routeDiagnostic{}, false
	}
	route, origin, ok := plan.At(ordinal)
	if !ok {
		return routeDiagnostic{}, false
	}
	diagnostic := routeDiagnostic{
		fromFamily: wtoLogicalFamily(keyspace.TermFamily(route.From)),
		toFamily:   wtoLogicalFamily(keyspace.TermFamily(route.To)),
		fromPhase:  wtoPhaseCSR,
		toPhase:    wtoPhaseCSR,
	}
	if keyspace.TermFamily(route.From) == keyspace.FamilyOutcome {
		_, outcomeKind, _, outcomeOK := s.proof.outs.Get(route.From)
		if !outcomeOK {
			return routeDiagnostic{}, false
		}
		diagnostic.fromOutcome = wtoOutcome(outcomeKind)
	}
	if keyspace.TermFamily(route.To) == keyspace.FamilyOutcome {
		_, outcomeKind, _, outcomeOK := s.proof.outs.Get(route.To)
		if !outcomeOK {
			return routeDiagnostic{}, false
		}
		diagnostic.toOutcome = wtoOutcome(outcomeKind)
	}
	from, to, endpointOK := origin.Endpoints()
	if !endpointOK {
		return routeDiagnostic{}, false
	}
	if from.OutcomePhase() {
		diagnostic.fromPhase = wtoPhaseOutcome
	}
	if to.OutcomePhase() {
		diagnostic.toPhase = wtoPhaseOutcome
	}
	return diagnostic, true
}

func matchesPlanRoute(plan *routeplan.Plan, ordinal int, from, to, decision keyspace.Term, truth bool, arm routeplan.Arm) bool {
	route, _, ok := plan.At(ordinal)
	return ok && route.From == from && route.To == to && route.Decision == decision && route.Truth == truth && route.Arm == arm
}

func (s *sealState) applyLocalBinding(row *edgeRow, bound recurrence.BoundRoute) error {
	if row == nil || !bound.Valid() {
		return errors.New("program/flow/causal: local recurrence certificate is unavailable")
	}
	component, member := bound.Member()
	head, first, past, hasMu := bound.Mu()
	if !canonicalComponent(component, member) {
		return errors.New("program/flow/causal: local component certificate is malformed")
	}
	row.component = component
	if !member {
		if component != 0 || hasMu || head != 0 || first != 0 || past != 0 {
			return errors.New("program/flow/causal: acyclic local route carried component or reset")
		}
		if row.From == row.To {
			return errors.New("program/flow/causal: Mu-less Edge is self-referential")
		}
		return nil
	}
	if component == 0 {
		return errors.New("program/flow/causal: cyclic local route lacks component")
	}
	if !hasMu {
		return nil // ordinary intra-SCC route: component retained only by the binding cut.
	}
	if head == 0 || past < first || !canonicalComponent(head, true) {
		return errors.New("program/flow/causal: Mu/reset disagrees with recurrence component")
	}
	streamCount, ok := s.proof.recur.DecisionCount(head)
	if !ok || streamCount < 0 || uint64(past) > uint64(streamCount) {
		return errors.New("program/flow/causal: recurrence reset range is unavailable")
	}
	row.Mu, row.resetStart, row.resetPast = head, first, past
	if component != head {
		if err := s.reset.ensureMuStream(component); err != nil {
			return err
		}
	}
	if err := s.reset.ensureMuStream(head); err != nil {
		return err
	}
	return nil
}

func canonicalComponent(component keyspace.Term, member bool) bool {
	if !member {
		return component == 0
	}
	if component == 0 || keyspace.TermOrdinal(component) == 0 {
		return false
	}
	family := keyspace.TermFamily(component)
	return family == keyspace.FamilyLabel || family == keyspace.FamilyLoop
}

func (s *sealState) validateArcPlanBijection(plan *routeplan.Plan) error {
	if s.plan == nil || len(s.plan.arcOrdinal) != len(s.arc.arcDisposition) {
		return errors.New("program/flow/causal: Arc plan ledger is unavailable")
	}
	if plan == nil || plan != s.plan.plan || plan.Count() != s.plan.nextOrdinal {
		return errors.New("program/flow/causal: Arc plan token or denominator disagrees")
	}
	seen := make([]bool, plan.Count())
	for index, disposition := range s.arc.arcDisposition {
		ordinal := s.plan.arcOrdinal[index]
		switch disposition {
		case arcLocal:
			if err := s.validatePlannedArc(plan, seen, index, ordinal); err != nil {
				return fmt.Errorf("program/flow/causal: claimed Arc %d has no planned route", index)
			}
		case arcBoundaryNormal:
			if err := s.validatePlannedArc(plan, seen, index, ordinal); err != nil {
				return fmt.Errorf("program/flow/causal: claimed boundary Arc %d has no planned route", index)
			}
		case arcUndisposed, arcLivenessOnly, arcDeadStatic:
			if ordinal >= 0 {
				return fmt.Errorf("program/flow/causal: non-route Arc %d has a plan ordinal", index)
			}
		default:
			return errors.New("program/flow/causal: Arc disposition is invalid")
		}
	}
	for ordinal := 0; ordinal < plan.Count(); ordinal++ {
		_, origin, ok := plan.At(ordinal)
		if !ok {
			return errors.New("program/flow/causal: planned route is malformed")
		}
		carrier, _ := origin.RecurrenceCarrier()
		if _, arc := carrier.ArcRef(); arc && !seen[ordinal] {
			return fmt.Errorf("program/flow/causal: OriginArc plan row %d has no exact Arc disposition", ordinal)
		}
	}
	return nil
}

func (s *sealState) validatePlannedArc(plan *routeplan.Plan, seen []bool, arcIndex, ordinal int) error {
	if ordinal < 0 || ordinal >= plan.Count() || seen[ordinal] {
		return errors.New("planned Arc ordinal is missing or duplicate")
	}
	_, origin, ok := plan.At(ordinal)
	if !ok {
		return errors.New("planned Arc route is unavailable")
	}
	carrier, _ := origin.RecurrenceCarrier()
	actual, ok := carrier.ArcRef()
	if !ok {
		return errors.New("planned Arc disposition lost its ArcRef")
	}
	expected, ok := s.proof.graph.ArcRefAt(arcIndex)
	if !ok || !sourcecontrol.SameArcRef(actual, expected) {
		return errors.New("planned ArcRef does not match its exact sourcecontrol Arc")
	}
	seen[ordinal] = true
	return nil
}

func (s *sealState) installComponentDirectory(components []recurrence.Component) error {
	if s == nil || s.pub == nil || s.pub.result == nil {
		return errors.New("program/flow/causal: component directory owner is unavailable")
	}
	headTerms := make([]keyspace.Term, len(components))
	headPaths := make([]identity.ContentID, len(components))
	headIndex := make(map[keyspace.Term]uint32, len(components))
	for index, component := range components {
		head, headOK := component.Head()
		path, pathOK := component.HeadPath()
		if !headOK || !pathOK || !canonicalComponent(head, true) {
			return errors.New("program/flow/causal: recurrence component directory is malformed")
		}
		headTerms[index], headPaths[index] = head, path
		headIndex[head] = uint32(index)
		if index > 0 && string(headPaths[index-1][:]) >= string(path[:]) {
			return errors.New("program/flow/causal: recurrence component directory is not canonical")
		}
	}
	s.pub.result.components = headTerms
	s.pub.result.componentIndex = headIndex
	s.pub.result.componentPaths = headPaths
	for _, row := range s.edges.edgeRows {
		if row.component != 0 && !s.pub.result.componentIssued(row.component) {
			return errors.New("program/flow/causal: local row component was not issued by recurrence")
		}
	}
	for _, row := range s.rows.boundaryRows {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if boundaryArmPresent(row, arm) && row.components[arm] != 0 && !s.pub.result.componentIssued(row.components[arm]) {
				return errors.New("program/flow/causal: CallBoundary component was not issued by recurrence")
			}
		}
	}
	return nil
}
