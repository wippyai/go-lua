package flow

import (
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/flow/returnprojection"
)

// BodyReturns is Flow's sole Body-owned executable Return projection. It
// accepts an existing Body boundary proof and returns only existing Causal
// Site proofs; raw Outcome and Values coordinates remain owner-private.
type BodyReturns struct {
	result     *returnprojection.Result
	causal     *causal.Result
	boundaries *functionboundary.Result
}

// ForBody returns the targetless OutcomeReturn and its ordered executable
// Values alternatives for this exact Body boundary. A Body with no executable
// Return has no projection.
func (view BodyReturns) ForBody(body functionboundary.BodyBoundary) (BodyReturn, bool) {
	if view.result == nil || view.causal == nil || !body.Available() {
		return BodyReturn{}, false
	}
	bodyTerm, bodyOK := body.Body()
	issued, issuedOK := view.boundaries.ForBody(bodyTerm)
	outcomeTerm, _, rowOK := view.result.ForBody(bodyTerm)
	boundaries := view.boundaries
	if !bodyOK || !boundaries.OwnsBody(body) || !issuedOK || !issued.Equal(body) || !rowOK {
		return BodyReturn{}, false
	}
	site, siteOK := view.causal.Sites().ForTerm(outcomeTerm)
	projection := BodyReturn{view: view, body: issued, outcome: site}
	return projection, siteOK && projection.Available()
}

// BodyReturn is an opaque owner-fenced Body Return proof. It deliberately
// carries no raw Outcome/Values term or independently derived relation.
type BodyReturn struct {
	view    BodyReturns
	body    functionboundary.BodyBoundary
	outcome causal.Site
}

func (projection BodyReturn) Available() bool {
	if projection.view.result == nil || projection.view.causal == nil || !projection.body.Available() || !projection.outcome.Available() {
		return false
	}
	bodyTerm, bodyOK := projection.body.Body()
	issued, issuedOK := projection.view.boundaries.ForBody(bodyTerm)
	outcomeTerm, _, rowOK := projection.view.result.ForBody(bodyTerm)
	boundaries := projection.view.boundaries
	if !bodyOK || !boundaries.OwnsBody(projection.body) || !issuedOK || !issued.Equal(projection.body) || !rowOK {
		return false
	}
	issuedSite, siteOK := projection.view.causal.Sites().ForTerm(outcomeTerm)
	return siteOK && projection.outcome.Equal(issuedSite)
}

func (projection BodyReturn) Outcome() (causal.Site, bool) {
	if !projection.Available() {
		return causal.Site{}, false
	}
	return projection.outcome, true
}

func (projection BodyReturn) ValuesCount() int {
	if !projection.Available() {
		return 0
	}
	body, bodyOK := projection.body.Body()
	_, count, rowOK := projection.view.result.ForBody(body)
	if !bodyOK || !rowOK {
		return 0
	}
	return count
}

func (projection BodyReturn) ValueAt(index int) (causal.Site, bool) {
	if !projection.Available() {
		return causal.Site{}, false
	}
	body, bodyOK := projection.body.Body()
	value, valueOK := projection.view.result.ValueAt(body, index)
	if !bodyOK || !valueOK {
		return causal.Site{}, false
	}
	site, siteOK := projection.view.causal.Sites().ForTerm(value)
	return site, siteOK && site.Available()
}
