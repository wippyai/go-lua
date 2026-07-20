package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalIdentitySupport is the finite may-set of singleton identities which
// can reach one symbolic value. An absent set is Bottom, never "all
// identities": a non-singleton executable identity remains represented by the
// ordinary identity lattice, while coordinate transport needs only the finite
// constructor/formal roots which can own descendant facts.
type formalIdentitySupport []identity.Term

type formalIdentityEnvironment struct {
	values    map[statekey.Value]formalIdentitySupport
	producers map[state.DynamicReadIdentityProducer]formalIdentitySupport
}

// formalDynamicIdentityPublication is the frozen source binding for one
// ProductDomain-owned producer descriptor. Multiple source terms are joined
// before the producer is replaced, preserving constructor aggregate semantics
// without teaching the transformer how member aliases are normalized.
type formalDynamicIdentityPublication struct {
	producer    state.DynamicReadIdentityProducer
	sources     []ValueTerm
	ownerSource ValueTerm
}

func freezeFormalDynamicIdentityPublications(
	body *relationProgramBody,
	keys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	step boundaryStep,
) ([]formalDynamicIdentityPublication, error) {
	if body == nil || keys == nil || !keys.Valid() || step.kind != boundaryStepEffect || step.effect == 0 ||
		body.relation.effects == nil || int(step.effect) >= len(body.relation.effects.nodes) {
		return nil, nil
	}
	effect := body.relation.effects.nodes[step.effect]
	var out []formalDynamicIdentityPublication
	appendPublications := func(publications []state.DynamicReadIdentityPublication, sources []ValueTerm) error {
		for _, publication := range publications {
			if publication.Source < 0 || publication.Source >= len(sources) || !publication.Producer.ValidFor(body.productDomain, keys) {
				return fmt.Errorf("transformer: dynamic-read identity publication is malformed")
			}
			// ProductDomain returns publications in canonical producer/source
			// order. Group adjacent entries in one pass; object literals may
			// publish thousands of members, so a linear search here is a
			// quadratic freeze defect.
			if len(out) == 0 || out[len(out)-1].producer != publication.Producer {
				out = append(out, formalDynamicIdentityPublication{producer: publication.Producer})
			}
			out[len(out)-1].sources = append(out[len(out)-1].sources, sources[publication.Source])
		}
		return nil
	}
	switch effect.kind {
	case EffectPathStore:
		if !effect.pathStoreHasStatic {
			return nil, nil
		}
		target, err := freezeFormalEffectPathKey(body, formalFiberDescriptorSpan{keys: keys, rekey: rekey}, effect.pathStoreStatic.Target)
		if err != nil {
			return nil, err
		}
		plan, err := body.productDomain.PrepareStaticMemberFactorPlan(keys, target, product.Bottom(body.productDomain.Registry()))
		if err != nil {
			return nil, err
		}
		publications, err := body.productDomain.StaticMemberIdentityPublications(plan)
		if err != nil {
			return nil, err
		}
		if err := appendPublications(publications, []ValueTerm{effect.pathStoreStatic.Value}); err != nil {
			return nil, err
		}
	case EffectObjectMaterialization:
		templates, err := formalObjectMaterializationTemplates(body.relation, step.effect)
		if err != nil {
			return nil, err
		}
		if len(templates) != len(effect.pathStoreObject.Heaps) {
			return nil, fmt.Errorf("transformer: object identity publication schema is incomplete")
		}
		shapes := make([]state.ObjectConstructorShape, len(templates))
		for objectIndex, object := range effect.pathStoreObject.Heaps {
			shapes[objectIndex] = state.ObjectConstructorShape{
				Identity: identity.AllocationTerm(templates[objectIndex]), StableShape: object.StableShape,
			}
			for _, member := range object.Members {
				shapes[objectIndex].MemberSuffixes = append(shapes[objectIndex].MemberSuffixes, member.Suffix)
			}
		}
		plan, err := body.productDomain.PrepareObjectConstructorPlan(keys, shapes)
		if err != nil {
			return nil, err
		}
		refs, err := body.productDomain.ObjectConstructorIdentitySourceRefs(plan)
		if err != nil {
			return nil, err
		}
		sources := make([]ValueTerm, len(refs))
		for sourceIndex, ref := range refs {
			objectIndex := ref.ObjectIndex()
			memberIndex, member := ref.MemberIndex()
			if !member || objectIndex < 0 || objectIndex >= len(effect.pathStoreObject.Heaps) ||
				memberIndex < 0 || memberIndex >= len(effect.pathStoreObject.Heaps[objectIndex].Members) {
				return nil, fmt.Errorf("transformer: object identity publication source is malformed")
			}
			sources[sourceIndex] = effect.pathStoreObject.Heaps[objectIndex].Members[memberIndex].Value
		}
		publications, err := body.productDomain.ObjectConstructorIdentityPublications(plan)
		if err != nil {
			return nil, err
		}
		if err := appendPublications(publications, sources); err != nil {
			return nil, err
		}
	case EffectIndexMutation:
		if effect.table.kind != effectTargetPath || effect.table.path == 0 || int(effect.table.path) >= len(body.relation.arena.paths) ||
			effect.key == 0 || effect.value == 0 {
			return nil, fmt.Errorf("transformer: dynamic-index identity publication schema is incomplete: table-kind=%d path=%d/%d key=%d value=%d point=%d",
				effect.table.kind, effect.table.path, len(body.relation.arena.paths), effect.key, effect.value, step.point)
		}
		table, _, err := freezeFormalEffectPath(body, formalFiberDescriptorSpan{keys: keys, rekey: rekey}, effect.table.path)
		if err != nil {
			return nil, err
		}
		keyClass := state.DynamicReadIdentityWildcardKeyClass()
		keyNode := body.relation.arena.values[effect.key]
		if (keyNode.op == valueConstant || keyNode.op == valueRefinement || keyNode.op == valueFalsyAbsentRefinement || keyNode.op == valueExpressionRefinement) &&
			product.BelongsToRegistry(body.productDomain.Registry(), keyNode.value) {
			keyClass, err = body.productDomain.PrepareDynamicReadIdentityKeyClass(body.relation.arena.typeValues, keyNode.value)
			if err != nil {
				return nil, err
			}
		}
		pathNode := body.relation.arena.paths[effect.table.path]
		ownerSource := body.relation.arena.Root(pathNode.root)
		if ownerSource == 0 && pathNode.environment != 0 {
			ownerSource, _ = body.relation.arena.environmentValue(pathNode.environment)
		}
		if ownerSource == 0 {
			return nil, fmt.Errorf("transformer: dynamic-index identity owner source is undeclared")
		}
		producers, err := body.productDomain.DynamicIndexDynamicReadIdentityProducerDeclarations(
			keys, []dynamicindex.Key{{Table: table, Site: dynamicindex.SiteForPoint(int(step.point))}}, keyClass, nil,
		)
		if err != nil {
			return nil, err
		}
		for _, producer := range producers {
			out = append(out, formalDynamicIdentityPublication{producer: producer, sources: []ValueTerm{effect.value}, ownerSource: ownerSource})
		}
	}
	return out, nil
}

