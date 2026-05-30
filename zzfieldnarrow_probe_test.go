package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Minimal reproductions isolating whether canonical discriminant narrowing
// fires when the narrow target is a member-access path (receipt.output) vs a
// plain variable, and whether a generic-instantiated receipt breaks it.

// Plain variable narrow target: baseline (expected clean).
func TestZZNarrowPlainVar(t *testing.T) {
	src := `
type RenderOutput = { kind: "rendered", body: string, label: string? }
type IndexOutput = { kind: "indexed", count: integer }
type Output = RenderOutput | IndexOutput
local function f(output: Output)
    if output.kind == "rendered" then
        local r: RenderOutput = output
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("PLAINVAR DIAG: %s", m)
	}
}

// Member-access narrow target: receipt.output narrowed via receipt.output.kind.
func TestZZNarrowMemberPath(t *testing.T) {
	src := `
type RenderOutput = { kind: "rendered", body: string, label: string? }
type IndexOutput = { kind: "indexed", count: integer }
type Output = RenderOutput | IndexOutput
type Receipt = { plugin: string, output: Output }
local function f(receipt: Receipt)
    if receipt.output.kind == "rendered" then
        local r: RenderOutput = receipt.output
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("MEMBERPATH DIAG: %s", m)
	}
}

// Member-access narrow target with a GENERIC-instantiated receipt, mirroring
// OutputReceipt = Receipt<Output>.
func TestZZNarrowMemberPathGeneric(t *testing.T) {
	src := `
type RenderOutput = { kind: "rendered", body: string, label: string? }
type IndexOutput = { kind: "indexed", count: integer }
type Output = RenderOutput | IndexOutput
type Receipt<T> = { plugin: string, output: T }
type OutputReceipt = Receipt<Output>
local function f(receipt: OutputReceipt)
    if receipt.output.kind == "rendered" then
        local r: RenderOutput = receipt.output
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("GENERIC DIAG: %s", m)
	}
}
