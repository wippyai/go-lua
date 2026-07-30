package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) canWidenTo(narrow, wide typ.Type) bool {
	if missingTypePair(narrow, wide) {
		return false
	}
	wide = unwrap.Alias(wide)
	narrow = unwrap.Alias(narrow)
	if pair, ok := newTypePair(narrow, wide); ok {
		if result, ok := c.widenMemo[pair]; ok {
			return result
		}
		if c.widenInProgress[pair] {
			// Widening is a structural relation over regular type graphs. Reaching
			// the same obligation again closes the current coinductive proof; the
			// enclosing conjunctions still have to establish every productive arm.
			return true
		}
		if c.widenInProgress == nil {
			c.widenInProgress = make(map[typePair]bool)
		}
		c.widenInProgress[pair] = true
		result := c.canWidenToUncached(narrow, wide)
		delete(c.widenInProgress, pair)
		if c.widenMemo == nil {
			c.widenMemo = make(map[typePair]bool)
		}
		c.widenMemo[pair] = result
		return result
	}
	return c.canWidenToUncached(narrow, wide)
}

func (c *checker) canWidenToUncached(narrow, wide typ.Type) bool {

	if inst, ok := narrow.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != narrow {
			return c.check(expanded, subst.Self(wide, inst)) ||
				c.canWidenTo(expanded, subst.Self(wide, inst))
		}
	}
	if inst, ok := wide.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != wide {
			return c.check(subst.Self(narrow, inst), expanded) ||
				c.canWidenTo(subst.Self(narrow, inst), expanded)
		}
	}
	if subRec, ok := narrow.(*typ.Recursive); ok && subRec.Body != nil && subRec.Body != narrow {
		if supRec, ok := wide.(*typ.Recursive); ok && supRec.Body != nil && supRec.Body != wide {
			return c.check(subRec.Body, supRec.Body) ||
				c.canWidenTo(subRec.Body, supRec.Body)
		}
		return c.check(subRec.Body, wide) || c.canWidenTo(subRec.Body, wide)
	}
	if supRec, ok := wide.(*typ.Recursive); ok && supRec.Body != nil && supRec.Body != wide {
		return c.check(narrow, supRec.Body) || c.canWidenTo(narrow, supRec.Body)
	}

	if typ.IsAny(wide) {
		return true
	}
	if typ.IsBuiltinTableTopMarker(wide) {
		return typetable.IsLike(narrow)
	}
	if narrow.Kind() == kind.Nil {
		if _, ok := wide.(*typ.Optional); ok {
			return true
		}
		if u, ok := wide.(*typ.Union); ok {
			for _, m := range u.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
		}
	}
	if opt, ok := wide.(*typ.Optional); ok {
		if narrowOpt, ok := narrow.(*typ.Optional); ok {
			return c.check(narrowOpt.Inner, opt.Inner) ||
				c.canWidenTo(narrowOpt.Inner, opt.Inner)
		}
		if c.check(narrow, opt.Inner) || c.canWidenTo(narrow, opt.Inner) {
			return true
		}
	}
	if u, ok := narrow.(*typ.Union); ok {
		if len(u.Members) == 0 {
			return false
		}
		for _, m := range u.Members {
			if c.check(m, wide) || c.canWidenTo(m, wide) {
				continue
			}
			return false
		}
		return true
	}
	if u, ok := wide.(*typ.Union); ok {
		for _, m := range u.Members {
			if m.Kind() == kind.Literal {
				continue
			}
			if c.check(narrow, m) || c.canWidenTo(narrow, m) {
				return true
			}
		}
		return false
	}
	if narrow.Kind() == kind.Integer && wide.Kind() == kind.Number {
		return true
	}
	if lit, ok := narrow.(*typ.Literal); ok {
		wideInner := wide
		if opt, ok := wide.(*typ.Optional); ok {
			wideInner = unwrap.Alias(opt.Inner)
		}
		switch lit.Base {
		case kind.Boolean:
			return wideInner.Kind() == kind.Boolean
		case kind.String:
			return wideInner.Kind() == kind.String
		case kind.Integer:
			return wideInner.Kind() == kind.Integer || wideInner.Kind() == kind.Number
		case kind.Number:
			return wideInner.Kind() == kind.Number
		}
	}
	if subRec, ok := narrow.(*typ.Record); ok {
		if emptyRecordAdoptsContainerShape(subRec, wide) {
			return true
		}
		if supRec, ok := wide.(*typ.Record); ok {
			return c.canWidenRecordTo(subRec, supRec)
		}
		if supArray, ok := wide.(*typ.Array); ok {
			return c.canWidenRecordToArray(subRec, supArray)
		}
		if supMap, ok := wide.(*typ.Map); ok {
			return c.canWidenRecordToMap(subRec, supMap)
		}
	}
	if subMap, ok := narrow.(*typ.Map); ok {
		if supMap, ok := wide.(*typ.Map); ok {
			return c.canWidenMapTo(subMap, supMap)
		}
	}
	if subArray, ok := narrow.(*typ.Array); ok {
		if supArray, ok := wide.(*typ.Array); ok {
			return c.check(subArray.Element, supArray.Element) ||
				c.canWidenTo(subArray.Element, supArray.Element)
		}
		if supMap, ok := wide.(*typ.Map); ok {
			return c.check(typ.Integer, supMap.Key) &&
				(c.check(subArray.Element, supMap.Value) ||
					c.canWidenTo(subArray.Element, supMap.Value))
		}
	}
	if subTuple, ok := narrow.(*typ.Tuple); ok {
		if supTuple, ok := wide.(*typ.Tuple); ok {
			if len(subTuple.Elements) != len(supTuple.Elements) {
				return false
			}
			for i := range subTuple.Elements {
				if c.check(subTuple.Elements[i], supTuple.Elements[i]) ||
					c.canWidenTo(subTuple.Elements[i], supTuple.Elements[i]) {
					continue
				}
				return false
			}
			return true
		}
	}
	if subFn, ok := narrow.(*typ.Function); ok {
		if supFn, ok := wide.(*typ.Function); ok {
			if !c.functionParamsEquivalent(subFn, supFn) {
				return false
			}
			for i := 0; i < len(supFn.Returns); i++ {
				subReturn := typ.Nil
				if i < len(subFn.Returns) {
					subReturn = subFn.Returns[i]
				}
				if c.check(subReturn, supFn.Returns[i]) || c.canWidenTo(subReturn, supFn.Returns[i]) {
					continue
				}
				return false
			}
			return true
		}
	}
	return false
}