func formalIdentityTermLess(left, right identity.Term) bool {
	return identity.Less(left, right)
}

func unionFormalIdentitySupport(inputs ...formalIdentitySupport) formalIdentitySupport {
	seen := make(map[identity.Term]struct{})
	for _, input := range inputs {
		for _, term := range input {
			if term.Valid() {
				seen[term] = struct{}{}
			}
		}
	}
	out := make(formalIdentitySupport, 0, len(seen))
	for term := range seen {
		out = append(out, term)
	}
	sort.Slice(out, func(i, j int) bool { return formalIdentityTermLess(out[i], out[j]) })
	return out
}

func formalIdentitySupportEqual(left, right formalIdentitySupport) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneFormalIdentityEnvironment(input formalIdentityEnvironment) formalIdentityEnvironment {
	var out formalIdentityEnvironment
	if len(input.values) != 0 {
		out.values = make(map[statekey.Value]formalIdentitySupport, len(input.values))
	}
	for slot, support := range input.values {
		out.values[slot] = append(formalIdentitySupport(nil), support...)
	}
	if len(input.producers) != 0 {
		out.producers = make(map[state.DynamicReadIdentityProducer]formalIdentitySupport, len(input.producers))
	}
	for producer, support := range input.producers {
		out.producers[producer] = append(formalIdentitySupport(nil), support...)
	}
	return out
}

