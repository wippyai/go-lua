package transfer

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type KeyProvenanceKind uint8

const (
	KeyProvenanceKeyedIteration KeyProvenanceKind = iota + 1
	KeyProvenanceKeyArrayAssignment
	KeyProvenanceIndexedKeyArrayIteration
	KeyProvenanceGuardedIndex
	KeyProvenanceDynamicIndexWrite
)

// KeyProvenanceEffect is the reducer payload for facts of the form
// "this value is a key of that table". It owns the KeyPresence axis and the
// value refinement that follows when an indexed iteration consumes a key-array
// proof.
type KeyProvenanceEffect struct {
	Kind      KeyProvenanceKind
	TablePath constraint.Path
	ArrayPath constraint.Path
	KeyPath   constraint.Path
	ValuePath constraint.Path
}

func (t *Transfer) applyKeyProvenanceEffect(out *flow.PointState, effect KeyProvenanceEffect) bool {
	if out == nil {
		return false
	}
	switch effect.Kind {
	case KeyProvenanceKeyedIteration, KeyProvenanceGuardedIndex, KeyProvenanceDynamicIndexWrite:
		if effect.TablePath.IsEmpty() || effect.KeyPath.IsEmpty() {
			return false
		}
		changed := false
		tableAddr, tableOK := flow.StableAddressOfPath(effect.TablePath)
		keyAddr, keyOK := flow.StableAddressOfPath(effect.KeyPath)
		if tableOK && keyOK {
			proof := flow.KeyPresenceProof{
				Table: tableAddr,
				Key:   keyAddr,
			}
			if !effect.ValuePath.IsEmpty() {
				if valueAddr, valueOK := flow.StableAddressOfPath(effect.ValuePath); valueOK {
					proof.ValuePath = valueAddr
					proof.HasValuePath = true
				}
			}
			changed = flow.ApplyKeyPresenceProof(out, proof) || changed
		}
		return changed
	case KeyProvenanceKeyArrayAssignment:
		if effect.ArrayPath.IsEmpty() || effect.TablePath.IsEmpty() {
			return false
		}
		arrayAddr, arrayOK := flow.StableAddressOfPath(effect.ArrayPath)
		tableAddr, tableOK := flow.StableAddressOfPath(effect.TablePath)
		if !arrayOK || !tableOK {
			return false
		}
		return flow.ApplyKeyArrayProof(out, flow.KeyArrayProof{
			Array: arrayAddr,
			Table: tableAddr,
		})
	case KeyProvenanceIndexedKeyArrayIteration:
		if effect.ArrayPath.IsEmpty() || effect.KeyPath.IsEmpty() {
			return false
		}
		arrayAddr, arrayOK := flow.StableAddressOfPath(effect.ArrayPath)
		keyAddr, keyOK := flow.StableAddressOfPath(effect.KeyPath)
		if !arrayOK || !keyOK {
			return false
		}
		var keyValue product.AbstractValue
		tableKeys, changed := flow.ApplyIndexedKeyArrayIterationProof(out, arrayAddr, keyAddr)
		for _, tableKey := range tableKeys {
			tableAddr, ok := flow.StableAddressFromKey(tableKey)
			if !ok {
				continue
			}
			if keyType, ok := flow.PointFactsOf(*out).KeyDomainAtAddress(tableAddr); ok {
				av := product.FromType(keyType)
				if keyValue.IsZero() {
					keyValue = av
				} else {
					keyValue = product.Join(keyValue, av)
				}
			}
		}
		if !keyValue.IsZero() && effect.KeyPath.Symbol != 0 {
			t.applyRefinementEffect(out, RefinementEffect{
				Place: Place{Root: effect.KeyPath.Symbol},
				Kind:  RefinementSetValue,
				Value: keyValue,
			})
			changed = true
		}
		return changed
	default:
		return false
	}
}
