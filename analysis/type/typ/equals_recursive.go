package typ

func typeEqualsGuard(a, b Type, seen *typePairSet) bool {
	a = NormalizeNil(a)
	b = NormalizeNil(b)

	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	var aOK, bOK bool
	a, aOK = unwrapAliasForEquals(a)
	b, bOK = unwrapAliasForEquals(b)
	if !aOK || !bOK {
		return false
	}

	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	// Unwrap Ref types
	if ra, ok := a.(*Ref); ok {
		if rb, ok := b.(*Ref); ok {
			return ra.Module == rb.Module && ra.Name == rb.Name
		}

		return false
	}

	if _, ok := b.(*Ref); ok {
		return false
	}

	if a.Kind() != b.Kind() {
		return false
	}
	if typeEqualsCanUseHashPrefilter(a, b) && EqualityHash(a) != EqualityHash(b) {
		return false
	}

	// For compound types, use pointer-based cycle detection to avoid hash collisions.
	if needsCycleCheck(a.Kind()) {
		ap := typePointer(a)
		bp := typePointer(b)

		if ap != 0 && bp != 0 {
			pair := typePair{a: ap, b: bp}

			if seen.seenOrAdd(pair) {
				return true // cycle, coinductively equal
			}
		}
	}

	a = UnwrapTransparentWrappers(a)
	b = UnwrapTransparentWrappers(b)

	switch va := a.(type) {
	case *Optional:
		vb, ok := b.(*Optional)
		return ok && typeEqualsGuard(va.Inner, vb.Inner, seen)
	case *Union:
		vb, ok := b.(*Union)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], seen) {
				return false
			}
		}
		return true
	case *Intersection:
		vb, ok := b.(*Intersection)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], seen) {
				return false
			}
		}
		return true
	case *Tuple:
		vb, ok := b.(*Tuple)
		if !ok || len(va.Elements) != len(vb.Elements) {
			return false
		}
		for i, e := range va.Elements {
			if !typeEqualsGuard(e, vb.Elements[i], seen) {
				return false
			}
		}
		return true
	case *Array:
		vb, ok := b.(*Array)
		return ok && typeEqualsGuard(va.Element, vb.Element, seen)
	case *Map:
		vb, ok := b.(*Map)
		return ok &&
			typeEqualsGuard(va.Key, vb.Key, seen) &&
			typeEqualsGuard(va.Value, vb.Value, seen)
	case *ReadonlyMap:
		vb, ok := b.(*ReadonlyMap)
		return ok &&
			typeEqualsGuard(va.Key, vb.Key, seen) &&
			typeEqualsGuard(va.Value, vb.Value, seen)
	case *Record:
		vb, ok := b.(*Record)
		if !ok || va.Open != vb.Open ||
			len(va.Fields) != len(vb.Fields) ||
			len(va.StaticMembers) != len(vb.StaticMembers) {
			return false
		}
		for i, f := range va.Fields {
			fb := vb.Fields[i]
			if f.Name != fb.Name || f.Optional != fb.Optional || f.Readonly != fb.Readonly {
				return false
			}
			if !typeEqualsGuard(f.Type, fb.Type, seen) {
				return false
			}
		}
		for i, m := range va.StaticMembers {
			mb := vb.StaticMembers[i]
			if m.Kind != mb.Kind || m.Name != mb.Name || m.Index != mb.Index ||
				m.Optional != mb.Optional || m.Readonly != mb.Readonly {
				return false
			}
			if !typeEqualsGuard(m.Type, mb.Type, seen) {
				return false
			}
		}
		if va.HasMapComponent() != vb.HasMapComponent() {
			return false
		}
		if va.HasMapComponent() {
			if !typeEqualsGuard(va.MapKey, vb.MapKey, seen) {
				return false
			}
			if !typeEqualsGuard(va.MapValue, vb.MapValue, seen) {
				return false
			}
		}
		if (va.Metatable == nil) != (vb.Metatable == nil) {
			return false
		}
		if va.Metatable != nil && !typeEqualsGuard(va.Metatable, vb.Metatable, seen) {
			return false
		}
		return true
	case *Function:
		vb, ok := b.(*Function)
		if !ok || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !typeEqualsGuard(tp, vb.TypeParams[i], seen) {
				return false
			}
		}
		if len(va.Params) != len(vb.Params) || len(va.Returns) != len(vb.Returns) {
			return false
		}
		for i, p := range va.Params {
			pb := vb.Params[i]
			if p.Optional != pb.Optional || p.Receiver != pb.Receiver {
				return false
			}
			if !typeEqualsGuard(p.Type, pb.Type, seen) {
				return false
			}
		}
		if (va.Variadic == nil) != (vb.Variadic == nil) {
			return false
		}
		if va.Variadic != nil && !typeEqualsGuard(va.Variadic, vb.Variadic, seen) {
			return false
		}
		for i, r := range va.Returns {
			if !typeEqualsGuard(r, vb.Returns[i], seen) {
				return false
			}
		}
		return true
	case *Generic:
		vb, ok := b.(*Generic)
		if !ok || va.Name != vb.Name || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !typeEqualsGuard(tp, vb.TypeParams[i], seen) {
				return false
			}
		}
		// Identity is the declaration content: name + type params + body
		// structure. Two declarations of the same body are one type regardless
		// of which compilation produced them, so an exported generic imported
		// into two modules compares equal; same-named declarations with
		// different bodies stay distinct. The coinductive guard above handles
		// self-recursive bodies (a forward-reference body is nil only on one
		// side at the placeholder stage).
		if (va.Body == nil) != (vb.Body == nil) {
			return false
		}
		return va.Body == nil || typeEqualsGuard(va.Body, vb.Body, seen)
	case *Instantiated:
		vb, ok := b.(*Instantiated)
		if !ok || !typeEqualsGuard(va.Generic, vb.Generic, seen) || len(va.TypeArgs) != len(vb.TypeArgs) {
			return false
		}
		for i, arg := range va.TypeArgs {
			if !typeEqualsGuard(arg, vb.TypeArgs[i], seen) {
				return false
			}
		}
		return true
	case *TypeParam:
		vb, ok := b.(*TypeParam)
		if !ok || va.Name != vb.Name {
			return false
		}
		if (va.Constraint == nil) != (vb.Constraint == nil) {
			return false
		}
		return va.Constraint == nil || typeEqualsGuard(va.Constraint, vb.Constraint, seen)
	case *Recursive:
		vb, ok := b.(*Recursive)
		if !ok {
			return false
		}
		if va.ID == vb.ID {
			return true
		}
		if va.Name != vb.Name {
			return false
		}
		return typeEqualsGuard(va.Body, vb.Body, seen)
	case *Interface:
		// Compare structurally while threading seen so a recursion that cycles
		// through a method signature is caught by the coinductive pair guard.
		// Falling to Interface.Equals would restart equality with a fresh seen-set
		// at each method's Function.Equals, dropping the guard and recursing
		// forever on a recursive-through-interface type.
		vb, ok := b.(*Interface)
		if !ok || va.Name != vb.Name || len(va.Methods) != len(vb.Methods) {
			return false
		}
		for i, m := range va.Methods {
			om := vb.Methods[i]
			if m.Name != om.Name {
				return false
			}
			if !typeEqualsGuard(m.Type, om.Type, seen) {
				return false
			}
		}
		return true
	case *Meta:
		// Thread seen through the wrapped type for the same coinductive reason.
		vb, ok := b.(*Meta)
		return ok && typeEqualsGuard(va.Of, vb.Of, seen)
	default:
		return a.Equals(b)
	}
}
