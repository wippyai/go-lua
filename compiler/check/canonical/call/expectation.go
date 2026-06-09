package call

import (
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
)

// CallExpectation is the selected call-site expectation carrier. It keeps
// contextual argument types and post-fixpoint argument obligations together so
// call evidence, callback entry typing, and boundary application consume the
// same selected call-site fact.
type CallExpectation struct {
	ExpectedArgs []typ.Type
	ArgDemands   []callobligation.Obligation
}

func CallExpectationOf(expectedArgs []typ.Type, argDemands []callobligation.Obligation) CallExpectation {
	return CallExpectation{
		ExpectedArgs: cloneExpectationTypes(expectedArgs),
		ArgDemands:   cloneExpectationArgDemands(argDemands),
	}
}

func (e CallExpectation) HasExpectedArgs() bool {
	return len(e.ExpectedArgs) > 0
}

func (e CallExpectation) HasArgDemands() bool {
	return len(e.ArgDemands) > 0
}

func (e CallExpectation) CloneExpectedArgs() []typ.Type {
	return cloneExpectationTypes(e.ExpectedArgs)
}

func (e CallExpectation) CloneArgDemands() []callobligation.Obligation {
	return cloneExpectationArgDemands(e.ArgDemands)
}

func cloneExpectationTypes(in []typ.Type) []typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make([]typ.Type, len(in))
	copy(out, in)
	return out
}

func cloneExpectationArgDemands(in []callobligation.Obligation) []callobligation.Obligation {
	if len(in) == 0 {
		return nil
	}
	out := make([]callobligation.Obligation, len(in))
	copy(out, in)
	return out
}
