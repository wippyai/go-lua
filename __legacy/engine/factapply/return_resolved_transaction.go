package factapply

import (
	"context"
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourceprojection"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type returnSlotPlan struct {
	source  int
	target  int
	project bool
}

// ReturnTransaction is one callback-free N5 descriptor. Sources are the
// complete recursive object-literal/source inventory required by the sole
// concrete return kernel, in deterministic discovery order. The source index
// is frozen once and gives execution-time providers O(1) lookup.
type ReturnTransaction struct {
	point             cfg.Point
	slots             []returnSlotPlan
	projectionTargets []int
	resultTargets     []int
	sources           []factflow.ValueSource
	index             map[factflow.ValueSource]int
	sealed            bool
}

// PlanReturnTransaction freezes the return at point and recursively inventories
// every object-literal entry and list-element source reachable from it. Object
// graphs are traversed as finite descriptor graphs: revisiting an expression
// identity closes the graph without imposing an artificial depth limit.
func PlanReturnTransaction(facts factflow.Facts, point cfg.Point) (ReturnTransaction, bool) {
	fact, ok := facts.Return(point)
	if !ok {
		return ReturnTransaction{}, false
	}
	return planReturnTransaction(facts, point, fact.Sources())
}

// PlanReturnTransactionSources freezes an already value-list-resolved return.
// Call-surface lowering uses it only to remove sources proven to contribute
// zero result slots; it does not synthesize nil/Top slots or bypass N5.
func PlanReturnTransactionSources(facts factflow.Facts, point cfg.Point, sources []factflow.ValueSource) (ReturnTransaction, bool) {
	return planReturnTransaction(facts, point, sources)
}

func planReturnTransaction(facts factflow.Facts, point cfg.Point, roots []factflow.ValueSource) (ReturnTransaction, bool) {
	if point <= 0 {
		return ReturnTransaction{}, false
	}
	transaction := ReturnTransaction{
		point:   point,
		slots:   make([]returnSlotPlan, 0, len(roots)),
		sources: make([]factflow.ValueSource, 0, len(roots)),
		index:   make(map[factflow.ValueSource]int, len(roots)),
	}
	validObjects := true
	activeObjects := make(map[factflow.ExprRef]bool)
	doneObjects := make(map[factflow.ExprRef]bool)
	var addSource func(factflow.ValueSource) int
	addSource = func(source factflow.ValueSource) int {
		index, exists := transaction.index[source]
		if !exists {
			index = len(transaction.sources)
			transaction.index[source] = index
			transaction.sources = append(transaction.sources, source)
		}
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return index
		}
		literal, object := facts.ObjectLiteralView(source.ExprRef)
		if !object {
			return index
		}
		if _, identified := literal.Identity(); !identified || activeObjects[source.ExprRef] {
			validObjects = false
			return index
		}
		if doneObjects[source.ExprRef] {
			return index
		}
		activeObjects[source.ExprRef] = true
		defer delete(activeObjects, source.ExprRef)
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			addSource(entry.Source())
			return validObjects
		})
		if validObjects {
			if list, present := literal.ListElementSource(); present {
				addSource(list)
			}
		}
		if validObjects {
			doneObjects[source.ExprRef] = true
		}
		return index
	}

	declaredTargets := make(map[int]struct{})
	for ordinal, source := range roots {
		target := source.TargetIndex
		if target < 0 {
			target = ordinal
		}
		transaction.slots = append(transaction.slots, returnSlotPlan{source: addSource(source), target: target})
		if returnSourceHasDeclaredContract(facts, source) {
			declaredTargets[target] = struct{}{}
		}
	}
	if !validObjects {
		return ReturnTransaction{}, false
	}
	seenProjection := make(map[int]struct{}, len(transaction.slots))
	for _, slot := range transaction.slots {
		if _, declared := declaredTargets[slot.target]; declared {
			continue
		}
		if _, seen := seenProjection[slot.target]; seen {
			continue
		}
		seenProjection[slot.target] = struct{}{}
		transaction.projectionTargets = append(transaction.projectionTargets, slot.target)
	}
	for index := range transaction.slots {
		_, declared := declaredTargets[transaction.slots[index].target]
		transaction.slots[index].project = !declared
	}
	resultSet := make(map[int]struct{}, len(transaction.slots))
	for _, slot := range transaction.slots {
		resultSet[slot.target] = struct{}{}
	}
	transaction.resultTargets = make([]int, 0, len(resultSet))
	for target := range resultSet {
		transaction.resultTargets = append(transaction.resultTargets, target)
	}
	sort.Ints(transaction.resultTargets)
	transaction.sealed = true
	return transaction, true
}

