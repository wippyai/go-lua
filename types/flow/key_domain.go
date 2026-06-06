package flow

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// KeyDomainAtAddress returns the abstract key type accepted by the map-like
// value currently known at addr.
func (f PointFacts) KeyDomainAtAddress(addr StableAddress) (typ.Type, bool) {
	value, ok := f.AddressValue(addr)
	if !ok || value.IsZero() {
		return nil, false
	}
	keyType := KeyDomainFromProductValue(value)
	return keyType, keyType != nil && !typ.IsAbsentOrUnknown(keyType)
}

// KeyDomainFromProductValue projects the map/key domain from a product value.
func KeyDomainFromProductValue(value product.AbstractValue) typ.Type {
	if value.IsZero() {
		return nil
	}
	return KeyDomainFromType(value.ProjectValue())
}

// KeyDomainFromType derives the key type accepted by map-like structural types.
func KeyDomainFromType(t typ.Type) typ.Type {
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
			if key := KeyDomainFromType(member); key != nil && !typ.IsAbsentOrUnknown(key) {
				keys = append(keys, key)
			}
		}
		return typ.NewUnion(keys...)
	default:
		return nil
	}
}
