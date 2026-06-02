package ops

import (
	"github.com/wippyai/go-lua/types/constraint"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// ExpectedTableElementType returns the contextual expected type for the element
// at index in a table literal used in array-like position.
func ExpectedTableElementType(expected typ.Type, index int) typ.Type {
	return expectedTableKeyType(expected, typ.LiteralInt(int64(index+1)))
}

// ExpectedTableFieldType returns the contextual expected type for a named
// field in a table literal. It uses write-side slot projection: table literals
// provide a value for the key, while ordinary reads may be nilable because Lua
// returns nil for absent keys.
func ExpectedTableFieldType(expected typ.Type, name string) typ.Type {
	return expectedTableKeyType(expected, typ.LiteralString(name))
}

// ExpectedTableEntryType returns the contextual expected type for a structural
// table constructor entry. Dot fields may satisfy record fields; exact bracket
// keys may satisfy static members or map/array tails, but they do not satisfy a
// closed record field merely because the string is equal.
func ExpectedTableEntryType(expected typ.Type, key constraint.Segment) typ.Type {
	switch key.Kind {
	case constraint.SegmentField:
		return ExpectedTableFieldType(expected, key.Name)
	case constraint.SegmentIndexString:
		return expectedTableIndexedKeyType(expected, typ.LiteralString(key.Name), key)
	case constraint.SegmentIndexInt:
		return expectedTableIndexedKeyType(expected, typ.LiteralInt(int64(key.Index)), key)
	default:
		return nil
	}
}

func expectedTableKeyType(expected typ.Type, key typ.Type) typ.Type {
	if expected == nil {
		return nil
	}

	expected = typ.UnwrapAnnotated(expected)

	switch v := expected.(type) {
	case *typ.Alias:
		return expectedTableKeyType(v.Target, key)
	case *typ.Optional:
		return expectedTableKeyType(v.Inner, key)
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return expectedTableKeyType(resolved, key)
		}
		return nil
	case *typ.Union:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableKeyType(member, key); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableKeyType(member, key); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewIntersection(members...)
	case *typ.ReadonlyMap:
		if subtype.IsSubtype(key, v.Key) {
			return v.Value
		}
		return nil
	default:
		if slot, ok := querycore.IndexWrite(expected, key); ok {
			return slot
		}
		return nil
	}
}

func expectedTableIndexedKeyType(expected typ.Type, keyType typ.Type, key constraint.Segment) typ.Type {
	if expected == nil {
		return nil
	}
	expected = typ.UnwrapAnnotated(expected)
	switch v := expected.(type) {
	case *typ.Alias:
		return expectedTableIndexedKeyType(v.Target, keyType, key)
	case *typ.Optional:
		return expectedTableIndexedKeyType(v.Inner, keyType, key)
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return expectedTableIndexedKeyType(resolved, keyType, key)
		}
		return nil
	case *typ.Union:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableIndexedKeyType(member, keyType, key); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableIndexedKeyType(member, keyType, key); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewIntersection(members...)
	case *typ.Record:
		if member := expectedStaticMember(v, key); member != nil {
			return member.Type
		}
		if v.HasMapComponent() && subtype.IsSubtype(keyType, v.MapKey) {
			return v.MapValue
		}
		return nil
	case *typ.ReadonlyMap:
		if subtype.IsSubtype(keyType, v.Key) {
			return v.Value
		}
		return nil
	default:
		if slot, ok := querycore.IndexWrite(expected, keyType); ok {
			return slot
		}
		return nil
	}
}

func expectedStaticMember(rec *typ.Record, key constraint.Segment) *typ.StaticMember {
	if rec == nil {
		return nil
	}
	switch key.Kind {
	case constraint.SegmentIndexString:
		return rec.GetStaticStringIndex(key.Name)
	case constraint.SegmentIndexInt:
		return rec.GetStaticIntIndex(int64(key.Index))
	default:
		return nil
	}
}
