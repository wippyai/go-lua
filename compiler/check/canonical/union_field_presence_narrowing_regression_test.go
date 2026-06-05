package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestCanonicalFieldPresenceFalseBranchNarrowsRootUnionMember(t *testing.T) {
	src := `
type Accepted = {
    id: string,
    attempt: number,
}

type Rejected = {
    id: string,
    reason: string,
}

type Decision = Accepted | Rejected

local function decide(flag: boolean): Decision
    if flag then
        return {id = "job", attempt = 2}
    end
    return {id = "job", reason = "retry_limit"}
end

local outcome = decide(true)
if outcome.reason then
    local reason: string = outcome.reason
else
    local attempt: number = outcome.attempt
end
`
	res := testutil.Check(src)
	fn := findFunctionWithLocalTarget(t, res.Session.Results, "attempt")
	outcomeSym := singleSymbolNamed(t, fn.Graph, "outcome")
	point, _, _ := assignmentSourceForTarget(t, fn.Graph, "attempt")
	path := constraint.NewPath(outcomeSym, "outcome")

	got := fn.NarrowedTypeAt(point, path)
	if !typeHasField(got, "attempt") || typeHasField(got, "reason") {
		t.Fatalf("NarrowedTypeAt(outcome at else read) = %v, want Accepted-like branch; diagnostics=%v", got, testutil.ErrorMessages(res.Diagnostics))
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean field-presence narrowing, got diagnostics: %v", msgs)
	}
}

func TestCanonicalImportedFieldPresenceFalseBranchDerivesFromNarrowedRoot(t *testing.T) {
	protocol := testutil.CheckAndExport(`
type Request = {
    id: string,
    retries: number,
}

type Accepted = {
    id: string,
    attempt: number,
}

type Rejected = {
    id: string,
    reason: string,
}

type Decision = Accepted | Rejected

local M = {}
M.Request = Request
M.Accepted = Accepted
M.Rejected = Rejected
M.Decision = Decision
return M
`, "protocol")
	if protocol.HasError() {
		t.Fatalf("protocol module errors: %v", testutil.ErrorMessages(protocol.Errors))
	}
	decision := testutil.CheckAndExport(`
local protocol = require("protocol")

local M = {}

function M.accept(request: protocol.Request): protocol.Accepted
    return {
        id = request.id,
        attempt = request.retries + 1,
    }
end

function M.reject(request: protocol.Request, reason: string): protocol.Rejected
    return {
        id = request.id,
        reason = reason,
    }
end

function M.decide(request: protocol.Request): protocol.Decision
    if request.retries > 3 then
        return M.reject(request, "retry_limit")
    end
    return M.accept(request)
end

return M
`, "decision", testutil.WithStdlib(), testutil.WithModule("protocol", protocol))
	if decision.HasError() {
		t.Fatalf("decision module errors: %v", testutil.ErrorMessages(decision.Errors))
	}
	res := testutil.Check(`
local decision = require("decision")
local protocol = require("protocol")

local request: protocol.Request = {
    id = "job",
    retries = 1,
}

local outcome = decision.decide(request)
if outcome.reason then
    local reason: string = outcome.reason
else
    local attempt: number = outcome.attempt
end
`, testutil.WithStdlib(), testutil.WithModule("protocol", protocol), testutil.WithModule("decision", decision))
	fn := findFunctionWithLocalTarget(t, res.Session.Results, "attempt")
	outcomeSym := singleSymbolNamed(t, fn.Graph, "outcome")
	point, _, _ := assignmentSourceForTarget(t, fn.Graph, "attempt")
	rootPath := constraint.NewPath(outcomeSym, "outcome")
	attemptPath := rootPath.Field("attempt")

	root := fn.NarrowedTypeAt(point, rootPath)
	attempt := fn.NarrowedTypeAt(point, attemptPath)
	if !typeHasField(root, "attempt") || typeHasField(root, "reason") {
		t.Fatalf("NarrowedTypeAt(imported outcome at else read) = %v, want Accepted-like branch; attempt=%v diagnostics=%v", root, attempt, testutil.ErrorMessages(res.Diagnostics))
	}
	if !typ.TypeEquals(attempt, typ.Number) {
		t.Fatalf("NarrowedTypeAt(imported outcome.attempt at else read) = %v, want number; root=%v diagnostics=%v", attempt, root, testutil.ErrorMessages(res.Diagnostics))
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean imported field-presence narrowing, got diagnostics: %v", msgs)
	}
}

func typeHasField(t typ.Type, field string) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return rec.GetField(field) != nil
}
