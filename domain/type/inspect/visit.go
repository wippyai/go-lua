package inspect

import "github.com/wippyai/go-lua/domain/type/typ"

// Visitor dispatches on concrete type implementations.
// Nil handlers fall back to Default when provided; otherwise return zero.
type Visitor[R any] struct {
	Optional     func(*typ.Optional) R
	Union        func(*typ.Union) R
	Intersection func(*typ.Intersection) R
	Array        func(*typ.Array) R
	Map          func(*typ.Map) R
	ReadonlyMap  func(*typ.ReadonlyMap) R
	Tuple        func(*typ.Tuple) R
	Function     func(*typ.Function) R
	Record       func(*typ.Record) R
	Alias        func(*typ.Alias) R
	Ref          func(*typ.Ref) R
	Meta         func(*typ.Meta) R
	Generic      func(*typ.Generic) R
	Instantiated func(*typ.Instantiated) R
	TypeParam    func(*typ.TypeParam) R
	Interface    func(*typ.Interface) R
	Recursive    func(*typ.Recursive) R
	Literal      func(*typ.Literal) R
	Default      func(typ.Type) R
}

// Visit applies the first matching handler in v to t.
func Visit[R any](t typ.Type, v Visitor[R]) R {
	t = unwrapTransparent(t)

	switch tt := t.(type) {
	case *typ.Optional:
		if v.Optional != nil {
			return v.Optional(tt)
		}
	case *typ.Union:
		if v.Union != nil {
			return v.Union(tt)
		}
	case *typ.Intersection:
		if v.Intersection != nil {
			return v.Intersection(tt)
		}
	case *typ.Array:
		if v.Array != nil {
			return v.Array(tt)
		}
	case *typ.Map:
		if v.Map != nil {
			return v.Map(tt)
		}
	case *typ.ReadonlyMap:
		if v.ReadonlyMap != nil {
			return v.ReadonlyMap(tt)
		}
	case *typ.Tuple:
		if v.Tuple != nil {
			return v.Tuple(tt)
		}
	case *typ.Function:
		if v.Function != nil {
			return v.Function(tt)
		}
	case *typ.Record:
		if v.Record != nil {
			return v.Record(tt)
		}
	case *typ.Alias:
		if v.Alias != nil {
			return v.Alias(tt)
		}
	case *typ.Ref:
		if v.Ref != nil {
			return v.Ref(tt)
		}
	case *typ.Meta:
		if v.Meta != nil {
			return v.Meta(tt)
		}
	case *typ.Generic:
		if v.Generic != nil {
			return v.Generic(tt)
		}
	case *typ.Instantiated:
		if v.Instantiated != nil {
			return v.Instantiated(tt)
		}
	case *typ.TypeParam:
		if v.TypeParam != nil {
			return v.TypeParam(tt)
		}
	case *typ.Interface:
		if v.Interface != nil {
			return v.Interface(tt)
		}
	case *typ.Recursive:
		if v.Recursive != nil {
			return v.Recursive(tt)
		}
	case *typ.Literal:
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
