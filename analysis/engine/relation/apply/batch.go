package apply

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Results is the immutable semantic side of one Apply node. It carries the
// exact sealed operation even when its extent is empty; it is not a relation
// and does not carry a second row representation. Each element is the closed
// outcome and proposal lease returned by one semantic worker invocation.
// Publish consumes the elements individually.
type Results struct {
	// operation is the exact schema-sealed operation redeemed by Execute.
	// It remains present for an empty extent, where no Application cell can
	// carry the operation identity to a later observation boundary.
	operation signature.Identity
	values    []Application
	// scopes is the complete authenticated publication cofiber set for this
	// extent. Non-empty application extents derive it from Application's
	// immutable invocation addresses; an empty extent may retain scopes from
	// the input batches so a closed denominator can still publish proven
	// absence without guessing a mounted scope.
	scopes []binding.ScopeToken
	sealed bool
}

func (results Results) Available() bool {
	if !results.sealed || !results.operation.Available() || results.values == nil || results.scopes == nil {
		return false
	}
	for _, scope := range results.scopes {
		if !scope.Available() {
			return false
		}
	}
	for _, value := range results.values {
		if !value.Available() || value.Operation() != results.operation {
			return false
		}
	}
	return true
}

// Operation returns the exact schema-sealed operation that produced this
// extent. Unlike Application.Operation, it remains available when the
// extent has zero applications (for example a closed NoSelection).
func (results Results) Operation() signature.Identity {
	if !results.Available() {
		return signature.Identity{}
	}
	return results.operation
}

func (results Results) Len() int {
	if !results.Available() {
		return 0
	}
	return len(results.values)
}

func (results Results) At(index int) (Application, bool) {
	if !results.Available() || index < 0 || index >= len(results.values) {
		return Application{}, false
	}
	return results.values[index], true
}

// Scopes returns the complete authenticated cofiber set represented by this
// sealed result extent. It is publication metadata, not an application or
// query identity; Application remains the authority for non-empty invocation
// addresses.
func (results Results) Scopes() []binding.ScopeToken {
	if !results.Available() {
		return nil
	}
	values := make([]binding.ScopeToken, len(results.scopes))
	copy(values, results.scopes)
	return values
}

