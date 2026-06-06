package transfer

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
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
			if keyType, ok := t.keyDomainForPathKey(out, tableKey); ok {
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

func (t *Transfer) keyDomainForPathKey(out *flow.PointState, key constraint.PathKey) (typ.Type, bool) {
	sym, segments, ok := flow.ParseSymbolPathKey(key)
	if !ok {
		return nil, false
	}
	base, ok := t.symbolValue(out, sym)
	if !ok || base.IsZero() {
		return nil, false
	}
	if len(segments) > 0 {
		base, ok = productMemberPathValue(base, segments)
		if !ok || base.IsZero() {
			return nil, false
		}
	}
	keyType := keyDomainFromProductValue(base)
	return keyType, keyType != nil && !typ.IsAbsentOrUnknown(keyType)
}

func keyDomainFromProductValue(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	return keyDomainFromType(av.ProjectValue())
}

func keyDomainFromType(t typ.Type) typ.Type {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Map:
		return v.Key
	case *typ.ReadonlyMap:
		return v.Key
	case *typ.Record:
		keys := make([]typ.Type, 0, len(v.Fields)+1)
		if v.HasMapComponent() {
			keys = append(keys, v.MapKey)
		}
		if v.Open && !v.HasMapComponent() {
			keys = append(keys, typ.String)
		} else {
			for _, field := range v.Fields {
				keys = append(keys, typ.LiteralString(field.Name))
			}
		}
		return typ.NewUnion(keys...)
	case *typ.Union:
		keys := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if key := keyDomainFromType(member); key != nil && !typ.IsAbsentOrUnknown(key) {
				keys = append(keys, key)
			}
		}
		return typ.NewUnion(keys...)
	default:
		return nil
	}
}
