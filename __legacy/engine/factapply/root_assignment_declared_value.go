package factapply

import (
	valueref "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// RootAssignmentDeclaredMode identifies the two distinct declaration
// operations carried by factflow.RootAssignment. A contract replaces the
// runtime value while retaining an eligible allocation identity; an overlay
// merges the declared claim onto the value produced by the source expression.
type RootAssignmentDeclaredMode uint8

const (
	RootAssignmentDeclaredInvalid RootAssignmentDeclaredMode = iota
	RootAssignmentDeclaredContract
	RootAssignmentDeclaredOverlay
)

// ComposeRootAssignmentDeclaredValue is the context-free value operation
// shared by concrete root transfer and the symbolic relation compiler. It is
// deliberately not a cast or a meet: the assertion, presence, witness, and
// runtime-identity rules are the root-assignment semantics themselves.
//
// sourceCarriesRuntimeIdentity is the factflow authority that the declaration
// source is an expression whose runtime allocation may be retained by a
// declared record/table contract.
func ComposeRootAssignmentDeclaredValue(
	reg *axis.Registry,
	source product.Value,
	declared product.Value,
	mode RootAssignmentDeclaredMode,
	sourceCarriesRuntimeIdentity bool,
) (product.Value, bool) {
	if reg == nil || !product.BelongsToRegistry(reg, source) || !product.BelongsToRegistry(reg, declared) {
		return product.Value{}, false
	}
	switch mode {
	case RootAssignmentDeclaredContract:
		if !sourceCarriesRuntimeIdentity || !RootAssignmentDeclaredContractNeedsSourceRuntimeIdentity(reg, declared) {
			return declared, true
		}
		id, ok := product.Get(reg, source, identity.Key).ID()
		if !ok || id == (identity.ID{}) {
			return declared, true
		}
		return product.Set(reg, declared, identity.Key, identity.Singleton(id)), true

	case RootAssignmentDeclaredOverlay:
		value := valueref.MergeDeclaredContract(reg, source, declared)
		if declaredClaim := product.Get(reg, declared, assertion.Key); !declaredClaim.IsBottom() && !declaredClaim.IsTop() {
			currentClaim := product.Get(reg, value, assertion.Key)
			value = product.Set(reg, value, assertion.Key, assertion.Combine(currentClaim, declaredClaim))
		}
		if declaredPresence := product.PresenceOf(declared); !declaredPresence.IsBottom() && !declaredPresence.IsTop() {
			value = product.WithPresence(reg, value, declaredPresence)
		}
		return value, true

	default:
		return product.Value{}, false
	}
}

// RootAssignmentDeclaredContractNeedsSourceRuntimeIdentity reports whether
// the concrete transaction is permitted and required to consult the source
// solely to retain its allocation identity. Callers use it to preserve the
// declaration transfer's lazy source-read contract.
func RootAssignmentDeclaredContractNeedsSourceRuntimeIdentity(reg *axis.Registry, declared product.Value) bool {
	if reg == nil || !product.BelongsToRegistry(reg, declared) {
		return false
	}
	if id, ok := product.Get(reg, declared, identity.Key).ID(); ok && id != (identity.ID{}) {
		return false
	}
	return declaredContractCanCarrySourceRuntimeIdentity(reg, declared)
}

func declaredContractCanCarrySourceRuntimeIdentity(reg *axis.Registry, declared product.Value) bool {
	t, ok := typevalue.TypeOf(reg, declared)
	if !ok || t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Record:
		return true
	default:
		return typ.IsBuiltinTableTopMarker(t)
	}
}
