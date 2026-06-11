package iteration

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
)

var _ effect.Label = Iterator{}

type IteratorKind int

const (
	IterateIndexed IteratorKind = iota
	IterateKeyed
)

type Iterator struct {
	Source effect.ParamRef
	Kind   IteratorKind
}

func (Iterator) EffectLabel() {}
func (i Iterator) String() string {
	kind := "indexed"
	if i.Kind == IterateKeyed {
		kind = "keyed"
	}
	return fmt.Sprintf("iterator(%s, %s)", i.Source, kind)
}
func (i Iterator) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Iterator); ok {
		return i.Source.Index == o.Source.Index && i.Kind == o.Kind
	}
	return false
}