// Execute redeems one exact sealed Apply binding over one ordered batch vector
// for every authored child expression. Children are never flattened: scalar
// alternatives are individual tuples, span alternatives are authenticated
// ranges, and Apply evaluates the ordered Cartesian product of DISTINCT child
// groups. Several semantic slots may name one child group; that group is
// selected once and every slot reads the same tuple/range. A combination is
// invoked only in the exact non-empty cofiber produced by Geometry; the
// operation's semantic worker sees one frame per compatible product member.
//
// seed is an optional already-authenticated scope for the invocation (zero
// means ordinary unseeded evaluation); correlated replay supplies the exact
// population Q scope so every resulting frame is restricted by that support.
// This is deliberately the only Apply physical shape. A single-child Apply
// takes the same path as a many-child Apply rather than preserving a parallel
// unary fast path. Empty scalar/bounded-span alternatives simply yield no
// product. An empty CompleteSpan remains one valid alternative and redeems
// its mounted denominator lineage; it is never turned into a synthetic row.
func Execute(plan arrangement.ApplyBinding, mounted witness.Mounted, children [][]tuple.Batch, view geometry.Geometry, seed witness.Scope, witnesses []binding.DenominatorWitness) (Results, bool) {
	if !plan.Available() || !mounted.Available() || !view.Available() || !view.Fence().Same(mounted.RuntimeFence()) {
		return Results{}, false
	}
	if seed.Available() && (!seed.ValidFor(mounted.RuntimeFence()) || !validScope(view, mounted, seed)) {
		return Results{}, false
	}
	deliveries := plan.Deliveries()
	slotSource := plan.SlotSource()
	if !validPlan(plan, mounted, deliveries, slotSource, witnesses) || len(children) != plan.ChildCount() {
		return Results{}, false
	}
	bound, boundOK := mounted.Binding(plan.Operation())
	if !boundOK || bound == nil {
		return Results{}, false
	}
	inputs := bound.Signature().Inputs()
	operationSignature := bound.Signature()
	operation := operationSignature.Identity()
	if !operation.Available() || operation != plan.Operation() || len(inputs) != len(deliveries) {
		return Results{}, false
	}

	groupDeliveries, groupOK := groupBindings(deliveries, slotSource, plan.ChildCount())
	if !groupOK {
		return Results{}, false
	}
	options := make([][]deliveryOption, len(groupDeliveries))
	for index, group := range groupDeliveries {
		value, ok := deliveryOptions(mounted, view, group, children[index])
		if !ok {
			return Results{}, false
		}
		// One child with no eligible alternative makes the complete cartesian
		// product empty. This is an ordinary no-selection result, not a
		// malformed operation and not a default empty frame.
		if len(value) == 0 {
			scopes, scopesOK := emptyExtentScopes(mounted, view, children, seed)
			if !scopesOK {
				return Results{}, false
			}
			return sealedResultsWithScopes(operation, nil, scopes), true
		}
		options[index] = value
	}

	// Bind the semantic adapter once. Its worker is serial and reused for
	// every frame; atScope changes only its mounted invocation scope.
	core, coreOK := prepareCore(mounted, plan.Operation())
	if !coreOK {
		return Results{}, false
	}
	selected := make([]deliveryOption, len(groupDeliveries))
	values := make([]Application, 0)
	var visit func(int, witness.Scope, bool) bool
	visit = func(index int, scope witness.Scope, hasScope bool) bool {
		if index == len(selected) {
			if !hasScope {
				return false
			}
			executor, executorOK := core.atScope(mounted, scope)
			if !executorOK {
				return false
			}
			frame, frameOK := frameForSelection(mounted, executor, scope, deliveries, slotSource, selected, witnesses)
			if !frameOK {
				return false
			}
			provenance, provenanceOK := lineageForSelection(mounted, selected, slotSource, inputs)
			if !provenanceOK {
				return false
			}
			scopeToken, scopeTokenOK := mounted.ScopeToken(scope)
			if !scopeTokenOK {
				return false
			}
			invocation, invocationOK := invocationAddressForSelection(scopeToken, selected)
			if !invocationOK {
				return false
			}
			destination, destinationOK := destinationForSelection(plan, operationSignature, frame)
			if !destinationOK {
				return false
			}
			application, invokeOK := executor.invoke(frame, provenance, invocation, destination)
			if !invokeOK {
				return false
			}
			values = append(values, application)
			return true
		}
		for _, candidate := range options[index] {
			next, compatible := conjoinScope(view, scope, hasScope, candidate.scope)
			if !compatible {
				// Disjoint valid cofibers have no common fact frame. They are
				// omitted from the product, never widened or made into a
				// Refused pseudo-row.
				continue
			}
			selected[index] = candidate
			if !visit(index+1, next, true) {
				return false
			}
		}
		return true
	}
	if !visit(0, seed, seed.Available()) {
		return Results{}, false
	}
	return sealedResults(operation, values), true
}

// destinationForSelection redeems the exact sealed output geometry from the
// mounted Apply plan. It never chooses a slot from cardinality or input order:
// the arrangement carries the source slot resolved at mount, and the frame
// carries the already-fenced scalar/span view for that slot.
func destinationForSelection(plan arrangement.ApplyBinding, operation signature.Signature, frame binding.Frame) (binding.DestinationView, bool) {
	if !plan.Available() || !operation.Available() || !frame.Available() {
		return binding.DestinationView{}, false
	}
	address := plan.OutputAddress()
	if !address.Available() {
		return binding.DestinationView{}, false
	}
	if address.IsOwnerNamed() {
		outputs := operation.Outputs()
		if len(outputs) == 0 {
			return binding.DestinationView{}, false
		}
		relation := outputs[0].Relation
		if !relation.Available() {
			return binding.DestinationView{}, false
		}
		for _, output := range outputs[1:] {
			if output.Relation != relation {
				return binding.DestinationView{}, false
			}
		}
		return binding.NewOwnerNamedDestination(relation), true
	}
	slotIndex, ok := plan.DestinationSlot()
	if !ok {
		return binding.DestinationView{}, false
	}
	slot, ok := frame.At(slotIndex)
	if !ok {
		return binding.DestinationView{}, false
	}
	if address.IsScalarSource() {
		cell, cellOK := slot.At(0)
		if !cellOK || !slot.IsScalar() {
			return binding.DestinationView{}, false
		}
		return binding.NewScalarDestination(cell)
	}
	if address.IsSpanSource() {
		span, spanOK := slot.Span()
		if !spanOK || !slot.IsSpan() {
			return binding.DestinationView{}, false
		}
		return binding.NewSpanDestination(span)
	}
	return binding.DestinationView{}, false
}

