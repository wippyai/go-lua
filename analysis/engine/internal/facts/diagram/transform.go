package diagram

import "github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"

// SoleTransform maps one existing sparse column. Returning the undefined
// value removes that key; transforms cannot invent keys which were absent
// from the immutable input root.
type SoleTransform[K scalar.Key, V any] func(K, Value[V]) (Value[V], bool)

// TransformSoleFactor applies one column transform while preserving the
// immutable key-tree shape wherever possible. Unlike ForEach+Put, it does not
// rebuild a persistent AVL search path for every key: unchanged subtrees are
// returned by pointer, changed balanced nodes are copied once, and deletion
// uses the height-general join shared by batch sparse updates.
func (builder *Builder[F, K, V]) TransformSoleFactor(input Root[F, K, V], transform SoleTransform[K, V]) (Root[F, K, V], bool) {
	if builder == nil || !builder.open || builder.diagram == nil || !builder.Valid(input) || transform == nil {
		return Root[F, K, V]{}, false
	}
	factor, sole := builder.diagram.SoleFactor()
	if !sole {
		return Root[F, K, V]{}, false
	}
	rank, ranked := builder.diagram.ranks[factor]
	if !ranked {
		return Root[F, K, V]{}, false
	}
	column := findFactor(input.root, rank)
	keys, count, ok := transformSoleKeys(builder, factorKeys(column), transform)
	if !ok || count < 0 || (keys == nil) != (count == 0) {
		return Root[F, K, V]{}, false
	}
	if keys == factorKeys(column) {
		return Root[F, K, V]{diagram: builder.diagram, root: input.root, count: input.count, lease: builder.lease}, true
	}
	var root *factorNode[F, K, V]
	if keys != nil {
		root = makeFactor(factor, rank, keys, nil, nil)
	}
	return Root[F, K, V]{diagram: builder.diagram, root: root, count: count, lease: builder.lease}, true
}

// transformSoleKeys visits in ascending key order. Recursion consumes only
// AVL height, not key cardinality, while keeping the transformed child roots
// available for exact pointer reuse at the parent.
func transformSoleKeys[F ~uint64, K scalar.Key, V any](builder *Builder[F, K, V], input *keyNode[K, V], transform SoleTransform[K, V]) (*keyNode[K, V], int, bool) {
	if input == nil {
		return nil, 0, true
	}
	left, leftCount, ok := transformSoleKeys(builder, input.left, transform)
	if !ok {
		return nil, 0, false
	}
	value, ok := transform(input.key, Value[V]{owner: builder.diagram.owner, node: input.value})
	if !ok || !builder.validValue(value) {
		return nil, 0, false
	}
	right, rightCount, ok := transformSoleKeys(builder, input.right, transform)
	if !ok {
		return nil, 0, false
	}
	if undefinedNode(value.node) {
		return concatKeys(left, right), leftCount + rightCount, true
	}
	count := leftCount + rightCount + 1
	if left == input.left && right == input.right && sameSparseNode(input.value, value.node) {
		return input, count, true
	}
	return joinKey3(left, input.key, value.node, right), count, true
}
