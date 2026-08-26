package tuple

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Batch is the one immutable replay boundary for tuple streams. A batch is
// not just a cofiber and a vector: every batch carries a private witness to
// the sealed arrangement Node that produced its exact range. The witness is
// intentionally not a RangeID/hash and cannot be minted by tuple callers.
// Empty extents retain that same proof.
type Batch struct {
	fence  binding.Fence
	mount  identity.ContentID
	scope  witness.Scope
	values []Tuple
	range_ rangeProof
	sealed bool
}

// rangeProof is private so callers can preserve a proof or redeem a new one
// only through the constructors below. keyValues is present only for Group
// and Merge ranges; it is the typed, mounted-algebra-checked key vector and
// remains ordered exactly as the arrangement key layout declares.
type rangeProof struct {
	authority arrangement.RangeBinding
	keys      []binding.ValueToken
	// witness is the exact denominator witness used to seal a Complete
	// range. Correlated replay receives a q-specific posting that is not
	// present in Mounted's global denominator catalogue.
	witness binding.DenominatorWitness
}

// NewRangeBatch admits an ordered tuple vector under an Input or Complete
// producer binding issued by arrangement.Node. Complete callers must supply
// the exact denominator witness (global or q-specific); non-Complete callers
// pass the zero witness. The opaque binding supplies all physical layout and
// range context.
func NewRangeBatch(mounted witness.Mounted, authority arrangement.RangeBinding, scope witness.Scope, values []Tuple, denominator binding.DenominatorWitness) (Batch, bool) {
	if !mounted.Available() || !authority.Available() || !scope.ValidFor(mounted.RuntimeFence()) || values == nil {
		return Batch{}, false
	}
	if authority.Kind() != algebra.KindInput && authority.Kind() != algebra.KindComplete {
		return Batch{}, false
	}
	return sealBatch(mounted, authority, scope, nil, values, denominator)
}

// NewKeyRangeBatch admits a Group or Merge key range. Key values are checked
// once against the exact mounted key layout and each type's admitted value
// algebra; the resulting proof is O(1) to redeem at downstream boundaries.
func NewKeyRangeBatch(mounted witness.Mounted, authority arrangement.RangeBinding, scope witness.Scope, keys []binding.ValueToken, values []Tuple) (Batch, bool) {
	if !mounted.Available() || !authority.Available() || !scope.ValidFor(mounted.RuntimeFence()) || keys == nil || values == nil {
		return Batch{}, false
	}
	if authority.Kind() != algebra.KindGroup && authority.Kind() != algebra.KindMerge {
		return Batch{}, false
	}
	if !validKeyVector(mounted, authority.Layout(), keys) {
		return Batch{}, false
	}
	return sealBatch(mounted, authority, scope, keys, values, binding.DenominatorWitness{})
}

// PreserveRange retains the exact producer proof while a Select/Project (or
// an oriented Join on its left range) changes the tuple vector or normalized
// scope. It does not accept a replacement authority, key, or denominator.
func PreserveRange(mounted witness.Mounted, source Batch, scope witness.Scope, values []Tuple) (Batch, bool) {
	if !source.ValidFor(mounted) || !scope.ValidFor(mounted.RuntimeFence()) || values == nil || !mountedScope(mounted, scope) {
		return Batch{}, false
	}
	return sealBatch(mounted, source.range_.authority, scope, source.range_.keys, values, source.range_.witness)
}

func sealBatch(mounted witness.Mounted, authority arrangement.RangeBinding, scope witness.Scope, keys []binding.ValueToken, values []Tuple, denominator binding.DenominatorWitness) (Batch, bool) {
	if !mounted.Available() || !authority.Available() || !scope.ValidFor(mounted.RuntimeFence()) || values == nil || !mountedScope(mounted, scope) {
		return Batch{}, false
	}
	if !authority.ValidFor(mounted.Fence()) {
		return Batch{}, false
	}
	if authority.Kind() == algebra.KindComplete {
		ref := authority.Denominator()
		if !denominator.Available() || !denominator.ValidFor(mounted.RuntimeFence()) || !denominator.Matches(ref) {
			return Batch{}, false
		}
	} else if denominator.Available() {
		// Input, Group, and Merge ranges have no Complete denominator
		// authority. A non-zero witness here would create a second, hidden
		// membership path instead of the single producer proof.
		return Batch{}, false
	}
	copyValues := append([]Tuple{}, values...)
	for _, value := range copyValues {
		if !value.ValidFor(mounted) || !value.Scope().Same(scope) {
			return Batch{}, false
		}
	}
	copyKeys := append([]binding.ValueToken(nil), keys...)
	if authority.Kind() == algebra.KindGroup || authority.Kind() == algebra.KindMerge {
		if !validKeyVector(mounted, authority.Layout(), copyKeys) {
			return Batch{}, false
		}
	} else if keys != nil {
		return Batch{}, false
	}
	result := Batch{
		fence: binding.Fence{}, mount: mounted.Digest(), scope: scope,
		values: copyValues, range_: rangeProof{authority: authority, keys: copyKeys, witness: denominator}, sealed: true,
	}
	result.fence = mounted.RuntimeFence()
	if !result.ValidFor(mounted) {
		return Batch{}, false
	}
	return result, true
}

