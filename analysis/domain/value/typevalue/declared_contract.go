package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// DeclaredTypeFactsAlreadySatisfied reports whether the type-derived lanes of a
// declared contract are already present on value.
func DeclaredTypeFactsAlreadySatisfied(reg *axis.Registry, value, declared product.Value) bool {
	if !presence.Equal(presence.Join(product.PresenceOf(value), product.PresenceOf(declared)), product.PresenceOf(value)) {
		return false
	}
	return declaredTypeFactsNonPresenceSatisfied(reg, value, declared)
}

// DeclaredTypeFactsPresenceOnly reports the merged presence when the declared
// contract adds no type-derived evidence beyond presence.
func DeclaredTypeFactsPresenceOnly(reg *axis.Registry, value, declared product.Value) (presence.Value, bool) {
	if !declaredTypeFactsNonPresenceSatisfied(reg, value, declared) {
		return presence.Bottom(), false
	}
	return presence.Join(product.PresenceOf(value), product.PresenceOf(declared)), true
}

// TrustScalarRuntimeProofForDeclaredContract clears stale top-origin evidence
// when a scalar runtime-kind proof satisfies a declared contract. typevalue owns
// the evidence/runtime-kind lanes, so higher-level refiners can ask for the
// contract operation without importing those carrier axes directly.
func TrustScalarRuntimeProofForDeclaredContract(reg *axis.Registry, value, declared product.Value) product.Value {
	if !declaredContractSatisfiedByScalarRuntimeProof(reg, value, declared) {
		return value
	}
	return product.Set(reg, value, evidence.Key, evidence.Top())
}

func declaredTypeFactsNonPresenceSatisfied(reg *axis.Registry, value, declared product.Value) bool {
	declaredKind := product.Get(reg, declared, runtimekind.Key)
	if !declaredKind.IsTop() {
		valueKind := product.Get(reg, value, runtimekind.Key)
		if !runtimekind.Equal(runtimekind.Join(valueKind, declaredKind), valueKind) {
			return false
		}
	}
	declaredEvidence := product.Get(reg, declared, evidence.Key)
	if !evidence.Equal(declaredEvidence, evidence.Top()) {
		valueEvidence := product.Get(reg, value, evidence.Key)
		if !evidence.Equal(evidence.Join(valueEvidence, declaredEvidence), valueEvidence) {
			return false
		}
	}
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	return declaredWitness.IsBottom() ||
		declaredWitness.IsTop() ||
		declaredWitnessSatisfiedByPresence(reg, value, declared, declaredWitness)
}

func declaredWitnessSatisfiedByPresence(reg *axis.Registry, value, declared product.Value, declaredWitness typewitness.Value) bool {
	valueWitness := product.Get(reg, value, typewitness.Key)
	if typewitness.Equal(valueWitness, declaredWitness) {
		return true
	}
	valueType, valueOK := valueWitness.Type()
	declaredType, declaredOK := declaredWitness.Type()
	if !valueOK || !declaredOK {
		return false
	}
	desiredPresence := presence.Join(product.PresenceOf(value), product.PresenceOf(declared))
	return typ.TypeEquals(TypeWithPresence(valueType, desiredPresence), declaredType)
}

func declaredContractSatisfiedByScalarRuntimeProof(reg *axis.Registry, value, declared product.Value) bool {
	if reg == nil {
		return false
	}
	valueKinds := product.Get(reg, value, runtimekind.Key)
	if valueKinds.IsTop() || valueKinds.IsBottom() || !runtimeKindsAreScalar(valueKinds) {
		return false
	}
	declaredKinds := product.Get(reg, declared, runtimekind.Key)
	if declaredKinds.IsTop() {
		if declaredType, ok := WitnessOf(reg, declared); ok {
			if kindValue, ok := RuntimeKindFromType(declaredType); ok {
				declaredKinds = kindValue
			}
		}
	}
	return !declaredKinds.IsTop() && !declaredKinds.IsBottom() && declaredKinds.Covers(valueKinds)
}

func runtimeKindsAreScalar(kinds runtimekind.Value) bool {
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil, runtimekind.Boolean, runtimekind.Number, runtimekind.String:
		default:
			return false
		}
	}
	return true
}
