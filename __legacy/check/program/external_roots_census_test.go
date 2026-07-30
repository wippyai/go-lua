package program

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func runExternalCensusChunk(t *testing.T, src string) {
	t.Helper()
	stmts := parseChunk(t, src)
	if _, err := RunChunk(stmts, Config{Check: body.Config{
		Registry: standard.Registry(),
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	}}); err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
}

// Distilled from app.test.channel:select_basic (wippy-golua-seam/tests/app).
func TestExternalCensusRootAssignmentScalar(t *testing.T) {
	runExternalCensusChunk(t, `
local ch = channel.new(1)
local result = channel.select{ ch:case_receive() }
`)
}

// Distilled from app.test.types:type_alias (wippy-golua-seam/tests/app).
func TestExternalCensusReturnScalar(t *testing.T) {
	runExternalCensusChunk(t, `
return math.sqrt(4)
`)
}

// Distilled from app.test.network:batch_probe (wippy-golua-seam/tests/app).
func TestExternalCensusReturnCallAuthority(t *testing.T) {
	runExternalCensusChunk(t, `
local function f()
	return 1
end

return { f() }
`)
}

// Distilled from wippy.migration:registry_test (framework/src/migration).
func TestExternalCensusPathDescendantInvalidations(t *testing.T) {
	runExternalCensusChunk(t, `
local store = {}
local entry = { id = "a" }
store[entry.id] = entry
`)
}
