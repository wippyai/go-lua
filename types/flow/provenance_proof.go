package flow

import "github.com/wippyai/go-lua/types/constraint"

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