func (c *formalCoordinateDependencyClosure) unionFormalIdentityEnvironments(bodyIndex int, inputs ...formalIdentityEnvironment) (formalIdentityEnvironment, error) {
	var out formalIdentityEnvironment
	for _, input := range inputs {
		for slot, support := range input.values {
			joined := unionFormalIdentitySupport(out.values[slot], support)
			if len(joined) == 0 {
				continue
			}
			if out.values == nil {
				out.values = make(map[statekey.Value]formalIdentitySupport)
			}
			out.values[slot] = joined
		}
		for producer, support := range input.producers {
			joined := unionFormalIdentitySupport(out.producers[producer], support)
			if len(joined) == 0 {
				continue
			}
			if out.producers == nil {
				out.producers = make(map[state.DynamicReadIdentityProducer]formalIdentitySupport)
			}
			out.producers[producer] = joined
		}
	}
	return out, nil
}

func (c *formalCoordinateDependencyClosure) formalIdentityEnvironmentEqual(left, right formalIdentityEnvironment) bool {
	if len(left.values) != len(right.values) || len(left.producers) != len(right.producers) {
		return false
	}
	for slot, support := range left.values {
		if !formalIdentitySupportEqual(support, right.values[slot]) {
			return false
		}
	}
	for producer, support := range left.producers {
		rightSupport, present := right.producers[producer]
		if !present || !formalIdentitySupportEqual(support, rightSupport) {
			return false
		}
	}
	return true
}

func (c *formalCoordinateDependencyClosure) extendIdentityProducerCatalog(bodyIndex int, environment formalIdentityEnvironment) (bool, error) {
	if bodyIndex < 0 || bodyIndex >= len(c.program.bodies) || bodyIndex >= len(c.identityProducerCatalogs) {
		return false, fmt.Errorf("transformer: identity producer catalog has a foreign body")
	}
	catalog := c.identityProducerCatalogs[bodyIndex]
	changed := false
	for producer := range environment.producers {
		if _, present := catalog[producer]; present {
			continue
		}
		catalog[producer] = struct{}{}
		changed = true
	}
	if !changed {
		return false, nil
	}
	atoms := make([]state.DynamicReadIdentityProducer, 0, len(catalog))
	for producer := range catalog {
		atoms = append(atoms, producer)
	}
	index, err := c.program.bodies[bodyIndex].productDomain.SealDynamicReadIdentityProducerIndex(c.keys[bodyIndex], atoms)
	if err != nil {
		return false, err
	}
	c.identityProducerIndexes[bodyIndex] = index
	return true, nil
}

func (c *formalCoordinateDependencyClosure) identityCellInput(index int) (formalIdentityEnvironment, error) {
	cell := c.cells[index]
	if c.region == nil || index < 0 || index >= len(c.cellIdentityFolds) {
		return formalIdentityEnvironment{}, nil
	}
	fold := &c.cellIdentityFolds[index]
	if cell == c.region.roots[cell.Variable-1] {
		seed, err := c.identityBodySeed(int(cell.Variable - 1))
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
		if _, err := fold.set(0, seed); err != nil {
			return formalIdentityEnvironment{}, err
		}
	}
	return fold.root(), nil
}

func (c *formalCoordinateDependencyClosure) identityBodySeed(bodyIndex int) (formalIdentityEnvironment, error) {
	body := &c.program.bodies[bodyIndex]
	arena := body.relation.arena
	var out formalIdentityEnvironment
	for _, entry := range arena.middle.entries {
		register, ok := arena.middle.register(entry.middle)
		if !ok || register.slot == 0 {
			continue
		}
		ordinal, vocabulary, exact := formalRootOrdinal(slotSpaceBody{id: body.body, shape: body.relation.Shape(), middle: arena.middle.count()}, entry.input)
		if !exact || vocabulary != formal.Input {
			continue
		}
		term := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(body.body, ordinal, vocabulary)))
		if !term.Valid() {
			continue
		}
		if out.values == nil {
			out.values = make(map[statekey.Value]formalIdentitySupport)
		}
		out.values[register.slot] = formalIdentitySupport{term}
	}
	inputs := []formalIdentityEnvironment{out}
	for frameIndex := range c.frames {
		if c.frames[frameIndex].target == bodyIndex {
			inputs = append(inputs, c.frames[frameIndex].inputIdentity)
		}
	}
	return c.unionFormalIdentityEnvironments(bodyIndex, inputs...)
}

