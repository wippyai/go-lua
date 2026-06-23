package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestRequireCheckAndExportedReturnedTableDottedMemberKeepsReturnType(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local n: number = provider.meta()
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}
}

func TestRequireCheckAndExportedReturnedTableDottedMemberNamesResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local n: number = provider.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "direct imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "provider.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedContainerMemberKeepsImportedResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local container = { client = provider }
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "container-injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "provider.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedConstructorReturnNamesMemberResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local function new_container(client)
			return { client = client }
		end
		local container = new_container(provider)
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "constructor-returned injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedContainerMemberReassignmentDropsStaleImportedResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local replacement = {}
		function replacement.meta(): number
			return 1
		end
		local container = { client = provider }
		container.client = replacement
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want no stale provider.meta evidence after member reassignment: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRequireCheckInjectedContainerMemberReassignmentUsesReplacementResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local replacement = {
			meta = function(): string
				return "replacement"
			end,
		}
		local container = { client = provider }
		container.client = replacement
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "reassigned injected member replacement result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta returns string")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckNestedFactoryDIDropsStaleBranchButKeepsSiblingEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	src := `
		local provider = require("provider")
		local replacement = {}
		function replacement.meta(): number
			return 1
		end
		local function new_layer(client)
			return {
				registry = {
					primary = client,
					backup = client,
				},
			}
		end
		local function expose(layer)
			return {
				api = layer.registry,
			}
		end
		local root = expose(new_layer(provider))
		root.api.primary = replacement
		local ok: number = root.api.primary.meta()
		local bad: number = root.api.backup.meta()
	`
	result := Check(src, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		debug := "<no checked result>"
		if result.checked != nil && result.checked.RootResult() != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %d, want one nested factory DI diagnostic: %#v\ncalls: %s", len(result.Diagnostics), result.Diagnostics, debug)
	}
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallResultAssignment,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            23,
		Column:          23,
		Span:            diagnostic.Span{StartLine: 23, StartCol: 23, EndLine: 23, EndCol: 42},
		MessageContains: []string{"call result", "not number"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"root.api.backup.meta returns"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"assignment target bad requires number"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"call result", "declared type"},
		HelpContains:  []string{"Assign the call result", "compatible target type", "callee return type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.call.direct.result_assignment]",
			"↓ declared type",
			"local bad: number = root.api.backup.meta()",
			"↑ call result",
			"1. proven: root.api.backup.meta returns",
			"2. claimed: assignment target bad requires number",
		},
		RenderNotContains: []string{
			"provider.meta returns",
			"root.api.primary.meta returns",
			"^~",
		},
	})
	requireDirectCallResultDiagnosticWithEvidence(t, result, "nested factory DI keeps sibling imported member evidence")
	requireEvidenceMessage(t, result.Diagnostics[0], "root.api.backup.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target bad requires number")
}

func TestRequireCheckInjectedHelperReturnKeepsImportedMemberResultType(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local function read_meta(client)
			return client.meta()
		end
		local n: number = read_meta(provider)
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one helper-return assignment diagnostic: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType, result.Diagnostics[0])
	}
}

func TestRequireCheckInjectedHelperReturnKeepsErrorReturnCorrelation(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local function load(injected)
			return injected.fetch("id")
		end
		local value, err = load(client)
		if err == nil then
			local n: number = value
		end
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want none for helper-preserved value/error correlation: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRequireCheckInjectedHelperNonFinalReturnDoesNotExpandImportedMultiReturn(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, boolean?)
			if id == "" then
				return nil, true
			end
			return 1, nil
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local function load(injected)
			return injected.fetch("id"), "marker"
		end
		local value, marker = load(client)
		local marker_string: string = marker
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want none for adjusted non-final imported multi-return: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}
