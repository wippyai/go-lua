package functionfact

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func functionFactTest(opts ...func(*api.FunctionFact)) api.FunctionFact {
	var ff api.FunctionFact
	for _, opt := range opts {
		if opt != nil {
			opt(&ff)
		}
	}
	return ff
}

func factCallParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Call.Params = product.LiftVector(params)
	}
}

func factBodyParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Body.Params = product.LiftVector(params)
	}
}

func factEntryParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Entry.Params = product.LiftVector(params)
	}
}

func factPreflowReturns(returns ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Returns.Preflow = product.LiftVector(returns)
	}
}

func factPostflowReturns(returns ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Returns.Postflow = product.LiftVector(returns)
	}
}

func factSignature(sig *typ.Function) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Public.Signature = sig
	}
}

func factRefinement(refinement *constraint.FunctionRefinement) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Effects.Refinement = refinement
	}
}

func factEnvReturns(envReturns ...contract.EnvReturnSpec) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Export.EnvReturns = envReturns
	}
}

func factReturnProjection(preflow, postflow []typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Returns.Preflow = product.LiftVector(preflow)
		ff.Returns.Postflow = product.LiftVector(postflow)
	}
}

func factCallParamTypesTest(ff api.FunctionFact) []typ.Type {
	return product.ProjectVector(ff.Call.Params)
}

func factBodyParamTypesTest(ff api.FunctionFact) []typ.Type {
	return product.ProjectVector(ff.Body.Params)
}

func factEntryParamTypesTest(ff api.FunctionFact) []typ.Type {
	return product.ProjectVector(ff.Entry.Params)
}

func factPreflowTypesTest(ff api.FunctionFact) []typ.Type {
	return product.ProjectVector(ff.Returns.Preflow)
}

func factPostflowTypesTest(ff api.FunctionFact) []typ.Type {
	return product.ProjectVector(ff.Returns.Postflow)
}
