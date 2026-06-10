package effect

import (
	"fmt"
	"strings"
)

type Var struct {
	Name string
}

func (v *Var) String() string {
	if v == nil {
		return ""
	}
	return v.Name
}

type Row struct {
	Labels []Label
	Tail   *Var
}

var Empty = Row{}

var Unknown = Row{Tail: &Var{Name: "?"}}

func (r Row) Pure() bool {
	return len(r.Labels) == 0 && r.Tail == nil
}

func (r Row) IsClosed() bool {
	return r.Tail == nil
}

func (r Row) IsOpen() bool {
	return r.Tail != nil
}

func (r Row) IsUnknown() bool {
	return r.Tail != nil && r.Tail.Name == "?"
}

func (r Row) Has(check func(Label) bool) bool {
	for _, l := range r.Labels {
		if check(l) {
			return true
		}
	}
	return false
}

func (r Row) HasThrow() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Throw); return ok })
}

func (r Row) HasIO() bool {
	return r.Has(func(l Label) bool { _, ok := l.(IO); return ok })
}

func (r Row) HasDiverge() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Diverge); return ok })
}

func (r Row) HasMutate() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Mutate); return ok })
}

func (r Row) HasIterator() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Iterator); return ok })
}

func (r Row) HasTableMutator() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TableMutator); return ok })
}

func (r Row) GetMutate(paramIdx int) *Mutate {
	for _, l := range r.Labels {
		if m, ok := l.(Mutate); ok && m.Target.Index == paramIdx {
			return &m
		}
	}
	return nil
}

func (r Row) GetReturn(retIdx int) *Return {
	for _, l := range r.Labels {
		if ret, ok := l.(Return); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}
	return nil
}

func (r Row) FlowIntoReturns(retIdx int) []FlowInto {
	var out []FlowInto
	for _, l := range r.Labels {
		if fi, ok := l.(FlowInto); ok && fi.ReturnIndex == retIdx {
			out = append(out, fi)
		}
	}
	return out
}

func (r Row) GetErrorReturn(valueIdx int) *ErrorReturn {
	for _, l := range r.Labels {
		if er, ok := l.(ErrorReturn); ok && er.ValueIndex == valueIdx {
			return &er
		}
	}
	return nil
}

func (r Row) GetCorrelatedReturn(idx int) *CorrelatedReturn {
	for _, l := range r.Labels {
		if cr, ok := l.(CorrelatedReturn); ok {
			for _, i := range cr.Indices {
				if i == idx {
					return &cr
				}
			}
		}
	}
	return nil
}

func (r Row) GetReturnLength(retIdx int) *ReturnLength {
	for _, l := range r.Labels {
		if ret, ok := l.(ReturnLength); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}
	return nil
}

func (r Row) GetIterator() *Iterator {
	for _, l := range r.Labels {
		if iter, ok := l.(Iterator); ok {
			return &iter
		}
	}
	return nil
}

func (r Row) GetTableMutator() *TableMutator {
	for _, l := range r.Labels {
		if mut, ok := l.(TableMutator); ok {
			return &mut
		}
	}
	return nil
}

func (r Row) IsIndexedIterator() bool {
	iter := r.GetIterator()
	return iter != nil && iter.Kind == IterateIndexed
}

func (r Row) IsKeyedIterator() bool {
	iter := r.GetIterator()
	return iter != nil && iter.Kind == IterateKeyed
}

