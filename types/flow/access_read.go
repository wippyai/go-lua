package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// AccessReadKind classifies a Lua table access after syntax has been lowered at
// the transfer boundary.
type AccessReadKind uint8

const (
	AccessReadStaticMember AccessReadKind = iota + 1
	AccessReadDynamicIndex
)

// AccessReadQuery asks the point-facts projection to read a table access through
// point-local facts, product runtime semantics, and callable identity overlays.
type AccessReadQuery struct {
	Kind    AccessReadKind
	Path    constraint.Path
	HasPath bool
	Base    product.AbstractValue
	Member  value.MemberKey
	Key     product.AbstractValue
	Policy  PointReadPolicy
}

// ReadAccess returns the best product value visible for an access. Transfer owns
// AST lowering and post-read numeric/admission refinements; flow owns composing
// point facts, product runtime reads, and callable overlays.
func (f PointFacts) ReadAccess(q AccessReadQuery) ProductValue {
	if q.HasPath {
		if fact := f.ReadStaticMemberValue(q.Path, q.Policy); fact.State == StateResolved {
			return fact
		}
	}
	if q.Base.IsZero() {
		return ProductValue{State: StateUnknown}
	}
	switch q.Kind {
	case AccessReadStaticMember:
		read, ok := product.RuntimeMemberOf(q.Base, q.Member)
		if !ok || read.IsZero() {
			return f.readKnownCallableAccess(q.Path, q.HasPath, read, q.Policy)
		}
		if callable := f.readKnownCallableAccess(q.Path, q.HasPath, read, q.Policy); callable.State == StateResolved {
			return callable
		}
		return ProductValue{Value: read, State: StateResolved}
	case AccessReadDynamicIndex:
		if q.Key.IsZero() {
			return ProductValue{State: StateUnknown}
		}
		read, ok := product.RuntimeIndexOf(q.Base, q.Key)
		if !ok || read.IsZero() {
			return ProductValue{State: StateUnknown}
		}
		return ProductValue{Value: read, State: StateResolved}
	default:
		return ProductValue{State: StateUnknown}
	}
}

func (f PointFacts) readKnownCallableAccess(
	path constraint.Path,
	hasPath bool,
	read product.AbstractValue,
	policy PointReadPolicy,
) ProductValue {
	if !hasPath {
		return ProductValue{State: StateUnknown}
	}
	return f.ReadKnownCallablePath(path, read, policy)
}