func (c *formalCoordinateDependencyClosure) transferIdentityCell(index int, input formalIdentityEnvironment) (formalIdentityEnvironment, error) {
	cell := c.cells[index]
	out := cloneFormalIdentityEnvironment(input)
	if cell.Kind != formalRelationCellStep {
		return out, nil
	}
	body := &c.program.bodies[cell.Variable-1]
	step := body.relation.code.nodes[cell.Root].steps[cell.Step-1]
	write := func(slot statekey.Value, support formalIdentitySupport) {
		if slot == 0 {
			return
		}
		if len(support) == 0 {
			delete(out.values, slot)
			return
		}
		if out.values == nil {
			out.values = make(map[statekey.Value]formalIdentitySupport)
		}
		out.values[slot] = append(formalIdentitySupport(nil), support...)
	}
	for _, publication := range c.cellIdentityWrites[index] {
		support, err := c.identityValuesSupport(int(cell.Variable-1), input, publication.sources)
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
		if len(support) == 0 {
			delete(out.producers, publication.producer)
			continue
		}
		if out.producers == nil {
			out.producers = make(map[state.DynamicReadIdentityProducer]formalIdentitySupport)
		}
		out.producers[publication.producer] = append(formalIdentitySupport(nil), support...)
		if publication.ownerSource != 0 {
			owners, ownerErr := c.identityValueSupport(int(cell.Variable-1), input, publication.ownerSource, make(map[ValueTerm]bool))
			if ownerErr != nil {
				return formalIdentityEnvironment{}, ownerErr
			}
			ownerAtoms, ownerErr := body.productDomain.DynamicReadIdentityProducerOwnerDeclarations(publication.producer, owners)
			if ownerErr != nil {
				return formalIdentityEnvironment{}, ownerErr
			}
			for _, ownerAtom := range ownerAtoms {
				out.producers[ownerAtom] = unionFormalIdentitySupport(out.producers[ownerAtom], support)
			}
		}
	}
	if _, err := c.extendIdentityProducerCatalog(int(cell.Variable-1), out); err != nil {
		return formalIdentityEnvironment{}, err
	}
	switch step.kind {
	case boundaryStepEnvironmentWrite:
		support, err := c.identityValueSupport(int(cell.Variable-1), input, step.value, make(map[ValueTerm]bool))
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
		write(step.slot, support)
	case boundaryStepRootAssignment:
		support, err := c.identityValuesSupport(int(cell.Variable-1), input, step.rootAssignment.sources)
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
		write(statekey.SymbolValue(step.rootAssignment.transaction.TargetSymbol()), support)
	case boundaryStepGenericFor:
		publication := step.genericIdentity
		if !publication.sealed || publication.projectionIdentity != genericForProjectionIdentityNoFinite {
			return formalIdentityEnvironment{}, fmt.Errorf("transformer: GenericFor identity publication is unsealed")
		}
		support, err := c.identityValuesSupport(int(cell.Variable-1), input, publication.finiteSources)
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
		write(publication.target, support)
	case boundaryStepApply:
		frameIndex, ok := c.frameByOwnerTerm[formalFrameFootprintKey{variable: cell.Variable, frame: step.apply.frame}]
		if !ok {
			// A frame with no normal coordinate footprint (notably a proven
			// nonreturning call) has no result producer in this relation.
			return out, nil
		}
		frame := &c.frames[frameIndex]
		for resultIndex, support := range frame.resultSupport {
			if resultIndex >= len(frame.frame.resultSelectors) {
				continue
			}
			for _, target := range frame.frame.resultSelectors[resultIndex].targets {
				if target.stateTarget && target.slot != 0 {
					write(target.slot, support)
				}
			}
		}
		var err error
		out, err = c.unionFormalIdentityEnvironments(int(cell.Variable-1), out, frame.outputIdentity)
		if err != nil {
			return formalIdentityEnvironment{}, err
		}
	}
	return out, nil
}

func (c *formalCoordinateDependencyClosure) identityValuesSupport(bodyIndex int, environment formalIdentityEnvironment, values []ValueTerm) (formalIdentitySupport, error) {
	inputs := make([]formalIdentitySupport, 0, len(values))
	for _, value := range values {
		support, err := c.identityValueSupport(bodyIndex, environment, value, make(map[ValueTerm]bool))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, support)
	}
	return unionFormalIdentitySupport(inputs...), nil
}

func (c *formalCoordinateDependencyClosure) identityRootSupport(bodyIndex int, environment formalIdentityEnvironment, root Root) (formalIdentitySupport, error) {
	if bodyIndex < 0 || bodyIndex >= len(c.program.bodies) {
		return nil, fmt.Errorf("transformer: identity root support has a foreign body")
	}
	body := &c.program.bodies[bodyIndex]
	if root.Kind == RootMiddle {
		register, ok := body.relation.arena.middle.register(root)
		if !ok {
			return nil, fmt.Errorf("transformer: identity root support Middle is outside its schema")
		}
		return append(formalIdentitySupport(nil), environment.values[register.slot]...), nil
	}
	ordinal, vocabulary, exact := formalRootOrdinal(slotSpaceBody{id: body.body, shape: body.relation.Shape(), middle: body.relation.arena.middle.count()}, root)
	if !exact || vocabulary != formal.Input {
		return nil, nil
	}
	term := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(body.body, ordinal, vocabulary)))
	if !term.Valid() {
		return nil, nil
	}
	return formalIdentitySupport{term}, nil
}