// invocationAddressForSelection copies the exact source vectors redeemed by
// the physical Apply product. It runs after scope conjunction and tuple/range
// authentication, so the resulting address contains no guessed row, reverse
// lineage lookup, or application ordering identity.
func invocationAddressForSelection(scope binding.ScopeToken, selected []deliveryOption) (invocation.InvocationAddress, bool) {
	if !scope.Available() || selected == nil || len(selected) == 0 {
		return invocation.InvocationAddress{}, false
	}
	children := make([]invocation.SourceVector, len(selected))
	for childIndex, choice := range selected {
		if choice.hasScalar {
			if !choice.scalar.Available() {
				return invocation.InvocationAddress{}, false
			}
			rows := choice.scalar.Sources()
			if rows == nil {
				return invocation.InvocationAddress{}, false
			}
			tuple, tupleOK := invocation.NewTupleSources(rows)
			if !tupleOK {
				return invocation.InvocationAddress{}, false
			}
			child, childOK := invocation.NewSourceVector([]invocation.TupleSources{tuple})
			if !childOK {
				return invocation.InvocationAddress{}, false
			}
			children[childIndex] = child
			continue
		}
		if !choice.rangeBatch.Available() {
			return invocation.InvocationAddress{}, false
		}
		tuples := make([]invocation.TupleSources, choice.rangeBatch.Len())
		for tupleIndex := 0; tupleIndex < choice.rangeBatch.Len(); tupleIndex++ {
			value, ok := choice.rangeBatch.At(tupleIndex)
			if !ok || !value.Available() || value.Sources() == nil {
				return invocation.InvocationAddress{}, false
			}
			rows := value.Sources()
			tuples[tupleIndex], ok = invocation.NewTupleSources(rows)
			if !ok {
				return invocation.InvocationAddress{}, false
			}
		}
		if tuples == nil {
			// Preserve an authenticated empty Complete extent as a non-nil
			// vector. It proves no selected tuple, not an unavailable child.
			tuples = []invocation.TupleSources{}
		}
		child, childOK := invocation.NewSourceVector(tuples)
		if !childOK {
			return invocation.InvocationAddress{}, false
		}
		children[childIndex] = child
	}
	return invocation.New(scope, children)
}

// deliveryOption preserves the input position selected for one Apply frame.
// Exactly one representation is present: scalar is an individual tuple;
// rangeBatch is an entire authenticated range. Keeping this distinction
// private prevents an evaluator from zipping or flattening arity away.
type deliveryOption struct {
	scalar     tuple.Tuple
	hasScalar  bool
	rangeBatch tuple.Batch
	scope      witness.Scope
}

func groupBindings(deliveries []arrangement.DeliveryBinding, groups []algebra.SlotSource, childCount int) ([][]arrangement.DeliveryBinding, bool) {
	if len(deliveries) == 0 || len(deliveries) != len(groups) || childCount == 0 {
		return nil, false
	}
	result := make([][]arrangement.DeliveryBinding, childCount)
	for slot, source := range groups {
		child := source.Child()
		if int(child) >= childCount || !deliveries[slot].Available() {
			return nil, false
		}
		result[child] = append(result[child], deliveries[slot])
	}
	for _, group := range result {
		if len(group) == 0 {
			return nil, false
		}
		for index := 1; index < len(group); index++ {
			if !sameGroupInput(group[0].Requirement().Input(), group[index].Requirement().Input()) {
				return nil, false
			}
		}
	}
	return result, true
}

func sameGroupInput(left, right signature.Input) bool {
	// Scalar slots can be projected from one sealed composite tuple. Their
	// nominal relations and denominators are intentionally independent; the
	// tuple's CellFor/SourceFor checks in frame construction redeem each slot
	// against its own owner. Span slots instead share one physical carrier
	// range. Their source relation may differ for a joined delivery, but their
	// carrier denominator and logical delivery order must remain exact.
	if left.Delivery.IsScalar() && right.Delivery.IsScalar() {
		return true
	}
	return left.Denominator == right.Denominator && left.Delivery == right.Delivery
}

