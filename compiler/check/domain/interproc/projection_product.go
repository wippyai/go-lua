package interproc

import "github.com/wippyai/go-lua/compiler/check/api"

// ProjectionProduct is the domain-owned convergence product for the old
// noncanonical postflow/export projection loop. Public checker APIs expose
// typed lanes instead of this mixed product.
type ProjectionProduct struct {
	FunctionFacts     api.FunctionFacts
	CapturedTypes     api.CapturedTypes
	CapturedFields    api.CapturedFieldAssigns
	ConstructorFields api.ConstructorFields
}