func (c *formalCoordinateDependencyClosure) identityQueryValue(bodyIndex int, environment formalIdentityEnvironment, term ValueTerm) (product.Value, error) {
	if bodyIndex < 0 || bodyIndex >= len(c.program.bodies) {
		return product.Value{}, fmt.Errorf("transformer: identity query has a foreign body")
	}
	body := &c.program.bodies[bodyIndex]
	arena := body.relation.arena
	if term == 0 || int(term) >= len(arena.values) {
		return product.Value{}, fmt.Errorf("transformer: identity query has a foreign value term")
	}
	node := arena.values[term]
	switch node.op {
	case valueConstant, valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement:
		if product.BelongsToRegistry(body.productDomain.Registry(), node.value) {
			return node.value, nil
		}
	case valueAllocationResult:
		if value, exact := arena.allocationResult(node.allocation, node.resultIndex); exact {
			return value, nil
		}
	}
	support, err := c.identityValueSupport(bodyIndex, environment, term, make(map[ValueTerm]bool))
	if err != nil {
		return product.Value{}, err
	}
	value := product.Top()
	if len(support) == 1 {
		value = product.Set(body.productDomain.Registry(), value, identity.Key, identity.SingletonTerm(support[0]))
	}
	return value, nil
}

func (c *formalCoordinateDependencyClosure) identityValueSupport(bodyIndex int, environment formalIdentityEnvironment, term ValueTerm, visiting map[ValueTerm]bool) (formalIdentitySupport, error) {
	if bodyIndex < 0 || bodyIndex >= len(c.program.bodies) {
		return nil, fmt.Errorf("transformer: identity support has a foreign body")
	}
	body := &c.program.bodies[bodyIndex]
	arena := body.relation.arena
	if term == 0 || int(term) >= len(arena.values) {
		return nil, fmt.Errorf("transformer: identity support contains a foreign value term")
	}
	if visiting[term] {
		return nil, nil
	}
	visiting[term] = true
	defer delete(visiting, term)
	node := arena.values[term]
	exactValue := func(value product.Value) formalIdentitySupport {
		identityTerm, exact := product.Get(arena.reg, value, identity.Key).Term()
		if exact && identityTerm.Valid() {
			return formalIdentitySupport{identityTerm}
		}
		return nil
	}
	switch node.op {
	case valueRoot:
		if node.root.Kind == RootMiddle {
			register, ok := arena.middle.register(node.root)
			if !ok {
				return nil, fmt.Errorf("transformer: identity support Middle root is outside its schema")
			}
			return append(formalIdentitySupport(nil), environment.values[register.slot]...), nil
		}
		ordinal, vocabulary, exact := formalRootOrdinal(slotSpaceBody{id: body.body, shape: body.relation.Shape(), middle: arena.middle.count()}, node.root)
		if !exact || vocabulary != formal.Input {
			return nil, fmt.Errorf("transformer: identity support root has no finite producer")
		}
		formalTerm := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(body.body, ordinal, vocabulary)))
		if formalTerm.Valid() {
			return formalIdentitySupport{formalTerm}, nil
		}
		return nil, nil
	case valueEnvironment:
		return append(formalIdentitySupport(nil), environment.values[node.slot]...), nil
	case valueConstant:
		return exactValue(node.value), nil
	case valueObjectLiteral:
		if id, exact := node.objectPlan.Identity(); exact {
			return formalIdentitySupport{identity.ConcreteTerm(id)}, nil
		}
		return nil, nil
	case valueJoin, valueSelect:
		return c.identityValuesSupport(bodyIndex, environment, node.args)
	case valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement:
		if support := exactValue(node.value); len(support) != 0 {
			return support, nil
		}
		if len(node.args) != 1 {
			return nil, fmt.Errorf("transformer: identity support refinement is malformed")
		}
		return c.identityValueSupport(bodyIndex, environment, node.args[0], visiting)
	case valueCallResult, valuePredicateObservation:
		if len(node.args) != 1 {
			return nil, fmt.Errorf("transformer: identity support forwarding node is malformed")
		}
		return c.identityValueSupport(bodyIndex, environment, node.args[0], visiting)
	case valueAllocationResult:
		value, exact := arena.allocationResult(node.allocation, node.resultIndex)
		if !exact {
			return nil, fmt.Errorf("transformer: identity support allocation result is unresolved")
		}
		return exactValue(value), nil
	case valueFrameResult:
		frameIndex, ok := c.frameByOwnerTerm[formalFrameFootprintKey{variable: body.variable, frame: node.frame}]
		if !ok || node.resultIndex < 0 || node.resultIndex >= len(c.frames[frameIndex].resultSupport) {
			return nil, fmt.Errorf("transformer: identity support frame result is unresolved")
		}
		return append(formalIdentitySupport(nil), c.frames[frameIndex].resultSupport[node.resultIndex]...), nil
	case valueStringConcat, valueLoopContinuation, valueLuaTypeName:
		return nil, nil
	case valueUnaryOperation:
		// Lua unary operators produce booleans/numbers/strings, never preserve
		// the operand's object identity.
		return nil, nil
	case valueBinaryOperation:
		if node.operator == "and" || node.operator == "or" {
			return c.identityValuesSupport(bodyIndex, environment, node.args)
		}
		return nil, nil
	case valueDynamicRead, valueDynamicTableRead:
		if len(node.args) != 2 {
			return nil, fmt.Errorf("transformer: dynamic-read identity query is malformed")
		}
		args := make([]product.Value, 0, len(node.args)+1)
		for _, arg := range node.args {
			value, err := c.identityQueryValue(bodyIndex, environment, arg)
			if err != nil {
				return nil, err
			}
			args = append(args, value)
		}
		if node.integerProof != 0 {
			value, err := c.identityQueryValue(bodyIndex, environment, node.integerProof)
			if err != nil {
				return nil, err
			}
			args = append(args, value)
		}
		span := formalFiberDescriptorSpan{variable: body.variable, keys: c.keys[bodyIndex], rekey: c.rekeys[bodyIndex]}
		query, err := formalDynamicValueQuery(body, span, node, args)
		if err != nil {
			return nil, err
		}
		selection, err := body.productDomain.PrepareDynamicReadSelection(query)
		if err != nil {
			return nil, err
		}
		tableSupport, err := c.identityValueSupport(bodyIndex, environment, node.args[0], visiting)
		if err != nil {
			return nil, err
		}
		producers, err := body.productDomain.PlanDynamicReadIdentityTopologyProducers(selection, tableSupport, c.bodies[bodyIndex],
			&c.identityProducerIndexes[bodyIndex])
		if err != nil {
			return nil, err
		}
		inputs := make([]formalIdentitySupport, 0, len(producers))
		for _, producer := range producers {
			inputs = append(inputs, environment.producers[producer])
		}
		return unionFormalIdentitySupport(inputs...), nil
	case valueCellResult, valueIteratorProjection, valueGenericForResult, valueStaticIndex:
		return nil, fmt.Errorf("transformer: identity support op %d requires a registered producer support contract: %s", node.op, arena.canonicalValue(term))
	case valueInvalid:
		return nil, fmt.Errorf("transformer: identity support contains an invalid value op")
	default:
		return nil, fmt.Errorf("transformer: identity support has unclassified value op %d", node.op)
	}
}

