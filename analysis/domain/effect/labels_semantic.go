package effect

type ModuleLoad struct{}

func (ModuleLoad) label()         {}
func (ModuleLoad) String() string { return "module_load" }
func (ModuleLoad) Equals(other Label) bool {
	_, ok := other.(ModuleLoad)
	return ok
}

type VariadicTransform struct{}

func (VariadicTransform) label()         {}
func (VariadicTransform) String() string { return "variadic_transform" }
func (VariadicTransform) Equals(other Label) bool {
	_, ok := other.(VariadicTransform)
	return ok
}

type TypePredicate struct{}

func (TypePredicate) label()         {}
func (TypePredicate) String() string { return "type_predicate" }
func (TypePredicate) Equals(other Label) bool {
	_, ok := other.(TypePredicate)
	return ok
}

type TypeValueMethod struct{}

func (TypeValueMethod) label()         {}
func (TypeValueMethod) String() string { return "type_value_method" }
func (TypeValueMethod) Equals(other Label) bool {
	_, ok := other.(TypeValueMethod)
	return ok
}

type CallableType struct{}

func (CallableType) label()         {}
func (CallableType) String() string { return "callable_type" }
func (CallableType) Equals(other Label) bool {
	_, ok := other.(CallableType)
	return ok
}
