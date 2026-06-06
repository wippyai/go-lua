package transform

import (
	"github.com/wippyai/go-lua/types/effect/returntransform"
	"github.com/wippyai/go-lua/types/typ"
)

// ApplyEffectTransform is kept as a compatibility bridge for synth callers.
// The return-effect algebra is owned by types/effect/returntransform.
func ApplyEffectTransform(fn *typ.Function, args []typ.Type, returnIdx int, baseReturn typ.Type) typ.Type {
	return returntransform.ApplyEffectTransform(fn, args, returnIdx, baseReturn)
}
