package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// PathAliasProof records point-local identity provenance between two stable
// addresses.
type PathAliasProof struct {
	Value  StableAddress
	Source StableAddress
}

// PathAliasPathProof is the structured-path form of PathAliasProof.
type PathAliasPathProof struct {
	ValuePath  constraint.Path
	SourcePath constraint.Path
}

// ValueOriginProof records semantic value provenance between two stable
// addresses.
type ValueOriginProof struct {
	Value    StableAddress
	Source   StableAddress
	Kind     ValueOriginKind
	VarIndex int
}

// ValueOriginPathProof is the path-level form of ValueOriginProof. It keeps the
// stable-address conversion in flow so producer layers publish provenance with
// structured paths instead of rebuilding fact keys.
type ValueOriginPathProof struct {
	ValuePath  constraint.Path
	SourcePath constraint.Path
	Kind       ValueOriginKind
	VarIndex   int
}

// AssignmentAliasTransaction applies the reduced-product consequences of one
// local assignment where Target now denotes Source. It owns the fan-out into
// alias, value-origin, key-presence, and index-readback facts.
type AssignmentAliasTransaction struct {
	Target      StableAddress
	Source      StableAddress
	SourceValue product.AbstractValue
}

// AssignmentAliasPathTransaction is the source-facing entry point for assignment
// alias publication. Flow lowers paths once, then applies the address transaction.
type AssignmentAliasPathTransaction struct {
	TargetPath  constraint.Path
	SourcePath  constraint.Path
	SourceValue product.AbstractValue
}

// ArrayElementKeyTransaction applies the reduced-product consequences of
// assigning an indexed key-array element into TargetKey.
type ArrayElementKeyTransaction struct {
	TargetKey StableAddress
	Array     StableAddress
	KeyValue  product.AbstractValue
}

// ArrayElementKeyPathTransaction is the source-facing entry point for indexed
// key-array element assignment provenance.
type ArrayElementKeyPathTransaction struct {
	TargetPath constraint.Path
	ArrayPath  constraint.Path
	KeyValue   product.AbstractValue
}

// ApplyPathAliasProof applies identity alias provenance to point state.
func ApplyPathAliasProof(out *PointState, proof PathAliasProof) bool {
	if out == nil || proof.Value.Key() == "" || proof.Source.Key() == "" {
		return false
	}
	before := out.PathAliases
	out.PathAliases = out.PathAliases.WithAddresses(proof.Value, proof.Source)
	return !PathAliasFactsDomain.Equal(before, out.PathAliases)
}

// ApplyPathAliasPathProof applies identity alias provenance from structured
// paths.
func ApplyPathAliasPathProof(out *PointState, proof PathAliasPathProof) bool {
	if out == nil || proof.ValuePath.IsEmpty() || proof.SourcePath.IsEmpty() {
		return false
	}
	valueAddr, ok := StableAddressOfPath(proof.ValuePath)
	if !ok {
		return false
	}
	sourceAddr, ok := StableAddressOfPath(proof.SourcePath)
	if !ok {
		return false
	}
	return ApplyPathAliasProof(out, PathAliasProof{
		Value:  valueAddr,
		Source: sourceAddr,
	})
}

// ApplyValueOriginProof applies semantic value-origin provenance to point state.
func ApplyValueOriginProof(out *PointState, proof ValueOriginProof) bool {
	if out == nil || proof.Value.Key() == "" || proof.Source.Key() == "" || proof.Kind == 0 {
		return false
	}
	before := out.ValueOrigins
	out.ValueOrigins = out.ValueOrigins.WithAddresses(proof.Value, proof.Source, proof.Kind, proof.VarIndex)
	return !ValueOriginFactsDomain.Equal(before, out.ValueOrigins)
}

// ApplyValueOriginPathProof applies semantic value-origin provenance from
// structured paths.
func ApplyValueOriginPathProof(out *PointState, proof ValueOriginPathProof) bool {
	if out == nil || proof.ValuePath.IsEmpty() || proof.SourcePath.IsEmpty() || proof.Kind == 0 {
		return false
	}
	valueAddr, ok := StableAddressOfPath(proof.ValuePath)
	if !ok {
		return false
	}
	sourceAddr, ok := StableAddressOfPath(proof.SourcePath)
	if !ok {
		return false
	}
	return ApplyValueOriginProof(out, ValueOriginProof{
		Value:    valueAddr,
		Source:   sourceAddr,
		Kind:     proof.Kind,
		VarIndex: proof.VarIndex,
	})
}

