package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) checkRecordToMap(sub *typ.Record, super *typ.Map) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key) {
			return false
		}
		if !c.check(f.Type, super.Value) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key) {
			return false
		}
		if !c.check(sub.MapValue, super.Value) {
			return false
		}
	}
	return true
}

func (c *checker) checkRecordToReadonlyMap(sub *typ.Record, super *typ.ReadonlyMap) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(f.Type), super.Value) {
			return false
		}
	}
	for _, m := range sub.StaticMembers {
		keyType, ok := readonlyStaticMemberKeyType(m)
		if !ok || !c.check(keyType, super.Key) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(m.Type), super.Value) {
			return false
		}
	}
	if sub.Open || sub.Metatable != nil {
		if !c.check(typ.String, super.Key) || !c.check(typ.Unknown, super.Value) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(sub.MapValue), super.Value) {
			return false
		}
	}
	return true
}

func (c *checker) checkMapToRecord(sub *typ.Map, super *typ.Record) bool {
	if sub == nil || super == nil || !super.HasMapComponent() {
		return false
	}
	if !c.checkMap(sub, typetable.NewMap(super.MapKey, super.MapValue)) {
		return false
	}
	for _, sf := range super.Fields {
		if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
			return false
		}
		if !c.check(typ.LiteralString(sf.Name), sub.Key) {
			continue
		}
		expected := sf.Type
		if sf.Optional && !unwrap.IsOptionalLike(expected) {
			expected = typeexpr.Optional(expected)
		}
		if !c.check(sub.Value, expected) {
			return false
		}
		// A map value may read as a wider optional field, but mutable record
		// fields must not admit writes the source map value cannot hold.
		if !c.check(expected, sub.Value) && !c.canWidenTo(sub.Value, expected) {
			return false
		}
	}
	return true
}

func (c *checker) checkArrayToMap(sub *typ.Array, super *typ.Map) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(typ.Integer, super.Key) && c.check(sub.Element, super.Value)
}

func (c *checker) checkTupleToReadonlyMap(sub *typ.Tuple, super *typ.ReadonlyMap) bool {
	if sub == nil || super == nil {
		return false
	}
	for i, elem := range sub.Elements {
		if !c.check(typ.LiteralInt(int64(i+1)), super.Key) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(elem), super.Value) {
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

func (c *checker) checkTupleToArray(sub *typ.Tuple, super *typ.Array) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Element) {
			return false
		}
	}
	return true
}

func (c *checker) checkTupleToMap(sub *typ.Tuple, super *typ.Map) bool {
	if sub == nil || super == nil {
		return false
	}
	if !c.check(typ.Integer, super.Key) {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Value) {
			return false
		}
	}
	return true
}

func checkTableTop(sub, super typ.Type) (bool, bool) {
	if !typ.IsBuiltinTableTopMarker(super) {
		return false, false
	}
	return typetable.IsLike(sub), true
}

func emptyRecordAdoptsContainerShape(sub *typ.Record, super typ.Type) bool {
	if sub == nil || super == nil || len(sub.Fields) != 0 || len(sub.StaticMembers) != 0 {
		return false
	}
	return super.Kind() == kind.Array || super.Kind() == kind.Map || super.Kind() == kind.ReadonlyMap
}
