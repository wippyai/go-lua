package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func encodeBindingSegments(w *framing.Writer, c *Contract, op Operation, binding int, owner bool) error {
	count := c.BindingMemberCountAt(op, binding)
	if owner {
		count = c.BindingOwnerCountAt(op, binding)
	}
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		var value ExactKey
		var ok bool
		if owner {
			value, ok = c.BindingOwnerKeyAt(op, binding, index)
		} else {
			value, ok = c.BindingMemberKeyAt(op, binding, index)
		}
		if !ok {
			return errors.New("target: malformed binding segment")
		}
		if err := encodeExactKey(w, c, value); err != nil {
			return err
		}
	}
	return nil
}

// encodeSubedge writes the sole internal-application relation in terms of
// semantic roles and full Values endpoints. It deliberately never encodes a
// Values handle as if handle equality were a flow edge.
func encodeSubedge(w *framing.Writer, c *Contract, owner Operation, edge SubedgeID) error {
	edgeOwner, ok := c.SubedgeOwner(edge)
	if !ok || edgeOwner != owner {
		return errors.New("target: malformed subedge owner")
	}
	role, ok := c.SubedgeRole(edge)
	if !ok || role == 0 {
		return errors.New("target: malformed subedge role")
	}
	family, ok := c.SubedgeFamily(edge)
	if !ok || !validSubedgeFamily(family) {
		return errors.New("target: malformed subedge family")
	}
	callee, ok := c.SubedgeCallee(edge)
	if !ok {
		return errors.New("target: malformed subedge callee")
	}
	admission, ok := c.SubedgeAdmission(edge)
	if !ok || !admission.Available() {
		return errors.New("target: malformed subedge admission")
	}
	arguments, ok := c.SubedgeArguments(edge)
	if !ok {
		return errors.New("target: malformed subedge arguments")
	}
	if err := w.Record(recordSubedge); err != nil {
		return err
	}
	if err := w.Uint(uint64(role)); err != nil {
		return err
	}
	if err := w.Uint(uint64(family)); err != nil {
		return err
	}
	if err := w.Uint(uint64(callee)); err != nil {
		return err
	}
	if err := w.Uint(uint64(admission)); err != nil {
		return err
	}
	switch callee {
	case SubedgeCalleeInvalid:
		if family == SubedgeFamilyCall {
			return errors.New("target: Call subedge lacks callee")
		}
	case SubedgeCalleeCallback:
		callback, found := c.SubedgeCallback(edge)
		if !found {
			return errors.New("target: malformed callback subedge callee")
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	case SubedgeCalleeCapturedInitialRead:
		root, key, found := c.SubedgeCapturedInitialRead(edge)
		if !found {
			return errors.New("target: malformed captured initial read")
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	case SubedgeCalleeMetaKey:
		key, found := c.SubedgeMetaKey(edge)
		if !found {
			return errors.New("target: malformed metakey subedge callee")
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	default:
		return errors.New("target: invalid subedge callee")
	}
	if err := encodeValues(w, c, arguments); err != nil {
		return err
	}
	ruleEntry, found := c.SubedgeRuleEntry(edge)
	if !found {
		return errors.New("target: malformed subedge entry authority")
	}
	if err := w.Bool(ruleEntry); err != nil {
		return err
	}
	originCount := c.ArgumentOriginCount(edge)
	if err := w.Count(uint64(originCount)); err != nil {
		return err
	}
	for index := 0; index < originCount; index++ {
		segment, ordinal, source, input, found := c.ArgumentOriginAt(edge, index)
		if !found || segment == ArgumentSegmentInvalid || source == ArgumentSourceInvalid {
			return errors.New("target: malformed subedge argument origin")
		}
		if err := encodeCoordinate(w, uint64(segment), uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if source == ArgumentSourceInput {
			if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
				return err
			}
		} else if input != (InputSource{}) {
			return errors.New("target: Rule argument origin carries input")
		}
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		terminal, found := c.SubedgeTerminal(edge, kind)
		if !found {
			return errors.New("target: malformed subedge terminal")
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
		if err := encodeValues(w, c, terminal); err != nil {
			return err
		}
	}
	failure, found := c.AdmissionFailure(edge)
	if !found {
		return errors.New("target: malformed subedge admission failure")
	}
	if err := encodeValues(w, c, failure); err != nil {
		return err
	}
	route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.AdmissionRoute(edge)
	if !found || route == RouteInvalid {
		return errors.New("target: malformed subedge admission route")
	}
	if err := encodeSubedgeRoute(w, c, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.SubedgeRouteAt(edge, kind)
		if !found || route == RouteInvalid {
			return errors.New("target: malformed subedge route")
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
		if err := encodeSubedgeRoute(w, c, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
			return err
		}
	}
	return nil
}

func encodeSubedgeRoute(w *framing.Writer, c *Contract, owner Operation, route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values) error {
	if err := w.Uint(uint64(route)); err != nil {
		return err
	}
	if err := w.Uint(uint64(adjustment)); err != nil {
		return err
	}
	if result == 0 {
		return errors.New("target: subedge route lacks Result")
	}
	if err := encodeValues(w, c, result); err != nil {
		return err
	}
	if err := w.Uint(uint64(placement)); err != nil {
		return err
	}
	if err := w.Uint(uint64(offset)); err != nil {
		return err
	}
	switch route {
	case RouteOutcome:
		if sibling != 0 || destination == 0 {
			return errors.New("target: malformed outcome subedge route")
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case RouteRejectYield:
		if destination == 0 {
			return errors.New("target: malformed C-boundary subedge route")
		}
		if err := w.Bool(sibling != 0); err != nil {
			return err
		}
		if sibling == 0 {
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
		} else {
			if outcome != 0 {
				return errors.New("target: C-boundary sibling route carries outcome")
			}
			siblingOwner, ownerOK := c.SubedgeOwner(sibling)
			siblingRole, roleOK := c.SubedgeRole(sibling)
			if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
				return errors.New("target: malformed C-boundary sibling role")
			}
			if err := w.Uint(uint64(siblingRole)); err != nil {
				return err
			}
		}
		return encodeValues(w, c, destination)
	case RouteSubedge:
		if outcome != 0 || sibling == 0 || destination == 0 {
			return errors.New("target: malformed sibling subedge route")
		}
		siblingOwner, ownerOK := c.SubedgeOwner(sibling)
		siblingRole, roleOK := c.SubedgeRole(sibling)
		if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
			return errors.New("target: malformed sibling semantic role")
		}
		if err := w.Uint(uint64(siblingRole)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case RouteContinue, RoutePropagateYield:
		if outcome != 0 || sibling != 0 || destination != 0 {
			return errors.New("target: malformed terminal-only subedge route")
		}
		return nil
	default:
		return errors.New("target: invalid subedge route")
	}
}
