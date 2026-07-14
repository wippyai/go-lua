package transformer

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// EvaluatedRootRequest carries independent identities for one transactional
// sparse projection. Identity is the root being evaluated; ExpectedIdentity
// is the caller-owned publication fence. Requirements and CallSurface are the
// sealed producer authorities behind the corresponding identity fields.
type EvaluatedRootRequest struct {
	Identity         evaluated.Identity
	ExpectedIdentity evaluated.Identity
	Requirements     operationplan.ObservationRequirements
	CallSurface      operationplan.CallSurface
}

// EvaluateSparseRoot specializes the already-reviewed sparse projection trace
// into a neutral immutable root. It is deliberately an inactive adapter:
// program scheduling and body solving do not call it. Any authority, shape,
// symbolic evaluation, coverage, or cancellation failure returns a zero root.
func (r Relation) EvaluateSparseRoot(
	ctx context.Context,
	request EvaluatedRootRequest,
	cursor BindingCursor,
	specialization SpecializationContext,
) (evaluated.Root, error) {
	if ctx == nil {
		return evaluated.Root{}, fmt.Errorf("transformer: evaluated root requires a context")
	}
	if err := ctx.Err(); err != nil {
		return evaluated.Root{}, err
	}
	if err := validateEvaluatedRootAuthority(r, request, cursor); err != nil {
		return evaluated.Root{}, err
	}
	if !emptySpecializationContext(specialization) {
		return evaluated.Root{}, fmt.Errorf("transformer: evaluated root slice requires callback-free terms and an empty specialization context")
	}
	if err := r.validateCallbackFreeEvaluatedRootTerms(ctx); err != nil {
		return evaluated.Root{}, err
	}
	evaluator, err := newEvaluatedTermEvaluator(ctx, r.arena, cursor)
	if err != nil {
		return evaluated.Root{}, err
	}

	trace := r.projectionTrace
	requirements := request.Requirements.Entries(false)
	if len(requirements) != len(trace.slots) {
		return evaluated.Root{}, fmt.Errorf("transformer: evaluated root slot count mismatch")
	}
	for index, requirement := range requirements {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.Root{}, err
			}
		}
		if trace.slots[index].requirement != requirement {
			return evaluated.Root{}, fmt.Errorf("transformer: evaluated root slot %d does not match sealed inventory", index)
		}
	}

	proof, guardWorlds, err := r.evaluateGuardWorldProof(ctx, evaluator)
	if err != nil {
		return evaluated.Root{}, err
	}

	ownerSummary, err := r.evaluateOwnerSummary(ctx, evaluator)
	if err != nil {
		return evaluated.Root{}, err
	}

	parts := evaluated.Parts{
		Identity: request.Identity, Proof: proof, Summary: ownerSummary,
	}
	for slotIndex, slot := range trace.slots {
		if slotIndex&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.Root{}, err
			}
		}
		requirement := slot.requirement
		slotID := uint32(slotIndex)
		switch requirement.Stage() {
		case operationplan.RequirementPoint:
			parts.Points = append(parts.Points, evaluated.PointReachability{
				Slot: slotID, Point: requirement.Point(), Worlds: guardWorlds[slot.guard],
			})
		case operationplan.RequirementBoundary:
			boundary, err := r.evaluateBoundary(ctx, slotID, slot, evaluator, guardWorlds)
			if err != nil {
				return evaluated.Root{}, err
			}
			parts.Boundaries = append(parts.Boundaries, boundary)
		case operationplan.RequirementEdge:
			to, ok := requirement.EdgeTarget()
			if !ok {
				return evaluated.Root{}, fmt.Errorf("transformer: evaluated root edge slot %d has no target", slotID)
			}
			parts.Edges = append(parts.Edges, evaluated.EdgeReachability{
				Slot: slotID, From: requirement.Point(), To: to, Worlds: guardWorlds[slot.guard],
			})
		case operationplan.RequirementObservation:
			observed, err := r.evaluateObservationSlot(ctx, slotID, slot, evaluator, guardWorlds)
			if err != nil {
				return evaluated.Root{}, err
			}
			parts.Observations = append(parts.Observations, observed)
		case operationplan.RequirementRoute:
			anchor, ok := requirement.Anchor()
			if !ok {
				return evaluated.Root{}, fmt.Errorf("transformer: evaluated root route slot %d has no anchor", slotID)
			}
			parts.Routes = append(parts.Routes, evaluated.Route{
				Slot: slotID, Point: requirement.Point(), Anchor: anchor, Worlds: guardWorlds[slot.guard],
			})
		default:
			return evaluated.Root{}, fmt.Errorf("transformer: evaluated root slot %d has unsupported stage", slotID)
		}
	}
	if err := ctx.Err(); err != nil {
		return evaluated.Root{}, err
	}
	root, err := evaluated.NewShadowRoot(ctx, r.arena.reg, request.Requirements, false, parts)
	if err != nil {
		return evaluated.Root{}, err
	}
	if err := ctx.Err(); err != nil {
		return evaluated.Root{}, err
	}
	return root, nil
}

