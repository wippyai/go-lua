package subtype

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (c *checker) checkArray(sub, super *typ.Array) bool {
	return c.check(sub.Element, super.Element)
}

func (c *checker) checkMap(sub, super *typ.Map) bool {
	if !c.check(sub.Key, super.Key) || !c.check(super.Key, sub.Key) {
		return false
	}
	if !c.check(sub.Value, super.Value) {
		return false
	}
	// A map is mutable, so its value is invariant: widening the value type (for
	// example to any) would let a write through the alias store a value the
	// original map's type forbids. Covariant read-only access uses ReadonlyMap.
	return c.check(super.Value, sub.Value)
}

func (c *checker) checkReadonlyMap(sub, super *typ.ReadonlyMap) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(sub.Key, super.Key) &&
		c.check(typetable.PresentReadonlyEntryValue(sub.Value), super.Value)
}

func (c *checker) checkTuple(sub, super *typ.Tuple) bool {
	if len(sub.Elements) != len(super.Elements) {
		return false
	}
	for i, e := range sub.Elements {
		if !c.check(e, super.Elements[i]) {
			return false
		}
	}
	return true
}

func (c *checker) checkInterface(sub, super *typ.Interface) bool {
	if sub == nil || super == nil {
		return false
	}
	// A method-less interface imposes no requirements, so any interface
	// structurally satisfies it: an empty interface is the structural top, not a
	// nominal marker keyed by name.
	if len(super.Methods) == 0 {
		return true
	}
	for _, superMethod := range super.Methods {
		found := false
		for _, subMethod := range sub.Methods {
			if subMethod.Name != superMethod.Name {
				continue
			}
			if !c.check(subMethod.Type, superMethod.Type) {
				return false
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func (c *checker) checkInstantiated(sub, super *typ.Instantiated) bool {
	if sub == nil || super == nil || sub.Generic == nil || super.Generic == nil {
		return false
	}
	if !typ.TypeEquals(sub.Generic, super.Generic) || len(sub.TypeArgs) != len(super.TypeArgs) {
		return false
	}
	for i, a := range sub.TypeArgs {
		if !c.check(a, super.TypeArgs[i]) || !c.check(super.TypeArgs[i], a) {
			return false
		}
	}
	return true
}
