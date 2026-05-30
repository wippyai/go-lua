package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestZZOutputLabel(t *testing.T) {
	src := `
type RenderOutput = { kind: "rendered", body: string, label: string? }
type IndexOutput = { kind: "indexed", count: integer }
type AuditOutput = { kind: "audited", note: string, retry_after: string? }
type Output = RenderOutput | IndexOutput | AuditOutput
local M = {}
function M.output_label(output: Output): string
    if output.kind == "rendered" then
        return output.body
    end
    if output.kind == "indexed" then
        return tostring(output.count)
    end
    return output.note
end
return M
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("DIAG: %s", m)
	}
}