func deliveryOptions(mounted witness.Mounted, view geometry.Geometry, group []arrangement.DeliveryBinding, batches []tuple.Batch) ([]deliveryOption, bool) {
	if len(group) == 0 || batches == nil {
		return nil, false
	}
	delivery := group[0]
	for index := 1; index < len(group); index++ {
		if !sameGroupInput(delivery.Requirement().Input(), group[index].Requirement().Input()) {
			return nil, false
		}
	}
	input := delivery.Requirement().Input()
	if !input.Available() {
		return nil, false
	}
	result := make([]deliveryOption, 0)
	for _, batch := range batches {
		if !batch.ValidFor(mounted) || !validScope(view, mounted, batch.Scope()) {
			return nil, false
		}
		if input.Delivery.IsScalar() {
			for tupleIndex := 0; tupleIndex < batch.Len(); tupleIndex++ {
				value, ok := batch.At(tupleIndex)
				if !ok || !value.ValidFor(mounted) || !value.Scope().Same(batch.Scope()) || !validScope(view, mounted, value.Scope()) {
					return nil, false
				}
				result = append(result, deliveryOption{scalar: value, hasScalar: true, scope: value.Scope()})
			}
			continue
		}
		if !input.Delivery.IsSpan() {
			return nil, false
		}
		for _, member := range group {
			if !validSpanBatch(mounted, batch, member) {
				return nil, false
			}
		}
		// A bounded span has no closed-world meaning when it is empty. Its
		// child contributes no alternative. CompleteSpan is different: its
		// denominator is a declared fact and makes the empty range real.
		if batch.Len() == 0 && !input.Delivery.IsComplete() {
			continue
		}
		result = append(result, deliveryOption{rangeBatch: batch, scope: batch.Scope()})
	}
	return result, true
}

func validScope(view geometry.Geometry, mounted witness.Mounted, scope witness.Scope) bool {
	return scope.ValidFor(mounted.RuntimeFence()) && view.Entails(scope, scope)
}

// conjoinScope uses inclusion first, then the physical cofiber intersection.
// The inclusion cases preserve an already-normalized scope identity; the
// intersection case is the sole admission of a new runtime formula. Both
// operands were independently redeemed before recursion, so a failed
// conjunction here means disjointness rather than a reason to fabricate a
// fallback scope.
func conjoinScope(view geometry.Geometry, current witness.Scope, hasCurrent bool, next witness.Scope) (witness.Scope, bool) {
	if !next.Available() {
		return witness.Scope{}, false
	}
	if !hasCurrent {
		return next, true
	}
	if view.Entails(current, next) {
		return current, true
	}
	if view.Entails(next, current) {
		return next, true
	}
	return view.Conjoin(current, next)
}

// validSpanBatch redeems the private arrangement witness carried by Batch
// against one authored span delivery. Apply may consume a range only when its
// exact mounted relation/key/denominator context matches the input position;
// it never rebuilds an Access or resolves one at runtime.
func validSpanBatch(mounted witness.Mounted, batch tuple.Batch, delivery arrangement.DeliveryBinding) bool {
	if !batch.ValidFor(mounted) || !delivery.Available() {
		return false
	}
	input := delivery.Requirement().Input()
	if !input.Delivery.IsSpan() {
		return false
	}
	authority := batch.Range()
	if !authority.Available() || !authority.ValidFor(mounted.Fence()) {
		return false
	}
	order, orderOK := delivery.Order()
	if !orderOK || !order.ValidFor(mounted.Fence()) {
		return false
	}
	layout := authority.Layout()
	// The signature's Delivery order—not the source-cell authority—is the only
	// authority for a span's physical key choreography. Group and Merge may
	// deliberately partition a relation by a different declared key. Exact
	// Layout equality also fences the producer to this mounted order handle.
	if !layout.ValidFor(mounted.Fence()) || layout.Access().Relation() != input.CarrierDenominator().Relation() || !layout.Equal(order) {
		return false
	}
	if input.Delivery.IsComplete() {
		return authority.Kind() == algebra.KindComplete && authority.Denominator() == input.Denominator
	}
	return authority.Kind() == algebra.KindGroup || authority.Kind() == algebra.KindMerge
}