func validateEvaluatedRootAuthority(r Relation, request EvaluatedRootRequest, cursor BindingCursor) error {
	identity := request.Identity
	if !identity.ShadowValid() || identity != request.ExpectedIdentity {
		return fmt.Errorf("transformer: evaluated root identity mismatch")
	}
	if r.arena == nil || r.contextual != "" || cursor.shape != r.shape || !r.observationComplete ||
		r.projectionTrace == nil || r.projectionTraceReason != "" {
		return fmt.Errorf("transformer: evaluated root relation is not projection-complete")
	}
	if !r.descriptors.validEvaluatedRootSchema(r.arena.reg) {
		return fmt.Errorf("transformer: evaluated root descriptor schema is not compiler-sealed")
	}
	for _, value := range cursor.values {
		if !product.BelongsToRegistry(r.arena.reg, value) {
			return fmt.Errorf("transformer: evaluated root binding belongs to a foreign registry")
		}
	}
	if !request.Requirements.Sealed() || request.Requirements.SchemaID() != identity.Schema ||
		request.Requirements.ConsumerInventoryID() != identity.Inventory ||
		r.projectionTrace.schema != identity.Schema || r.projectionTrace.inventory != identity.Inventory {
		return fmt.Errorf("transformer: evaluated root observation authority mismatch")
	}
	surface := request.CallSurface
	if !surface.Complete() || !surface.Digest().Available() || surface.Owner() != identity.Body ||
		surface.Digest() != identity.CallSurface || surface.PointCount() != int(identity.PointCount) {
		return fmt.Errorf("transformer: evaluated root call surface mismatch")
	}
	return nil
}

func collectSparseProjectionGuards(r Relation) []Guard {
	seen := make(map[Guard]struct{})
	for _, row := range r.rows {
		seen[row.Guard] = struct{}{}
		for _, item := range row.Observations {
			seen[item.Guard] = struct{}{}
		}
		for _, item := range row.observationObligations {
			seen[item.Guard] = struct{}{}
		}
	}
	for _, slot := range r.projectionTrace.slots {
		seen[slot.guard] = struct{}{}
		for _, fragment := range slot.fragments {
			seen[fragment.guard] = struct{}{}
		}
		for _, item := range slot.observed {
			seen[item.Guard] = struct{}{}
		}
		for _, item := range slot.owed {
			seen[item.Guard] = struct{}{}
		}
	}
	out := make([]Guard, 0, len(seen))
	for guard := range seen {
		out = append(out, guard)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := r.arena.canonicalGuard(out[i]), r.arena.canonicalGuard(out[j])
		if left != right {
			return left < right
		}
		return out[i] < out[j]
	})
	return out
}

func emptySpecializationContext(context SpecializationContext) bool {
	return context.CellResult == nil && context.DynamicRead == nil && context.DynamicTableRead == nil && context.IteratorProjection == nil
}

