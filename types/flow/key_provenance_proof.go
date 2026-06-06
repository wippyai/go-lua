package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

type KeyProvenanceKind uint8

const (
	KeyProvenanceKeyedIteration KeyProvenanceKind = iota + 1
	KeyProvenanceKeyArrayAssignment
	KeyProvenanceIndexedKeyArrayIteration
	KeyProvenanceGuardedIndex
	KeyProvenanceDynamicIndexWrite
)

// KeyProvenancePathProof is the path-level publication form for facts of the
// form "this value is a key of that table". Flow owns the conversion to stable
// addresses and the cross-reductions among key-presence, key-array, and delayed
// index-write facts.
type KeyProvenancePathProof struct {
	Kind      KeyProvenanceKind
	TablePath constraint.Path
	ArrayPath constraint.Path
	KeyPath   constraint.Path
	ValuePath constraint.Path
}

// KeyProvenanceResult reports non-flow refinements derived while applying a
// key-provenance proof. Producers may consume these through their own policy
// layer; flow only computes the evidence.
type KeyProvenanceResult struct {
	KeyRefinementPath  constraint.Path
	KeyRefinementValue product.AbstractValue
}

// ApplyKeyProvenancePathProof applies a key-provenance proof from structured
// paths.
func ApplyKeyProvenancePathProof(out *PointState, proof KeyProvenancePathProof) (KeyProvenanceResult, bool) {
	if out == nil {
		return KeyProvenanceResult{}, false
	}
	switch proof.Kind {
	case KeyProvenanceKeyedIteration, KeyProvenanceGuardedIndex, KeyProvenanceDynamicIndexWrite:
		return applyKeyPresencePathProof(out, proof)
	case KeyProvenanceKeyArrayAssignment:
		return applyKeyArrayPathProof(out, proof)
	case KeyProvenanceIndexedKeyArrayIteration:
		return applyIndexedKeyArrayIterationPathProof(out, proof)
	default:
		return KeyProvenanceResult{}, false
	}
}

func applyKeyPresencePathProof(out *PointState, proof KeyProvenancePathProof) (KeyProvenanceResult, bool) {
	if proof.TablePath.IsEmpty() || proof.KeyPath.IsEmpty() {
		return KeyProvenanceResult{}, false
	}
	tableAddr, tableOK := StableAddressOfPath(proof.TablePath)
	keyAddr, keyOK := StableAddressOfPath(proof.KeyPath)
	if !tableOK || !keyOK {
		return KeyProvenanceResult{}, false
	}
	keyProof := KeyPresenceProof{
		Table: tableAddr,
		Key:   keyAddr,
	}
	if !proof.ValuePath.IsEmpty() {
		if valueAddr, valueOK := StableAddressOfPath(proof.ValuePath); valueOK {
			keyProof.ValuePath = valueAddr
			keyProof.HasValuePath = true
		}
	}
	return KeyProvenanceResult{}, ApplyKeyPresenceProof(out, keyProof)
}

func applyKeyArrayPathProof(out *PointState, proof KeyProvenancePathProof) (KeyProvenanceResult, bool) {
	if proof.ArrayPath.IsEmpty() || proof.TablePath.IsEmpty() {
		return KeyProvenanceResult{}, false
	}
	arrayAddr, arrayOK := StableAddressOfPath(proof.ArrayPath)
	tableAddr, tableOK := StableAddressOfPath(proof.TablePath)
	if !arrayOK || !tableOK {
		return KeyProvenanceResult{}, false
	}
	changed := ApplyKeyArrayProof(out, KeyArrayProof{
		Array: arrayAddr,
		Table: tableAddr,
	})
	return KeyProvenanceResult{}, changed
}

func applyIndexedKeyArrayIterationPathProof(out *PointState, proof KeyProvenancePathProof) (KeyProvenanceResult, bool) {
	if proof.ArrayPath.IsEmpty() || proof.KeyPath.IsEmpty() {
		return KeyProvenanceResult{}, false
	}
	arrayAddr, arrayOK := StableAddressOfPath(proof.ArrayPath)
	keyAddr, keyOK := StableAddressOfPath(proof.KeyPath)
	if !arrayOK || !keyOK {
		return KeyProvenanceResult{}, false
	}
	keyValue, _ := PointFactsOf(*out).AddressValue(keyAddr)
	iteration, changed := ApplyIndexedKeyArrayIterationProof(out, IndexedKeyArrayIterationProof{
		Array:    arrayAddr,
		Key:      keyAddr,
		KeyValue: keyValue,
	})
	result := KeyProvenanceResult{
		KeyRefinementPath:  proof.KeyPath,
		KeyRefinementValue: keyDomainFromIterationTables(out, iteration.Tables),
	}
	return result, changed
}

func keyDomainFromIterationTables(out *PointState, tables []StableAddress) product.AbstractValue {
	if out == nil {
		return product.AbstractValue{}
	}
	var keyDomain product.AbstractValue
	for _, tableAddr := range tables {
		if keyType, ok := PointFactsOf(*out).KeyDomainAtAddress(tableAddr); ok && !typ.IsAbsentOrUnknown(keyType) {
			av := product.FromType(keyType)
			if keyDomain.IsZero() {
				keyDomain = av
			} else {
				keyDomain = product.Join(keyDomain, av)
			}
		}
	}
	return keyDomain
}
