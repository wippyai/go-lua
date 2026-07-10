package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDeclaredLifecycleProtocolsEndToEnd(t *testing.T) {
	resources := declaredLifecycleResourceManifest(t)
	opts := []Option{
		WithManifest("resource", resources),
		WithGlobals("connect", "close", "begin", "commit", "rollback", "query", "flag"),
	}

	t.Run("use-after-close", func(t *testing.T) {
		result := Check(`
local conn = connect()
close(conn)
query(conn)
`, opts...)
		requireDeclaredLifecycleDiagnostic(t, result, diagnostics.CodeTypestateInvalidTransition, diagnostic.SeverityError, "connection", "expected `open`", "found `closed`")
	})

	t.Run("alias-close-propagates", func(t *testing.T) {
		result := Check(`
local conn = connect()
local alias = conn
close(alias)
query(conn)
`, opts...)
		requireDeclaredLifecycleDiagnostic(t, result, diagnostics.CodeTypestateInvalidTransition, diagnostic.SeverityError, "connection", "expected `open`", "found `closed`")
	})

	t.Run("double-commit", func(t *testing.T) {
		result := Check(`
local conn = connect()
local tx = begin(conn)
commit(tx)
commit(tx)
close(conn)
`, opts...)
		requireDeclaredLifecycleDiagnostic(t, result, diagnostics.CodeTypestateInvalidTransition, diagnostic.SeverityError, "transaction", "expected `active`", "found `committed`")
	})

	t.Run("leak-on-some-path", func(t *testing.T) {
		result := Check(`
local conn = connect()
if flag then
    close(conn)
end
`, opts...)
		requireDeclaredLifecycleDiagnostic(t, result, diagnostics.CodeResourceUnreleased, diagnostic.SeverityWarning, "connection", "expected", "closed")
	})

	t.Run("correct-usage-clean", func(t *testing.T) {
		result := Check(`
local conn = connect()
local tx = begin(conn)
commit(tx)
close(conn)
`, opts...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
		}
	})

	t.Run("opaque-callee-escape-silences", func(t *testing.T) {
		result := Check(`
local function handoff(writer: (any) -> ())
    local conn = connect()
    writer(conn)
end
`, opts...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want opaque handoff to silence local obligation", result.Diagnostics)
		}
	})
}

// Error-path transport through pcall is intentionally not modeled by the
// interprocedural lifecycle summary. A final transition that happens only before
// a raised error must therefore remain unproven at the caller, never be treated
// as satisfying the obligation. This pins the known L-gap as a warning rather
// than an unsound clean result.
func TestDeclaredLifecyclePCallErrorPathFinalRemainsUnproven(t *testing.T) {
	resources := declaredLifecycleResourceManifest(t)
	result := Check(`
local conn = connect()
pcall(function()
    close(conn)
    error("boom")
end)
`, WithStdlib(), WithManifest("resource", resources), WithGlobals("connect", "close"))
	requireDeclaredLifecycleDiagnostic(t, result, diagnostics.CodeResourceUnreleased, diagnostic.SeverityWarning, "connection", "expected", "closed")
}

func requireDeclaredLifecycleDiagnostic(t *testing.T, result Result, code diagnostic.Code, severity diagnostic.Severity, wants ...string) {
	t.Helper()
	for _, got := range result.Diagnostics {
		if got.Code != code || got.Severity != severity {
			continue
		}
		for _, want := range wants {
			if !diagnosticContains(got, want) {
				t.Fatalf("diagnostic %s does not contain %q: %#v", got.Code, want, got)
			}
		}
		return
	}
	t.Fatalf("diagnostics = %#v, want %s/%s", result.Diagnostics, code, severity)
}

func diagnosticContains(d diagnostic.Diagnostic, want string) bool {
	return strings.Contains(d.Message, want)
}

func declaredLifecycleResourceManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	m := manifest.New("resource")
	for _, def := range []typestate.Definition{
		{
			Protocol:    "connection",
			States:      []typestate.State{"open", "closed"},
			FinalStates: []typestate.State{"closed"},
			Transitions: []typestate.TransitionDecl{{From: "open", To: "open"}, {From: "open", To: "closed"}},
		},
		{
			Protocol:    "transaction",
			States:      []typestate.State{"active", "committed", "rolledback"},
			FinalStates: []typestate.State{"committed", "rolledback"},
			Transitions: []typestate.TransitionDecl{{From: "active", To: "committed"}, {From: "active", To: "rolledback"}},
		},
	} {
		if err := m.DefineTypestateProtocol(def); err != nil {
			t.Fatalf("DefineTypestateProtocol(%s): %v", def.Protocol, err)
		}
	}
	m.DefineFunctionSignature("connect", signature.Function{
		Type: typ.Func().Returns(typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
			Target:     pathdom.Path{Root: "ret[0]"},
			Kind:       signature.LifecycleAcquire,
			Protocol:   "connection",
			To:         "open",
			Obligation: typestate.Obligation{Final: "closed"},
		}}},
	})
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().Param("conn", typ.Any).Returns(typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
			Target:     pathdom.Path{Root: "ret[0]"},
			Kind:       signature.LifecycleAcquire,
			Protocol:   "transaction",
			To:         "active",
			Obligation: typestate.Obligation{Finals: typestate.NewFinalStates("committed", "rolledback")},
		}}},
	})
	for _, operation := range []struct {
		name     string
		protocol typestate.Protocol
		from     typestate.State
		to       typestate.State
	}{
		{"close", "connection", "open", "closed"},
		{"query", "connection", "open", "open"},
		{"commit", "transaction", "active", "committed"},
		{"rollback", "transaction", "active", "rolledback"},
	} {
		m.DefineFunctionSignature(operation.name, signature.Function{
			Type: typ.Func().Param("resource", typ.Any).Build(),
			OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0),
				Kind:     signature.LifecycleTransition,
				Protocol: operation.protocol,
				From:     operation.from,
				To:       operation.to,
			}}},
		})
	}
	data, err := manifest.Encode(m)
	if err != nil {
		t.Fatalf("manifest.Encode: %v", err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("manifest.Decode: %v", err)
	}
	return decoded
}