func imageFormalIdentitySupport(support formalIdentitySupport, image *state.CoordinateIdentityTermImage) formalIdentitySupport {
	var inputs []formalIdentitySupport
	for _, term := range support {
		terms, exact := image.Image(term)
		if exact {
			inputs = append(inputs, formalIdentitySupport(terms))
		}
	}
	return unionFormalIdentitySupport(inputs...)
}

func preimageFormalIdentitySupport(support formalIdentitySupport, image *state.CoordinateIdentityTermImage, target *relationProgramBody) (formalIdentitySupport, error) {
	if image == nil || target == nil {
		return nil, fmt.Errorf("transformer: identity support preimage is unowned")
	}
	bindings := image.Bindings()
	var inputs []formalIdentitySupport
	for _, term := range support {
		variable, isFormal := term.Formal()
		if !isFormal {
			inputs = append(inputs, formalIdentitySupport{term})
			continue
		}
		if variable.Root().Owner() == target.body {
			inputs = append(inputs, formalIdentitySupport{term})
			continue
		}
		var preimage formalIdentitySupport
		for _, binding := range bindings {
			for _, candidate := range binding.Images {
				if candidate == term {
					preimage = append(preimage, binding.Source)
					break
				}
			}
		}
		if len(preimage) == 0 {
			return nil, fmt.Errorf("transformer: foreign formal component owner")
		}
		inputs = append(inputs, preimage)
	}
	return unionFormalIdentitySupport(inputs...), nil
}

