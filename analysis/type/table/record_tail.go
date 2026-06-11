package table

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RecordTailFieldType returns the tail type for a missing exact dot field on a
// Lua table record. Tail checks use overlap policy for the exact missing key;
// static read admission for arbitrary key types belongs in typeaccess.
func RecordTailFieldType(r *typ.Record, name string) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	if r.HasMapComponent() && MapComponentKeyMayContainString(r.MapKey, name) {
		return typ.NewOptional(r.MapValue), true
	}
	if r.Open {
		return typ.Unknown, true
	}
	return nil, false
}

// RecordTailStaticMemberType returns the tail type for a missing exact bracket
// member on a Lua table record. Tail checks use overlap policy for this exact
// member; static read admission for arbitrary key types belongs in typeaccess.
func RecordTailStaticMemberType(r *typ.Record, member typ.StaticMember) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	if r.HasMapComponent() && MapComponentKeyMayContainStaticMember(r.MapKey, member) {
		return typ.NewOptional(r.MapValue), true
	}
	if r.Open {
		return typ.Unknown, true
	}
	return nil, false
}

// RecordMapTailMayContainFieldName reports whether a record map component may
// contain the exact dot-field key under record-tail overlap policy.
func RecordMapTailMayContainFieldName(r *typ.Record, name string) bool {
	return r != nil && r.HasMapComponent() && MapComponentKeyMayContainString(r.MapKey, name)
}

// RecordMapTailMayContainStaticMember reports whether a record map component
// may contain the exact bracket member key under record-tail overlap policy.
func RecordMapTailMayContainStaticMember(r *typ.Record, member typ.StaticMember) bool {
	return r != nil && r.HasMapComponent() && MapComponentKeyMayContainStaticMember(r.MapKey, member)
}

// MapComponentKeyMayContainStaticMember reports whether a map-component key
// domain may include the exact bracket member key.
func MapComponentKeyMayContainStaticMember(key typ.Type, member typ.StaticMember) bool {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return MapComponentKeyMayContainString(key, member.Name)
	case typ.StaticMemberIntIndex:
		return MapComponentKeyMayContainInt(key, member.Index)
	default:
		return false
	}
}

// MapComponentKeyMayContainString reports whether a map-component key domain
// may include the string key.
func MapComponentKeyMayContainString(key typ.Type, name string) bool {
	if key == nil {
		return false
	}
	if typ.IsAny(key) || typ.IsUnknown(key) {
		return true
	}
	switch k := key.(type) {
	case *typ.Alias:
		return MapComponentKeyMayContainString(k.Target, name)
	case *typ.Union:
		for _, member := range k.Members {
			if MapComponentKeyMayContainString(member, name) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return k.Base == kind.String && k.Value == name
	default:
		return k.Kind() == kind.String
	}
}

// MapComponentKeyMayContainInt reports whether a map-component key domain may
// include the integer key.
func MapComponentKeyMayContainInt(key typ.Type, index int64) bool {
	if key == nil {
		return false
	}
	if typ.IsAny(key) || typ.IsUnknown(key) {
		return true
	}
	switch k := key.(type) {
	case *typ.Alias:
		return MapComponentKeyMayContainInt(k.Target, index)
	case *typ.Union:
		for _, member := range k.Members {
			if MapComponentKeyMayContainInt(member, index) {
				return true
			}
		}
		return false
	case *typ.Literal:
		switch k.Base {
		case kind.Integer:
			return k.Value == index
		case kind.Number:
			number, ok := k.Value.(float64)
			return ok && number == float64(index)
		default:
			return false
		}
	default:
		return k.Kind() == kind.Integer || k.Kind() == kind.Number
	}
}
