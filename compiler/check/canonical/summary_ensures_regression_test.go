package canonical_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestIsNilSummaryEnsuresRegression(t *testing.T) {
	testSrc := `
local M = {}
function M.is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end
return M
`
	canon := testutil.CheckAndExport(testSrc, "test", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	summary, ok := canon.Manifest.AllSummaries()["is_nil"]
	if !ok {
		t.Fatalf("expected canonical summary for is_nil, got %v", canon.Manifest.AllSummaries())
	}
	if got := fmt.Sprint(summary.Ensures); !strings.Contains(got, "isnil($0)") {
		t.Fatalf("expected is_nil summary to ensure isnil($0), got %s", got)
	}
}
