package ownership

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
)

var (
	_ effect.Label = Borrow{}
	_ effect.Label = Retain{}
	_ effect.Label = Store{}
	_ effect.Label = BorrowAll{}
	_ effect.Label = Send{}
	_ effect.Label = SendParam{}
	_ effect.Label = Export{}
	_ effect.Label = Opaque{}
	_ effect.Label = Freeze{}
)

type Borrow struct {
	Param effect.ParamRef
}

func (Borrow) EffectLabel() {}
func (b Borrow) String() string {
	return fmt.Sprintf("borrow(%s)", b.Param)
}
func (b Borrow) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Borrow); ok {
		return b.Param.Index == o.Param.Index
	}
	return false
}

type Retain struct {
	Param effect.ParamRef
}

func (Retain) EffectLabel() {}
func (r Retain) String() string {
	return fmt.Sprintf("retain(%s)", r.Param)
}
func (r Retain) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Retain); ok {
		return r.Param.Index == o.Param.Index
	}
	return false
}

type Store struct {
	Param effect.ParamRef
	Into  effect.ParamRef
}

func (Store) EffectLabel() {}
func (s Store) String() string {
	if s.Into.Index >= 0 {
		return fmt.Sprintf("store(%s into %s)", s.Param, s.Into)
	}
	return fmt.Sprintf("store(%s)", s.Param)
}
func (s Store) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Store); ok {
		return s.Param.Index == o.Param.Index && s.Into.Index == o.Into.Index
	}
	return false
}

type BorrowAll struct{}

func (BorrowAll) EffectLabel()   {}
func (BorrowAll) String() string { return "borrow_all" }
func (BorrowAll) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(BorrowAll)
	return ok
}

type Send struct {
	FromParam int
}

func (Send) EffectLabel() {}
func (s Send) String() string {
	return fmt.Sprintf("send(params[%d:])", s.FromParam)
}
func (s Send) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Send); ok {
		return s.FromParam == o.FromParam
	}
	return false
}

type SendParam struct {
	Param effect.ParamRef
}

func (SendParam) EffectLabel() {}
func (s SendParam) String() string {
	return fmt.Sprintf("send(%s)", s.Param)
}
func (s SendParam) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(SendParam); ok {
		return s.Param.Index == o.Param.Index
	}
	return false
}

type Export struct {
	Param effect.ParamRef
}

func (Export) EffectLabel() {}
func (e Export) String() string {
	return fmt.Sprintf("export(%s)", e.Param)
}
func (e Export) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Export); ok {
		return e.Param.Index == o.Param.Index
	}
	return false
}

type Opaque struct {
	Param effect.ParamRef
}

func (Opaque) EffectLabel() {}
func (o Opaque) String() string {
	return fmt.Sprintf("opaque(%s)", o.Param)
}
func (o Opaque) Equals(other effect.Label) bool {
	if other, ok := effect.NormalizeLabel(other).(Opaque); ok {
		return o.Param.Index == other.Param.Index
	}
	return false
}

type Freeze struct {
	Param effect.ParamRef
}

func (Freeze) EffectLabel() {}
func (f Freeze) String() string {
	return fmt.Sprintf("freeze(%s)", f.Param)
}
func (f Freeze) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Freeze); ok {
		return f.Param.Index == o.Param.Index
	}
	return false
}
