// Package callreturn owns post-call return projections that depend on runtime
// call arguments and contract effect rows.
package callreturn

import (
	"github.com/wippyai/go-lua/types/callshape"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect/returntransform"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// EffectTransformInput describes a completed call whose pipeline returns still
// need the callee's runtime-argument-sensitive effect transforms.
type EffectTransformInput struct {
	Ctx                 *db.QueryContext
	Query               querycore.TypeOps
	Callee              typ.Type
	Args                []typ.Type
	Returns             []typ.Type
	Receiver            typ.Type
	IsMethod            bool
	ForceMethodReceiver bool
}

// ApplyEffectTransforms applies a callee's return-effect transforms to a return
// tuple. It is copy-on-write: the original Returns slice is reused when no slot
// changes, and a fresh slice is allocated only for real refinements.
func ApplyEffectTransforms(in EffectTransformInput) []typ.Type {
	if len(in.Returns) == 0 {
		return in.Returns
	}
	fn := functionShape(in.Callee)
	if fn == nil {
		return in.Returns
	}
	effectArgs := callshape.RuntimeArgsForEffects(
		in.Ctx,
		in.Query,
		fn,
		in.Args,
		in.Receiver,
		in.IsMethod,
		in.ForceMethodReceiver,
	)
	var out []typ.Type
	for i := range in.Returns {
		transformed := returntransform.ApplyEffectTransform(fn, effectArgs, i, in.Returns[i])
		if transformed == nil || transformed == in.Returns[i] {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(in.Returns))
			copy(out, in.Returns)
		}
		out[i] = transformed
	}
	if out != nil {
		return out
	}
	return in.Returns
}

func functionShape(t typ.Type) *typ.Function {
	t = unwrap.Alias(t)
	if g, ok := t.(*typ.Generic); ok {
		t = g.Body
	}
	if inst, ok := t.(*typ.Instantiated); ok {
		resolved, err := querycore.ResolveInstantiated(inst)
		if err == nil && resolved != nil {
			t = resolved
		}
	}
	t = unwrap.Alias(t)
	fn, _ := t.(*typ.Function)
	return fn
}
