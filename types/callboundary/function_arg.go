package callboundary

import "github.com/wippyai/go-lua/types/typ"

// ProjectContextualFunctionArg forms the public call-boundary type for a
// contextual function argument. The callee's expected parameter slots are the
// authority for what the callback accepts; the synthesized callback body is the
// authority for what it returns.
func ProjectContextualFunctionArg(expected, candidate typ.Type) typ.Type {
	expectedFn, ok := expected.(*typ.Function)
	if !ok || expectedFn == nil {
		return candidate
	}
	candidateFn, ok := candidate.(*typ.Function)
	if !ok || candidateFn == nil || len(expectedFn.Params) != len(candidateFn.Params) {
		return candidate
	}

	builder := typ.Func()
	for _, tp := range candidateFn.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for i, param := range candidateFn.Params {
		expectedType := expectedFn.Params[i].Type
		if expectedType == nil {
			expectedType = param.Type
		}
		if param.Optional || expectedFn.Params[i].Optional {
			builder = builder.OptParam(param.Name, expectedType)
		} else {
			builder = builder.Param(param.Name, expectedType)
		}
	}
	if expectedFn.Variadic != nil {
		builder = builder.Variadic(expectedFn.Variadic)
	} else if candidateFn.Variadic != nil {
		builder = builder.Variadic(candidateFn.Variadic)
	}
	if len(candidateFn.Returns) > 0 {
		builder = builder.Returns(candidateFn.Returns...)
	} else if len(expectedFn.Returns) > 0 {
		builder = builder.Returns(expectedFn.Returns...)
	}
	if candidateFn.Effects != nil {
		builder = builder.Effects(candidateFn.Effects)
	}
	if candidateFn.Spec != nil {
		builder = builder.Spec(candidateFn.Spec)
	}
	if candidateFn.Refinement != nil {
		builder = builder.WithRefinement(candidateFn.Refinement)
	}
	return builder.Build()
}
