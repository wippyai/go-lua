package subtype

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) checkRecordToMap(sub *typ.Record, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key, depth+1) {
			return false
		}
		if !c.check(f.Type, super.Value, depth+1) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkRecordToReadonlyMap(sub *typ.Record, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(f.Type), super.Value, depth+1) {
			return false
		}
	}
	for _, m := range sub.StaticMembers {
		keyType, ok := readonlyStaticMemberKeyType(m)
		if !ok || !c.check(keyType, super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(m.Type), super.Value, depth+1) {
			return false
		}
	}
	if sub.Open || sub.Metatable != nil {
		if !c.check(typ.String, super.Key, depth+1) || !c.check(typ.Unknown, super.Value, depth+1) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(sub.MapValue), super.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkMapToRecord(sub *typ.Map, super *typ.Record, depth int) bool {
	if sub == nil || super == nil || !super.HasMapComponent() {
		return false
	}
	if !c.checkMap(sub, typetable.NewMap(super.MapKey, super.MapValue), depth+1) {
		return false
	}
	for _, sf := range super.Fields {
		if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
			return false
		}
		if !c.check(typ.LiteralString(sf.Name), sub.Key, depth+1) {
			continue
		}
		expected := sf.Type
		if sf.Optional && !unwrap.IsOptionalLike(expected) {
			expected = typ.NewOptional(expected)
		}
		if !c.check(sub.Value, expected, depth+1) {
			return false
		}
		if !c.check(expected, sub.Value, depth+1) && !c.canWidenTo(sub.Value, expected, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkArrayToMap(sub *typ.Array, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(typ.Integer, super.Key, depth+1) && c.check(sub.Element, super.Value, depth+1)
}

func (c *checker) checkTupleToReadonlyMap(sub *typ.Tuple, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for i, elem := range sub.Elements {
		if !c.check(typ.LiteralInt(int64(i+1)), super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(elem), super.Value, depth+1) {
			return false
		}
	}
	return true
}

func readonlyStaticMemberKeyType(member typ.StaticMember) (typ.Type, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return typ.LiteralString(member.Name), true
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index), true
	default:
		return nil, false
	}
}

func (c *checker) checkTupleToArray(sub *typ.Tuple, super *typ.Array, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Element, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkTupleToMap(sub *typ.Tuple, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	if !c.check(typ.Integer, super.Key, depth+1) {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Value, depth+1) {
			return false
		}
	}
	return true
}

func isTableLikeType(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isTableLikeType(v.UnaliasedTarget())
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && isTableLikeType(v.Body)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
}
