package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// formalRootEntrySeed is one invocation-owned concrete root boundary after
// the canonical entry transaction has been frozen into the formal product
// vocabulary. It owns no State and is never installed into the sealed
// relation template.
type formalRootEntrySeed struct {
	program  *RelationProgram
	variable relationVar
	constant formalRelationTupleConstant
}

// formalRootEntrySubstitution is the sole invocation-owned operation over a
// frozen relation.  It deliberately owns only the already-frozen formal
// constant for one selected root: the template and its recursive equations
// remain entry-free syntax.  Keeping this as a capability, rather than an
// algebra field named after an entry tuple, makes the eventual summary path
// explicit: a stabilized relation will consume this operation after, not
// during, its fixed point.
//
// Today the executor still installs the result at the root equation before
// solving.  That compatibility bridge is intentionally narrow and is not a
// summary cache: relation completion must first make every root-reading factor
// law symbolic.  In particular, no caller may turn this into a second solve
// or a concrete fallback.
type formalRootEntrySubstitution struct {
	seed formalRootEntrySeed
}

func newFormalRootEntrySubstitution(seed formalRootEntrySeed) (formalRootEntrySubstitution, error) {
	if !seed.validFor(seed.program) {
		return formalRootEntrySubstitution{}, fmt.Errorf("transformer: formal root substitution is unowned")
	}
	return formalRootEntrySubstitution{seed: seed}, nil
}

func (s formalRootEntrySubstitution) validFor(program *RelationProgram) bool {
	return s.seed.validFor(program)
}

func (s formalRootEntrySubstitution) substitute(a *formalTupleAlgebra, root *formalRootInputTemplate) (formalRelationTuple, bool, error) {
	if a == nil || root == nil || !s.validFor(a.program) {
		return formalRelationTuple{}, false, fmt.Errorf("transformer: formal root substitution is foreign")
	}
	if s.seed.variable != root.variable {
		return formalRelationTuple{}, false, nil
	}
	tuple, err := a.instantiatePreparedConstant(s.seed.constant)
	return tuple, true, err
}

