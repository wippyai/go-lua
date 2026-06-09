package functionfact

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// carrier.go is the single seam between the api.FunctionFact value-domain
// carriers ([]product.AbstractValue) and the rich per-slot semantic merge
// engines (paramevidence, returnsummary) that operate on []typ.Type.
//
// The carrier stores interned product.AbstractValue per slot, so the convergence
// equality and the stored representation are value-domain. The merge engines keep
// their precise typ.Type logic; this file projects the carriers in for the engines
// (egress, ProjectValue per slot) and lifts the engine result back out (admission,
// FromType per slot). A round-trip through here is the value-domain lossless
// inverse FromType<->ProjectValue, so engine precision is unchanged.

func paramsTypes(ff api.FunctionFact) []typ.Type      { return product.ProjectVector(ff.Call.Params) }
func bodyParamsTypes(ff api.FunctionFact) []typ.Type  { return product.ProjectVector(ff.Body.Params) }
func entryParamsTypes(ff api.FunctionFact) []typ.Type { return product.ProjectVector(ff.Entry.Params) }
func summaryTypes(ff api.FunctionFact) []typ.Type     { return product.ProjectVector(ff.Returns.Preflow) }
func narrowTypes(ff api.FunctionFact) []typ.Type      { return product.ProjectVector(ff.Returns.Postflow) }