func (t ReturnTransaction) Valid() bool {
	// All representation fields are private and are validated exactly once by
	// PlanReturnTransaction. Keeping the seal makes every execution-boundary
	// validity check O(1); the hot path never rescans the frozen source index.
	return t.sealed && t.point > 0 && len(t.index) == len(t.sources)
}

func (t ReturnTransaction) Clone() ReturnTransaction {
	t.slots = append([]returnSlotPlan(nil), t.slots...)
	t.projectionTargets = append([]int(nil), t.projectionTargets...)
	t.resultTargets = append([]int(nil), t.resultTargets...)
	t.sources = append([]factflow.ValueSource(nil), t.sources...)
	t.index = make(map[factflow.ValueSource]int, len(t.sources))
	for index, source := range t.sources {
		t.index[source] = index
	}
	return t
}

func (t ReturnTransaction) Point() cfg.Point { return t.point }

func (t ReturnTransaction) SourceCount() int { return len(t.sources) }

func (t ReturnTransaction) ResultTargetCount() int { return len(t.resultTargets) }

func (t ReturnTransaction) ResultTarget(index int) (int, bool) {
	if index < 0 || index >= len(t.resultTargets) {
		return 0, false
	}
	return t.resultTargets[index], true
}

// ResultBindingCount reports the number of direct return-source to result-slot
// bindings. Recursively inventoried object members remain sources, but are not
// result bindings of their own.
func (t ReturnTransaction) ResultBindingCount() int { return len(t.slots) }

// ResultBinding returns one frozen direct source/result association. The
// source index addresses Source and the transformer term tuple in the same
// transaction.
func (t ReturnTransaction) ResultBinding(index int) (source, target int, ok bool) {
	if index < 0 || index >= len(t.slots) {
		return 0, 0, false
	}
	slot := t.slots[index]
	return slot.source, slot.target, true
}

// ResultBindingProjectsHeap reports whether one direct result binding may
// refine its type witness from the already-materialized heap container. The
// flag is frozen with the binding so coordinate-native and concrete N5 tails
// consume the same declared-contract decision.
func (t ReturnTransaction) ResultBindingProjectsHeap(index int) (bool, bool) {
	if index < 0 || index >= len(t.slots) {
		return false, false
	}
	return t.slots[index].project, true
}

func (t ReturnTransaction) Source(index int) (factflow.ValueSource, bool) {
	if index < 0 || index >= len(t.sources) {
		return factflow.ValueSource{}, false
	}
	return t.sources[index], true
}

type ResolvedReturnTransaction struct {
	plan   ReturnTransaction
	values []product.Value
}

// Bind borrows values for one synchronous ReturnAuthority.Apply call. Neither
// the resolved transaction nor its provider may be retained or published.
func (t ReturnTransaction) Bind(reg *axis.Registry, values []product.Value) (ResolvedReturnTransaction, bool) {
	if reg == nil || !t.Valid() || len(values) != len(t.sources) {
		return ResolvedReturnTransaction{}, false
	}
	for _, value := range values {
		if !product.BelongsToRegistry(reg, value) {
			return ResolvedReturnTransaction{}, false
		}
	}
	return ResolvedReturnTransaction{plan: t, values: values}, true
}

// ReturnAuthority is the body-scoped semantic authority for N5. Facts, path
// identity, and type projection are retained once per prepared body, never per
// return or per application.
type ReturnAuthority struct {
	paths *PathSemanticAuthority
	facts factflow.Facts
}

func NewReturnAuthority(paths *PathSemanticAuthority, facts factflow.Facts) *ReturnAuthority {
	if paths == nil || !paths.Valid() {
		return nil
	}
	return &ReturnAuthority{paths: paths, facts: facts}
}

func (a *ReturnAuthority) Valid() bool { return a != nil && a.paths != nil && a.paths.Valid() }

