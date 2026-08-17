package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestAssemblyNonNilClaimHasNoStaticTarget(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	operand := c.Integer(assemblyTestSpan(), body, 1)
	claim := c.ValueClaim(assemblyTestSpan(), body, kind.ValueClaimNonNil, operand, 0)
	if claim == 0 {
		t.Fatal("ValueClaim rejected a valid non-nil claim")
	}
}