func (c *formalCoordinateDependencyClosure) transportFrameIdentityEnvironments(
	frameIndex int,
	callerInput formalIdentityEnvironment,
	image *state.CoordinateIdentityTermImage,
) (formalIdentityEnvironment, formalIdentityEnvironment, error) {
	frame := &c.frames[frameIndex]
	if len(frame.wirePlans) == 0 {
		return formalIdentityEnvironment{}, formalIdentityEnvironment{}, nil
	}
	targetSeed := formalIdentityEnvironment{producers: make(map[state.DynamicReadIdentityProducer]formalIdentitySupport)}
	for callerProducer, support := range callerInput.producers {
		pulled, err := c.program.bodies[frame.target].productDomain.PullbackDynamicReadIdentityProducersFormal(
			c.program.bodies[frame.caller].productDomain, frame.wirePlans, image, []state.DynamicReadIdentityProducer{callerProducer},
		)
		if err != nil {
			return formalIdentityEnvironment{}, formalIdentityEnvironment{}, err
		}
		if len(pulled) == 0 {
			continue
		}
		preimage, err := preimageFormalIdentitySupport(support, image, &c.program.bodies[frame.target])
		if err != nil {
			return formalIdentityEnvironment{}, formalIdentityEnvironment{}, err
		}
		for _, targetProducer := range pulled {
			targetSeed.producers[targetProducer] = unionFormalIdentitySupport(targetSeed.producers[targetProducer], preimage)
		}
	}
	var targetOutputs []formalIdentityEnvironment
	for _, outcomeCell := range c.region.outcomes[frame.target] {
		if cellIndex, present := c.cellIndex[outcomeCell]; present {
			targetOutputs = append(targetOutputs, c.cellIdentity[cellIndex])
		}
	}
	targetOutput, err := c.unionFormalIdentityEnvironments(frame.target, targetOutputs...)
	if err != nil {
		return formalIdentityEnvironment{}, formalIdentityEnvironment{}, err
	}
	callerOutput := formalIdentityEnvironment{producers: make(map[state.DynamicReadIdentityProducer]formalIdentitySupport)}
	for producer, support := range targetOutput.producers {
		mapped, mapErr := c.program.bodies[frame.target].productDomain.TransportDynamicReadIdentityProducersFormal(
			c.program.bodies[frame.caller].productDomain, frame.wirePlans, image, []state.DynamicReadIdentityProducer{producer},
		)
		if mapErr != nil {
			return formalIdentityEnvironment{}, formalIdentityEnvironment{}, mapErr
		}
		imaged := imageFormalIdentitySupport(support, image)
		for _, callerProducer := range mapped {
			callerOutput.producers[callerProducer] = unionFormalIdentitySupport(callerOutput.producers[callerProducer], imaged)
		}
	}
	return targetSeed, callerOutput, nil
}