func (c *checker) canWidenMapTo(narrow, wide *typ.Map) bool {
	if narrow == nil || wide == nil {
		return false
	}
	if !c.check(narrow.Key, wide.Key) || !c.check(wide.Key, narrow.Key) {
		return false
	}
	// A map is mutable, so widening its value type lets a write through the wider
	// alias store a value the narrow map's type forbids (unsound covariance). A
	// covariant value widen is therefore sound only for a fresh map with no concrete
	// values yet (value Never); a map carrying a concrete value type must match it
	// invariantly, exactly as checkMap requires.
	if typ.IsNever(narrow.Value) {
		return c.check(narrow.Value, wide.Value) ||
			c.canWidenTo(narrow.Value, wide.Value)
	}
	return c.check(narrow.Value, wide.Value) &&
		c.check(wide.Value, narrow.Value)
}

// canWidenRecordToMap reports whether a fresh record literal widens to a map
// type. Each field name must inhabit the map key and each field value must be a
// subtype of (or widen to) the map value, matching record-to-map subtyping while
// allowing the per-field literal widening fresh constructors rely on. A record
// map-component, when present, must likewise widen to the target map.
func (c *checker) canWidenRecordToMap(narrow *typ.Record, wide *typ.Map) bool {
	if narrow == nil || wide == nil {
		return false
	}
	for _, f := range narrow.Fields {
		if !c.check(typ.LiteralString(f.Name), wide.Key) {
			return false
		}
		if !c.check(f.Type, wide.Value) && !c.canWidenTo(f.Type, wide.Value) {
			return false
		}
	}
	if narrow.HasMapComponent() {
		if !c.check(narrow.MapKey, wide.Key) {
			return false
		}
		if !c.check(narrow.MapValue, wide.Value) && !c.canWidenTo(narrow.MapValue, wide.Value) {
			return false
		}
	}
	return true
}

func (c *checker) canWidenRecordToArray(narrow *typ.Record, wide *typ.Array) bool {
	if narrow == nil || wide == nil {
		return false
	}
	if len(narrow.Fields) != 0 {
		return false
	}
	for _, m := range narrow.StaticMembers {
		if m.Kind != typ.StaticMemberIntIndex {
			return false
		}
		if !c.check(m.Type, wide.Element) && !c.canWidenTo(m.Type, wide.Element) {
			return false
		}
	}
	if narrow.HasMapComponent() {
		if !c.check(narrow.MapKey, typ.Integer) {
			return false
		}
		if !c.check(narrow.MapValue, wide.Element) && !c.canWidenTo(narrow.MapValue, wide.Element) {
			return false
		}
	}
	return true
}

func (c *checker) functionParamsEquivalent(a, b *typ.Function) bool {
	if a == nil || b == nil || len(a.Params) != len(b.Params) {
		return false
	}
	for i := 0; i < len(a.Params); i++ {
		ap := a.Params[i]
		bp := b.Params[i]
		if ap.Optional != bp.Optional {
			return false
		}
		if !c.check(ap.Type, bp.Type) || !c.check(bp.Type, ap.Type) {
			return false
		}
	}
	if a.Variadic == nil && b.Variadic == nil {
		return true
	}
	if a.Variadic == nil || b.Variadic == nil {
		return false
	}
	return c.check(a.Variadic, b.Variadic) && c.check(b.Variadic, a.Variadic)
}

func (c *checker) canWidenRecordTo(narrow, wide *typ.Record) bool {
	if narrow == nil || wide == nil {
		return false
	}
	for _, wf := range wide.Fields {
		nf := narrow.GetField(wf.Name)
		if nf == nil {
			if !wf.Optional && !unwrap.IsOptionalLike(wf.Type) {
				return false
			}
			continue
		}
		if !c.check(nf.Type, wf.Type) && !c.canWidenTo(nf.Type, wf.Type) {
			return false
		}
	}
	for _, wm := range wide.StaticMembers {
		nm := narrow.GetStaticMember(wm.Kind, wm.Name, wm.Index)
		if nm == nil {
			if !wm.Optional && !unwrap.IsOptionalLike(wm.Type) {
				return false
			}
			continue
		}
		if !c.check(nm.Type, wm.Type) && !c.canWidenTo(nm.Type, wm.Type) {
			return false
		}
	}
	return true
}