func (r Relation) validateCallbackFreeEvaluatedRootTerms(ctx context.Context) error {
	if r.arena == nil {
		return fmt.Errorf("transformer: evaluated root has no term arena")
	}
	seen := make(map[ValueTerm]bool)
	steps := uint64(0)
	check := func(term ValueTerm) error { return r.arena.validateCallbackFreeValueTerm(ctx, term, seen, &steps) }
	for _, row := range r.rows {
		if len(row.Effects) != 0 {
			return fmt.Errorf("transformer: evaluated root slice rejects effects")
		}
		for _, operation := range row.Ops {
			if err := check(operation.Value); err != nil {
				return err
			}
		}
		for _, proof := range row.Proofs {
			if proof.Key != 0 {
				if err := check(proof.Key); err != nil {
					return err
				}
			}
		}
		for _, refinement := range row.PathRefinements {
			if err := check(refinement.Value); err != nil {
				return err
			}
		}
		for _, item := range row.Observations {
			if err := check(item.Actual); err != nil {
				return err
			}
			if item.Expected != 0 {
				if err := check(item.Expected); err != nil {
					return err
				}
			}
		}
	}
	for _, guard := range collectSparseProjectionGuards(r) {
		if err := r.arena.validateCallbackFreeGuard(ctx, guard, seen, &steps); err != nil {
			return err
		}
	}
	for _, slot := range r.projectionTrace.slots {
		for _, fragment := range slot.fragments {
			for _, operation := range fragment.operations {
				if err := check(operation.Value); err != nil {
					return err
				}
			}
		}
		for _, item := range slot.observed {
			if err := check(item.Actual); err != nil {
				return err
			}
			if item.Expected != 0 {
				if err := check(item.Expected); err != nil {
					return err
				}
			}
		}
	}
	return ctx.Err()
}