func (r Row) String() string {
	if r.Pure() {
		return "{}"
	}
	parts := make([]string, 0, len(r.Labels))
	for _, l := range r.Labels {
		parts = append(parts, l.String())
	}
	if r.Tail != nil {
		if len(parts) == 0 {
			return fmt.Sprintf("{%s}", r.Tail.Name)
		}
		return fmt.Sprintf("{%s | %s}", strings.Join(parts, ", "), r.Tail.Name)
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

func (r Row) With(labels ...Label) Row {
	newLabels := make([]Label, 0, len(r.Labels)+len(labels))
	newLabels = append(newLabels, r.Labels...)
	result := Row{Labels: newLabels, Tail: r.Tail}
	for _, l := range labels {
		if !containsLabelEquals(result.Labels, l) {
			result.Labels = append(result.Labels, l)
		}
	}
	return result
}

func (r Row) Without(match func(Label) bool) Row {
	var newLabels []Label
	for _, l := range r.Labels {
		if !match(l) {
			newLabels = append(newLabels, l)
		}
	}
	return Row{Labels: newLabels, Tail: r.Tail}
}

func (r Row) Equals(other any) bool {
	otherRow, ok := other.(Row)
	if !ok {
		return false
	}
	return r.equalsRow(otherRow)
}

func (r Row) IsEffectInfo() {}

func (r Row) equalsRow(other Row) bool {
	if len(r.Labels) != len(other.Labels) {
		return false
	}
	for _, l := range r.Labels {
		if !containsLabelEquals(other.Labels, l) {
			return false
		}
	}
	if r.Tail == nil && other.Tail == nil {
		return true
	}
	if r.Tail == nil || other.Tail == nil {
		return false
	}
	return r.Tail.Name == other.Tail.Name
}

func Throws() Row {
	return Row{Labels: []Label{Throw{}}}
}

func Mutates(paramIdx int, transform TypeTransform) Row {
	return Row{Labels: []Label{Mutate{
		Target:    ParamRef{Index: paramIdx},
		Transform: transform,
	}}}
}

func Returns(retIdx int, derive ReturnType) Row {
	return Row{Labels: []Label{Return{ReturnIndex: retIdx, Transform: derive}}}
}

func WithIO() Row {
	return Row{Labels: []Label{IO{}}}
}

func MayDiverge() Row {
	return Row{Labels: []Label{Diverge{}}}
}

func BorrowsOnly() Row {
	return Row{Labels: []Label{BorrowAll{}}}
}

func StoresParam(paramIdx int, intoIdx int) Row {
	return Row{Labels: []Label{Store{
		Param: ParamRef{Index: paramIdx},
		Into:  ParamRef{Index: intoIdx},
	}}}
}

func (r Row) HasBorrow() bool {
	return r.Has(func(l Label) bool {
		if _, ok := l.(Borrow); ok {
			return true
		}
		_, ok := l.(BorrowAll)
		return ok
	})
}

func (r Row) HasStore() bool {
	return r.Has(func(l Label) bool { _, ok := l.(Store); return ok })
}

func (r Row) OnlyBorrows() bool {
	return r.HasBorrow() && !r.HasStore() && !r.HasMutate()
}

func (r Row) GetBorrow(paramIdx int) *Borrow {
	for _, l := range r.Labels {
		if b, ok := l.(Borrow); ok && b.Param.Index == paramIdx {
			return &b
		}
	}
	return nil
}

func (r Row) GetStore(paramIdx int) *Store {
	for _, l := range r.Labels {
		if s, ok := l.(Store); ok && s.Param.Index == paramIdx {
			return &s
		}
	}
	return nil
}

func (r Row) BorrowsAllParams() bool {
	return r.Has(func(l Label) bool { _, ok := l.(BorrowAll); return ok })
}

func (r Row) HasModuleLoad() bool {
	return r.Has(func(l Label) bool { _, ok := l.(ModuleLoad); return ok })
}

func (r Row) HasVariadicTransform() bool {
	return r.Has(func(l Label) bool { _, ok := l.(VariadicTransform); return ok })
}

func (r Row) HasTypePredicate() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TypePredicate); return ok })
}

func (r Row) HasTypeValueMethod() bool {
	return r.Has(func(l Label) bool { _, ok := l.(TypeValueMethod); return ok })
}

func (r Row) HasCallableType() bool {
	return r.Has(func(l Label) bool { _, ok := l.(CallableType); return ok })
}

func WithModuleLoad() Row {
	return Row{Labels: []Label{ModuleLoad{}}}
}

func WithVariadicTransform() Row {
	return Row{Labels: []Label{VariadicTransform{}}}
}

func WithTypePredicate() Row {
	return Row{Labels: []Label{TypePredicate{}}}
}

func WithTypeValueMethod() Row {
	return Row{Labels: []Label{TypeValueMethod{}}}
}

func WithCallableType() Row {
	return Row{Labels: []Label{CallableType{}}}
}
