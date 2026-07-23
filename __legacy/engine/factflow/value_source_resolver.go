package factflow

import "github.com/wippyai/go-lua/analysis/domain/value/product"

// ValueSourceResolver resolves lowered value-source descriptors in the current
// analysis context. It lets semantic helpers request entry values without
// knowing how point/state/read recursion and memoization are represented.
type ValueSourceResolver interface {
	ResolveValueSource(ValueSource) (product.Value, bool)
}

// ValueSourceResolverFunc adapts a function to ValueSourceResolver.
type ValueSourceResolverFunc func(ValueSource) (product.Value, bool)

func (fn ValueSourceResolverFunc) ResolveValueSource(source ValueSource) (product.Value, bool) {
	return fn(source)
}
