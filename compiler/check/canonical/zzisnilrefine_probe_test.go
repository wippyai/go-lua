package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestZZIsNilRefine(t *testing.T) {
	testSrc := `
local M = {}
function M.is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end
return M
`
	legacy := testutil.CheckAndExport(testSrc, "test", testutil.WithStdlib())
	for name, s := range legacy.Manifest.AllSummaries() {
		t.Logf("LEGACY %s: ensures=%v exprEnsures=%v effects=%v returnsParam=%d", name, s.Ensures, s.ExprEnsures, s.Effects, s.ReturnsParam)
	}
	canon := testutil.CheckAndExport(testSrc, "test", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for name, s := range canon.Manifest.AllSummaries() {
		t.Logf("CANON %s: ensures=%v exprEnsures=%v effects=%v returnsParam=%d", name, s.Ensures, s.ExprEnsures, s.Effects, s.ReturnsParam)
	}
	if len(canon.Manifest.AllSummaries()) == 0 {
		t.Logf("CANON: NO summaries")
	}
}