func (a *Arena) validateCallbackFreeGuard(ctx context.Context, guard Guard, seen map[ValueTerm]bool, steps *uint64) error {
	*steps++
	if *steps&63 == 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if guard == 0 || int(guard) >= len(a.guards) {
		return fmt.Errorf("transformer: evaluated root has invalid guard term")
	}
	node := a.guards[guard]
	switch node.op {
	case guardTrue, guardFalse:
		return nil
	case guardTruthy, guardFalsy:
		return a.validateCallbackFreeValueTerm(ctx, node.value, seen, steps)
	case guardAnd, guardOr:
		for _, child := range node.args {
			if err := a.validateCallbackFreeGuard(ctx, child, seen, steps); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("transformer: evaluated root has unsupported guard term")
	}
}

func (a *Arena) validateCallbackFreeValueTerm(ctx context.Context, term ValueTerm, seen map[ValueTerm]bool, steps *uint64) error {
	*steps++
	if *steps&63 == 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if term == 0 || int(term) >= len(a.values) {
		return fmt.Errorf("transformer: evaluated root has invalid value term")
	}
	if seen[term] {
		return nil
	}
	seen[term] = true
	node := a.values[term]
	switch node.op {
	case valueCellResult, valueDynamicRead, valueDynamicTableRead, valueIteratorProjection, valueAllocationResult:
		return fmt.Errorf("transformer: evaluated root value term requires callback or allocation semantics")
	}
	for _, argument := range node.args {
		if err := a.validateCallbackFreeValueTerm(ctx, argument, seen, steps); err != nil {
			return err
		}
	}
	if node.op <= valueInvalid || node.op > valueLuaTypeName {
		return fmt.Errorf("transformer: evaluated root has unsupported value term")
	}
	return nil
}

func (r Relation) evaluateGuardWorldProof(ctx context.Context, evaluator *evaluatedTermEvaluator) (evaluated.WorldProof, map[Guard]evaluated.WorldSet, error) {
	guards := collectSparseProjectionGuards(r)
	scratch := newObservationCoverageScratch()
	scratch.reset(r.arena)
	for _, guard := range guards {
		if err := scratch.collectGuardAtoms(ctx, guard); err != nil {
			return evaluated.WorldProof{}, nil, fmt.Errorf("transformer: evaluated root malformed guard partition: %w", err)
		}
	}
	if err := scratch.rankAtoms(); err != nil {
		return evaluated.WorldProof{}, nil, err
	}
	for index := 1; index < len(scratch.atoms); index++ {
		left, right := scratch.atoms[index-1], scratch.atoms[index]
		if scratch.names[left] == scratch.names[right] && left != right {
			// canonicalValue intentionally uses compact product hashes. An
			// unequal collision has no registry-owned total ordering, so using
			// arena IDs would make artifact bytes depend on construction order.
			return evaluated.WorldProof{}, nil, fmt.Errorf("transformer: evaluated root atom identity collision")
		}
	}
	bdds := make(map[Guard]coverageBDD, len(guards))
	for _, guard := range guards {
		bdd, ok := scratch.guard(ctx, guard)
		if !ok {
			return evaluated.WorldProof{}, nil, fmt.Errorf("transformer: evaluated root guard ROBDD construction failed")
		}
		bdds[guard] = bdd
	}
	type atomDomain struct{ canFalse, canTrue bool }
	domains := make([]atomDomain, len(scratch.atoms))
	proof := evaluated.WorldProof{Predicates: make([]evaluated.Predicate, len(scratch.atoms))}
	expressions := make(map[ValueTerm]evaluated.ExpressionID)
	for index, atom := range scratch.atoms {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.WorldProof{}, nil, err
			}
		}
		value, err := evaluator.value(atom)
		if err != nil {
			return evaluated.WorldProof{}, nil, err
		}
		domains[index] = atomDomain{canFalse: valueref.CanBeFalsy(r.arena.reg, value), canTrue: valueref.CanBeTruthy(r.arena.reg, value)}
		if !domains[index].canFalse && !domains[index].canTrue {
			return evaluated.WorldProof{}, nil, fmt.Errorf("transformer: evaluated root guard atom has no valuation")
		}
		expression, err := r.appendEvaluatedExpression(ctx, atom, expressions, &proof.Expressions)
		if err != nil {
			return evaluated.WorldProof{}, nil, err
		}
		proof.Predicates[index] = evaluated.Predicate{ID: uint32(index), Value: expression}
	}
	type decisionKey struct {
		predicate uint32
		low, high coverageBDD
	}
	type compactNode struct {
		predicate uint32
		low, high coverageBDD
	}
	var nodes []compactNode
	unique := make(map[decisionKey]coverageBDD)
	memo := make(map[coverageBDD]coverageBDD)
	steps := uint64(0)
	var reduce func(coverageBDD) (coverageBDD, error)
	reduce = func(root coverageBDD) (coverageBDD, error) {
		if root < 2 {
			return root, nil
		}
		if prior, ok := memo[root]; ok {
			return prior, nil
		}
		steps++
		if steps&63 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		node := scratch.nodes[root]
		low, err := reduce(node.low)
		if err != nil {
			return 0, err
		}
		high, err := reduce(node.high)
		if err != nil {
			return 0, err
		}
		domain := domains[node.variable]
		if !domain.canFalse {
			memo[root] = high
			return high, nil
		}
		if !domain.canTrue {
			memo[root] = low
			return low, nil
		}
		if low == high {
			memo[root] = low
			return low, nil
		}
		key := decisionKey{predicate: node.variable, low: low, high: high}
		if prior, ok := unique[key]; ok {
			memo[root] = prior
			return prior, nil
		}
		id := coverageBDD(len(nodes) + 2)
		nodes = append(nodes, compactNode(key))
		unique[key], memo[root] = id, id
		return id, nil
	}
	compactRoots := make(map[Guard]coverageBDD, len(guards))
	for _, guard := range guards {
		root, err := reduce(bdds[guard])
		if err != nil {
			return evaluated.WorldProof{}, nil, err
		}
		compactRoots[guard] = root
	}
	oldToNew := map[coverageBDD]evaluated.DecisionID{coverageFalse: evaluated.DecisionFalse, coverageTrue: evaluated.DecisionTrue}
	for predicate := len(scratch.atoms) - 1; predicate >= 0; predicate-- {
		var ids []coverageBDD
		for index, node := range nodes {
			if int(node.predicate) == predicate {
				ids = append(ids, coverageBDD(index+2))
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			a, b := nodes[int(ids[i])-2], nodes[int(ids[j])-2]
			if oldToNew[a.low] != oldToNew[b.low] {
				return oldToNew[a.low] < oldToNew[b.low]
			}
			return oldToNew[a.high] < oldToNew[b.high]
		})
		for _, old := range ids {
			node := nodes[int(old)-2]
			id := evaluated.DecisionID(len(proof.Decisions) + 2)
			proof.Decisions = append(proof.Decisions, evaluated.Decision{
				ID: id, Predicate: node.predicate, Low: oldToNew[node.low], High: oldToNew[node.high],
			})
			oldToNew[old] = id
		}
	}
	worlds := make(map[Guard]evaluated.WorldSet, len(guards))
	for _, guard := range guards {
		worlds[guard] = evaluated.WorldSet{Root: oldToNew[compactRoots[guard]]}
	}
	return proof, worlds, nil
}