// ProjectFactoredHeapContainer applies the canonical N5 dynamic-container
// witness law from the exact factored observations it consumes. Static heap
// members are intentionally absent: they cannot affect this projection.
func (a *ReturnAuthority) ProjectFactoredHeapContainer(
	reg *axis.Registry,
	value product.Value,
	root product.Value,
	visitFacts func(func(dynamicindex.Fact)),
) (product.Value, bool) {
	if reg == nil || !a.Valid() {
		return product.Value{}, false
	}
	projected, ok := sourceprojection.HeapObjectContainerTypeFromFactors(reg, a.paths.typeValues, value, root, visitFacts)
	if !ok {
		return value, false
	}
	return typevalue.WithWitness(reg, value, projected), true
}

// Apply executes one resolved N5 transaction atomically. Cancellation or an
// incomplete frozen source inventory returns the exact pre-N5 output, so no
// heap, placement, projection, or return-slot prefix is published.
func (a *ReturnAuthority) Apply(ctx context.Context, reg *axis.Registry, transaction ResolvedReturnTransaction, pointInput, output state.State) (state.State, error) {
	return a.apply(ctx, reg, transaction, pointInput, output, true)
}

// ApplyPostMaterialized executes the canonical N5 tail after the transaction's
// object-literal graph has already been installed by the ordered N4 object
// materialization effect. It shares the same return-source resolution,
// placement, projection, and result publication kernel as Apply; only the
// already-completed constructor write is omitted.
func (a *ReturnAuthority) ApplyPostMaterialized(ctx context.Context, reg *axis.Registry, transaction ResolvedReturnTransaction, pointInput, output state.State) (state.State, error) {
	return a.apply(ctx, reg, transaction, pointInput, output, false)
}

func (a *ReturnAuthority) apply(ctx context.Context, reg *axis.Registry, transaction ResolvedReturnTransaction, pointInput, output state.State, materializeObjects bool) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || !transaction.plan.Valid() || len(transaction.values) != len(transaction.plan.sources) {
		return output, fmt.Errorf("factapply: invalid resolved return transaction")
	}
	if err := ctx.Err(); err != nil {
		return output, err
	}
	provider := resolvedReturnSources{index: transaction.plan.index, values: transaction.values}
	foreignRead := false
	result := applyConcreteReturnCore(ConcreteReturnCoreRequest{
		Context: transfer.NodeContext{
			Context: ctx, Session: cancellation.FromContext(ctx), Registry: reg, Point: transaction.plan.point,
		},
		Facts: a.facts, Sources: &provider,
		Read: func(point cfg.Point) state.State {
			if point != transaction.plan.point {
				foreignRead = true
				return state.State{}
			}
			return pointInput
		},
		Input: pointInput, Output: output, Transaction: transaction.plan,
		Resolver: a.paths.resolver, ProjectPath: a.paths.projectPath, TypeValues: a.paths.typeValues, Authority: a,
	}, materializeObjects)
	if err := ctx.Err(); err != nil {
		return output, err
	}
	if result.Err != nil {
		return output, result.Err
	}
	if foreignRead || provider.missing || !result.Applied {
		return output, fmt.Errorf("factapply: resolved return transaction required an unfrozen source")
	}
	return result.Output, nil
}

type resolvedReturnSources struct {
	index   map[factflow.ValueSource]int
	values  []product.Value
	missing bool
}

var _ sourcevalue.SourceValues = (*resolvedReturnSources)(nil)

func (s *resolvedReturnSources) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	if s == nil {
		return product.Value{}, false
	}
	index, ok := s.index[source]
	if !ok || index < 0 || index >= len(s.values) {
		s.missing = true
		return product.Value{}, false
	}
	return s.values[index], true
}

// ConcreteReturnCoreRequest is the complete input to the sole N5 semantic
// kernel. Input is the immutable point-entry source snapshot; Output contains
// the completed N0..N4 state on which the return transaction publishes.
type ConcreteReturnCoreRequest struct {
	Context     transfer.NodeContext
	Facts       factflow.Facts
	Sources     sourcevalue.SourceValues
	Read        func(cfg.Point) state.State
	Input       state.State
	Output      state.State
	Transaction ReturnTransaction
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
	Authority   *ReturnAuthority
}

// ConcreteReturnCoreResult reports whether a valid return descriptor was
// executed. A zero-source return is valid and Applied=true with unchanged State.
type ConcreteReturnCoreResult struct {
	Output  state.State
	Applied bool
	Err     error
}

