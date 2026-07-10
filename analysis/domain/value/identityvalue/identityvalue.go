package identityvalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ExactID returns the singleton runtime identity carried by value, if any.
func ExactID(reg *axis.Registry, value product.Value) (identity.ID, bool) {
	if reg == nil {
		return identity.ID{}, false
	}
	return product.Get(reg, value, identity.Key).ID()
}

// HasExact reports whether value carries a singleton runtime identity.
func HasExact(reg *axis.Registry, value product.Value) bool {
	_, ok := ExactID(reg, value)
	return ok
}

// WithExact records a singleton runtime identity on value.
func WithExact(reg *axis.Registry, value product.Value, id identity.ID) product.Value {
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}

// Present materializes a present value carrying a singleton runtime identity.
func Present(reg *axis.Registry, id identity.ID) product.Value {
	return WithExact(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), id)
}
