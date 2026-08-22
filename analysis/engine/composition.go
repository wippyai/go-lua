package engine

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// CompositionID is the canonical identity of a sealed Schema. It is retained
// as composition provenance; no declaration root or cold execution owner exists in
// the engine package.
type CompositionID struct{ digest [32]byte }

func (id CompositionID) Available() bool  { return id.digest != [32]byte{} }
func (id CompositionID) Digest() [32]byte { return id.digest }

// Schema is the immutable cold grammar consumed by SchemaBinding.
type Schema struct {
	cold      *coldcomposition.Composition
	id        CompositionID
	available bool
}

// Available reports whether this Schema is a sealed cold grammar. SchemaBuilder
// Seal is the sole issuer and seals the verdict over the canonicalized
// composition; the zero Schema is unavailable.
func (schema *Schema) Available() bool { return schema != nil && schema.available }

func (schema *Schema) completeGrammar() bool {
	return schema != nil && schema.cold != nil && schema.id.Available()
}
func (schema *Schema) ID() CompositionID {
	if !schema.Available() {
		return CompositionID{}
	}
	return schema.id
}
func (schema *Schema) coldID() composition.ID {
	if !schema.Available() {
		return composition.ID{}
	}
	return schema.cold.ID()
}
func (schema *Schema) shapeCount() (factors, rules, queries, activations int, ok bool) {
	if !schema.Available() {
		return 0, 0, 0, 0, false
	}
	return schema.cold.ShapeCount()
}
func (schema *Schema) factorCount() uint64 {
	n, _, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return uint64(n)
}
func (schema *Schema) ruleCount() uint64 {
	_, n, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return uint64(n)
}
func (schema *Schema) activationCount() uint64 {
	_, _, _, n, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return uint64(n)
}
func (schema *Schema) factorFormCount(index uint64) (int, bool) {
	if !schema.Available() {
		return 0, false
	}
	return schema.cold.FactorFormCount(index)
}
func (schema *Schema) factorFormShapeAt(factor, form uint64) (coldcomposition.FactorFormShape, bool) {
	if !schema.Available() {
		return coldcomposition.FactorFormShape{}, false
	}
	return schema.cold.FactorFormShapeAt(factor, form)
}
func (schema *Schema) factorSemanticAt(index uint64) composition.Key {
	if !schema.Available() {
		return composition.Key{}
	}
	return schema.cold.FactorKeyAt(index)
}
func (schema *Schema) factorOrdinalOf(key composition.Key) (uint64, bool) {
	if !schema.Available() || !key.Available() {
		return 0, false
	}
	return schema.cold.FactorIndex(key)
}
func (schema *Schema) activationOrdinalOf(key composition.Key) (uint64, bool) {
	if !schema.Available() || !key.Available() {
		return 0, false
	}
	return schema.cold.ActivationIndex(key)
}
func (schema *Schema) ruleShapeAt(index uint64) (coldcomposition.RuleShape, bool) {
	if !schema.Available() {
		return coldcomposition.RuleShape{}, false
	}
	return schema.cold.RuleShapeAt(index)
}
func (schema *Schema) ruleSemanticAt(index uint64) composition.Key {
	if !schema.Available() {
		return composition.Key{}
	}
	return schema.cold.RuleKeyAt(index)
}
func (schema *Schema) ruleOrdinalOf(key composition.Key) (uint64, bool) {
	if !schema.Available() || !key.Available() {
		return 0, false
	}
	return schema.cold.RuleIndex(key)
}
func (schema *Schema) querySemanticAt(index uint64) composition.Key {
	if !schema.Available() {
		return composition.Key{}
	}
	return schema.cold.QueryKeyAt(index)
}
func (schema *Schema) queryOrdinalOf(key composition.Key) (uint64, bool) {
	if !schema.Available() || !key.Available() {
		return 0, false
	}
	return schema.cold.QueryIndex(key)
}
func (schema *Schema) queryShapeAt(index uint64) (coldcomposition.QueryShape, bool) {
	if !schema.Available() {
		return coldcomposition.QueryShape{}, false
	}
	return schema.cold.QueryShapeAt(index)
}
func (schema *Schema) queryProjectionShapeAt(query, projection uint64) (coldcomposition.QueryProjectionShape, bool) {
	if !schema.Available() {
		return coldcomposition.QueryProjectionShape{}, false
	}
	return schema.cold.QueryProjectionShapeAt(query, projection)
}
func (schema *Schema) ruleReadShapeAt(rule, read uint64) (coldcomposition.RuleReadShape, bool) {
	if !schema.Available() {
		return coldcomposition.RuleReadShape{}, false
	}
	return schema.cold.RuleReadShapeAt(rule, read)
}
func (schema *Schema) ruleReadDependencyAt(rule, read, dependency uint64) (uint64, bool) {
	if !schema.Available() {
		return 0, false
	}
	return schema.cold.RuleReadDependencyAt(rule, read, dependency)
}
func (schema *Schema) ruleCarryShapeAt(rule, carry uint64) (coldcomposition.RuleCarryShape, bool) {
	if !schema.Available() {
		return coldcomposition.RuleCarryShape{}, false
	}
	return schema.cold.RuleCarryShapeAt(rule, carry)
}
func (schema *Schema) ruleWriteShapeAt(rule, write uint64) (coldcomposition.RuleWriteShape, bool) {
	if !schema.Available() {
		return coldcomposition.RuleWriteShape{}, false
	}
	return schema.cold.RuleWriteShapeAt(rule, write)
}

// Solver is the runtime owner of one sealed program. Activation revisions
// publish structural overlays over that immutable program; they never rebuild
// its factors, members, queries, observations, or carrier.
type Solver struct {
	mu      sync.Mutex
	runtime *solverRuntime
	// store is this Solver's live store identity. It is issued once at
	// compilation and never reused, so every address a completed State hands out
	// names exactly this Solver and is meaningless in any other.
	store identity.StoreID
	// relation is this Solver's one published activation relation: the accepted
	// Members, the structural digest derived at that publication, and the
	// Generation stamp every completed State is fenced against. Accepting a
	// frontier replaces it atomically; nothing advances the stamp separately.
	relation equation.Relation
	// completion is the publication stamp of installed results. It fences a
	// different lifetime than relation: several completions may be published
	// within one activation relation.
	completion identity.Generation
	// lastSolved is the last sealed generation this store published. A later
	// completion derives a NewDelta from it instead of resealing every column.
	lastSolved SolvedSnapshot
}
