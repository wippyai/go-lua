package lifecycle

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

var (
	_ effect.Label = Acquire{}
	_ effect.Label = Transition{}
	_ effect.Label = Escape{}
)

type Acquire struct {
	Target     effect.ParamRef
	Protocol   typestate.Protocol
	State      typestate.State
	Obligation typestate.Obligation
}

func (Acquire) EffectLabel() {}
func (a Acquire) String() string {
	if a.Obligation.Final != "" {
		return fmt.Sprintf("lifecycle.acquire(%s, %s:%s -> %s)", a.Target, a.Protocol, a.State, a.Obligation.Final)
	}
	return fmt.Sprintf("lifecycle.acquire(%s, %s:%s)", a.Target, a.Protocol, a.State)
}
func (a Acquire) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Acquire); ok {
		return a.Target.Index == o.Target.Index &&
			a.Protocol == o.Protocol &&
			a.State == o.State &&
			a.Obligation == o.Obligation
	}
	return false
}

type Transition struct {
	Target   effect.ParamRef
	Protocol typestate.Protocol
	From     typestate.State
	To       typestate.State
}

func (Transition) EffectLabel() {}
func (t Transition) String() string {
	if t.From != "" {
		return fmt.Sprintf("lifecycle.transition(%s, %s:%s -> %s)", t.Target, t.Protocol, t.From, t.To)
	}
	return fmt.Sprintf("lifecycle.transition(%s, %s:* -> %s)", t.Target, t.Protocol, t.To)
}
func (t Transition) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Transition); ok {
		return t.Target.Index == o.Target.Index &&
			t.Protocol == o.Protocol &&
			t.From == o.From &&
			t.To == o.To
	}
	return false
}

type Escape struct {
	Target   effect.ParamRef
	Protocol typestate.Protocol
}

func (Escape) EffectLabel() {}
func (e Escape) String() string {
	return fmt.Sprintf("lifecycle.escape(%s, %s)", e.Target, e.Protocol)
}
func (e Escape) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Escape); ok {
		return e.Target.Index == o.Target.Index && e.Protocol == o.Protocol
	}
	return false
}
