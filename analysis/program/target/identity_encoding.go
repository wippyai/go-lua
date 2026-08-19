package target

import (
	"errors"
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
	row, ok := c.operation(op)
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
	if err := w.Count(uint64(row.subedges.len())); err != nil {
		return err
	}
	for i := row.subedges.start; i < row.subedges.end; i++ {
		if err := c.encodePortableSubedge(w, op, c.subedges[i]); err != nil {
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
	if err := w.Uint(uint64(row.effectTail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.effectVar)); err != nil {
		return err
	}
	if err := w.Count(uint64(row.effects.len())); err != nil {
		return err
	}
	for i := row.effects.start; i < row.effects.end; i++ {
		if err := c.encodePortableEffect(w, c.effects[i]); err != nil {
			return err
		}
	}
	if row.subedgeRelation == 0 {
		return w.Bool(false)
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	relation := c.subedgeRelations[row.subedgeRelation-1]
	for _, v := range []uint64{uint64(relation.operand), uint64(relation.selector), uint64(relation.resultOutcome), uint64(relation.result)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	role := c.subedges[relation.subedge-1].role
	if err := w.Uint(uint64(role)); err != nil {
		return err
	}
	if err := w.Count(uint64(relation.effects.len())); err != nil {
		return err
	}
	for i := relation.effects.start; i < relation.effects.end; i++ {
		if err := w.Uint(uint64(c.subedgeRelationEffects[i])); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableCallback(w *framing.Writer, id vocabulary.CallbackID) error {
	row, ok := c.callback(id)
	if !ok {
		return errors.New("target: malformed callback")
	}
	selector, ok := c.callbackSelector(id)
	if !ok {
		return errors.New("target: missing callback selector")
	}
	if err := w.Bytes(selector[:]); err != nil {
		return err
	}
	if err := encodeInput(w, row.function); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.arguments); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.admission)); err != nil {
		return err
	}
	for _, v := range row.outcomes {
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
	if err := w.Uint(uint64(row.effectTail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.effectVar)); err != nil {
		return err
	}
	if err := w.Count(uint64(row.effects.len())); err != nil {
		return err
	}
	for i := row.effects.start; i < row.effects.end; i++ {
		if err := c.encodePortableEffect(w, c.effects[i]); err != nil {
			return err
		}
	}
	if row.release == 0 {
		return w.Bool(false)
	}
	r := c.callbackReleases[row.release-1]
	target, ok := c.anchor(r.operation)
	if !ok {
		return errors.New("target: malformed release target")
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	if err := w.Bytes(target[:]); err != nil {
		return err
	}
	if err := w.Uint(uint64(r.input)); err != nil {
		return err
	}
	for _, outcome := range []uint32{r.outcome, r.zeroOutcome} {
		if r.zeroBehavior == vocabulary.CallbackReleaseZeroSuppress && outcome == r.zeroOutcome {
			if err := w.Bool(false); err != nil {
				return err
			}
			continue
		}
		i, ok := c.outcomeIndex(r.operation, int(outcome))
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
	for _, v := range []uint64{uint64(r.mode), uint64(r.zeroBehavior)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableSubedge(w *framing.Writer, owner vocabulary.Operation, row subedgeRow) error {
	for _, v := range []uint64{uint64(row.role), uint64(row.family), uint64(row.callee), uint64(row.admission)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	switch row.callee {
	case vocabulary.SubedgeCalleeCallback:
		s, ok := c.callbackSelector(row.callback)
		if !ok {
			return errors.New("target: malformed subedge callback")
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeCapturedInitialRead:
		identity, ok := c.InitialRootIdentity(row.readRoot)
		if !ok {
			return errors.New("target: malformed subedge root")
		}
		if err := w.String(identity); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, row.readKey); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeMetaKey:
		if err := encodeExactKey(w, c, row.metaKey); err != nil {
			return err
		}
	case vocabulary.SubedgeCalleeInvalid:
	default:
		return errors.New("target: malformed subedge callee")
	}
	if err := encodeValues(w, c, row.arguments); err != nil {
		return err
	}
	if err := w.Bool(row.ruleEntry); err != nil {
		return err
	}
	if err := w.Count(uint64(row.argumentOrigins.len())); err != nil {
		return err
	}
	for i := row.argumentOrigins.start; i < row.argumentOrigins.end; i++ {
		x := c.subedgeOrigins[i]
		for _, v := range []uint64{uint64(x.segment), uint64(x.index), uint64(x.kind)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeInput(w, x.source); err != nil {
			return err
		}
	}
	for _, v := range row.outcomes {
		if err := encodeValues(w, c, v); err != nil {
			return err
		}
	}
	if err := encodeValues(w, c, row.admissionFailure); err != nil {
		return err
	}
	if err := c.encodePortableRoute(w, owner, row.admissionRoute); err != nil {
		return err
	}
	for _, r := range row.routes {
		if err := c.encodePortableRoute(w, owner, r); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableRoute(w *framing.Writer, owner vocabulary.Operation, row subedgeRouteRow) error {
	for _, v := range []uint64{uint64(row.route), uint64(row.adjustment)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if err := encodeValues(w, c, row.result); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(row.placement), uint64(row.offset), uint64(row.outcome)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if row.subedge != 0 {
		edge, ok := c.subedge(row.subedge)
		if !ok || edge.owner != owner {
			return errors.New("target: malformed route sibling")
		}
		if err := w.Uint(uint64(edge.role)); err != nil {
			return err
		}
	} else if err := w.Uint(0); err != nil {
		return err
	}
	return encodeOptionalValues(w, c, row.destination)
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
func (c *Contract) encodePortableEffect(w *framing.Writer, row effectRow) error {
	a, ok := c.anchor(row.target)
	if !ok {
		return errors.New("target: malformed effect target")
	}
	if err := w.Bytes(a[:]); err != nil {
		return err
	}
	for _, xs := range []indexRange{row.values, row.types, row.valuesVar, row.rows} {
		if err := w.Count(uint64(xs.len())); err != nil {
			return err
		}
	}
	for i := row.values.start; i < row.values.end; i++ {
		if err := w.Uint(uint64(c.effectVals[i])); err != nil {
			return err
		}
	}
	for i := row.types.start; i < row.types.end; i++ {
		if err := w.Uint(uint64(c.effectType[i])); err != nil {
			return err
		}
	}
	for i := row.valuesVar.start; i < row.valuesVar.end; i++ {
		if err := w.Uint(uint64(c.effectVars[i])); err != nil {
			return err
		}
	}
	for i := row.rows.start; i < row.rows.end; i++ {
		if err := w.Uint(uint64(c.effectRows[i])); err != nil {
			return err
		}
	}
	return nil
}
