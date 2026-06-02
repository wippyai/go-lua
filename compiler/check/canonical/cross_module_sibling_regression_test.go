package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCrossModuleSiblingNarrowingRegression(t *testing.T) {
	testSrc := `
local M = {}
function M.is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end
return M
`
	clientSrc := `
local M = {}
type Response = { metadata: { response_id: string } }
function M.request(ok: boolean): (Response?, string?)
    if ok then
        return { metadata = { response_id = "resp-123" } }, nil
    end
    return nil, "failed"
end
return M
`
	mainSrc := `
local test = require("test")
local client = require("client")
local response, err = client.request(true)
test.is_nil(err, "no error expected")
local id: string = response.metadata.response_id
return id
`
	testMod := testutil.CheckAndExport(testSrc, "test", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	clientMod := testutil.CheckAndExport(clientSrc, "client", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	res := testutil.Check(mainSrc, testutil.WithStdlib(),
		testutil.WithModule("test", testMod),
		testutil.WithModule("client", clientMod),
		testutil.WithCheckOption(check.WithCanonicalFlow()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected cross-module sibling narrowing to be clean, got diagnostics: %v", msgs)
	}
}
