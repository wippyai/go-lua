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
	if err := w.Count(uint64(c.BehaviorResultCount(op))); err != nil {
		return err
	}
	for index := 0; index < c.BehaviorResultCount(op); index++ {
		outcome, result, source, relation, ok := c.BehaviorResultAt(op, index)
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
	if err := w.Count(uint64(c.BehaviorPredicateCount(op))); err != nil {
		return err
	}
	for index := 0; index < c.BehaviorPredicateCount(op); index++ {
		outcome, result, subject, relation, ok := c.BehaviorPredicateAt(op, index)
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
	count := c.BindingCount(op)
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		ns, ok := c.BindingNamespaceAt(op, i)
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
	if err := w.Count(uint64(row.typeFormals.len())); err != nil {
		return err
	}
	for i := 0; i < row.typeFormals.len(); i++ {
		value := c.formals[row.typeFormals.start+uint32(i)]
		if err := w.Bool(value != 0); err != nil {
			return err
		}
		if value != 0 {
			if err := encodeType(w, c, value); err != nil {
				return err
			}
		}
	}
	if err := w.Uint(uint64(row.valuesVars)); err != nil {
		return err
	}
	for i := 0; i < int(row.valuesVars); i++ {
		if err := encodeType(w, c, c.valuesVarTypes[row.valuesTypes.start+uint32(i)]); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(row.rowFormals)); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.input); err != nil {
		return err
	}
	if err := w.Count(uint64(row.callbacks.len())); err != nil {
		return err
	}
	for i := row.callbacks.start; i < row.callbacks.end; i++ {
		if err := c.encodePortableCallback(w, vocabulary.CallbackID(i+1)); err != nil {
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
	if err := w.Count(uint64(row.outcomes.len())); err != nil {
		return err
	}
	for i := row.outcomes.start; i < row.outcomes.end; i++ {
		if err := c.encodePortableOutcome(w, op, int(i)); err != nil {
			return err
		}
	}
	if err := c.encodeBehavior(w, op); err != nil {
		return err
	}
	if err := w.Count(uint64(c.suspensionCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.suspensionCount(op); i++ {
		y, r, s, m, ok := c.suspensionAt(op, i)
		if !ok {
			return errors.New("target: malformed suspension")
		}
		for _, v := range []uint64{uint64(y), uint64(r), uint64(s), uint64(m)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.spawns.len())); err != nil {
		return err
	}
	for i := row.spawns.start; i < row.spawns.end; i++ {
		x := c.spawns[i]
		child, ok := c.callbackSelector(x.child)
		if !ok {
			return errors.New("target: malformed spawn callback")
		}
		if err := encodeInput(w, x.function); err != nil {
			return err
		}
		if err := w.Bytes(child[:]); err != nil {
			return err
		}
		for _, v := range []uint64{uint64(x.yield), uint64(x.parentResume)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, x.childEntry); err != nil {
			return err
		}
		if err := encodeValues(w, c, x.resumeValues); err != nil {
			return err
		}
		for _, v := range x.alternatives {
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.resumes.len())); err != nil {
		return err
	}
	for i := row.resumes.start; i < row.resumes.end; i++ {
		x := c.resumes[i]
		for _, v := range []uint64{uint64(x.source), uint64(x.carrier)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, x.arguments); err != nil {
			return err
		}
		for _, v := range x.outcomes {
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.transfers.len())); err != nil {
		return err
	}
	for i := row.transfers.start; i < row.transfers.end; i++ {
		if err := c.encodeTransferRow(w, c.transfers[i]); err != nil {
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
	if err := w.Uint(uint64(row.lifecycle)); err != nil {
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

func (c *Contract) encodePortableOutcome(w *framing.Writer, owner vocabulary.Operation, flat int) error {
	if flat < 0 || flat >= len(c.outcomes) {
		return errors.New("target: malformed outcome")
	}
	row := c.outcomes[flat]
	if err := w.Uint(uint64(row.kind)); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.values); err != nil {
		return err
	}
	if err := w.Count(uint64(row.produced.len())); err != nil {
		return err
	}
	for i := row.produced.start; i < row.produced.end; i++ {
		p := c.produced[i]
		a, ok := c.anchor(p.target)
		if !ok {
			return errors.New("target: malformed produced anchor")
		}
		if err := w.Uint(uint64(p.result)); err != nil {
			return err
		}
		if err := w.Bytes(a[:]); err != nil {
			return err
		}
		if err := w.Count(uint64(p.captures.len())); err != nil {
			return err
		}
		for j := p.captures.start; j < p.captures.end; j++ {
			x := c.captures[j]
			if err := w.Uint(uint64(x.kind)); err != nil {
				return err
			}
			if x.kind == vocabulary.CaptureCallback {
				s, ok := c.callbackSelector(vocabulary.CallbackID(x.ordinal))
				if !ok {
					return errors.New("target: malformed produced callback")
				}
				if err := w.Bytes(s[:]); err != nil {
					return err
				}
			} else if err := w.Uint(uint64(x.ordinal)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.callbackResults.len())); err != nil {
		return err
	}
	for i := row.callbackResults.start; i < row.callbackResults.end; i++ {
		x := c.callbackResults[i]
		s, ok := c.callbackSelector(x.callback)
		if !ok {
			return errors.New("target: malformed callback result")
		}
		if err := w.Uint(uint64(x.result)); err != nil {
			return err
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.resultAliases.len())); err != nil {
		return err
	}
	for i := row.resultAliases.start; i < row.resultAliases.end; i++ {
		x := c.resultAliases[i]
		if err := w.Uint(uint64(x.result)); err != nil {
			return err
		}
		if err := encodeInput(w, x.source); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.fresh.len())); err != nil {
		return err
	}
	for i := row.fresh.start; i < row.fresh.end; i++ {
		x := c.fresh[i]
		for _, v := range []uint64{uint64(x.result), uint64(x.ordinal), uint64(x.kind)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	_ = owner
	return nil
}

func (c *Contract) encodeTransferRow(w *framing.Writer, row transferRow) error {
	if err := w.Uint(uint64(row.endpoint.Kind)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.endpoint.Input)); err != nil {
		return err
	}
	if err := encodeInput(w, row.payload); err != nil {
		return err
	}
	if err := encodeInput(w, row.alias); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(row.identity), uint64(row.capabilities)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.outcomes.len())); err != nil {
		return err
	}
	for i := row.outcomes.start; i < row.outcomes.end; i++ {
		if err := w.Uint(uint64(i - row.outcomes.start)); err != nil {
			return err
		}
		if err := w.Uint(uint64(c.transferOutcomes[i])); err != nil {
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
