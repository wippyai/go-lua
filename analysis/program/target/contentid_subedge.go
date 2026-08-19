package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/internal/framing"
)

func encodeBindingSegments(w *framing.Writer, c *Contract, op vocabulary.Operation, binding int, owner bool) error {
	count := c.Operations.BindingMemberCountAt(op, binding)
	if owner {
		count = c.Operations.BindingOwnerCountAt(op, binding)
	}
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		var value vocabulary.ExactKey
		var ok bool
		if owner {
			value, ok = c.Operations.BindingOwnerKeyAt(op, binding, index)
		} else {
			value, ok = c.Operations.BindingMemberKeyAt(op, binding, index)
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
func encodeSubedge(w *framing.Writer, c *Contract, owner vocabulary.Operation, edge vocabulary.SubedgeID) error {
	edgeOwner, ok := c.subedgeOwner(edge)
	if !ok || edgeOwner != owner {
		return errors.New("target: malformed subedge owner")
	}
	role, ok := c.subedgeRole(edge)
	if !ok || role == 0 {
		return errors.New("target: malformed subedge role")
	}
	family, ok := c.SubedgeFamily(edge)
	if !ok || !vocabulary.ValidSubedgeFamily(family) {
		return errors.New("target: malformed subedge family")
	}
	callee, ok := c.subedgeCallee(edge)
	if !ok {
		return errors.New("target: malformed subedge callee")
	}
	admission, ok := c.subedgeAdmission(edge)
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
	case vocabulary.SubedgeCalleeInvalid:
		if family == vocabulary.SubedgeFamilyCall {
			return errors.New("target: Call subedge lacks callee")
		}
	case vocabulary.SubedgeCalleeCallback:
		callback, found := c.subedgeCallback(edge)
		if !found {
			return errors.New("target: malformed callback subedge callee")
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeCapturedInitialRead:
		root, key, found := c.subedgeCapturedInitialRead(edge)
		if !found {
			return errors.New("target: malformed captured initial read")
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeMetaKey:
		key, found := c.subedgeMetaKey(edge)
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
	ruleEntry, found := c.subedgeRuleEntry(edge)
	if !found {
		return errors.New("target: malformed subedge entry authority")
	}
	if err := w.Bool(ruleEntry); err != nil {
		return err
	}
	originCount := c.argumentOriginCount(edge)
	if err := w.Count(uint64(originCount)); err != nil {
		return err
	}
	for index := 0; index < originCount; index++ {
		segment, ordinal, source, input, found := c.ArgumentOriginAt(edge, index)
		if !found || segment == vocabulary.ArgumentSegmentInvalid || source == vocabulary.ArgumentSourceInvalid {
			return errors.New("target: malformed subedge argument origin")
		}
		if err := encodeCoordinate(w, uint64(segment), uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if source == vocabulary.ArgumentSourceInput {
			if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
				return err
			}
		} else if input != (vocabulary.InputSource{}) {
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
	route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.admissionRoute(edge)
	if !found || route == vocabulary.RouteInvalid {
		return errors.New("target: malformed subedge admission route")
	}
	if err := encodeSubedgeRoute(w, c, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.subedgeRouteAt(edge, kind)
		if !found || route == vocabulary.RouteInvalid {
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

func encodeSubedgeRoute(w *framing.Writer, c *Contract, owner vocabulary.Operation, route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset uint32, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values) error {
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
	case vocabulary.RouteOutcome:
		if sibling != 0 || destination == 0 {
			return errors.New("target: malformed outcome subedge route")
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case vocabulary.RouteRejectYield:
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
			siblingOwner, ownerOK := c.subedgeOwner(sibling)
			siblingRole, roleOK := c.subedgeRole(sibling)
			if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
				return errors.New("target: malformed C-boundary sibling role")
			}
			if err := w.Uint(uint64(siblingRole)); err != nil {
				return err
			}
		}
		return encodeValues(w, c, destination)
	case vocabulary.RouteSubedge:
		if outcome != 0 || sibling == 0 || destination == 0 {
			return errors.New("target: malformed sibling subedge route")
		}
		siblingOwner, ownerOK := c.subedgeOwner(sibling)
		siblingRole, roleOK := c.subedgeRole(sibling)
		if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
			return errors.New("target: malformed sibling semantic role")
		}
		if err := w.Uint(uint64(siblingRole)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case vocabulary.RouteContinue, vocabulary.RoutePropagateYield:
		if outcome != 0 || sibling != 0 || destination != 0 {
			return errors.New("target: malformed terminal-only subedge route")
		}
		return nil
	default:
		return errors.New("target: invalid subedge route")
	}
}
