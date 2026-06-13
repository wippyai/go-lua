package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) checkRecord(sub, super *typ.Record, depth int) bool {
	for _, sf := range super.Fields {
		subField := sub.GetField(sf.Name)
		if subField == nil {
			if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subField.Type, subField.Optional, subField.Readonly, sf.Type, sf.Optional, sf.Readonly, depth+1) {
			return false
		}
	}
	for _, sm := range super.StaticMembers {
		subMember := sub.GetStaticMember(sm.Kind, sm.Name, sm.Index)
		if subMember == nil {
			if !sm.Optional && !unwrap.IsOptionalLike(sm.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subMember.Type, subMember.Optional, subMember.Readonly, sm.Type, sm.Optional, sm.Readonly, depth+1) {
			return false
		}
	}

	if super.HasMapComponent() {
		if !sub.HasMapComponent() {
			return false
		}
		if !c.check(sub.MapKey, super.MapKey, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.MapValue, depth+1) {
			return false
		}
	}
	return c.metaSubtype(sub.Metatable, super.Metatable, depth+1)
}

func (c *checker) checkRecordMember(subType typ.Type, subOptional, subReadonly bool, superType typ.Type, superOptional, superReadonly bool, depth int) bool {
	if superOptional && subType != nil && subType.Kind() == kind.Nil {
		return true
	}
	effectiveSuper := superType
	if superOptional {
		effectiveSuper = typeexpr.Optional(superType)
	}
	if superReadonly {
		if !c.check(subType, effectiveSuper, depth+1) {
			return false
		}
	} else {
		if subReadonly {
			return false
		}
		if !c.check(subType, effectiveSuper, depth+1) {
			return false
		}
		if !c.check(effectiveSuper, subType, depth+1) && !c.canWidenTo(subType, effectiveSuper, depth+1) {
			return false
		}
	}
	if !superOptional && !unwrap.IsOptionalLike(superType) && subOptional {
		return false
	}
	return true
}

func (c *checker) metaSubtype(subMT, superMT typ.Type, depth int) bool {
	if subMT == nil && superMT == nil {
		return true
	}
	subUnconstrained := subMT != nil && typetable.IsMetatableUnconstrained(subMT)
	superUnconstrained := superMT != nil && typetable.IsMetatableUnconstrained(superMT)
	if subUnconstrained && (superMT == nil || superUnconstrained) {
		return true
	}
	if superUnconstrained || (superMT != nil && typ.IsUnknown(superMT)) {
		return true
	}
	if subMT != nil && typ.IsUnknown(subMT) {
		return false
	}
	if subUnconstrained {
		return false
	}
	if subMT == nil || superMT == nil {
		return false
	}
	return c.check(subMT, superMT, depth)
}

func (c *checker) checkRecordToInterface(sub *typ.Record, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, method := range super.Methods {
		field := sub.GetField(method.Name)
		if field == nil {
			return false
		}
		methodType := subst.Self(method.Type, sub)
		if !c.check(field.Type, methodType, depth+1) {
			return false
		}
	}
	return true
}
