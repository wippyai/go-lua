package typ

const typeEqualsWorkInlineCapacity = 32

// typeEqualsWorkStack is the complete finite traversal state for one equality
// query. Its result is carried with the pending work, so child comparisons
// never re-enter TypeEquals or consume the Go call stack.
type typeEqualsWorkStack struct {
	inline   [typeEqualsWorkInlineCapacity]typeEqualsWork
	inlineN  uint8
	overflow []typeEqualsWork
	result   bool
}

type typeEqualsWork struct {
	a Type
	b Type
}

func (s *typeEqualsWorkStack) push(a, b Type) {
	if s.overflow != nil {
		s.overflow = append(s.overflow, typeEqualsWork{a: a, b: b})
		return
	}
	if s.inlineN < uint8(len(s.inline)) {
		s.inline[s.inlineN] = typeEqualsWork{a: a, b: b}
		s.inlineN++
		return
	}

	s.overflow = make([]typeEqualsWork, 0, typeEqualsWorkInlineCapacity*2)
	s.overflow = append(s.overflow, s.inline[:]...)
	s.inlineN = 0
	s.overflow = append(s.overflow, typeEqualsWork{a: a, b: b})
}

func (s *typeEqualsWorkStack) pop() (typeEqualsWork, bool) {
	if n := len(s.overflow); n != 0 {
		work := s.overflow[n-1]
		s.overflow[n-1] = typeEqualsWork{}
		s.overflow = s.overflow[:n-1]
		return work, true
	}
	if s.inlineN == 0 {
		return typeEqualsWork{}, false
	}
	s.inlineN--
	work := s.inline[s.inlineN]
	s.inline[s.inlineN] = typeEqualsWork{}
	return work, true
}

func typeEqualsIterative(a, b Type, seen *typePairSet) bool {
	work := typeEqualsWorkStack{result: true}
	work.push(a, b)
	for work.result {
		pair, ok := work.pop()
		if !ok {
			break
		}
		if !typeEqualsStep(pair.a, pair.b, seen, &work) {
			work.result = false
		}
	}
	return work.result
}