// lineageForSelection joins exactly the selected semantic inputs in authored
// input order. A scalar contributes its one tuple once. A span contributes
// every tuple in its selected range once. An empty complete span contributes
// only the mounted denominator witness. No empty/default provenance exists.
func lineageForSelection(mounted witness.Mounted, selected []deliveryOption, slotSource []algebra.SlotSource, inputs []signature.Input) (model.LineageRef, bool) {
	authority, ok := mounted.Lineage()
	if !ok || authority == nil || len(selected) == 0 || len(slotSource) != len(inputs) {
		return model.LineageRef{}, false
	}
	refs := make([]model.LineageRef, 0)
	seen := make([]bool, len(selected))
	for index, input := range inputs {
		group := slotSource[index].Child()
		if int(group) >= len(selected) {
			return model.LineageRef{}, false
		}
		if seen[group] {
			continue
		}
		seen[group] = true
		choice := selected[group]
		if input.Delivery.IsScalar() {
			if !choice.hasScalar || !choice.scalar.ValidFor(mounted) {
				return model.LineageRef{}, false
			}
			refs = append(refs, choice.scalar.Lineage())
			continue
		}
		if !input.Delivery.IsSpan() || choice.hasScalar || !choice.rangeBatch.ValidFor(mounted) {
			return model.LineageRef{}, false
		}
		if choice.rangeBatch.Len() == 0 {
			if !input.Delivery.IsComplete() {
				return model.LineageRef{}, false
			}
			witnessValue, witnessOK := choice.rangeBatch.DenominatorWitness()
			if !witnessOK || !witnessValue.Matches(input.Denominator) || !witnessValue.ValidFor(mounted.RuntimeFence()) {
				return model.LineageRef{}, false
			}
			ref, refOK := mounted.DenominatorLineage(input.Denominator)
			if !refOK {
				return model.LineageRef{}, false
			}
			refs = append(refs, ref)
			continue
		}
		for tupleIndex := 0; tupleIndex < choice.rangeBatch.Len(); tupleIndex++ {
			value, valueOK := choice.rangeBatch.At(tupleIndex)
			if !valueOK || !value.ValidFor(mounted) {
				return model.LineageRef{}, false
			}
			refs = append(refs, value.Lineage())
		}
	}
	if len(refs) == 0 {
		return model.LineageRef{}, false
	}
	result := refs[0]
	if !result.Available() || !authority.Validate(result) {
		return model.LineageRef{}, false
	}
	for _, ref := range refs[1:] {
		if !ref.Available() || !authority.Validate(ref) {
			return model.LineageRef{}, false
		}
		joined, joinOK := authority.Join(result, ref)
		if !joinOK || !joined.Available() || !authority.Validate(joined) {
			return model.LineageRef{}, false
		}
		result = joined
	}
	return result, true
}

func sealedResults(operation signature.Identity, values []Application) Results {
	if !operation.Available() {
		return Results{}
	}
	// Preserve a non-nil empty extent. An empty cartesian product is a valid,
	// closed NoSelection result; copying it through append(nil, ...) would turn
	// it into the zero/unavailable representation and force callers to invent
	// a special-case result.
	result := make([]Application, len(values))
	copy(result, values)
	scopes := make([]binding.ScopeToken, 0, len(result))
	for _, application := range result {
		if !application.Available() {
			return Results{}
		}
		scope := application.Invocation().Scope()
		seen := false
		for _, prior := range scopes {
			if prior.Same(scope) {
				seen = true
				break
			}
		}
		if !seen {
			scopes = append(scopes, scope)
		}
	}
	return sealedResultsWithScopes(operation, result, scopes)
}

func sealedResultsWithScopes(operation signature.Identity, values []Application, scopes []binding.ScopeToken) Results {
	if !operation.Available() {
		return Results{}
	}
	if values == nil {
		values = []Application{}
	}
	if scopes == nil {
		scopes = []binding.ScopeToken{}
	}
	copyValues := make([]Application, len(values))
	copy(copyValues, values)
	copyScopes := make([]binding.ScopeToken, len(scopes))
	copy(copyScopes, scopes)
	for _, application := range copyValues {
		if !application.Available() || application.Operation() != operation {
			return Results{}
		}
	}
	for index, scope := range copyScopes {
		if !scope.Available() {
			return Results{}
		}
		for _, prior := range copyScopes[:index] {
			if prior.Same(scope) {
				return Results{}
			}
		}
	}
	return Results{operation: operation, values: copyValues, scopes: copyScopes, sealed: true}
}

