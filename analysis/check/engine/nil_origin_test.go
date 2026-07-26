package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func nilOriginDiagnostics(t *testing.T, source string) []engine.PublishedDiagnostic {
	t.Helper()
	result, err := engine.CheckWithImportsResolverAndGlobalsAndRelations(source, nil, nil, nil, nil, "main.lua")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var out []engine.PublishedDiagnostic
	for _, item := range result.PublishedDiagnostics {
		if item.Code == "type.nil.unsafe_use" {
			out = append(out, item)
		}
	}
	return out
}

func requireNoNilOriginDiagnostic(t *testing.T, name, source string) {
	t.Helper()
	if items := nilOriginDiagnostics(t, source); len(items) != 0 {
		t.Fatalf("%s: nil-origin diagnostics = %#v, want none", name, items)
	}
}

func evidenceLine(t *testing.T, item engine.PublishedDiagnostic, contains string) engine.DiagnosticEvidence {
	t.Helper()
	for _, evidence := range item.Evidence {
		if strings.Contains(evidence.Message, contains) {
			return evidence
		}
	}
	t.Fatalf("evidence %q missing from %#v", contains, item.Evidence)
	return engine.DiagnosticEvidence{}
}

func TestNilOriginUnsafeUsePublishesBirthDeclarationJoinAndUse(t *testing.T) {
	items := nilOriginDiagnostics(t, `
local function compute(): string
    return "hello"
end

local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = compute()
    end
    return x:upper()
end

return run`)
	if len(items) != 1 {
		t.Fatalf("nil-origin diagnostics = %#v, want exactly one", items)
	}
	item := items[0]
	if item.Message != "x may be nil at method call" {
		t.Fatalf("message = %q", item.Message)
	}
	if item.Span.StartLine != 11 || item.Span.StartCol != 12 {
		t.Fatalf("span = %#v, want the method receiver at 11:12", item.Span)
	}
	if item.Help != "Guard x against nil before the method call, or assign it on every branch." {
		t.Fatalf("help = %q", item.Help)
	}
	if len(item.Labels) != 1 || item.Labels[0].Message != "possibly-nil value" || item.Labels[0].Span != item.Span {
		t.Fatalf("labels = %#v", item.Labels)
	}
	if len(item.Evidence) != 4 {
		t.Fatalf("evidence = %#v, want birth, declaration, one join, and the use", item.Evidence)
	}
	birth := evidenceLine(t, item, "x born nil at main.lua:7 (else branch had no assignment)")
	if birth.Kind != diag.EvidenceAbstractFact || birth.Trust != diag.TrustProven || birth.Span.StartLine != 7 || birth.Span.StartCol != 11 {
		t.Fatalf("birth evidence = %#v, want a proven fact at the declared name", birth)
	}
	declaration := evidenceLine(t, item, "x declared with optional type string?")
	if declaration.Kind != diag.EvidenceUserAssertion || declaration.Trust != diag.TrustClaimed || declaration.Span.StartLine != 7 || declaration.Span.StartCol != 14 {
		t.Fatalf("declaration evidence = %#v, want a claimed assertion at the annotation", declaration)
	}
	join := evidenceLine(t, item, "x survives the if/else join at main.lua:10 (no else assignment)")
	if join.Kind != diag.EvidenceAbstractFact || join.Trust != diag.TrustProven || join.Span.StartLine != 10 || join.Span.StartCol != 5 {
		t.Fatalf("join evidence = %#v, want a proven fact at the closing end", join)
	}
	use := evidenceLine(t, item, "x reaches use at main.lua:11 (method call on possibly-nil value)")
	if use.Kind != diag.EvidenceAbstractFact || use.Trust != diag.TrustProven || use.Span != item.Span {
		t.Fatalf("use evidence = %#v", use)
	}
}