// ApplyAssignmentAliasPathTransaction lowers a source-level assignment alias once
// before applying all reduced-product consequences in address space.
func ApplyAssignmentAliasPathTransaction(out *PointState, tx AssignmentAliasPathTransaction) bool {
	if out == nil || tx.TargetPath.IsEmpty() || tx.SourcePath.IsEmpty() {
		return false
	}
	target, targetOK := StableAddressOfPath(tx.TargetPath)
	source, sourceOK := StableAddressOfPath(tx.SourcePath)
	if !targetOK || !sourceOK {
		return false
	}
	sourceValue := tx.SourceValue
	if sourceValue.IsZero() {
		if value, ok := PointFactsOf(*out).AddressValue(source); ok && !value.IsZero() {
			sourceValue = value
		}
	}
	return ApplyAssignmentAliasTransaction(out, AssignmentAliasTransaction{
		Target:      target,
		Source:      source,
		SourceValue: sourceValue,
	})
}

// ApplyAssignmentAliasTransaction applies all facts implied by a local alias
// assignment. Self/ancestor aliases are not published as identity/value-origin
// facts, but key-presence and readback aliases are still replayed.
func ApplyAssignmentAliasTransaction(out *PointState, tx AssignmentAliasTransaction) bool {
	if out == nil || tx.Target.Key() == "" || tx.Source.Key() == "" {
		return false
	}
	changed := false
	if !tx.Target.Overlaps(tx.Source) {
		changed = ApplyPathAliasProof(out, PathAliasProof{
			Value:  tx.Target,
			Source: tx.Source,
		}) || changed
		if assignmentAliasOriginAdmissible(tx.SourceValue) {
			changed = ApplyValueOriginProof(out, ValueOriginProof{
				Value:  tx.Target,
				Source: tx.Source,
				Kind:   ValueOriginAssignmentAlias,
			}) || changed
		}
	}
	changed = ApplyKeyPresenceAliasProof(out, KeyPresenceAliasProof{
		SourceKey: tx.Source,
		TargetKey: tx.Target,
	}) || changed
	sourceValue := tx.SourceValue
	if sourceValue.IsZero() {
		sourceValue = product.FromType(typ.Unknown)
	}
	changed = ApplyIndexWriteKeyAliasReadbackProof(out, IndexWriteKeyAliasReadbackProof{
		SourceKey:   tx.Source,
		TargetKey:   tx.Target,
		SourceValue: sourceValue,
	}) || changed
	return changed
}

func assignmentAliasOriginAdmissible(value product.AbstractValue) bool {
	if value.IsZero() {
		return false
	}
	t := product.ProjectValueOrUnknown(value)
	return !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

// ApplyArrayElementKeyPathTransaction lowers source paths once before applying
// key-array and value-origin consequences in address space.
func ApplyArrayElementKeyPathTransaction(out *PointState, tx ArrayElementKeyPathTransaction) bool {
	if out == nil || tx.TargetPath.IsEmpty() || tx.ArrayPath.IsEmpty() {
		return false
	}
	target, targetOK := StableAddressOfPath(tx.TargetPath)
	array, arrayOK := StableAddressOfPath(tx.ArrayPath)
	if !targetOK || !arrayOK {
		return false
	}
	return ApplyArrayElementKeyTransaction(out, ArrayElementKeyTransaction{
		TargetKey: target,
		Array:     array,
		KeyValue:  tx.KeyValue,
	})
}

// ApplyArrayElementKeyTransaction applies all facts implied by assigning an
// indexed key-array element into a local key variable.
func ApplyArrayElementKeyTransaction(out *PointState, tx ArrayElementKeyTransaction) bool {
	if out == nil || tx.TargetKey.Key() == "" || tx.Array.Key() == "" {
		return false
	}
	_, presenceChanged := ApplyKeyArrayElementKeyProof(out, KeyArrayElementKeyProof{
		Array:     tx.Array,
		TargetKey: tx.TargetKey,
		KeyValue:  tx.KeyValue,
	})
	changed := presenceChanged
	changed = ApplyValueOriginProof(out, ValueOriginProof{
		Value:    tx.TargetKey,
		Source:   tx.Array,
		Kind:     ValueOriginIndexedIterator,
		VarIndex: 1,
	}) || changed
	return changed
}
