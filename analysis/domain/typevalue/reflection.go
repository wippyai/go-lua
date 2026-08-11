package typevalue

import "github.com/wippyai/go-lua/analysis/semantic/typeauthority"

// Form reports the exact VM-visible kind of a known descriptor. OtherInner
// has no fabricated kind and therefore returns decided=false.
func (a *Authority) Form(descriptor Descriptor) (typeauthority.Form, bool) {
	row, ok := a.descriptor(descriptor)
	if !ok || row.innerKind != innerExact {
		return 0, false
	}
	return a.runtime.Form(row.inner)
}

// Reflected returns the exact structural child for elem/key/val/inner/ret.
// present=false,decided=true is the VM's nil result for an exact incompatible
// descriptor. decided=false is reserved for OtherInner.
func (a *Authority) Reflected(descriptor Descriptor, selector Selector) (child Descriptor, present, decided bool) {
	row, ok := a.descriptor(descriptor)
	if !ok {
		return Descriptor{}, false, false
	}
	if row.innerKind != innerExact {
		child, present = a.summaryChild()
		return child, present, false
	}
	runtime := a.runtime
	var inner typeauthority.RuntimeInner
	switch selector {
	case SelectorElem:
		inner, present = runtime.Element(row.inner)
	case SelectorKey:
		inner, _, present = runtime.Mapping(row.inner)
	case SelectorVal:
		_, inner, present = runtime.Mapping(row.inner)
	case SelectorInner:
		inner, present = runtime.Inner(row.inner)
	case SelectorRet:
		inner, present = runtime.Return(row.inner)
	default:
		return Descriptor{}, false, false
	}
	if !present {
		return Descriptor{}, false, true
	}
	child, present = a.descriptorFor(descriptorRow{innerKind: innerExact, inner: inner, nameKind: NameNone, resolverKind: resolverNone})
	return child, present, present
}

func (a *Authority) summaryChild() (Descriptor, bool) {
	return a.descriptorFor(descriptorRow{innerKind: innerOther, nameKind: NameNone, resolverKind: resolverNone})
}

func (a *Authority) descriptorFor(row descriptorRow) (Descriptor, bool) {
	if a == nil {
		return Descriptor{}, false
	}
	index, ok := a.descriptorIndex[descriptorKey(row)]
	return Descriptor{owner: a, index: index}, ok
}

// IteratorLength reports the exact entry count for a compatible reflection
// iterator. compatible=false,decided=true means the selector returns nil.
func (a *Authority) IteratorLength(descriptor Descriptor, selector Selector) (length int, compatible, decided bool) {
	row, ok := a.descriptor(descriptor)
	if !ok {
		return 0, false, false
	}
	if row.innerKind != innerExact {
		return 0, true, false
	}
	runtime := a.runtime
	form, ok := runtime.Form(row.inner)
	if !ok {
		return 0, false, false
	}
	switch selector {
	case SelectorFields:
		if form != typeauthority.FormRecord {
			return 0, false, true
		}
		return runtime.FieldCount(row.inner), true, true
	case SelectorVariants:
		if form != typeauthority.FormUnion {
			return 0, false, true
		}
		return runtime.VariantCount(row.inner), true, true
	case SelectorParams:
		if form != typeauthority.FormFunction {
			return 0, false, true
		}
		return runtime.ParameterCount(row.inner), true, true
	case SelectorTparams:
		if form != typeauthority.FormGeneric {
			return 0, false, true
		}
		return runtime.TypeParameterCount(row.inner), true, true
	default:
		return 0, false, false
	}
}

