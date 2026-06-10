package effect

type LabelVisitor[R any] struct {
	Return           func(Return) R
	ErrorReturn      func(ErrorReturn) R
	ReturnLength     func(ReturnLength) R
	PassThrough      func(PassThrough) R
	FlowInto         func(FlowInto) R
	CorrelatedReturn func(CorrelatedReturn) R
	Default          func(Label) R
}

func VisitLabel[R any](l Label, v LabelVisitor[R]) R {
	switch ll := l.(type) {
	case Return:
		if v.Return != nil {
			return v.Return(ll)
		}
	case *Return:
		if v.Return != nil {
			return v.Return(*ll)
		}
	case ErrorReturn:
		if v.ErrorReturn != nil {
			return v.ErrorReturn(ll)
		}
	case *ErrorReturn:
		if v.ErrorReturn != nil {
			return v.ErrorReturn(*ll)
		}
	case ReturnLength:
		if v.ReturnLength != nil {
			return v.ReturnLength(ll)
		}
	case *ReturnLength:
		if v.ReturnLength != nil {
			return v.ReturnLength(*ll)
		}
	case PassThrough:
		if v.PassThrough != nil {
			return v.PassThrough(ll)
		}
	case *PassThrough:
		if v.PassThrough != nil {
			return v.PassThrough(*ll)
		}
	case FlowInto:
		if v.FlowInto != nil {
			return v.FlowInto(ll)
		}
	case *FlowInto:
		if v.FlowInto != nil {
			return v.FlowInto(*ll)
		}
	case CorrelatedReturn:
		if v.CorrelatedReturn != nil {
			return v.CorrelatedReturn(ll)
		}
	case *CorrelatedReturn:
		if v.CorrelatedReturn != nil {
			return v.CorrelatedReturn(*ll)
		}
	}
	if v.Default != nil {
		return v.Default(l)
	}
	var zero R
	return zero
}

type ReturnTypeVisitor[R any] struct {
	ElementOf             func(ElementOf) R
	OptionalElementOf     func(OptionalElementOf) R
	CallbackReturn        func(CallbackReturn) R
	ArrayOfCallbackReturn func(ArrayOfCallbackReturn) R
	SameAs                func(SameAs) R
	DeepElementOf         func(DeepElementOf) R
	StringUnpackValue     func(StringUnpackValue) R
	TypeProjection        func(TypeProjection) R
	SelectCaseOfParam     func(SelectCaseOfParam) R
	SelectResultOfCases   func(SelectResultOfCases) R
	Default               func(ReturnType) R
}

func VisitReturnType[R any](t ReturnType, v ReturnTypeVisitor[R]) R {
	switch tt := t.(type) {
	case ElementOf:
		if v.ElementOf != nil {
			return v.ElementOf(tt)
		}
	case *ElementOf:
		if v.ElementOf != nil {
			return v.ElementOf(*tt)
		}
	case OptionalElementOf:
		if v.OptionalElementOf != nil {
			return v.OptionalElementOf(tt)
		}
	case *OptionalElementOf:
		if v.OptionalElementOf != nil {
			return v.OptionalElementOf(*tt)
		}
	case CallbackReturn:
		if v.CallbackReturn != nil {
			return v.CallbackReturn(tt)
		}
	case *CallbackReturn:
		if v.CallbackReturn != nil {
			return v.CallbackReturn(*tt)
		}
	case ArrayOfCallbackReturn:
		if v.ArrayOfCallbackReturn != nil {
			return v.ArrayOfCallbackReturn(tt)
		}
	case *ArrayOfCallbackReturn:
		if v.ArrayOfCallbackReturn != nil {
			return v.ArrayOfCallbackReturn(*tt)
		}
	case SameAs:
		if v.SameAs != nil {
			return v.SameAs(tt)
		}
	case *SameAs:
		if v.SameAs != nil {
			return v.SameAs(*tt)
		}
	case DeepElementOf:
		if v.DeepElementOf != nil {
			return v.DeepElementOf(tt)
		}
	case *DeepElementOf:
		if v.DeepElementOf != nil {
			return v.DeepElementOf(*tt)
		}
	case StringUnpackValue:
		if v.StringUnpackValue != nil {
			return v.StringUnpackValue(tt)
		}
	case *StringUnpackValue:
		if v.StringUnpackValue != nil {
			return v.StringUnpackValue(*tt)
		}
	case TypeProjection:
		if v.TypeProjection != nil {
			return v.TypeProjection(tt)
		}
	case *TypeProjection:
		if v.TypeProjection != nil {
			return v.TypeProjection(*tt)
		}
	case SelectCaseOfParam:
		if v.SelectCaseOfParam != nil {
			return v.SelectCaseOfParam(tt)
		}
	case *SelectCaseOfParam:
		if v.SelectCaseOfParam != nil {
			return v.SelectCaseOfParam(*tt)
		}
	case SelectResultOfCases:
		if v.SelectResultOfCases != nil {
			return v.SelectResultOfCases(tt)
		}
	case *SelectResultOfCases:
		if v.SelectResultOfCases != nil {
			return v.SelectResultOfCases(*tt)
		}
	}
	if v.Default != nil {
		return v.Default(t)
	}
	var zero R
	return zero
}