// emptyExtentScopes computes the authenticated common cofibers of the input
// batch extent. It is used only when no Apply application exists: if a child
// has no batch or the child scopes are disjoint, no evaluated scope exists
// and the extent must refuse publication rather than invent absence.
func emptyExtentScopes(mounted witness.Mounted, view geometry.Geometry, children [][]tuple.Batch, seed witness.Scope) ([]binding.ScopeToken, bool) {
	if !mounted.Available() || !view.Available() || children == nil || len(children) == 0 {
		return nil, false
	}
	current := []witness.Scope{}
	if seed.Available() {
		if !seed.ValidFor(mounted.RuntimeFence()) || !validScope(view, mounted, seed) {
			return nil, false
		}
		current = append(current, seed)
	}
	for childIndex, batches := range children {
		if batches == nil || len(batches) == 0 {
			return []binding.ScopeToken{}, true
		}
		childScopes := make([]witness.Scope, 0, len(batches))
		for _, batch := range batches {
			if !batch.ValidFor(mounted) || !validScope(view, mounted, batch.Scope()) {
				return nil, false
			}
			duplicate := false
			for _, prior := range childScopes {
				if prior.Same(batch.Scope()) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				childScopes = append(childScopes, batch.Scope())
			}
		}
		if len(childScopes) == 0 {
			return []binding.ScopeToken{}, true
		}
		if childIndex == 0 && len(current) == 0 {
			current = childScopes
			continue
		}
		next := make([]witness.Scope, 0, len(current)*len(childScopes))
		for _, left := range current {
			for _, right := range childScopes {
				joined, ok := view.Conjoin(left, right)
				if !ok {
					continue
				}
				duplicate := false
				for _, prior := range next {
					if prior.Same(joined) {
						duplicate = true
						break
					}
				}
				if !duplicate {
					next = append(next, joined)
				}
			}
		}
		current = next
		if len(current) == 0 {
			return []binding.ScopeToken{}, true
		}
	}
	result := make([]binding.ScopeToken, 0, len(current))
	for _, scope := range current {
		token, ok := mounted.ScopeToken(scope)
		if !ok || !token.Available() {
			return nil, false
		}
		result = append(result, token)
	}
	return result, true
}