func (c *formalCoordinateDependencyClosure) evaluateFrameIdentity(index int, selector state.CoordinateFactorInventory) (*state.CoordinateIdentityTermImage, []formalIdentitySupport, bool, error) {
	frame := &c.frames[index]
	caller, target := &c.program.bodies[frame.caller], &c.program.bodies[frame.target]
	rawResults := make([]formalIdentitySupport, int(frame.frame.shape.Results))
	for _, outcomeCell := range c.region.outcomes[target.variable-1] {
		cellIndex, present := c.cellIndex[outcomeCell]
		if !present {
			return nil, nil, false, fmt.Errorf("transformer: formal Apply identity outcome is undeclared")
		}
		outcome := target.relation.code.outcomes[outcomeCell.Outcome]
		transaction := outcome.returnTransaction.transaction
		for bindingIndex := 0; bindingIndex < transaction.ResultBindingCount(); bindingIndex++ {
			sourceIndex, resultIndex, exact := transaction.ResultBinding(bindingIndex)
			if !exact || sourceIndex < 0 || sourceIndex >= len(outcome.returnTransaction.sources) || resultIndex < 0 || resultIndex >= len(rawResults) {
				return nil, nil, false, fmt.Errorf("transformer: formal Apply identity result binding is malformed: source=%d/%d result=%d/%d", sourceIndex, len(outcome.returnTransaction.sources), resultIndex, len(rawResults))
			}
			support, err := c.identityValueSupport(frame.target, c.cellIdentity[cellIndex], outcome.returnTransaction.sources[sourceIndex], make(map[ValueTerm]bool))
			if err != nil {
				return nil, nil, false, err
			}
			rawResults[resultIndex] = unionFormalIdentitySupport(rawResults[resultIndex], support)
		}
	}
	requiredTerms, err := target.productDomain.CoordinateFactorInventoryIdentityTerms(selector)
	if err != nil {
		return nil, nil, false, err
	}
	required := make(map[identity.Term]struct{}, len(requiredTerms))
	for _, term := range requiredTerms {
		if _, formalTerm := term.Formal(); formalTerm {
			required[term] = struct{}{}
		}
	}
	for _, support := range rawResults {
		for _, term := range support {
			if _, formalTerm := term.Formal(); formalTerm {
				required[term] = struct{}{}
			}
		}
	}
	inputs := make([]formalIdentityEnvironment, 0, len(frame.cells))
	for _, cellIndex := range frame.cells {
		if c.cells[cellIndex].Variable == caller.variable {
			inputs = append(inputs, c.cellInputIdentity[cellIndex])
		}
	}
	inputEnvironment, err := c.unionFormalIdentityEnvironments(frame.caller, inputs...)
	if err != nil {
		return nil, nil, false, err
	}
	// Producer pullback may require any input owner term even when no ordinary
	// coordinate/result currently demands it. The atom set is finite, so make
	// the boundary image complete whenever producer provenance crosses it.
	demandAllInputs := len(inputEnvironment.producers) != 0
	bindings := make([]state.CoordinateIdentityTermBinding, len(frame.frame.rootCircuit))
	for wireIndex, wire := range frame.frame.rootCircuit {
		slot, present := c.program.formalSlots.Slot(target.body, wire.root)
		if !present {
			return nil, nil, false, fmt.Errorf("transformer: formal Apply identity input %d has no target root", wireIndex)
		}
		root, present := slot.Root()
		if !present {
			return nil, nil, false, fmt.Errorf("transformer: formal Apply identity input %d is malformed", wireIndex)
		}
		source := identity.FormalTerm(identity.NewFormalVarRoot(root))
		var support formalIdentitySupport
		if _, demanded := required[source]; demandAllInputs || demanded {
			var err error
			support, err = c.identityValueSupport(frame.caller, inputEnvironment, wire.value, make(map[ValueTerm]bool))
			if err != nil {
				return nil, nil, false, fmt.Errorf("transformer: formal Apply identity input %d: %w", wireIndex, err)
			}
		}
		bindings[wireIndex] = state.CoordinateIdentityTermBinding{
			Source: source,
			Images: append([]identity.Term(nil), support...),
		}
	}
	image, valid := state.NewCoordinateIdentityTermImage(bindings)
	if !valid {
		return nil, nil, false, fmt.Errorf("transformer: formal Apply identity image is malformed")
	}
	results := make([]formalIdentitySupport, len(rawResults))
	for resultIndex, support := range rawResults {
		results[resultIndex] = imageFormalIdentitySupport(support, image)
	}
	inputIdentity, outputIdentity, err := c.transportFrameIdentityEnvironments(index, inputEnvironment, image)
	if err != nil {
		return nil, nil, false, err
	}
	inputCatalogChanged, err := c.extendIdentityProducerCatalog(frame.target, inputIdentity)
	if err != nil {
		return nil, nil, false, err
	}
	outputCatalogChanged, err := c.extendIdentityProducerCatalog(frame.caller, outputIdentity)
	if err != nil {
		return nil, nil, false, err
	}
	changed := frame.identityImage == nil || !frame.identityImage.Equal(image) ||
		!c.formalIdentityEnvironmentEqual(frame.inputIdentity, inputIdentity) ||
		!c.formalIdentityEnvironmentEqual(frame.outputIdentity, outputIdentity) ||
		inputCatalogChanged || outputCatalogChanged ||
		len(frame.resultSupport) != len(results)
	if !changed {
		for resultIndex := range results {
			if !formalIdentitySupportEqual(frame.resultSupport[resultIndex], results[resultIndex]) {
				changed = true
				break
			}
		}
	}
	frame.inputIdentity, frame.outputIdentity = inputIdentity, outputIdentity
	return image, results, changed, nil
}
