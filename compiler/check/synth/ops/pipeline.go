package ops

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// ArgReSynth is called to re-synthesize an argument with contextual typing.
type ArgReSynth func(idx int, expected typ.Type) typ.Type

// CallPipeline executes staged call synthesis.
type CallPipeline struct {
	ctx      *db.QueryContext
	def      CallDef
	argCount int
	reSynth  ArgReSynth
	infer    InferResult
	finished bool
}

// NewCallPipeline creates a new call pipeline with the given definition.
func NewCallPipeline(ctx *db.QueryContext, def CallDef, argCount int) *CallPipeline {
	return &CallPipeline{
		ctx:      ctx,
		def:      def,
		argCount: argCount,
	}
}

// WithReSynth sets the re-synthesis callback for contextual typing.
func (p *CallPipeline) WithReSynth(reSynth ArgReSynth) *CallPipeline {
	p.reSynth = reSynth
	return p
}

// WithExpected sets the expected return type for bidirectional generic inference.
func (p *CallPipeline) WithExpected(expected typ.Type) *CallPipeline {
	p.def.ExpectedReturn = expected
	return p
}

// Infer runs the callee-resolution and type-argument inference stage.
func (p *CallPipeline) Infer() InferResult {
	p.infer = InferCall(p.ctx, p.def)
	return p.infer
}

// ExpectedArgType returns the expected type for argument at index idx.
func (p *CallPipeline) ExpectedArgType(idx int) typ.Type {
	if idx < len(p.infer.ExpectedArgs) {
		return p.infer.ExpectedArgs[idx]
	}
	return p.infer.ExpectedVariadic
}

// ReSynthAndReInfer re-synthesizes arguments and re-infers if needed.
func (p *CallPipeline) ReSynthAndReInfer() bool {
	if p.reSynth == nil || p.argCount == 0 {
		return false
	}

	updatedArgs, changed := p.reSynthArgs()
	if !changed {
		return false
	}

	p.def.Args = updatedArgs
	if len(p.def.TypeArgs) == 0 {
		p.infer = ReInfer(p.ctx, p.def, p.infer)
	}
	return true
}

// Finish completes the call and returns the result.
func (p *CallPipeline) Finish() CallResult {
	p.finished = true
	return FinishCall(p.ctx, p.def, p.infer)
}

// Run executes the full pipeline: Infer -> ReSynthAndReInfer -> Finish.
func (p *CallPipeline) Run() CallResult {
	p.Infer()
	p.ReSynthAndReInfer()
	return p.Finish()
}

// reSynthArgs re-synthesizes arguments using the callback.
func (p *CallPipeline) reSynthArgs() ([]typ.Type, bool) {
	result := make([]typ.Type, p.argCount)
	copy(result, p.def.Args)
	changed := false

	for i := 0; i < p.argCount; i++ {
		expected := p.ExpectedArgType(i)
		if expected == nil {
			continue
		}

		reSynthed := p.reSynth(i, expected)
		if selected, ok := refinedArg(result[i], reSynthed); ok {
			result[i] = selected
			changed = true
		}
	}

	return result, changed
}

func refinedArg(existing, candidate typ.Type) (typ.Type, bool) {
	if candidate == nil || typ.TypeEquals(existing, candidate) {
		return existing, false
	}
	if typ.IsAbsentOrUnknown(existing) {
		return candidate, true
	}
	if typ.IsAbsentOrUnknown(candidate) {
		return existing, false
	}
	if typ.IsAny(existing) && !typ.IsAny(candidate) {
		return candidate, true
	}
	if typ.IsAny(candidate) && !typ.IsAny(existing) {
		return existing, false
	}
	if typ.ContainsRecursive(existing) || typ.ContainsRecursive(candidate) {
		return existing, false
	}
	if subtype.IsSubtype(candidate, existing) {
		return candidate, true
	}
	if subtype.IsSubtype(existing, candidate) {
		return existing, false
	}
	return candidate, true
}