func TestNilOriginUnsafeUseRetainsEveryCrossedJoin(t *testing.T) {
	items := nilOriginDiagnostics(t, `
local function run(p: boolean, q: boolean): string
    local x: string? = nil
    if p then
        x = "a"
    end
    if q then
        x = "b"
    end
    return x:upper()
end

return run`)
	if len(items) != 1 || len(items[0].Evidence) != 5 {
		t.Fatalf("nil-origin diagnostics = %#v, want one diagnostic with both joins", items)
	}
	evidenceLine(t, items[0], "x survives the if/else join at main.lua:6 (no else assignment)")
	evidenceLine(t, items[0], "x survives the if/else join at main.lua:9 (no else assignment)")
}

func TestNilOriginOptionalFieldUsePublishesDeclarationAndUse(t *testing.T) {
	items := nilOriginDiagnostics(t, `type Cfg = { name: string, hook: string? }

local function run(c: Cfg): number
    return c.hook:len()
end

return run`)
	if len(items) != 1 {
		t.Fatalf("nil-origin diagnostics = %#v, want exactly one", items)
	}
	item := items[0]
	if item.Message != "c.hook may be nil at method call" {
		t.Fatalf("message = %q", item.Message)
	}
	if item.Span.StartLine != 4 || item.Span.StartCol != 12 {
		t.Fatalf("span = %#v", item.Span)
	}
	if item.Help != "Guard c.hook against nil before the method call." {
		t.Fatalf("help = %q", item.Help)
	}
	if len(item.Labels) != 1 || item.Labels[0].Message != "possibly-nil field" {
		t.Fatalf("labels = %#v", item.Labels)
	}
	declaration := evidenceLine(t, item, "field hook declared optional at main.lua:1 (type string?)")
	if declaration.Kind != diag.EvidenceUserAssertion || declaration.Trust != diag.TrustClaimed || declaration.Span.StartLine != 1 || declaration.Span.StartCol != 28 {
		t.Fatalf("declaration evidence = %#v, want the authored field name", declaration)
	}
	use := evidenceLine(t, item, "c.hook reaches use at main.lua:4 (method call on possibly-nil field)")
	if use.Kind != diag.EvidenceAbstractFact || use.Trust != diag.TrustProven {
		t.Fatalf("use evidence = %#v", use)
	}
}

func TestNilOriginUnsafeUseStaysSilentWhenAProofExists(t *testing.T) {
	requireNoNilOriginDiagnostic(t, "guarded use", `
local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = "a"
    end
    if x then
        return x:upper()
    end
    return ""
end

return run`)
	requireNoNilOriginDiagnostic(t, "every branch assigns", `
local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = "a"
    else
        x = "b"
    end
    return x:upper()
end

return run`)
	requireNoNilOriginDiagnostic(t, "unconditional replacement", `
local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = "a"
    end
    x = "b"
    return x:upper()
end

return run`)
	requireNoNilOriginDiagnostic(t, "guard cannot be false", `
local function run(name: string): string
    local x: string? = nil
    if name then
        x = "a"
    end
    return x:upper()
end

return run`)
	requireNoNilOriginDiagnostic(t, "captured binding", `
local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = "a"
    end
    local set = function()
        x = "c"
    end
    set()
    return x:upper()
end

return run`)
	requireNoNilOriginDiagnostic(t, "guarded optional field", `type Cfg = { name: string, hook: string? }

local function run(c: Cfg): number
    if c.hook then
        return c.hook:len()
    end
    return 0
end

return run`)
	requireNoNilOriginDiagnostic(t, "non-optional field", `type Cfg = { name: string, hook: string }

local function run(c: Cfg): number
    return c.hook:len()
end

return run`)
}

// TestNilOriginUnsafeUseRequiresASourceName pins the fail-closed prose rule:
// the origin trace cites the file it proves, so a caller that supplies no
// source name receives no origin diagnostic instead of an unnamed location.
func TestNilOriginUnsafeUseRequiresASourceName(t *testing.T) {
	result, err := engine.Check(`
local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = "a"
    end
    return x:upper()
end

return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.PublishedDiagnostics {
		if item.Code == "type.nil.unsafe_use" {
			t.Fatalf("unnamed source published an origin trace: %#v", item)
		}
	}
}