// IteratorEntry returns the VM-order name/type payload at one exact cursor.
// namePresent distinguishes variants/params from fields/tparams; childPresent
// preserves the runtime nil used by absent constraints.
func (a *Authority) IteratorEntry(descriptor Descriptor, selector Selector, index int) (name string, namePresent bool, child Descriptor, childPresent bool, ok bool) {
	row, valid := a.descriptor(descriptor)
	if !valid || row.innerKind != innerExact || index < 0 {
		return "", false, Descriptor{}, false, false
	}
	runtime := a.runtime
	var inner typeauthority.RuntimeInner
	switch selector {
	case SelectorFields:
		name, inner, childPresent, valid = runtime.FieldAt(row.inner, index)
		namePresent = valid
	case SelectorVariants:
		inner, childPresent, valid = runtime.VariantAt(row.inner, index)
	case SelectorParams:
		inner, childPresent, valid = runtime.ParameterAt(row.inner, index)
	case SelectorTparams:
		name, inner, childPresent, valid = runtime.TypeParameterAt(row.inner, index)
		namePresent = valid
	default:
		return "", false, Descriptor{}, false, false
	}
	if !valid {
		return "", false, Descriptor{}, false, false
	}
	if !childPresent {
		return name, namePresent, Descriptor{}, false, true
	}
	child, childPresent = a.descriptorFor(descriptorRow{innerKind: innerExact, inner: inner, nameKind: NameNone, resolverKind: resolverNone})
	return name, namePresent, child, childPresent, childPresent
}

// RecordField resolves the non-reserved exact record-member branch. found is
// distinct from childPresent because the VM returns nil for a declared field
// whose reflected type is absent. decided=false is reserved for OtherInner.
func (a *Authority) RecordField(descriptor Descriptor, name string) (child Descriptor, childPresent, found, decided bool) {
	row, ok := a.descriptor(descriptor)
	if !ok {
		return Descriptor{}, false, false, false
	}
	if row.innerKind != innerExact {
		return Descriptor{}, false, false, false
	}
	runtime := a.runtime
	form, ok := runtime.Form(row.inner)
	if !ok {
		return Descriptor{}, false, false, false
	}
	if form != typeauthority.FormRecord {
		return Descriptor{}, false, false, true
	}
	inner, childPresent, found := runtime.Field(row.inner, name)
	if !found || !childPresent {
		return Descriptor{}, childPresent, found, true
	}
	child, childPresent = a.descriptorFor(descriptorRow{innerKind: innerExact, inner: inner, nameKind: NameNone, resolverKind: resolverNone})
	return child, childPresent, true, childPresent
}

// Instantiate applies only an existing canonical Runtime instantiation. If the
// exact result is outside D it returns the subject's declared summary image;
// it never grows either authority or constructs a type graph.
func (a *Authority) Instantiate(subject Descriptor, arguments []Descriptor) (Descriptor, bool, bool) {
	row, ok := a.descriptor(subject)
	if !ok || row.innerKind != innerExact {
		return Descriptor{}, false, false
	}
	runtime := a.runtime
	hasSummaryArgument := false
	for _, argument := range arguments {
		argumentRow, valid := a.descriptor(argument)
		if !valid {
			return Descriptor{}, false, false
		}
		if argumentRow.innerKind != innerExact {
			hasSummaryArgument = true
		}
	}
	if form, valid := runtime.Form(row.inner); !valid || form != typeauthority.FormGeneric || len(arguments) != runtime.TypeParameterCount(row.inner) {
		return Descriptor{}, false, true
	}
	if hasSummaryArgument {
		return a.genericSummary(row)
	}
	match, exact := runtime.BeginInstantiation(row.inner)
	if !exact {
		return a.genericSummary(row)
	}
	for _, argument := range arguments {
		argumentRow, _ := a.descriptor(argument)
		match, exact = runtime.MatchInstantiationArgument(match, argumentRow.inner)
		if !exact {
			return a.genericSummary(row)
		}
	}
	result, exact := runtime.FinishInstantiation(match)
	if !exact {
		return a.genericSummary(row)
	}
	descriptor, admitted := a.descriptorFor(descriptorRow{innerKind: innerExact, inner: result, nameKind: row.nameKind, name: row.name, resolverKind: resolverNone})
	if admitted {
		return descriptor, true, true
	}
	return a.genericSummary(row)
}

func (a *Authority) genericSummary(subject descriptorRow) (Descriptor, bool, bool) {
	descriptor, ok := a.descriptorFor(descriptorRow{innerKind: innerOther, nameKind: subject.nameKind, name: subject.name, resolverKind: resolverNone})
	return descriptor, false, ok
}
