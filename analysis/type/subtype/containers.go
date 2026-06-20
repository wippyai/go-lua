package subtype

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (c *checker) checkArray(sub, super *typ.Array, depth int) bool {
	return c.check(sub.Element, super.Element, depth+1)
}

func (c *checker) checkMap(sub, super *typ.Map, depth int) bool {
	if !c.check(sub.Key, super.Key, depth+1) || !c.check(super.Key, sub.Key, depth+1) {
		return false
	}
	if !c.check(sub.Value, super.Value, depth+1) {
		return false
	}
	return c.check(super.Value, sub.Value, depth+1) || typ.IsAny(super.Value)
}

func (c *checker) checkReadonlyMap(sub, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(sub.Key, super.Key, depth+1) &&
		c.check(typetable.PresentReadonlyEntryValue(sub.Value), super.Value, depth+1)
}

func (c *checker) checkTuple(sub, super *typ.Tuple, depth int) bool {
	if len(sub.Elements) != len(super.Elements) {
		return false
	}
	for i, e := range sub.Elements {
		if !c.check(e, super.Elements[i], depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkInterface(sub, super *typ.Interface, depth int) bool {
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
			if !c.check(subMethod.Type, superMethod.Type, depth+1) {
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

func (c *checker) checkInstantiated(sub, super *typ.Instantiated, depth int) bool {
	if sub == nil || super == nil || sub.Generic == nil || super.Generic == nil {
		return false
	}
	if !typ.TypeEquals(sub.Generic, super.Generic) || len(sub.TypeArgs) != len(super.TypeArgs) {
		return false
	}
	for i, a := range sub.TypeArgs {
		if !c.check(a, super.TypeArgs[i], depth+1) || !c.check(super.TypeArgs[i], a, depth+1) {
			return false
		}
	}
	return true
}
