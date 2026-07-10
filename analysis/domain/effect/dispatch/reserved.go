package dispatch

import "github.com/wippyai/go-lua/analysis/domain/effect"

var (
	_ effect.Label = VariadicTransform{}
	_ effect.Label = TypePredicate{}
)

// VariadicTransform is reserved high-risk dispatch metadata. Variadic lowering
// is syntax and call-shape owned, so stdlib signatures must not declare this
// label while it is inactive.
type VariadicTransform struct{}

func (VariadicTransform) EffectLabel()   {}
func (VariadicTransform) String() string { return "variadic_transform" }
func (VariadicTransform) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(VariadicTransform)
	return ok
}

// TypePredicate is reserved high-risk dispatch metadata. Type narrowing is
// syntax and factflow owned, so stdlib signatures must not declare this label
// while it is inactive.
type TypePredicate struct{}

func (TypePredicate) EffectLabel()   {}
func (TypePredicate) String() string { return "type_predicate" }
func (TypePredicate) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(TypePredicate)
	return ok
}