func (r Relation) appendEvaluatedExpression(
	ctx context.Context,
	term ValueTerm,
	memo map[ValueTerm]evaluated.ExpressionID,
	out *[]evaluated.Expression,
) (evaluated.ExpressionID, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if prior, ok := memo[term]; ok {
		return prior, nil
	}
	if term == 0 || int(term) >= len(r.arena.values) {
		return 0, fmt.Errorf("transformer: evaluated root expression term is invalid")
	}
	node := r.arena.values[term]
	args := make([]evaluated.ExpressionID, len(node.args))
	for index, argument := range node.args {
		converted, err := r.appendEvaluatedExpression(ctx, argument, memo, out)
		if err != nil {
			return 0, err
		}
		args[index] = converted
	}
	expression := evaluated.Expression{Args: args, Root: node.root.Index, Constant: node.value}
	switch node.op {
	case valueRoot:
		expression.Op = evaluated.ExpressionRoot
		switch node.root.Kind {
		case RootParam:
			expression.RootKind = evaluated.RootParam
		case RootCapture:
			expression.RootKind = evaluated.RootCapture
		case RootGlobal:
			expression.RootKind = evaluated.RootGlobal
		case RootResult:
			expression.RootKind = evaluated.RootResult
		case RootHeapTemplate:
			expression.RootKind = evaluated.RootHeapTemplate
		default:
			return 0, fmt.Errorf("transformer: evaluated root expression has invalid boundary root")
		}
	case valueConstant:
		expression.Op = evaluated.ExpressionConstant
		if scalar, ok := evaluatedScalarLiteral(r.arena.reg, node.value); ok {
			expression.Constant = product.Top()
			expression.Scalar = scalar
		}
	case valueJoin:
		expression.Op = evaluated.ExpressionJoin
	case valueRefinement:
		expression.Op = evaluated.ExpressionRefinement
	case valueRuntimeValidation:
		expression.Op = evaluated.ExpressionRuntimeValidation
	case valueStringConcat:
		expression.Op = evaluated.ExpressionStringConcat
	case valueScalarEqual:
		expression.Op = evaluated.ExpressionScalarEqual
	case valueScalarNotEqual:
		expression.Op = evaluated.ExpressionScalarNotEqual
	case valueScalarAnd:
		expression.Op = evaluated.ExpressionScalarAnd
	case valueScalarOr:
		expression.Op = evaluated.ExpressionScalarOr
	case valueStaticIndex:
		expression.Op = evaluated.ExpressionStaticIndex
	case valueLuaTypeName:
		expression.Op = evaluated.ExpressionLuaTypeName
	default:
		return 0, fmt.Errorf("transformer: evaluated root expression requires unsupported or callback semantics")
	}
	expression.ID = evaluated.ExpressionID(len(*out) + 1)
	*out = append(*out, expression)
	memo[term] = expression.ID
	return expression.ID, nil
}

func evaluatedScalarLiteral(reg *axis.Registry, value product.Value) (evaluated.Scalar, bool) {
	witness, ok := typevalue.WitnessOf(reg, value)
	if !ok {
		return evaluated.Scalar{}, false
	}
	literal, ok := witness.(*typ.Literal)
	if !ok || literal == nil {
		return evaluated.Scalar{}, false
	}
	var scalar evaluated.Scalar
	var canonical product.Value
	switch literal.Base {
	case kind.Boolean:
		v, ok := literal.Value.(bool)
		if !ok {
			return evaluated.Scalar{}, false
		}
		scalar, canonical = evaluated.Scalar{Kind: evaluated.ScalarBoolean, Boolean: v}, typevalue.LiteralBool(reg, v)
	case kind.Integer:
		v, ok := literal.Value.(int64)
		if !ok {
			return evaluated.Scalar{}, false
		}
		scalar, canonical = evaluated.Scalar{Kind: evaluated.ScalarInteger, Integer: v}, typevalue.LiteralInt(reg, v)
	case kind.Number:
		v, ok := literal.Value.(float64)
		if !ok {
			return evaluated.Scalar{}, false
		}
		scalar, canonical = evaluated.Scalar{Kind: evaluated.ScalarNumber, Number: v}, typevalue.LiteralNumber(reg, v)
	case kind.String:
		v, ok := literal.Value.(string)
		if !ok {
			return evaluated.Scalar{}, false
		}
		scalar, canonical = evaluated.Scalar{Kind: evaluated.ScalarString, String: v}, typevalue.LiteralString(reg, v)
	default:
		return evaluated.Scalar{}, false
	}
	if !product.Equal(reg, value, canonical) {
		return evaluated.Scalar{}, false
	}
	return scalar, true
}

