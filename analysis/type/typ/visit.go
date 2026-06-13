package typ

// Visitor dispatches on concrete type implementations.
// Nil handlers fall back to Default when provided; otherwise return zero.
type Visitor[R any] struct {
	Optional     func(*Optional) R
	Union        func(*Union) R
	Intersection func(*Intersection) R
	Array        func(*Array) R
	Map          func(*Map) R
	ReadonlyMap  func(*ReadonlyMap) R
	Tuple        func(*Tuple) R
	Function     func(*Function) R
	Record       func(*Record) R
	Alias        func(*Alias) R
	Ref          func(*Ref) R
	Meta         func(*Meta) R
	Generic      func(*Generic) R
	Instantiated func(*Instantiated) R
	TypeParam    func(*TypeParam) R
	Interface    func(*Interface) R
	Recursive    func(*Recursive) R
	Literal      func(*Literal) R
	Default      func(Type) R
}

// Visit applies the first matching handler in v to t.
func Visit[R any](t Type, v Visitor[R]) R {
	t = unwrapTransparentWrappers(t)

	switch tt := t.(type) {
	case *Optional:
		if v.Optional != nil {
			return v.Optional(tt)
		}
	case *Union:
		if v.Union != nil {
			return v.Union(tt)
		}
	case *Intersection:
		if v.Intersection != nil {
			return v.Intersection(tt)
		}
	case *Array:
		if v.Array != nil {
			return v.Array(tt)
		}
	case *Map:
		if v.Map != nil {
			return v.Map(tt)
		}
	case *ReadonlyMap:
		if v.ReadonlyMap != nil {
			return v.ReadonlyMap(tt)
		}
	case *Tuple:
		if v.Tuple != nil {
			return v.Tuple(tt)
		}
	case *Function:
		if v.Function != nil {
			return v.Function(tt)
		}
	case *Record:
		if v.Record != nil {
			return v.Record(tt)
		}
	case *Alias:
		if v.Alias != nil {
			return v.Alias(tt)
		}
	case *Ref:
		if v.Ref != nil {
			return v.Ref(tt)
		}
	case *Meta:
		if v.Meta != nil {
			return v.Meta(tt)
		}
	case *Generic:
		if v.Generic != nil {
			return v.Generic(tt)
		}
	case *Instantiated:
		if v.Instantiated != nil {
			return v.Instantiated(tt)
		}
	case *TypeParam:
		if v.TypeParam != nil {
			return v.TypeParam(tt)
		}
	case *Interface:
		if v.Interface != nil {
			return v.Interface(tt)
		}
	case *Recursive:
		if v.Recursive != nil {
			return v.Recursive(tt)
		}
	case *Literal:
		if v.Literal != nil {
			return v.Literal(tt)
		}
	}
	if v.Default != nil {
		return v.Default(t)
	}
	var zero R
	return zero
}

func unwrapTransparentWrappers(t Type) Type {
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

func unwrapAnnotatedOrNil(t Type) Type {
	if t == nil {
		return nil
	}
	return unwrapAnnotated(t)
}