func validKeyVector(mounted witness.Mounted, layout arrangement.Layout, keys []binding.ValueToken) bool {
	if !mounted.Available() || !layout.Available() || !layout.ValidFor(mounted.Fence()) || keys == nil {
		return false
	}
	columns := layout.KeyColumns()
	if len(columns) == 0 || len(keys) != len(columns) {
		return false
	}
	types := make(map[model.ColumnID]model.TypeID, len(mounted.Columns()))
	for _, schema := range mounted.Columns() {
		if !schema.Available() || !schema.Type().Available() {
			return false
		}
		types[schema.ID()] = schema.Type()
	}
	for index, column := range columns {
		typeID, ok := types[column]
		if !ok || !keys[index].Available() || keys[index].Type() != typeID || !keys[index].ValidFor(mounted.RuntimeFence()) {
			return false
		}
		// Redeem the key through the separately mounted semantic equality
		// authority. Equatable-only keys must not gain Join/Widen authority,
		// and an Ascending key used only here must not enter the state ascent
		// map merely to prove reflexive equality.
		if equality, equalityOK := mounted.Equality(typeID); !equalityOK || equality == nil || equality.Type() != typeID || !equality.Equal(keys[index], keys[index]) {
			return false
		}
	}
	return true
}

func mountedScope(mounted witness.Mounted, scope witness.Scope) bool {
	if !mounted.Available() || !scope.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	_, ok := mounted.ScopeToken(scope)
	return ok
}

// Available reports whether this immutable batch envelope is complete. It is
// deliberately O(1); constructors prove every nested tuple and key once.
func (batch Batch) Available() bool {
	return batch.sealed && batch.fence.Available() && batch.mount.Available() && batch.scope.ValidFor(batch.fence) && batch.values != nil && batch.range_.authority.Available()
}

// ValidFor redeems the sealed batch envelope against one exact mounted
// runtime and common cofiber in O(1). The private tuple vector and key vector
// are immutable constructor proofs, not repeated width work at each boundary.
func (batch Batch) ValidFor(mounted witness.Mounted) bool {
	if !batch.Available() || !mounted.Available() || batch.mount != mounted.Digest() || !batch.fence.Same(mounted.RuntimeFence()) || !batch.scope.ValidFor(mounted.RuntimeFence()) || !mountedScope(mounted, batch.scope) {
		return false
	}
	authority := batch.range_.authority
	if !authority.Available() || !authority.ValidFor(mounted.Fence()) {
		return false
	}
	if authority.Kind() == algebra.KindComplete {
		denominator := authority.Denominator()
		witnessValue := batch.range_.witness
		return witnessValue.Available() && witnessValue.ValidFor(mounted.RuntimeFence()) && witnessValue.Matches(denominator)
	}
	return authority.Kind() == algebra.KindInput || authority.Kind() == algebra.KindGroup || authority.Kind() == algebra.KindMerge
}

// Range returns the sealed producer authority without exposing the private
// tuple proof internals. Consumers that need to preserve it must use
// PreserveRange rather than constructing a replacement.
func (batch Batch) Range() arrangement.RangeBinding {
	if !batch.Available() {
		return arrangement.RangeBinding{}
	}
	return batch.range_.authority
}

// RangeKeys returns a defensive copy of typed Group/Merge key values. It is
// diagnostic projection only; callers cannot use it to mint another range.
func (batch Batch) RangeKeys() []binding.ValueToken {
	if !batch.Available() {
		return nil
	}
	return append([]binding.ValueToken(nil), batch.range_.keys...)
}

// DenominatorWitness returns the exact Complete witness retained by this
// batch. It is available for both ordinary global Complete ranges and
// correlated q-specific postings; callers must not reopen Mounted's global
// denominator map to recover it.
func (batch Batch) DenominatorWitness() (binding.DenominatorWitness, bool) {
	if !batch.Available() || batch.range_.authority.Kind() != algebra.KindComplete {
		return binding.DenominatorWitness{}, false
	}
	witnessValue := batch.range_.witness
	return witnessValue, witnessValue.Available() && witnessValue.Matches(batch.range_.authority.Denominator()) && witnessValue.ValidFor(batch.fence)
}

// Fence returns the exact runtime fence captured by construction.
func (batch Batch) Fence() binding.Fence {
	if !batch.Available() {
		return binding.Fence{}
	}
	return batch.fence
}

// Scope returns the one common normalized cofiber carried by this batch.
func (batch Batch) Scope() witness.Scope {
	if !batch.Available() {
		return witness.Scope{}
	}
	return batch.scope
}

// Len returns the ordered tuple count. It is zero for both an authenticated
// empty batch and an unavailable batch; callers use Available/ValidFor to
// distinguish those states.
func (batch Batch) Len() int {
	if !batch.Available() {
		return 0
	}
	return len(batch.values)
}

// At returns one tuple by stable authored order.
func (batch Batch) At(index int) (Tuple, bool) {
	if !batch.Available() || index < 0 || index >= len(batch.values) {
		return Tuple{}, false
	}
	return batch.values[index], true
}

// Tuples returns a defensive copy of the ordered vector.
func (batch Batch) Tuples() []Tuple {
	if !batch.Available() {
		return nil
	}
	return append([]Tuple(nil), batch.values...)
}

// Same compares exact immutable batch contents in authored order, including
// the producer witness and typed range key values.
func (batch Batch) Same(other Batch) bool {
	if !batch.sameEnvelope(other) || len(batch.range_.keys) != len(other.range_.keys) || len(batch.values) != len(other.values) {
		return false
	}
	for index := range batch.range_.keys {
		if !batch.range_.keys[index].Same(other.range_.keys[index]) {
			return false
		}
	}
	for index := range batch.values {
		if !batch.values[index].Same(other.values[index]) {
			return false
		}
	}
	return true
}

func (batch Batch) sameEnvelope(other Batch) bool {
	if !batch.Available() || !other.Available() || !batch.fence.Same(other.fence) || batch.mount != other.mount || !batch.scope.Same(other.scope) || batch.range_.authority.Producer() != other.range_.authority.Producer() || batch.range_.authority.Kind() != other.range_.authority.Kind() {
		return false
	}
	return true
}
