package effect

import "fmt"

type IteratorKind int

const (
	IterateIndexed IteratorKind = iota
	IterateKeyed
)

type Iterator struct {
	Source ParamRef
	Kind   IteratorKind
}

func (Iterator) label() {}
func (i Iterator) String() string {
	kind := "indexed"
	if i.Kind == IterateKeyed {
		kind = "keyed"
	}
	return fmt.Sprintf("iterator(%s, %s)", i.Source, kind)
}
func (i Iterator) Equals(other Label) bool {
	if o, ok := other.(Iterator); ok {
		return i.Source.Index == o.Source.Index && i.Kind == o.Kind
	}
	return false
}
