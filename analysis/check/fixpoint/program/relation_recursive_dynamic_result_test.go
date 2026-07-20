package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationProgramRecursiveResultFeedsDynamicIndexWrite(t *testing.T) {
	stmts := parseChunk(t, `
local function resolve(value: unknown): unknown
    if type(value) ~= "table" then
        return value
    end
    local resolved: {unknown} = {}
    for i, item in ipairs(value) do
        resolved[i] = resolve(item)
    end
    return resolved
end
return resolve({})
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ipairs", "type"}})
	if _, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{
		Registry: standard.Registry(), Globals: []string{"ipairs", "type"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}}); err != nil {
		t.Fatal(err)
	}
}