// typeEqualsStep compares one pair's local shape and schedules its children in
// reverse source order. Popping restores the old depth-first, left-to-right
// evaluation order while keeping the complete graph traversal iterative.
func typeEqualsStep(a, b Type, seen *typePairSet, work *typeEqualsWorkStack) bool {
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

	if ra, ok := a.(*Ref); ok {
		rb, ok := b.(*Ref)
		return ok && ra.Module == rb.Module && ra.Name == rb.Name
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

	if needsCycleCheck(a.Kind()) {
		ap, bp := typePointer(a), typePointer(b)
		if ap != 0 && bp != 0 && seen.seenOrAdd(typePair{a: ap, b: bp}) {
			return true
		}
	}

	a = UnwrapTransparentWrappers(a)
	b = UnwrapTransparentWrappers(b)

	switch va := a.(type) {
	case *Optional:
		vb, ok := b.(*Optional)
		if !ok {
			return false
		}
		work.push(va.Inner, vb.Inner)
	case *Union:
		vb, ok := b.(*Union)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i := len(va.Members) - 1; i >= 0; i-- {
			work.push(va.Members[i], vb.Members[i])
		}
	case *Intersection:
		vb, ok := b.(*Intersection)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i := len(va.Members) - 1; i >= 0; i-- {
			work.push(va.Members[i], vb.Members[i])
		}
	case *Tuple:
		vb, ok := b.(*Tuple)
		if !ok || len(va.Elements) != len(vb.Elements) {
			return false
		}
		for i := len(va.Elements) - 1; i >= 0; i-- {
			work.push(va.Elements[i], vb.Elements[i])
		}
	case *Array:
		vb, ok := b.(*Array)
		if !ok {
			return false
		}
		work.push(va.Element, vb.Element)
	case *Map:
		vb, ok := b.(*Map)
		if !ok {
			return false
		}
		work.push(va.Value, vb.Value)
		work.push(va.Key, vb.Key)
	case *ReadonlyMap:
		vb, ok := b.(*ReadonlyMap)
		if !ok {
			return false
		}
		work.push(va.Value, vb.Value)
		work.push(va.Key, vb.Key)
	case *Record:
		vb, ok := b.(*Record)
		if !ok || va.Open != vb.Open || len(va.Fields) != len(vb.Fields) || len(va.StaticMembers) != len(vb.StaticMembers) {
			return false
		}
		if va.HasMapComponent() != vb.HasMapComponent() || (va.Metatable == nil) != (vb.Metatable == nil) {
			return false
		}
		if va.Metatable != nil {
			work.push(va.Metatable, vb.Metatable)
		}
		if va.HasMapComponent() {
			work.push(va.MapValue, vb.MapValue)
			work.push(va.MapKey, vb.MapKey)
		}
		for i := len(va.StaticMembers) - 1; i >= 0; i-- {
			member, other := va.StaticMembers[i], vb.StaticMembers[i]
			if member.Kind != other.Kind || member.Name != other.Name || member.Index != other.Index || member.Optional != other.Optional || member.Readonly != other.Readonly {
				return false
			}
			work.push(member.Type, other.Type)
		}
		for i := len(va.Fields) - 1; i >= 0; i-- {
			field, other := va.Fields[i], vb.Fields[i]
			if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
				return false
			}
			work.push(field.Type, other.Type)
		}
	case *Function:
		vb, ok := b.(*Function)
		if !ok || len(va.TypeParams) != len(vb.TypeParams) || len(va.Params) != len(vb.Params) || len(va.Returns) != len(vb.Returns) || (va.Variadic == nil) != (vb.Variadic == nil) {
			return false
		}
		for i := len(va.Returns) - 1; i >= 0; i-- {
			work.push(va.Returns[i], vb.Returns[i])
		}
		if va.Variadic != nil {
			work.push(va.Variadic, vb.Variadic)
		}
		for i := len(va.Params) - 1; i >= 0; i-- {
			param, other := va.Params[i], vb.Params[i]
			if param.Optional != other.Optional || param.Receiver != other.Receiver {
				return false
			}
			work.push(param.Type, other.Type)
		}
		for i := len(va.TypeParams) - 1; i >= 0; i-- {
			work.push(va.TypeParams[i], vb.TypeParams[i])
		}
	case *Generic:
		vb, ok := b.(*Generic)
		if !ok || va.Name != vb.Name || len(va.TypeParams) != len(vb.TypeParams) || (va.Body == nil) != (vb.Body == nil) {
			return false
		}
		if va.Body != nil {
			work.push(va.Body, vb.Body)
		}
		for i := len(va.TypeParams) - 1; i >= 0; i-- {
			work.push(va.TypeParams[i], vb.TypeParams[i])
		}
	case *Instantiated:
		vb, ok := b.(*Instantiated)
		if !ok || len(va.TypeArgs) != len(vb.TypeArgs) {
			return false
		}
		for i := len(va.TypeArgs) - 1; i >= 0; i-- {
			work.push(va.TypeArgs[i], vb.TypeArgs[i])
		}
		work.push(va.Generic, vb.Generic)
	case *TypeParam:
		vb, ok := b.(*TypeParam)
		if !ok || va.Name != vb.Name || (va.Constraint == nil) != (vb.Constraint == nil) {
			return false
		}
		if va.Constraint != nil {
			work.push(va.Constraint, vb.Constraint)
		}
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
		work.push(va.Body, vb.Body)
	case *Interface:
		vb, ok := b.(*Interface)
		if !ok || va.Name != vb.Name || len(va.Methods) != len(vb.Methods) {
			return false
		}
		for i := len(va.Methods) - 1; i >= 0; i-- {
			method, other := va.Methods[i], vb.Methods[i]
			if method.Name != other.Name {
				return false
			}
			work.push(method.Type, other.Type)
		}
	case *Meta:
		vb, ok := b.(*Meta)
		if !ok {
			return false
		}
		work.push(va.Of, vb.Of)
	default:
		return a.Equals(b)
	}
	return true
}