// specializeStabilized is the post-WTO half of the root-entry seam.  It
// consumes only completed tuple roots and the already-prepared entry constant;
// it neither evaluates an equation nor asks the solver for another iteration.
//
// Product groups are re-materialized through their registered descriptors, so
// coordinate lexical classes, guarded-equality carriers, class advances, and
// provider write alphabets retain exactly the vocabulary frozen by the formal
// registry.  The only changed codomain is Values: entry-dependent terminals
// are interpreted against the prepared entry tuple and replaced by ground
// product values.
func (s formalRootEntrySubstitution) specializeStabilized(ctx context.Context, execution *formalRelationExecution) (*formalRelationExecution, error) {
	if ctx == nil || execution == nil || execution.algebra == nil || !s.validFor(execution.algebra.program) || execution.values == nil {
		return nil, fmt.Errorf("transformer: formal root specialization is unowned")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := execution.algebra
	entry, err := s.specializationEntry(a)
	if err != nil {
		return nil, err
	}
	regions, err := a.tupleLeafRegions(entry)
	if err != nil || len(regions) != 1 {
		if err == nil {
			err = fmt.Errorf("transformer: prepared root entry is not one concrete region")
		}
		return nil, err
	}
	values := make(map[formalRelationCell]formalRelationTuple, len(execution.values))
	for cell, tuple := range execution.values {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if tuple.bottom() || tuple.variable != s.seed.variable {
			values[cell] = tuple
			continue
		}
		specialized, specializeErr := s.specializeTuple(a, tuple, regions[0].evaluator)
		if specializeErr != nil {
			return nil, fmt.Errorf("transformer: specialize cell %+v: %w", cell, specializeErr)
		}
		values[cell] = specialized
	}
	// Publication is authorized only on this completed concrete image.  The
	// compatibility executor installs the same capability before solving; this
	// is its deliberately later counterpart.
	copy := s
	a.entrySubstitution = &copy
	return &formalRelationExecution{algebra: a, values: values, internalCallOutcomes: execution.internalCallOutcomes}, nil
}

// specializationEntry augments the prepared concrete tuple with its immutable
// IN aliases.  The normal concrete root path stores evolving entry values in
// MID slots; symbolic binding terms retain their IN spelling.  This private
// interpreter tuple makes those spellings equivalent without changing either
// the frozen constant or published State vocabulary.
func (s formalRootEntrySubstitution) specializationEntry(a *formalTupleAlgebra) (formalRelationTuple, error) {
	entry, err := a.instantiatePreparedConstant(s.seed.constant)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if s.seed.variable == 0 || int(s.seed.variable) > len(a.program.formalTemplate.rootInputs) {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	root := &a.program.formalTemplate.rootInputs[s.seed.variable-1]
	span, _, _, ok := a.span(s.seed.variable)
	if !ok {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	group, ok := span.valuesGroup()
	if !ok {
		return formalRelationTuple{}, errFormalComponentMalformed
	}
	regions, err := a.tupleLeafRegions(entry)
	if err != nil || len(regions) != 1 {
		if err == nil {
			err = errFormalComponentMalformed
		}
		return formalRelationTuple{}, err
	}
	values, err := regions[0].evaluator.valuesFactor()
	if err != nil || values.Top {
		return entry, err
	}
	if values.Values == nil {
		values.Values = make(map[FormalSlot]product.Value)
	} else {
		copied := make(map[FormalSlot]product.Value, len(values.Values)+len(root.bindings))
		for slot, value := range values.Values {
			copied[slot] = value
		}
		values.Values = copied
	}
	for _, binding := range root.bindings {
		input, inputOK := a.program.formalSlots.Slot(a.program.bodies[s.seed.variable-1].body, binding.input)
		middle, middleOK := a.program.formalSlots.Slot(a.program.bodies[s.seed.variable-1].body, binding.middle)
		if !inputOK || !middleOK {
			return formalRelationTuple{}, errFormalComponentMalformed
		}
		value := values.Values[middle]
		if !product.Equal(a.program.registry, value, product.Bottom(a.program.registry)) {
			values.Values[input] = value
		}
	}
	if len(values.Values) == 0 {
		values.Values = nil
	}
	return a.writeValuesFactor(entry, group, values)
}

func (s formalRootEntrySubstitution) specializeTuple(a *formalTupleAlgebra, source formalRelationTuple, entry formalTupleLeafEvaluator) (formalRelationTuple, error) {
	regions, err := a.tupleLeafRegions(source)
	if err != nil {
		return formalRelationTuple{}, err
	}
	span, directory, _, ok := a.span(source.variable)
	if !ok {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	var out formalRelationTuple
	for _, region := range regions {
		row := formalRelationTuple{variable: source.variable, root: directory.defaultRoot()}
		row, err = a.writeCare(row, decisionTrue)
		if err != nil {
			return formalRelationTuple{}, err
		}
		for _, group := range span.groupDescriptors() {
			leaves, leafErr := region.evaluator.leaves.group(group)
			if leafErr != nil {
				return formalRelationTuple{}, leafErr
			}
			switch group.kind {
			case formalFiberGroupValues:
				values, valuesErr := a.materializeFormalValuesGroup(region.evaluator.authority, group, leaves)
				if valuesErr != nil {
					return formalRelationTuple{}, valuesErr
				}
				values, valuesErr = s.specializeValues(region.evaluator.authority, entry, values)
				if valuesErr != nil {
					return formalRelationTuple{}, valuesErr
				}
				row, err = a.writeFormalValuesFactor(row, formalValuesFiberGroup{descriptor: group}, values)
			case formalFiberGroupOrdinaryLane:
				factor, factorErr := a.materializeOrdinaryGroup(region.evaluator.authority, group, leaves)
				if factorErr != nil {
					return formalRelationTuple{}, factorErr
				}
				row, err = a.writeOrdinaryFactor(row, formalOrdinaryLaneFiberGroup{descriptor: group}, factor)
			case formalFiberGroupCoordinateLane:
				factor, factorErr := a.materializeCoordinateGroup(region.evaluator.authority, span, group, leaves)
				if factorErr != nil {
					return formalRelationTuple{}, factorErr
				}
				row, err = a.writeCoordinateFactor(row, formalCoordinateLaneFiberGroup{descriptor: group}, factor)
			default:
				err = errFormalComponentMalformed
			}
			if err != nil {
				return formalRelationTuple{}, err
			}
		}
		for ordinal, descriptor := range span.descriptors() {
			if descriptor.role == formalFiberCare || descriptor.role == formalFiberMiddleValue || descriptor.role == formalFiberMiddlePath {
				continue
			}
			if _, grouped := span.groupForOrdinal(formalFiberOrdinal(ordinal)); grouped {
				continue
			}
			leaf, present := region.evaluator.leaves.leaf(formalFiberOrdinal(ordinal))
			if !present {
				return formalRelationTuple{}, errFormalComponentMalformed
			}
			row, err = a.writeScalar(row, descriptor, a.decisions.terminal(leaf))
			if err != nil {
				return formalRelationTuple{}, err
			}
		}
		row, err = a.restrictTupleCare(row, region.guard)
		if err != nil {
			return formalRelationTuple{}, err
		}
		out = a.combine(formalComponentJoin, out, row)
		if err := a.err(); err != nil {
			return formalRelationTuple{}, err
		}
	}
	return out, a.validateTuple(out)
}

func (s formalRootEntrySubstitution) specializeValues(authority *formalComponentTerminalAuthority, entry formalTupleLeafEvaluator, values formalValuesFactor) (formalValuesFactor, error) {
	if values.Top || len(values.Values) == 0 {
		return values, nil
	}
	for slot, value := range values.Values {
		ground, err := s.specializeValue(authority, entry, value)
		if err != nil {
			return formalValuesFactor{}, err
		}
		values.Values[slot] = formalGroundValue(ground)
	}
	return values, nil
}

func (s formalRootEntrySubstitution) specializeValue(authority *formalComponentTerminalAuthority, entry formalTupleLeafEvaluator, value formalValue) (product.Value, error) {
	if ground, concrete := value.concrete(); concrete {
		return ground, nil
	}
	set, err := formalSymbolicValueSetFromLeaf(authority, value.symbolicLeaf)
	if err != nil {
		return product.Value{}, err
	}
	var out product.Value
	if set.hasGround {
		out = set.ground
	} else {
		out = product.Bottom(authority.product.Registry())
	}
	for _, binding := range set.bindings {
		resolved, exact := entry.evalQualifiedValue(binding)
		if !exact {
			return product.Value{}, fmt.Errorf("transformer: entry specialization cannot resolve symbolic ValueTerm")
		}
		out = product.Join(authority.product.Registry(), out, resolved)
	}
	return out, nil
}

func (s formalRootEntrySeed) validFor(program *RelationProgram) bool {
	return program != nil && s.program == program && s.variable != 0 &&
		int(s.variable) <= len(program.bodies) && s.constant.valid() &&
		s.constant.forest == program.formalFibers && s.constant.variable == s.variable
}

// prepareRelationRootEntry is the sole concrete edge law for a root
// invocation. InitialStatePlan owns the entry coordinate when present;
// EntrySeedPlan then fills missing Values; reachability and domain
// normalization close the transaction. Both coordinate and formal executors
// consume this exact law.
func prepareRelationRootEntry(program *RelationProgram, body *relationProgramBody, entry state.State) (state.State, error) {
	if program == nil || program.registry == nil || body == nil || body.graph == nil ||
		!body.initialStatePlan.ValidFor(body.body, body.graph.ID(), body.graph.Size()) || !body.entrySeedPlan.Valid() {
		return state.State{}, fmt.Errorf("transformer: root entry transaction is unowned")
	}
	seed := entry
	if initial, present := body.initialStatePlan.At(state.InitialCoordinate(body.graph.Entry())); present {
		seed = initial
	}
	seed, err := body.entrySeedPlan.Apply(program.registry, seed)
	if err != nil {
		return state.State{}, fmt.Errorf("transformer: invocation root EntrySeed: %w", err)
	}
	return state.NormalizeForDomain(body.domain, state.Reachable(seed)), nil
}

// freezeFormalRootEntrySeed transposes one selected production invocation at
// the edge. The returned capability retains only registered full-product
// factors in the body's formal vocabulary.
func freezeFormalRootEntrySeed(program *RelationProgram, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (formalRootEntrySeed, error) {
	if program == nil || program.formalTemplate == nil || !program.formalTemplate.validFor(program) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry has no sealed equation system")
	}
	variable, present := program.byBody[bodyID]
	if !present || variable == 0 || int(variable) > len(program.bodies) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry has no body %s", bodyID)
	}
	body := &program.bodies[variable-1]
	prepared, err := prepareRelationRootEntry(program, body, entry)
	if err != nil {
		return formalRootEntrySeed{}, err
	}
	constant, err := freezeFormalRelationTupleConstant(program, variable, prepared)
	if err != nil {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: freeze formal root entry: %w", err)
	}
	seed := formalRootEntrySeed{program: program, variable: variable, constant: constant}
	if !seed.validFor(program) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry is incomplete")
	}
	return seed, nil
}