func (r Relation) evaluateBoundary(
	ctx context.Context,
	slotID uint32,
	slot sparseProjectionSlot,
	evaluator *evaluatedTermEvaluator,
	guardWorlds map[Guard]evaluated.WorldSet,
) (evaluated.Boundary, error) {
	fact, ok := slot.requirement.FactKind()
	if !ok || fact != operationplan.Return {
		return evaluated.Boundary{}, fmt.Errorf("transformer: evaluated root boundary slot %d is not Return", slotID)
	}
	out := evaluated.Boundary{Slot: slotID, Point: slot.requirement.Point()}
	descriptors := r.descriptors
	if descriptors == nil {
		descriptors = DefaultDescriptorRegistry()
	}
	for index, fragment := range slot.fragments {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.Boundary{}, err
			}
		}
		fragmentWorlds := guardWorlds[fragment.guard]
		if fragmentWorlds.Root == evaluated.DecisionFalse {
			continue
		}
		candidate := fragment.output.Clone()
		operationSlots := make([]uint32, 0, len(fragment.operations))
		for _, operation := range fragment.operations {
			if operation.Descriptor != DescriptorReturn {
				return evaluated.Boundary{}, fmt.Errorf("transformer: evaluated root boundary slot %d contains non-Return operation", slotID)
			}
			value, err := evaluator.value(operation.Value)
			if err != nil {
				return evaluated.Boundary{}, err
			}
			operationSlots = append(operationSlots, operation.Slot)
			handler := descriptors.handlers[DescriptorReturn]
			if handler == nil || fragment.guard != r.arena.True() && !handler.ConditionalAllowed() ||
				handler.Apply(r.arena.reg, &candidate, operation.Slot, value) != nil {
				return evaluated.Boundary{}, fmt.Errorf("transformer: evaluated root Return descriptor rejected slot %d", slotID)
			}
		}
		sort.Slice(operationSlots, func(i, j int) bool { return operationSlots[i] < operationSlots[j] })
		for index := 1; index < len(operationSlots); index++ {
			if operationSlots[index-1] == operationSlots[index] {
				return evaluated.Boundary{}, fmt.Errorf("transformer: evaluated root boundary has duplicate Return slot")
			}
		}
		candidate, err := summary.NormalizeContext(ctx, r.arena.reg, candidate)
		if err != nil {
			return evaluated.Boundary{}, err
		}
		values := make([]evaluated.IndexedValue, len(candidate.Returns))
		for index, value := range candidate.Returns {
			values[index] = evaluated.IndexedValue{Index: uint32(index), Value: value}
		}
		out.Fragments = append(out.Fragments, evaluated.BoundaryFragment{Worlds: fragmentWorlds, Values: values, Summary: candidate})
	}
	return out, nil
}

func (r Relation) evaluateObservationSlot(
	ctx context.Context,
	slotID uint32,
	slot sparseProjectionSlot,
	evaluator *evaluatedTermEvaluator,
	guardWorlds map[Guard]evaluated.WorldSet,
) (evaluated.ObservationSlot, error) {
	out := evaluated.ObservationSlot{Slot: slotID, Point: slot.requirement.Point()}
	for index, term := range slot.observed {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.ObservationSlot{}, err
			}
		}
		worlds := guardWorlds[term.Guard]
		if worlds.Root == evaluated.DecisionFalse {
			continue
		}
		actual, err := evaluator.value(term.Actual)
		if err != nil {
			return evaluated.ObservationSlot{}, err
		}
		item := evaluated.Observation{
			Owner: term.BodyOwner, Invocation: term.Route, Kind: term.Kind,
			Anchor: term.Anchor, Slot: term.Slot, Actual: actual,
		}
		if term.Expected != 0 {
			item.Expected, err = evaluator.value(term.Expected)
			if err != nil {
				return evaluated.ObservationSlot{}, err
			}
			item.HasExpected = true
		}
		item.Worlds = worlds
		out.Observed = append(out.Observed, item)
	}
	for index, obligation := range slot.owed {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return evaluated.ObservationSlot{}, err
			}
		}
		worlds := guardWorlds[obligation.Guard]
		if worlds.Root == evaluated.DecisionFalse {
			continue
		}
		out.Obligations = append(out.Obligations, evaluated.Obligation{
			Worlds: worlds, Owner: obligation.BodyOwner, Invocation: obligation.Route, Anchor: obligation.Anchor,
		})
	}
	return out, nil
}
