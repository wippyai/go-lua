package operation

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/rows"
)

// CompileGeometry validates, canonicalizes, and seals neutral operation
// geometry. It is the sole issuer of dense operation handles and callback IDs.
func CompileGeometry(input Input) (Geometry, error) {
	if _, err := vocabulary.CheckedStoredLength("operation geometry", len(input.Operations)); err != nil {
		return Geometry{}, err
	}
	order, err := canonicalOrder(input.Operations)
	if err != nil {
		return Geometry{}, err
	}
	canonical := make([]OperationInput, len(input.Operations))
	for index, source := range order {
		canonical[index] = input.Operations[source]
	}
	input.Operations = canonical

	var (
		segmentPool  rows.PoolBuilder[string]
		bindingRows  rows.PoolBuilder[bindingRow]
		outcomeRows  rows.PoolBuilder[outcomeRow]
		anchorPool   rows.PoolBuilder[byte]
		callbackRows []callbackRow
		producedRows rows.PoolBuilder[producedRow]
	)
	operations := make([]operationRow, len(input.Operations)+1)
	sources := make([]sourceRow, len(input.Operations))
	sourceSeen := make([]bool, len(input.Operations))
	producedInputs := make([][]ProducedInput, len(input.Operations))

	for index, item := range input.Operations {
		if item.Source < 0 || item.Source >= len(input.Operations) || sourceSeen[item.Source] {
			return Geometry{}, fmt.Errorf("target/operation: invalid source coordinate %d", item.Source)
		}
		sourceSeen[item.Source] = true
		sources[item.Source] = sourceRow{operation: vocabulary.Operation(index + 1)}
		if item.InputFormalCount < 0 {
			return Geometry{}, fmt.Errorf("target/operation: negative input formal count for operation %d", index)
		}
		if item.TypeFormalCount < 0 {
			return Geometry{}, fmt.Errorf("target/operation: negative type formal count for operation %d", index)
		}
		if item.RowFormalCount < 0 {
			return Geometry{}, fmt.Errorf("target/operation: negative row formal count for operation %d", index)
		}
		if _, err := vocabulary.CheckedStoredLength("operation row formal count", item.RowFormalCount); err != nil {
			return Geometry{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("operation type formal count", item.TypeFormalCount); err != nil {
			return Geometry{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("operation input formal count", item.InputFormalCount); err != nil {
			return Geometry{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("operation outcome table", len(item.OutcomeValueSlots)); err != nil {
			return Geometry{}, err
		}
		if len(item.OutcomeValueSlots) == 0 {
			return Geometry{}, fmt.Errorf("target/operation: operation %d has no outcomes", index)
		}
		if _, err := vocabulary.CheckedStoredLength("operation callback table", len(item.Callbacks)); err != nil {
			return Geometry{}, err
		}
		if _, err := vocabulary.CheckedStoredLength("operation binding table", len(item.Bindings)); err != nil {
			return Geometry{}, err
		}

		bindingValues := make([]bindingRow, 0, len(item.Bindings))
		for bindingIndex, binding := range item.Bindings {
			if !vocabulary.ValidBinding(binding) {
				return Geometry{}, fmt.Errorf("target/operation: invalid binding %d in operation %d", bindingIndex, index)
			}
			owner, ok := appendSegments(&segmentPool, binding.Owner)
			if !ok {
				return Geometry{}, errors.New("target/operation: operation owner segments overflow")
			}
			member, ok := appendSegments(&segmentPool, binding.Member)
			if !ok {
				return Geometry{}, errors.New("target/operation: operation member segments overflow")
			}
			bindingValues = append(bindingValues, bindingRow{namespace: binding.Namespace, owner: owner, member: member})
		}
		bindingRange, ok := bindingRows.Append(bindingValues)
		if !ok {
			return Geometry{}, errors.New("target/operation: operation binding range overflow")
		}

		outcomeValues := make([]outcomeRow, 0, len(item.OutcomeValueSlots))
		for _, outcome := range item.OutcomeValueSlots {
			selector, ok := appendBytes(&anchorPool, outcome.Anchor)
			if !ok {
				return Geometry{}, errors.New("target/operation: outcome anchor overflow")
			}
			outcomeValues = append(outcomeValues, outcomeRow{slots: outcome.ValueSlots, anchor: selector})
		}
		outcomeRange, ok := outcomeRows.Append(outcomeValues)
		if !ok {
			return Geometry{}, errors.New("target/operation: operation outcome range overflow")
		}

		callbackValues := make([]callbackRow, 0, len(item.Callbacks))
		callbackSeen := make([]bool, len(item.Callbacks))
		for callbackIndex, callback := range item.Callbacks {
			if callback.Source < 0 || callback.Source >= len(item.Callbacks) {
				return Geometry{}, fmt.Errorf("target/operation: callback %d source outside operation", callbackIndex)
			}
			if callbackSeen[callback.Source] {
				return Geometry{}, fmt.Errorf("target/operation: duplicate callback source %d", callback.Source)
			}
			callbackSeen[callback.Source] = true
			if !validCallbackLifecycle(callback.Lifecycle) {
				return Geometry{}, fmt.Errorf("target/operation: callback %d has invalid lifecycle", callbackIndex)
			}
			callbackValues = append(callbackValues, callbackRow{
				id: vocabulary.CallbackID(len(callbackRows) + len(callbackValues) + 1), owner: vocabulary.Operation(index + 1),
				source: callback.Source, function: callback.Function, ordinal: uint32(callbackIndex), lifecycle: callback.Lifecycle,
			})
		}
		callbackStart, err := vocabulary.CheckedStoredLength("operation callback table", len(callbackRows))
		if err != nil {
			return Geometry{}, err
		}
		if _, err := vocabulary.CheckedStoredTotal("operation callback table", len(callbackRows), len(callbackValues)); err != nil {
			return Geometry{}, err
		}
		callbackRows = append(callbackRows, callbackValues...)
		callbackRange := callbackRange{start: callbackStart, end: callbackStart + uint32(len(callbackValues))}
		if callbackRange.end < callbackRange.start {
			return Geometry{}, errors.New("target/operation: operation callback range overflow")
		}
		operations[index] = operationRow{
			bindings: bindingRange, outcomes: outcomeRange, callbacks: callbackRange,
			input: uint32(item.InputFormalCount), typeForms: uint32(item.TypeFormalCount), rowForms: uint32(item.RowFormalCount), valuesVar: item.ValuesVars,
		}
		producedInputs[index] = append([]ProducedInput(nil), item.Produced...)
	}
	for index := range sourceSeen {
		if !sourceSeen[index] {
			return Geometry{}, errors.New("target/operation: source coordinates are not a complete mapping")
		}
	}

	// Produced rows are appended only after every source-to-handle mapping is
	// known. The operation handle still comes from this owner; authoring source
	// coordinates never escape into the sealed rows.
	for parentIndex, inputs := range producedInputs {
		producedValues := make([]producedRow, 0, len(inputs))
		for producedIndex, produced := range inputs {
			if produced.TargetSource < 0 || produced.TargetSource >= len(input.Operations) {
				return Geometry{}, fmt.Errorf("target/operation: produced %d target outside scope", producedIndex)
			}
			parent := vocabulary.Operation(parentIndex + 1)
			child := sources[produced.TargetSource].operation
			parentRow := operations[parentIndex]
			if uint64(produced.Outcome) >= uint64(parentRow.outcomes.Len()) {
				return Geometry{}, fmt.Errorf("target/operation: produced %d outcome outside scope", producedIndex)
			}
			outcomeInput := input.Operations[parentIndex].OutcomeValueSlots[produced.Outcome]
			if produced.Result >= outcomeInput.ValueSlots {
				return Geometry{}, fmt.Errorf("target/operation: produced %d result outside scope", producedIndex)
			}
			producedValues = append(producedValues, producedRow{parent: parent, child: child, outcome: produced.Outcome, result: produced.Result})
		}
		rangeOut, ok := producedRows.Append(producedValues)
		if !ok {
			return Geometry{}, errors.New("target/operation: operation produced range overflow")
		}
		operations[parentIndex].produced = rangeOut
	}

	// The opaque operation is part of the canonical handle/anchor domain but is
	// intentionally absent from the source map consumed by Protocol and Boot.
	opaque := vocabulary.Operation(len(input.Operations) + 1)
	unknownRange, ok := outcomeRows.Append([]outcomeRow{{}, {}, {}, {}})
	if !ok {
		return Geometry{}, errors.New("target/operation: opaque outcome range overflow")
	}
	opaqueCallbackStart, err := vocabulary.CheckedStoredLength("operation callback table", len(callbackRows))
	if err != nil {
		return Geometry{}, err
	}
	if _, err := vocabulary.CheckedStoredTotal("operation callback table", len(callbackRows), 1); err != nil {
		return Geometry{}, err
	}
	callbackRows = append(callbackRows, callbackRow{id: vocabulary.CallbackID(len(callbackRows) + 1), owner: opaque, ordinal: 0, lifecycle: vocabulary.CallbackRetainedOptionalMany})
	opaqueCallbackRange := callbackRange{start: opaqueCallbackStart, end: opaqueCallbackStart + 1}
	if opaqueCallbackRange.end < opaqueCallbackRange.start {
		return Geometry{}, errors.New("target/operation: opaque callback rows overflow")
	}
	operations[len(input.Operations)] = operationRow{outcomes: unknownRange, callbacks: opaqueCallbackRange}

	geometry := Geometry{
		operations: rows.NewRows(operations), bindings: bindingRows.Seal(), segments: segmentPool.Seal(),
		outcomes: outcomeRows.Seal(), anchors: anchorPool.Seal(), callbacks: rows.NewRows(callbackRows),
		produced: producedRows.Seal(), sources: rows.NewRows(sources), sourceN: len(input.Operations), boundN: boundCount(input.Operations),
	}
	return geometry, nil
}

func boundCount(operations []OperationInput) int {
	count := 0
	for _, operation := range operations {
		if len(operation.Bindings) == 0 {
			break
		}
		count++
	}
	return count
}

// canonicalOrder is the operation owner's source-order-independent catalogue.
// Bound roots sort by their canonical first binding; produced-only rows follow
// the lexicographically ordered produced path of each root. The temporary
// graph is discarded before Geometry is published.
func canonicalOrder(input []OperationInput) ([]int, error) {
	type edge struct {
		target  int
		anchor  []byte
		result  uint32
		outcome uint32
	}
	n := len(input)
	edges := make([][]edge, n)
	incoming := make([]int, n)
	sourceIndex := make([]int, n)
	sourceSeen := make([]bool, n)
	for index, item := range input {
		if item.Source < 0 || item.Source >= n || sourceSeen[item.Source] {
			return nil, fmt.Errorf("target/operation: invalid source coordinate %d", item.Source)
		}
		sourceSeen[item.Source] = true
		sourceIndex[item.Source] = index
	}
	for source, seen := range sourceSeen {
		if !seen {
			return nil, fmt.Errorf("target/operation: source coordinates are not a complete mapping (missing %d)", source)
		}
	}
	owners := make([]struct {
		binding vocabulary.BindingSpec
	}, 0)
	for source, item := range input {
		for _, binding := range item.Bindings {
			for _, owner := range owners {
				if compareBinding(owner.binding, binding) == 0 {
					return nil, errors.New("target/operation: binding belongs to multiple operations")
				}
			}
			owners = append(owners, struct{ binding vocabulary.BindingSpec }{binding: binding})
		}
		for producedIndex, produced := range item.Produced {
			if produced.TargetSource < 0 || produced.TargetSource >= n {
				return nil, fmt.Errorf("target/operation: produced %d target outside scope", producedIndex)
			}
			if produced.Outcome >= uint32(len(item.OutcomeValueSlots)) {
				return nil, fmt.Errorf("target/operation: produced %d outcome outside scope", producedIndex)
			}
			outcome := item.OutcomeValueSlots[produced.Outcome]
			if produced.Result >= outcome.ValueSlots {
				return nil, fmt.Errorf("target/operation: produced %d result outside scope", producedIndex)
			}
			targetIndex := sourceIndex[produced.TargetSource]
			if len(input[targetIndex].Bindings) == 0 {
				incoming[targetIndex]++
			}
			edges[source] = append(edges[source], edge{
				target: targetIndex, anchor: append([]byte(nil), outcome.Anchor...),
				result: produced.Result, outcome: produced.Outcome,
			})
		}
	}
	roots := make([]int, 0, n)
	resolved := make([]bool, n)
	for source, item := range input {
		if len(item.Bindings) != 0 {
			roots = append(roots, source)
			continue
		}
		if incoming[source] != 1 {
			return nil, errors.New("target/operation: produced-only operation requires exactly one incoming produced anchor")
		}
	}
	// A root is ordered by its first canonical binding. Its complete binding
	// set was already validated by Target and duplicate ownership above.
	sort.Slice(roots, func(left, right int) bool {
		return compareBinding(input[roots[left]].Bindings[0], input[roots[right]].Bindings[0]) < 0
	})
	for source := range edges {
		sort.Slice(edges[source], func(left, right int) bool {
			if compared := bytes.Compare(edges[source][left].anchor, edges[source][right].anchor); compared != 0 {
				return compared < 0
			}
			if edges[source][left].result != edges[source][right].result {
				return edges[source][left].result < edges[source][right].result
			}
			return edges[source][left].target < edges[source][right].target
		})
		for index := 1; index < len(edges[source]); index++ {
			left, right := edges[source][index-1], edges[source][index]
			if bytes.Equal(left.anchor, right.anchor) && left.result == right.result {
				return nil, errors.New("target/operation: duplicate produced anchor step")
			}
		}
	}
	order := make([]int, 0, n)
	for _, root := range roots {
		resolved[root] = true
		order = append(order, root)
	}
	for _, root := range roots {
		stack := make([]int, 0, len(edges[root]))
		for index := len(edges[root]) - 1; index >= 0; index-- {
			child := edges[root][index].target
			if len(input[child].Bindings) == 0 {
				stack = append(stack, child)
			}
		}
		for len(stack) > 0 {
			last := len(stack) - 1
			current := stack[last]
			stack = stack[:last]
			if resolved[current] {
				return nil, errors.New("target/operation: produced anchor cycle")
			}
			resolved[current] = true
			order = append(order, current)
			for index := len(edges[current]) - 1; index >= 0; index-- {
				child := edges[current][index].target
				if len(input[child].Bindings) == 0 {
					stack = append(stack, child)
				}
			}
		}
	}
	for _, ok := range resolved {
		if !ok {
			return nil, errors.New("target/operation: produced-only operation is unreachable or cyclic")
		}
	}
	return order, nil
}

func compareBinding(left, right vocabulary.BindingSpec) int {
	if left.Namespace < right.Namespace {
		return -1
	}
	if left.Namespace > right.Namespace {
		return 1
	}
	if compared := compareSegments(left.Owner, right.Owner); compared != 0 {
		return compared
	}
	return compareSegments(left.Member, right.Member)
}

func compareSegments(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func appendSegments(pool *rows.PoolBuilder[string], values []string) (rows.Span, bool) {
	copyValues := append([]string(nil), values...)
	return pool.Append(copyValues)
}

func appendBytes(pool *rows.PoolBuilder[byte], values []byte) (rows.Span, bool) {
	copyValues := append([]byte(nil), values...)
	return pool.Append(copyValues)
}

func validCallbackLifecycle(value vocabulary.CallbackLifecycle) bool {
	switch value {
	case vocabulary.CallbackSyncRequiredOnce, vocabulary.CallbackSyncOptionalOnce,
		vocabulary.CallbackSyncOptionalMany, vocabulary.CallbackSyncRequiredMany,
		vocabulary.CallbackRetainedRequiredOnce, vocabulary.CallbackRetainedOptionalOnce,
		vocabulary.CallbackRetainedRequiredMany, vocabulary.CallbackRetainedOptionalMany:
		return true
	default:
		return false
	}
}
