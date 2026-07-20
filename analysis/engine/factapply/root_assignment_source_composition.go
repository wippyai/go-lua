package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
)

// RootAssignmentSourceComposition is the complete context-free value algebra
// of N4 before path/object side effects. Evidence producers decide only the
// booleans in this descriptor; both concrete and factor execution call the
// same composition law.
type RootAssignmentSourceComposition struct {
	Declared                     product.Value
	DeclaredMode                 RootAssignmentDeclaredMode
	HasDeclared                  bool
	SourceCarriesRuntimeIdentity bool
	// SourceCellExecutes separates control authority from abstract value
	// knowledge. A normal call-result phi cell executes even when its scalar
	// value is lattice-bottom; its structural path facts must still be assigned.
	SourceCellExecutes bool
	DefinitelyPresent  bool
}

// ComposeRootAssignmentSourceValue returns the exact value written by N4 and
// whether the assignment is productive. A declared contract is intentionally
// productive without a resolved runtime source; an ordinary assignment is not.
func ComposeRootAssignmentSourceValue(
	reg *axis.Registry,
	source product.Value,
	hasSource bool,
	composition RootAssignmentSourceComposition,
) (product.Value, bool) {
	if reg == nil || hasSource && !product.BelongsToRegistry(reg, source) || composition.HasDeclared && !product.BelongsToRegistry(reg, composition.Declared) {
		return product.Value{}, false
	}
	if hasSource && product.Equal(reg, source, product.Bottom(reg)) {
		hasSource = false
	}
	if composition.HasDeclared && composition.DeclaredMode == RootAssignmentDeclaredContract {
		value, ok := ComposeRootAssignmentDeclaredValue(
			reg, source, composition.Declared, RootAssignmentDeclaredContract,
			hasSource && composition.SourceCarriesRuntimeIdentity,
		)
		return value, ok
	}
	if !hasSource {
		if !composition.HasDeclared {
			if composition.SourceCellExecutes {
				return product.Bottom(reg), true
			}
			return product.Value{}, false
		}
		return composition.Declared, true
	}
	value := source
	if composition.DefinitelyPresent {
		value = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
	}
	if composition.HasDeclared && composition.DeclaredMode == RootAssignmentDeclaredOverlay {
		var ok bool
		value, ok = ComposeRootAssignmentDeclaredValue(
			reg, value, composition.Declared, RootAssignmentDeclaredOverlay,
			composition.SourceCarriesRuntimeIdentity,
		)
		if !ok {
			return product.Value{}, false
		}
	}
	return value, true
}
