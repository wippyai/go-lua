package contract

import (
	"errors"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/internal/framing"
)

func encodeInput(w *framing.Writer, value vocabulary.InputSource) error {
	if err := w.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	return w.Uint(uint64(value.Ordinal))
}

// encodeBehavior writes the complete neutral operation behavior descriptor.
// Relation identities are already sealed schema identities; this codec merely
// carries their bytes and never decodes or classifies them.
func (c *Contract) encodeBehavior(w *framing.Writer, op vocabulary.Operation) error {
	if err := w.Count(uint64(c.Operations.BehaviorResultCount(op))); err != nil {
		return err
	}
	for index := 0; index < c.Operations.BehaviorResultCount(op); index++ {
		outcome, result, source, relation, ok := c.Operations.BehaviorResultAt(op, index)
		if !ok || !relation.Available() {
			return errors.New("target: malformed behavior result")
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := encodeInput(w, source); err != nil {
			return err
		}
		if err := w.Bytes(relation[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.Operations.BehaviorPredicateCount(op))); err != nil {
		return err
	}
	for index := 0; index < c.Operations.BehaviorPredicateCount(op); index++ {
		outcome, result, subject, relation, ok := c.Operations.BehaviorPredicateAt(op, index)
		if !ok || !relation.Available() {
			return errors.New("target: malformed behavior predicate")
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := encodeInput(w, subject); err != nil {
			return err
		}
		if err := w.Bytes(relation[:]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodeBindings(w *framing.Writer, op vocabulary.Operation) error {
	count := c.Operations.BindingCount(op)
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		ns, ok := c.Operations.BindingNamespaceAt(op, i)
		if !ok {
			return errors.New("target: malformed binding")
		}
		if err := w.Uint(uint64(ns)); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, i, true); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, i, false); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableOperation(w *framing.Writer, op vocabulary.Operation) error {
	_, ok := c.Operations.OperationAt(int(op) - 1)
	if !ok {
		return errors.New("target: malformed operation")
	}
	if err := c.encodeBindings(w, op); err != nil {
		return err
	}
	typeFormals := c.Operations.TypeFormalCount(op)
	if err := w.Count(uint64(typeFormals)); err != nil {
		return err
	}
	for i := 0; i < typeFormals; i++ {
		value, valueOK := c.Operations.TypeFormalConstraint(op, vocabulary.TypeFormal(i))
		if !valueOK {
			value = 0
		}
		if err := w.Bool(value != 0); err != nil {
			return err
		}
		if value != 0 {
			if err := encodeType(w, c, value); err != nil {
				return err
			}
		}
	}
	valuesVars := c.Operations.ValuesVarCount(op)
	if err := w.Uint(uint64(valuesVars)); err != nil {
		return err
	}
	for i := 0; i < valuesVars; i++ {
		value, valueOK := c.Operations.ValuesVarType(op, vocabulary.ValuesVar(i))
		if !valueOK {
			return errors.New("target: malformed Values variable type")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(c.Operations.RowFormalCount(op))); err != nil {
		return err
	}
	input, inputOK := c.Operations.Input(op)
	if !inputOK {
		return errors.New("target: malformed operation input")
	}
	if err := encodeValues(w, c, input); err != nil {
		return err
	}
	callbackCount := c.Operations.CallbackCount(op)
	if err := w.Count(uint64(callbackCount)); err != nil {
		return err
	}
	for i := 0; i < callbackCount; i++ {
		callback, callbackOK := c.Operations.CallbackAt(op, i)
		if !callbackOK {
			return errors.New("target: malformed callback geometry")
		}
		if err := c.encodePortableCallback(w, callback); err != nil {
			return err
		}
	}
	subedges := c.Operations.SubedgeCount(op)
	if err := w.Count(uint64(subedges)); err != nil {
		return err
	}
	for i := 0; i < subedges; i++ {
		edge, edgeOK := c.Operations.SubedgeAt(op, i)
		if !edgeOK {
			return errors.New("target: malformed subedge")
		}
		if err := c.encodePortableSubedge(w, op, edge); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.Operations.OutcomeCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.OutcomeCount(op); i++ {
		if err := c.encodePortableOutcome(w, op, i); err != nil {
			return err
		}
	}
	if err := c.encodeBehavior(w, op); err != nil {
		return err
	}
	if err := w.Count(uint64(c.Operations.SuspensionCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.SuspensionCount(op); i++ {
		y, r, s, m, ok := c.Operations.SuspensionAt(op, i)
		if !ok {
			return errors.New("target: malformed suspension")
		}
		for _, v := range []uint64{uint64(y), uint64(r), uint64(s), uint64(m)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(c.Operations.SpawnCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.SpawnCount(op); i++ {
		spawn, ok := c.Operations.SpawnIDAt(op, i)
		if !ok {
			return errors.New("target: malformed spawn")
		}
		owner, function, childID, yield, parentResume, childEntry, resumeValues, ok := c.Operations.SpawnRelation(spawn)
		if !ok || owner != op {
			return errors.New("target: malformed spawn")
		}
		child, ok := c.callbackSelector(childID)
		if !ok {
			return errors.New("target: malformed spawn callback")
		}
		if err := encodeInput(w, function); err != nil {
			return err
		}
		if err := w.Bytes(child[:]); err != nil {
			return err
		}
		for _, v := range []uint64{uint64(yield), uint64(parentResume)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, childEntry); err != nil {
			return err
		}
		if err := encodeValues(w, c, resumeValues); err != nil {
			return err
		}
		for sibling := 0; sibling < c.Operations.SpawnSiblingCount(spawn); sibling++ {
			v, siblingOK := c.Operations.SpawnSiblingAt(spawn, sibling)
			if !siblingOK {
				return errors.New("target: malformed spawn sibling")
			}
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(c.Operations.ResumeCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.ResumeCount(op); i++ {
		resume, ok := c.Operations.ResumeIDAt(op, i)
		if !ok {
			return errors.New("target: malformed resume")
		}
		owner, source, carrier, arguments, ok := c.Operations.Resume(resume)
		if !ok || owner != op {
			return errors.New("target: malformed resume")
		}
		for _, v := range []uint64{uint64(source), uint64(carrier)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, arguments); err != nil {
			return err
		}
		for outcome := 0; outcome < c.Operations.ResumeOutcomeCount(resume); outcome++ {
			_, v, outcomeOK := c.Operations.ResumeOutcomeAt(resume, outcome)
			if !outcomeOK {
				return errors.New("target: malformed resume outcome")
			}
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	transferCount := c.Operations.TransferCount(op)
	if err := w.Count(uint64(transferCount)); err != nil {
		return err
	}
	for i := 0; i < transferCount; i++ {
		if err := c.encodeTransfer(w, op, i); err != nil {
			return err
		}
	}
	tail, variable, tailOK := c.Operations.EffectTail(op)
	if !tailOK {
		return errors.New("target: malformed operation effect row")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	effects := c.Operations.EffectCount(op)
	if err := w.Count(uint64(effects)); err != nil {
		return err
	}
	for i := 0; i < effects; i++ {
		if err := c.encodePortableEffect(w, op, i, false); err != nil {
			return err
		}
	}
	operand, selector, subedge, resultOutcome, result, relationOK := c.Operations.OperationSubedgeRelation(op)
	if !relationOK {
		return w.Bool(false)
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(operand), uint64(selector), uint64(resultOutcome), uint64(result)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	role, roleOK := c.Operations.SubedgeRole(subedge)
	if !roleOK {
		return errors.New("target: malformed operation subedge relation")
	}
	if err := w.Uint(uint64(role)); err != nil {
		return err
	}
	aliases := c.Operations.OperationSubedgeRelationEffectAliasCount(op)
	if err := w.Count(uint64(aliases)); err != nil {
		return err
	}
	for i := 0; i < aliases; i++ {
		effect, effectOK := c.Operations.OperationSubedgeRelationEffectAliasAt(op, i)
		if !effectOK {
			return errors.New("target: malformed operation subedge relation effect alias")
		}
		if err := w.Uint(uint64(effect)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableCallback(w *framing.Writer, id vocabulary.CallbackID) error {
	if c == nil || id == 0 {
		return errors.New("target: malformed callback")
	}
	selector, ok := c.callbackSelector(id)
	if !ok {
		return errors.New("target: missing callback selector")
	}
	if err := w.Bytes(selector[:]); err != nil {
		return err
	}
	source, sourceOK := c.Operations.CallbackSource(id)
	if !sourceOK {
		return errors.New("target: malformed callback source")
	}
	if err := encodeInput(w, source); err != nil {
		return err
	}
	arguments, argumentsOK := c.Operations.CallbackArguments(id)
	if !argumentsOK {
		return errors.New("target: malformed callback arguments")
	}
	if err := encodeValues(w, c, arguments); err != nil {
		return err
	}
	admission, admissionOK := c.Operations.CallbackAdmission(id)
	if !admissionOK {
		return errors.New("target: malformed callback admission")
	}
	if err := w.Uint(uint64(admission)); err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		v, outcomeOK := c.Operations.CallbackOutcome(id, kind)
		if !outcomeOK {
			return errors.New("target: malformed callback outcome")
		}
		if err := encodeValues(w, c, v); err != nil {
			return err
		}
	}
	lifecycle, lifecycleOK := c.Operations.CallbackLifecycle(id)
	if !lifecycleOK {
		return errors.New("target: malformed callback lifecycle")
	}
	if err := w.Uint(uint64(lifecycle)); err != nil {
		return err
	}
	tail, variable, tailOK := c.Operations.CallbackEffectTail(id)
	if !tailOK {
		return errors.New("target: malformed callback effect row")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	effects := c.Operations.CallbackEffectCount(id)
	if err := w.Count(uint64(effects)); err != nil {
		return err
	}
	owner, ownerOK := c.Operations.CallbackOwner(id)
	if !ownerOK {
		return errors.New("target: malformed callback effect owner")
	}
	for i := 0; i < effects; i++ {
		if err := c.encodePortableEffect(w, owner, i, true, id); err != nil {
			return err
		}
	}
	releaseOperation, releaseInput, releaseOutcome, releaseMode, hasRelease := c.Operations.CallbackRelease(id)
	if !hasRelease {
		return w.Bool(false)
	}
	zeroBehavior, zeroOutcome, zeroOK := c.Operations.CallbackReleaseZero(id)
	if !zeroOK || !vocabulary.ValidCallbackReleaseZeroBehavior(zeroBehavior) {
		return errors.New("target: malformed callback release zero behavior")
	}
	target, ok := c.anchor(releaseOperation)
	if !ok {
		return errors.New("target: malformed release target")
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	if err := w.Bytes(target[:]); err != nil {
		return err
	}
	if err := w.Uint(uint64(releaseInput)); err != nil {
		return err
	}
	for _, outcome := range []uint32{releaseOutcome, zeroOutcome} {
		if zeroBehavior == vocabulary.CallbackReleaseZeroSuppress && outcome == zeroOutcome {
			if err := w.Bool(false); err != nil {
				return err
			}
			continue
		}
		i, ok := c.outcomeIndex(releaseOperation, int(outcome))
		if !ok {
			return errors.New("target: malformed release outcome")
		}
		if err := w.Bool(true); err != nil {
			return err
		}
		selector := c.outcomeSelectors[i]
		if err := w.Bytes(selector[:]); err != nil {
			return err
		}
	}
	for _, v := range []uint64{uint64(releaseMode), uint64(zeroBehavior)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableSubedge(w *framing.Writer, owner vocabulary.Operation, edge vocabulary.SubedgeID) error {
	ownerID, ownerOK := c.Operations.SubedgeOwner(edge)
	if !ownerOK || ownerID != owner {
		return errors.New("target: malformed subedge owner")
	}
	role, roleOK := c.Operations.SubedgeRole(edge)
	family, familyOK := c.Operations.SubedgeFamily(edge)
	callee, calleeOK := c.Operations.SubedgeCallee(edge)
	admission, admissionOK := c.Operations.SubedgeAdmission(edge)
	if !roleOK || !familyOK || !calleeOK || !admissionOK {
		return errors.New("target: malformed subedge")
	}
	for _, v := range []uint64{uint64(role), uint64(family), uint64(callee), uint64(admission)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	switch callee {
	case vocabulary.SubedgeCalleeCallback:
		callback, callbackOK := c.Operations.SubedgeCallback(edge)
		s, ok := c.callbackSelector(callback)
		if !callbackOK || !ok {
			return errors.New("target: malformed subedge callback")
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeCapturedInitialRead:
		root, key, readOK := c.Operations.SubedgeCapturedInitialRead(edge)
		identity, ok := c.InitialRootIdentity(root)
		if !readOK || !ok {
			return errors.New("target: malformed subedge root")
		}
		if err := w.String(identity); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeMetaKey:
		key, keyOK := c.Operations.SubedgeMetaKey(edge)
		if !keyOK {
			return errors.New("target: malformed subedge meta key")
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeInvalid:
	default:
		return errors.New("target: malformed subedge callee")
	}
	arguments, argumentsOK := c.Operations.SubedgeArguments(edge)
	if !argumentsOK {
		return errors.New("target: malformed subedge arguments")
	}
	if err := encodeValues(w, c, arguments); err != nil {
		return err
	}
	ruleEntry, ruleOK := c.Operations.SubedgeRuleEntry(edge)
	if !ruleOK {
		return errors.New("target: malformed subedge entry authority")
	}
	if err := w.Bool(ruleEntry); err != nil {
		return err
	}
	originCount := c.Operations.SubedgeArgumentOriginCount(edge)
	if err := w.Count(uint64(originCount)); err != nil {
		return err
	}
	for i := 0; i < originCount; i++ {
		segment, ordinal, source, input, originOK := c.Operations.SubedgeArgumentOriginAt(edge, i)
		if !originOK {
			return errors.New("target: malformed subedge argument origin")
		}
		for _, v := range []uint64{uint64(segment), uint64(ordinal), uint64(source)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeInput(w, input); err != nil {
			return err
		}
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		v, outcomeOK := c.Operations.SubedgeTerminal(edge, kind)
		if !outcomeOK {
			return errors.New("target: malformed subedge outcome")
		}
		if err := encodeValues(w, c, v); err != nil {
			return err
		}
	}
	failure, failureOK := c.Operations.SubedgeAdmissionFailure(edge)
	if !failureOK {
		return errors.New("target: malformed subedge admission failure")
	}
	if err := encodeValues(w, c, failure); err != nil {
		return err
	}
	route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := c.Operations.SubedgeAdmissionRoute(edge)
	if !routeOK {
		return errors.New("target: malformed subedge admission route")
	}
	if err := c.encodePortableRoute(w, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK = c.Operations.SubedgeRouteAt(edge, kind)
		if !routeOK {
			return errors.New("target: malformed subedge route")
		}
		if err := c.encodePortableRoute(w, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableRoute(w *framing.Writer, owner vocabulary.Operation, route vocabulary.SubedgeRoute, adjustment vocabulary.Adjustment, result vocabulary.Values, placement vocabulary.Placement, offset, outcome uint32, sibling vocabulary.SubedgeID, destination vocabulary.Values) error {
	for _, v := range []uint64{uint64(route), uint64(adjustment)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if err := encodeValues(w, c, result); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(placement), uint64(offset), uint64(outcome)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if sibling != 0 {
		siblingOwner, siblingOK := c.Operations.SubedgeOwner(sibling)
		role, roleOK := c.Operations.SubedgeRole(sibling)
		if !siblingOK || !roleOK || siblingOwner != owner {
			return errors.New("target: malformed route sibling")
		}
		if err := w.Uint(uint64(role)); err != nil {
			return err
		}
	} else if err := w.Uint(0); err != nil {
		return err
	}
	return encodeOptionalValues(w, c, destination)
}
func encodeOptionalValues(w *framing.Writer, c *Contract, v vocabulary.Values) error {
	if err := w.Bool(v != 0); err != nil {
		return err
	}
	if v == 0 {
		return nil
	}
	return encodeValues(w, c, v)
}

func (c *Contract) encodePortableOutcome(w *framing.Writer, owner vocabulary.Operation, outcome int) error {
	kind, values, ok := c.Operations.OutcomeAt(owner, outcome)
	if !ok {
		return errors.New("target: malformed outcome")
	}
	if err := w.Uint(uint64(kind)); err != nil {
		return err
	}
	if err := encodeValues(w, c, values); err != nil {
		return err
	}
	if err := w.Count(uint64(c.Operations.ProducedCount(owner, outcome))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.ProducedCount(owner, outcome); i++ {
		result, target, ok := c.Operations.ProducedAt(owner, outcome, i)
		if !ok {
			return errors.New("target: malformed produced")
		}
		a, ok := c.anchor(target)
		if !ok {
			return errors.New("target: malformed produced anchor")
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Bytes(a[:]); err != nil {
			return err
		}
		if err := w.Count(uint64(c.Operations.ProducedCaptureCount(owner, outcome, i))); err != nil {
			return err
		}
		for j := 0; j < c.Operations.ProducedCaptureCount(owner, outcome, i); j++ {
			kind, ordinal, ok := c.Operations.ProducedCaptureAt(owner, outcome, i, j)
			if !ok {
				return errors.New("target: malformed produced capture")
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if kind == vocabulary.CaptureCallback {
				s, ok := c.callbackSelector(vocabulary.CallbackID(ordinal))
				if !ok {
					return errors.New("target: malformed produced callback")
				}
				if err := w.Bytes(s[:]); err != nil {
					return err
				}
			} else if err := w.Uint(uint64(ordinal)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(c.Operations.CallbackResultCount(owner, outcome))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.CallbackResultCount(owner, outcome); i++ {
		result, callback, ok := c.Operations.CallbackResultAt(owner, outcome, i)
		if !ok {
			return errors.New("target: malformed callback result")
		}
		s, ok := c.callbackSelector(callback)
		if !ok {
			return errors.New("target: malformed callback result")
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.Operations.ResultAliasCount(owner, outcome))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.ResultAliasCount(owner, outcome); i++ {
		result, kind, ordinal, ok := c.Operations.ResultAliasAt(owner, outcome, i)
		if !ok {
			return errors.New("target: malformed result alias")
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := encodeInput(w, vocabulary.InputSource{Kind: kind, Ordinal: ordinal}); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.Operations.FreshResultCount(owner, outcome))); err != nil {
		return err
	}
	for i := 0; i < c.Operations.FreshResultCount(owner, outcome); i++ {
		result, ordinal, kind, ok := c.Operations.FreshResultAt(owner, outcome, i)
		if !ok {
			return errors.New("target: malformed fresh result")
		}
		for _, v := range []uint64{uint64(result), uint64(ordinal), uint64(kind)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	_ = owner
	return nil
}

func (c *Contract) encodeTransfer(w *framing.Writer, op vocabulary.Operation, index int) error {
	endpoint, payload, alias, identity, capabilities, ok := c.Operations.TransferDeclaration(func() vocabulary.TransferID {
		id, _ := c.Operations.TransferIDAt(op, index)
		return id
	}())
	if !ok {
		return errors.New("target: malformed transfer")
	}
	if err := w.Uint(uint64(endpoint.Kind)); err != nil {
		return err
	}
	if err := w.Uint(uint64(endpoint.Input)); err != nil {
		return err
	}
	if err := encodeInput(w, payload); err != nil {
		return err
	}
	if err := encodeInput(w, alias); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(identity), uint64(capabilities)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	transfer, transferOK := c.Operations.TransferIDAt(op, index)
	if !transferOK {
		return errors.New("target: malformed transfer handle")
	}
	outcomes := c.Operations.TransferOutcomeCount(op, index)
	if err := w.Count(uint64(outcomes)); err != nil {
		return err
	}
	for i := 0; i < outcomes; i++ {
		if err := w.Uint(uint64(i)); err != nil {
			return err
		}
		_, possibility, possibilityOK := c.Operations.TransferDeclarationOutcomeAt(transfer, i)
		if !possibilityOK {
			return errors.New("target: malformed transfer outcome")
		}
		if err := w.Uint(uint64(possibility)); err != nil {
			return err
		}
	}
	return nil
}
func (c *Contract) encodePortableEffect(w *framing.Writer, owner vocabulary.Operation, effect int, callback bool, callbackID ...vocabulary.CallbackID) error {
	var target vocabulary.Operation
	if callback {
		if len(callbackID) != 1 {
			return errors.New("target: malformed callback effect owner")
		}
		var ok bool
		target, ok = c.Operations.CallbackEffectTarget(callbackID[0], effect)
		if !ok {
			return errors.New("target: malformed callback effect")
		}
	} else {
		var ok bool
		target, ok = c.Operations.EffectTarget(owner, effect)
		if !ok {
			return errors.New("target: malformed effect")
		}
	}
	a, ok := c.anchor(target)
	if !ok {
		return errors.New("target: malformed effect target")
	}
	if err := w.Bytes(a[:]); err != nil {
		return err
	}
	valueCount := c.Operations.EffectValueArgumentCount(owner, effect)
	typeCount := c.Operations.EffectTypeArgumentCount(owner, effect)
	valuesVarCount := c.Operations.EffectValuesArgumentCount(owner, effect)
	rowCount := c.Operations.EffectRowArgumentCount(owner, effect)
	if callback {
		valueCount = c.Operations.CallbackEffectValueArgumentCount(callbackID[0], effect)
		typeCount = c.Operations.CallbackEffectTypeArgumentCount(callbackID[0], effect)
		valuesVarCount = c.Operations.CallbackEffectValuesArgumentCount(callbackID[0], effect)
		rowCount = c.Operations.CallbackEffectRowArgumentCount(callbackID[0], effect)
	}
	for _, count := range []int{valueCount, typeCount, valuesVarCount, rowCount} {
		if err := w.Count(uint64(count)); err != nil {
			return err
		}
	}
	for i := 0; i < valueCount; i++ {
		var value vocabulary.ValueFormal
		var valueOK bool
		if callback {
			value, valueOK = c.Operations.CallbackEffectValueArgumentAt(callbackID[0], effect, i)
		} else {
			value, valueOK = c.Operations.EffectValueArgumentAt(owner, effect, i)
		}
		if !valueOK {
			return errors.New("target: malformed portable effect value argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	for i := 0; i < typeCount; i++ {
		var value vocabulary.TypeFormal
		var valueOK bool
		if callback {
			value, valueOK = c.Operations.CallbackEffectTypeArgumentAt(callbackID[0], effect, i)
		} else {
			value, valueOK = c.Operations.EffectTypeArgumentAt(owner, effect, i)
		}
		if !valueOK {
			return errors.New("target: malformed portable effect type argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	for i := 0; i < valuesVarCount; i++ {
		var value vocabulary.ValuesVar
		var valueOK bool
		if callback {
			value, valueOK = c.Operations.CallbackEffectValuesArgumentAt(callbackID[0], effect, i)
		} else {
			value, valueOK = c.Operations.EffectValuesArgumentAt(owner, effect, i)
		}
		if !valueOK {
			return errors.New("target: malformed portable effect Values argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	for i := 0; i < rowCount; i++ {
		var value vocabulary.RowVar
		var valueOK bool
		if callback {
			value, valueOK = c.Operations.CallbackEffectRowArgumentAt(callbackID[0], effect, i)
		} else {
			value, valueOK = c.Operations.EffectRowArgumentAt(owner, effect, i)
		}
		if !valueOK {
			return errors.New("target: malformed portable effect row argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	return nil
}
