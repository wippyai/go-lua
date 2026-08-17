package engine

import "github.com/wippyai/go-lua/analysis/identity"

// Read is the typed, positional capability issued by SchemaBinding.
// Its origin is sealed to one binding state and Rule ordinal; no declaration
// object or cold execution carrier is retained here.
type Read[S any] struct {
	origin  *schemaRuleReadOrigin
	index   int
	resolve func(*productSession, int, uint64) (S, bool)
}

func (read Read[S]) matchesRuntimeOwner(owner interface{ runtimeRuleProof() *ruleRuntimeProof }) bool {
	if owner == nil || read.index < 0 || read.resolve == nil || read.origin == nil {
		return false
	}
	return read.origin.matches(owner.runtimeRuleProof(), uint64(read.index))
}

func (read Read[S]) matchesRuleProof(proof *ruleRuntimeProof) bool {
	return proof != nil && read.index >= 0 && read.resolve != nil && read.origin != nil && read.origin.matches(proof, uint64(read.index))
}

// Access is the typed execution frame issued by receipt assembly. It
// carries no declaration callback or cold composition capability.
type Access[V, O any] struct {
	execution *ruleExecution
	owner     *boundRule[V, O]
	epoch     identity.Generation
	output    outputAccess[V]
}

// ActivationCoordinates is the exact accepted dynamic relation that
// materialized one Rule row. Ordinary Rules have no coordinates.
type ActivationCoordinates struct {
	binding     identity.SemanticKey
	application identity.SemanticKey
	target      identity.SemanticKey
	endpoint    identity.SemanticKey
}

func (value ActivationCoordinates) Available() bool {
	return value.binding.Available() && value.application.Available() && value.target.Available() && value.endpoint.Available()
}
