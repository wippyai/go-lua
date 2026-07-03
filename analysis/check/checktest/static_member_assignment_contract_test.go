package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

func TestStaticMemberAssignmentAllowsInferredTableShapeToMutate(t *testing.T) {
	result := Check(`
local scratch = { local_only = true }
scratch.local_only = false
local b: boolean = scratch.local_only
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for mutable inferred table member", result.Diagnostics)
	}
}

func TestStaticMemberAssignmentRejectsDeclaredTableShapeMismatch(t *testing.T) {
	src := strings.TrimLeft(`
type Scratch = { local_only: true }

local scratch: Scratch = { local_only = true }
scratch.local_only = false
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            4,
		Column:          22,
		MessageContains: []string{
			"cannot assign false to true",
		},
		EvidenceOrdered: []string{
			"assigned value has literal value false",
			"assignment target scratch.local_only requires true",
			"no proof on this path shows assigned value is true",
		},
	})
}
