package ops

import (
	querycore "github.com/wippyai/go-lua/types/query/core"
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
	default:
		if slot, ok := querycore.IndexWrite(expected, key); ok {
			return slot
		}
		return nil
	}
}
