package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) canWidenTo(narrow, wide typ.Type, depth int) bool {
	if stopDepthPair(narrow, wide, depth) {
		return false
	}
	wide = unwrap.Alias(wide)
	narrow = unwrap.Alias(narrow)

	if typ.IsAny(wide) {
		return true
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
			return c.check(narrowOpt.Inner, opt.Inner, depth+1) ||
				c.canWidenTo(narrowOpt.Inner, opt.Inner, depth+1)
		}
		if c.check(narrow, opt.Inner, depth+1) || c.canWidenTo(narrow, opt.Inner, depth+1) {
			return true
		}
	}
	if u, ok := narrow.(*typ.Union); ok {
		if len(u.Members) == 0 {
			return false
		}
		for _, m := range u.Members {
			if c.check(m, wide, depth+1) || c.canWidenTo(m, wide, depth+1) {
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
			if c.check(narrow, m, depth+1) || c.canWidenTo(narrow, m, depth+1) {
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
		if supRec, ok := wide.(*typ.Record); ok {
			return c.canWidenRecordTo(subRec, supRec, depth+1)
		}
		if supMap, ok := wide.(*typ.Map); ok {
			return c.canWidenRecordToMap(subRec, supMap, depth+1)
		}
	}
	if subMap, ok := narrow.(*typ.Map); ok {
		if supMap, ok := wide.(*typ.Map); ok {
			return c.canWidenMapTo(subMap, supMap, depth+1)
		}
	}
	if subArray, ok := narrow.(*typ.Array); ok {
		if supArray, ok := wide.(*typ.Array); ok {
			return c.check(subArray.Element, supArray.Element, depth+1) ||
				c.canWidenTo(subArray.Element, supArray.Element, depth+1)
		}
		if supMap, ok := wide.(*typ.Map); ok {
			return c.check(typ.Integer, supMap.Key, depth+1) &&
				(c.check(subArray.Element, supMap.Value, depth+1) ||
					c.canWidenTo(subArray.Element, supMap.Value, depth+1))
		}
	}
	if rec, ok := narrow.(*typ.Recursive); ok && rec.Body != nil && rec.Body != rec {
		return c.check(rec.Body, wide, depth+1) || c.canWidenTo(rec.Body, wide, depth+1)
	}
	if subTuple, ok := narrow.(*typ.Tuple); ok {
		if supTuple, ok := wide.(*typ.Tuple); ok {
			if len(subTuple.Elements) != len(supTuple.Elements) {
				return false
			}
			for i := range subTuple.Elements {
				if c.check(subTuple.Elements[i], supTuple.Elements[i], depth+1) ||
					c.canWidenTo(subTuple.Elements[i], supTuple.Elements[i], depth+1) {
					continue
				}
				return false
			}
			return true
		}
	}
	if subFn, ok := narrow.(*typ.Function); ok {
		if supFn, ok := wide.(*typ.Function); ok {
			if !c.functionParamsEquivalent(subFn, supFn, depth+1) {
				return false
			}
			for i := 0; i < len(supFn.Returns); i++ {
				subReturn := typ.Nil
				if i < len(subFn.Returns) {
					subReturn = subFn.Returns[i]
				}
				if c.check(subReturn, supFn.Returns[i], depth+1) || c.canWidenTo(subReturn, supFn.Returns[i], depth+1) {
					continue
				}
				return false
			}
			return true
		}
	}
	return false
}

func (c *checker) canWidenMapTo(narrow, wide *typ.Map, depth int) bool {
	if narrow == nil || wide == nil {
		return false
	}
	if !c.check(narrow.Key, wide.Key, depth+1) || !c.check(wide.Key, narrow.Key, depth+1) {
		return false
	}
	// A map is mutable, so widening its value type lets a write through the wider
	// alias store a value the narrow map's type forbids (unsound covariance). A
	// covariant value widen is therefore sound only for a fresh map with no concrete
	// values yet (value Never); a map carrying a concrete value type must match it
	// invariantly, exactly as checkMap requires.
	if typ.IsNever(narrow.Value) {
		return c.check(narrow.Value, wide.Value, depth+1) ||
			c.canWidenTo(narrow.Value, wide.Value, depth+1)
	}
	return c.check(narrow.Value, wide.Value, depth+1) &&
		c.check(wide.Value, narrow.Value, depth+1)
}

// canWidenRecordToMap reports whether a fresh record literal widens to a map
// type. Each field name must inhabit the map key and each field value must be a
// subtype of (or widen to) the map value, matching record-to-map subtyping while
// allowing the per-field literal widening fresh constructors rely on. A record
// map-component, when present, must likewise widen to the target map.
func (c *checker) canWidenRecordToMap(narrow *typ.Record, wide *typ.Map, depth int) bool {
	if narrow == nil || wide == nil {
		return false
	}
	for _, f := range narrow.Fields {
		if !c.check(typ.LiteralString(f.Name), wide.Key, depth+1) {
			return false
		}
		if !c.check(f.Type, wide.Value, depth+1) && !c.canWidenTo(f.Type, wide.Value, depth+1) {
			return false
		}
	}
	if narrow.HasMapComponent() {
		if !c.check(narrow.MapKey, wide.Key, depth+1) {
			return false
		}
		if !c.check(narrow.MapValue, wide.Value, depth+1) && !c.canWidenTo(narrow.MapValue, wide.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) functionParamsEquivalent(a, b *typ.Function, depth int) bool {
	if a == nil || b == nil || len(a.Params) != len(b.Params) {
		return false
	}
	for i := 0; i < len(a.Params); i++ {
		ap := a.Params[i]
		bp := b.Params[i]
		if ap.Optional != bp.Optional {
			return false
		}
		if !c.check(ap.Type, bp.Type, depth+1) || !c.check(bp.Type, ap.Type, depth+1) {
			return false
		}
	}
	if a.Variadic == nil && b.Variadic == nil {
		return true
	}
	if a.Variadic == nil || b.Variadic == nil {
		return false
	}
	return c.check(a.Variadic, b.Variadic, depth+1) && c.check(b.Variadic, a.Variadic, depth+1)
}

func (c *checker) canWidenRecordTo(narrow, wide *typ.Record, depth int) bool {
	if narrow == nil || wide == nil {
		return false
	}
	for _, wf := range wide.Fields {
		nf := narrow.GetField(wf.Name)
		if nf == nil {
			continue
		}
		if !c.check(nf.Type, wf.Type, depth+1) && !c.canWidenTo(nf.Type, wf.Type, depth+1) {
			return false
		}
	}
	for _, wm := range wide.StaticMembers {
		nm := narrow.GetStaticMember(wm.Kind, wm.Name, wm.Index)
		if nm == nil {
			continue
		}
		if !c.check(nm.Type, wm.Type, depth+1) && !c.canWidenTo(nm.Type, wm.Type, depth+1) {
			return false
		}
	}
	return true
}