// ApplyConcreteReturnCore performs object heap materialization, return escape,
// heap-container projection, and return-slot publication exactly once.
func ApplyConcreteReturnCore(req ConcreteReturnCoreRequest) ConcreteReturnCoreResult {
	return applyConcreteReturnCore(req, true)
}

func applyConcreteReturnCore(req ConcreteReturnCoreRequest, materializeObjects bool) ConcreteReturnCoreResult {
	transaction := req.Transaction
	if req.Context.Registry == nil || req.Context.Point != transaction.point || !transaction.Valid() {
		return ConcreteReturnCoreResult{Output: req.Output}
	}
	out := req.Output
	resolvedSources := make([]product.Value, len(transaction.sources))
	sourceResolved := make([]bool, len(transaction.sources))
	for index, source := range transaction.sources {
		resolvedSources[index], sourceResolved[index] = returnSourceValue(
			req.Context, req.Facts, req.Sources, req.Read, req.Input, out,
			source, req.Resolver, req.ProjectPath, req.TypeValues,
		)
		if !sourceResolved[index] {
			resolvedSources[index] = product.Top()
		}
	}
	values := make([]product.Value, len(transaction.slots))
	resolved := make([]bool, len(transaction.slots))
	roots := make([]factflow.ValueSource, len(transaction.slots))
	for index, slot := range transaction.slots {
		source := transaction.sources[slot.source]
		roots[index] = source
		values[index], resolved[index] = resolvedSources[slot.source], sourceResolved[slot.source]
	}
	if materializeObjects && len(transaction.slots) != 0 {
		cache := newObjectLiteralSourceCache(req.Context.Point, req.Sources, req.Read, req.Input, out)
		for index, source := range roots {
			if resolved[index] {
				cache.seed(source, values[index])
			}
		}
		out = materializeObjectLiteralHeapBatchWithCache(
			req.Context, req.Resolver, req.Facts.ObjectLiteralView,
			out, roots, req.TypeValues, cache,
		)
	}
	if req.Authority == nil || !req.Authority.Valid() {
		return ConcreteReturnCoreResult{Output: req.Output, Err: fmt.Errorf("factapply: concrete N5 has no factor authority")}
	}
	domain := state.RegisteredProductDomain(req.Context.Registry)
	topology, err := domain.SealReturnFactorTopology()
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	lanes := topology.Lanes()
	factors, err := domain.DecomposeLanes(out, lanes)
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	returnLanes, err := BindReturnFactorLanes(domain, req.Authority.paths.resolver.KeySpace(), topology, factors)
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	_, valueFactor := state.DecomposeValueLane(state.Domain(req.Context.Registry), out)
	targets := make([]ReturnFactorTarget[statekey.Value], transaction.ResultTargetCount())
	for index := range targets {
		target, _ := transaction.ResultTarget(index)
		targets[index] = ReturnFactorTarget[statekey.Value]{
			Index: target, Slot: statekey.ReturnSlot(target),
			Path: req.Authority.paths.resolver.KeySpace().FromPath(pathdom.Path{Root: fmt.Sprintf("ret[%d]", target)}),
		}
	}
	factored, err := ApplyReturnFactorTransaction(req.Context.Context, req.Authority, ReturnFactorTransaction[statekey.Value]{
		Return: transaction, Sources: resolvedSources, Targets: targets, Values: valueFactor, Lanes: returnLanes,
		Domain: domain, Keys: req.Authority.paths.resolver.KeySpace(), Topology: topology,
	})
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	factors, err = ComposeReturnFactorLanes(domain, req.Authority.paths.resolver.KeySpace(), topology, factored.Lanes)
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	allLanes := domain.NonValuesLaneInventory()
	allFactors, err := domain.DecomposeLanes(out, allLanes)
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	for index, factor := range factors {
		productIndex, ok := topology.ProductIndex(index)
		if !ok || productIndex < 0 || productIndex >= len(allFactors) {
			return ConcreteReturnCoreResult{Output: req.Output, Err: fmt.Errorf("factapply: N5 factor topology is malformed")}
		}
		allFactors[productIndex] = factor
	}
	residual, err := domain.ComposeSparse(allFactors)
	if err != nil {
		return ConcreteReturnCoreResult{Output: req.Output, Err: err}
	}
	out = state.RecomposeValueLane(req.Context.Registry, state.Domain(req.Context.Registry), residual, factored.Values)
	return ConcreteReturnCoreResult{Output: out, Applied: true}
}