// validPlan verifies that the public sealed binding is the exact operation
// contract redeemed by the mounted semantic worker. In particular, a
// delivery may not be dropped, reordered, or silently substituted by a
// runtime adapter.
func validPlan(plan arrangement.ApplyBinding, mounted witness.Mounted, deliveries []arrangement.DeliveryBinding, slotSource []algebra.SlotSource, witnesses []binding.DenominatorWitness) bool {
	childCount := plan.ChildCount()
	if len(deliveries) == 0 || len(deliveries) != len(slotSource) || len(witnesses) != len(deliveries) || childCount == 0 {
		return false
	}
	// slotSource is a sealed dense coordinate map. Keep this check allocation
	// free: Execute is on the hot path and the binding already carries the
	// validated child count. Every child must be named by at least one slot;
	// otherwise the runtime would silently drop an authored child expression.
	for _, source := range slotSource {
		if int(source.Child()) >= childCount {
			return false
		}
	}
	for expected := 0; expected < childCount; expected++ {
		found := false
		for _, source := range slotSource {
			if int(source.Child()) == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	bindingValue, ok := mounted.Binding(plan.Operation())
	if !ok || bindingValue == nil {
		return false
	}
	operation := bindingValue.Signature()
	if !operation.Available() || operation.Identity() != plan.Operation() || len(deliveries) != operation.InputLen() {
		return false
	}
	for index, delivery := range deliveries {
		if !delivery.Available() || delivery.Requirement().Operation() != plan.Operation() || int(delivery.Requirement().Index()) != index {
			return false
		}
		input, inputOK := operation.InputAt(index)
		if !inputOK || !input.Available() || !sameInput(delivery.Requirement().Input(), input) {
			return false
		}
		witnessValue := witnesses[index]
		if !witnessValue.Available() || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(input.Denominator) {
			return false
		}
		sourceDenominator, sourceOK := input.SourceDenominator()
		if !sourceOK {
			return false
		}
		layout := delivery.Layout()
		if !layout.ValidFor(mounted.Fence()) || layout.Access().Relation() != input.Relation || layout.Access().Key() != sourceDenominator.Key() {
			return false
		}
		columns := layout.Columns()
		if len(columns) != 1 || columns[0] != input.Column {
			return false
		}
		if input.Delivery.IsSpan() {
			order, orderOK := delivery.Order()
			if !orderOK || !order.ValidFor(mounted.Fence()) || order.Access().Key() != input.Delivery.OrderKey() {
				return false
			}
		} else if _, orderOK := delivery.Order(); orderOK {
			return false
		}
	}
	return true
}
func sameInput(left, right signature.Input) bool {
	return left.Same(right)
}

// frameForSelection issues every delivered cell under the already-conjoined
// invocation scope. Tuple identity and payloads remain borrowed source facts;
// Apply only redeems the mounted cell address needed by the semantic ABI.
func frameForSelection(mounted witness.Mounted, executor Executor, scope witness.Scope, deliveries []arrangement.DeliveryBinding, slotSource []algebra.SlotSource, selected []deliveryOption, witnesses []binding.DenominatorWitness) (binding.Frame, bool) {
	if !executor.Available() || !scope.ValidFor(mounted.RuntimeFence()) || len(deliveries) != len(slotSource) || len(witnesses) != len(deliveries) {
		return binding.Frame{}, false
	}
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok || !scopeToken.ValidFor(mounted.RuntimeFence()) {
		return binding.Frame{}, false
	}
	slots := make([]binding.Slot, len(deliveries))
	for index, delivery := range deliveries {
		input := delivery.Requirement().Input()
		if !input.Available() {
			return binding.Frame{}, false
		}
		source := slotSource[index]
		if int(source.Child()) >= len(selected) {
			return binding.Frame{}, false
		}
		choice := selected[source.Child()]
		carrierWitness := witnesses[index]
		if !carrierWitness.Available() || !carrierWitness.ValidFor(mounted.RuntimeFence()) || !carrierWitness.Matches(input.CarrierDenominator()) {
			return binding.Frame{}, false
		}
		sourceWitness, sourceWitnessOK := sourceWitnessForInput(mounted, input, carrierWitness)
		if !sourceWitnessOK {
			return binding.Frame{}, false
		}
		var slot binding.Slot
		var slotOK bool
		if input.Delivery.IsScalar() {
			if !choice.hasScalar {
				return binding.Frame{}, false
			}
			cell, cellOK := semanticCell(mounted, scopeToken, input, source, choice.scalar, sourceWitness)
			if !cellOK {
				return binding.Frame{}, false
			}
			slot, slotOK = binding.NewScalarSlot(cell)
		} else if input.Delivery.IsSpan() {
			if choice.hasScalar {
				return binding.Frame{}, false
			}
			cells, rangeRows, cellsOK := spanCells(mounted, scopeToken, input, source, choice.rangeBatch, sourceWitness, carrierWitness)
			if !cellsOK {
				return binding.Frame{}, false
			}
			if len(cells) == 0 {
				if !input.Delivery.IsComplete() {
					return binding.Frame{}, false
				}
				slot, slotOK = binding.NewEmptySpanSlot(carrierWitness)
			} else if input.IsJoined() {
				slot, slotOK = binding.NewJoinedSpanSlot(cells, rangeRows, carrierWitness)
			} else {
				slot, slotOK = binding.NewSpanSlot(cells)
			}
		} else {
			return binding.Frame{}, false
		}
		if !slotOK {
			return binding.Frame{}, false
		}
		slots[index] = slot
	}
	frame, frameOK := executor.Frame(slots...)
	if !frameOK || !frame.Scope().Same(scopeToken) {
		return binding.Frame{}, false
	}
	return frame, true
}

// sourceWitnessForInput redeems source authority without turning a joined
// input into a second carrier lookup.  The caller supplies the exact range
// witness selected by Complete (which may be a correlated posting).  Only a
// declared joined source resolves its distinct globally mounted witness.
func sourceWitnessForInput(mounted witness.Mounted, input signature.Input, carrier binding.DenominatorWitness) (binding.DenominatorWitness, bool) {
	if !mounted.Available() || !input.Available() || !carrier.Available() || !carrier.ValidFor(mounted.RuntimeFence()) || !carrier.Matches(input.CarrierDenominator()) {
		return binding.DenominatorWitness{}, false
	}
	source, sourceOK := input.SourceDenominator()
	if !sourceOK || !source.Available() {
		return binding.DenominatorWitness{}, false
	}
	if input.IsHomogeneous() {
		if source != input.CarrierDenominator() {
			return binding.DenominatorWitness{}, false
		}
		return carrier, true
	}
	if !input.IsJoined() || source == input.CarrierDenominator() {
		return binding.DenominatorWitness{}, false
	}
	witnessValue, witnessOK := mounted.Denominator(source)
	if !witnessOK || !witnessValue.Available() || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(source) {
		return binding.DenominatorWitness{}, false
	}
	return witnessValue, true
}

// semanticCell redeems the exact source occurrence selected by SlotSource.
// It does not scan tuple sources by relation: Cell.Source is the sealed
// occurrence identity for the delivered column, and sourceWitness is already
// resolved from the input's source authority.
func semanticCell(mounted witness.Mounted, scope binding.ScopeToken, input signature.Input, source algebra.SlotSource, value tuple.Tuple, sourceWitness binding.DenominatorWitness) (binding.Cell, bool) {
	if !mounted.Available() || !input.Available() || !scope.ValidFor(mounted.RuntimeFence()) || !value.ValidFor(mounted) {
		return binding.Cell{}, false
	}
	cell, cellOK := value.At(int(source.Cell()))
	if !cellOK || cell.Column() != input.Column || !cell.Presence().Available() || cell.Type() != input.Type || !input.Presence.Allows(cell.Presence()) {
		return binding.Cell{}, false
	}
	row, rowOK := value.SourceAt(int(cell.Source()))
	if !rowOK || row.Relation() != input.Relation {
		return binding.Cell{}, false
	}
	sourceDenominator, sourceOK := input.SourceDenominator()
	if !sourceOK || !sourceWitness.Available() || !sourceWitness.ValidFor(mounted.RuntimeFence()) || !sourceWitness.Matches(sourceDenominator) || !sourceWitness.Contains(row) {
		return binding.Cell{}, false
	}
	scopeValue, scopeOK := mounted.ScopeForToken(scope)
	if !scopeOK {
		return binding.Cell{}, false
	}
	var address binding.CellToken
	var addressOK bool
	address, addressOK = mounted.IssueCell(sourceWitness, scopeValue, input.Column, row)
	if !addressOK {
		return binding.Cell{}, false
	}
	return binding.NewCell(address, cell.Type(), cell.Value(), cell.Presence())
}

// spanCells preserves both columns of a joined delivery pair.  semanticCell
// redeems the source row from the exact Cell.Source occurrence; rangeAnchor
// redeems the carrier row from the sealed tuple's source vector and refuses a
// duplicate carrier relation rather than selecting a positional occurrence.
func spanCells(mounted witness.Mounted, scope binding.ScopeToken, input signature.Input, source algebra.SlotSource, batch tuple.Batch, sourceWitness, carrierWitness binding.DenominatorWitness) ([]binding.Cell, []model.RowID, bool) {
	if !mounted.Available() || !input.Available() || !scope.ValidFor(mounted.RuntimeFence()) || !batch.ValidFor(mounted) || !sourceWitness.ValidFor(mounted.RuntimeFence()) || !carrierWitness.ValidFor(mounted.RuntimeFence()) || !carrierWitness.Matches(input.CarrierDenominator()) {
		return nil, nil, false
	}
	if limit, bounded := input.Delivery.Limit(); bounded && uint64(batch.Len()) > uint64(limit) {
		// The sealed Delivery owns the bound. A batch that exceeds it is a
		// malformed invocation, not a reason to truncate or split the range in
		// Apply.
		return nil, nil, false
	}
	result := make([]binding.Cell, 0, batch.Len())
	rangeRows := make([]model.RowID, 0, batch.Len())
	for index := 0; index < batch.Len(); index++ {
		value, valueOK := batch.At(index)
		if !valueOK || !value.ValidFor(mounted) {
			return nil, nil, false
		}
		cell, cellOK := semanticCell(mounted, scope, input, source, value, sourceWitness)
		if !cellOK {
			return nil, nil, false
		}
		rangeRow, rangeRowOK := rangeAnchor(value, input, carrierWitness)
		if !rangeRowOK {
			return nil, nil, false
		}
		result = append(result, cell)
		rangeRows = append(rangeRows, rangeRow)
	}
	return result, rangeRows, true
}

// rangeAnchor resolves the exact carrier occurrence from the tuple's sealed
// source vector.  SourceFor is intentionally not a database/domain lookup:
// it sees only authored tuple occurrences and hard-refuses a self-join with
// two candidate carrier rows, preventing a hidden first-match convention.
func rangeAnchor(value tuple.Tuple, input signature.Input, carrier binding.DenominatorWitness) (model.RowID, bool) {
	if !value.Available() || !input.Available() || !carrier.Available() || !carrier.Matches(input.CarrierDenominator()) {
		return model.RowID{}, false
	}
	row, ok := value.SourceFor(input.CarrierDenominator().Relation())
	if !ok || !carrier.Contains(row) {
		return model.RowID{}, false
	}
	return row, true
}
