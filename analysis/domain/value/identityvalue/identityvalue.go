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
	if _, registered := reg.LookupErased(identity.Key.ID()); !registered {
		return identity.ID{}, false
	}
	return product.Get(reg, value, identity.Key).ID()
}

// ExactTerm returns the exact relational identity atom carried by value.
// Concrete execution may narrow this with ExactID; relation construction must
// retain formal variables and allocation templates as typed terms.
func ExactTerm(reg *axis.Registry, value product.Value) (identity.Term, bool) {
	if reg == nil || !product.BelongsToRegistry(reg, value) {
		return identity.Term{}, false
	}
	if _, registered := reg.LookupErased(identity.Key.ID()); !registered {
		return identity.Term{}, false
	}
	return product.Get(reg, value, identity.Key).Term()
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

func WithExactTerm(reg *axis.Registry, value product.Value, term identity.Term) product.Value {
	return product.Set(reg, value, identity.Key, identity.SingletonTerm(term))
}

// Present materializes a present value carrying a singleton runtime identity.
func Present(reg *axis.Registry, id identity.ID) product.Value {
	return WithExact(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), id)
}

func PresentTerm(reg *axis.Registry, term identity.Term) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.SingletonTerm(term))
}
