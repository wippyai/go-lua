package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// providerTerms names the external boundary each call in one body resolved to.
func providerTerms(artifact equation.Artifact) []string {
	providers := make([]string, 0)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "provider" {
				providers = append(providers, string(operand.Term.Encoding))
			}
		}
	}
	return providers
}

// TestExternalBoundaryClassifiesOnlyGlobalRootedCallees pins the boundary a
// lexical dispatch depends on: a call whose callee root is a local binding has
// its callable identity inside this compilation, so it stays an ordinary
// application no matter how the name is spelled.
func TestExternalBoundaryClassifiesOnlyGlobalRootedCallees(t *testing.T) {
	compilation, err := front.Compile(`
local function helper(value)
    return value
end

local function local_callee()
    return helper(1)
end

local function global_callee()
    return unresolved_provider(1)
end

return local_callee, global_callee
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(compilation.Nested) != 3 {
		t.Fatalf("nested bodies = %d, want helper plus two callers", len(compilation.Nested))
	}
	if providers := providerTerms(compilation.Nested[1].Artifact); len(providers) != 0 {
		t.Fatalf("local callee providers = %v, want no external boundary", providers)
	}
	providers := providerTerms(compilation.Nested[2].Artifact)
	if len(providers) != 1 || providers[0] != `provider/global/"unresolved_provider"` {
		t.Fatalf("global callee providers = %v, want the host boundary", providers)
	}
}

// TestExternalBoundaryClassifiesOnlyGlobalRootedReceivers pins the same rule for
// a method call: a locally bound receiver keeps its member dispatch, while a
// global-rooted one keeps the generic host boundary.
func TestExternalBoundaryClassifiesOnlyGlobalRootedReceivers(t *testing.T) {
	local, err := front.CompileBody(`
local receiver = {}
local value = receiver:invoke(1)
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	if providers := providerTerms(local); len(providers) != 0 {
		t.Fatalf("local receiver providers = %v, want no external boundary", providers)
	}
	global, err := front.CompileBody(`local value = host_service:invoke(1)`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	if providers := providerTerms(global); len(providers) != 1 || providers[0] != `provider/global/"host_service"` {
		t.Fatalf("global receiver providers = %v, want the host boundary", providers)
	}
}

// TestExactRequireModuleNeedsTheGlobalRequire pins the module-load boundary to
// the global binding it names. A local of the same name is a different value,
// so its call resolves no module.
func TestExactRequireModuleNeedsTheGlobalRequire(t *testing.T) {
	global, err := front.CompileBody(`local module = require("target")`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	if providers := providerTerms(global); len(providers) != 1 || providers[0] != `provider/module-load/"target"` {
		t.Fatalf("global require providers = %v, want the module-load boundary", providers)
	}
	shadowed, err := front.CompileBody(`
local require = other
local module = require("target")
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, provider := range providerTerms(shadowed) {
		if provider == `provider/module-load/"target"` {
			t.Fatalf("shadowed require resolved a module boundary: %v", providerTerms(shadowed))
		}
	}
}

// TestCallResultDisplayCarriesTheMethodSpelling pins the display authority a
// method-call diagnostic reads: the receiver path and selector this same
// publication carries, never a name recovered from the resolved provider.
func TestCallResultDisplayCarriesTheMethodSpelling(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "local receiver", source: "local receiver = {}\nlocal value = receiver:invoke(1)", want: "receiver:invoke(...)"},
		{name: "global receiver", source: `local value = host_service:invoke(1)`, want: "host_service:invoke(...)"},
		{name: "member receiver", source: "local root = {}\nlocal value = root.child:invoke(1)", want: "root.child:invoke(...)"},
		{name: "direct callee", source: `local value = helper(1)`, want: "helper(...)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := front.CompileBody(test.source)
			if err != nil {
				t.Fatalf("CompileBody: %v", err)
			}
			displays := make([]string, 0)
			for _, operation := range artifact.Equations {
				if operation.Occurrence.Kind != "call-results" {
					continue
				}
				for _, operand := range operation.Operands {
					if operand.Role == "result-display" {
						displays = append(displays, string(operand.Term.Encoding))
					}
				}
			}
			if len(displays) != 1 || displays[0] != test.want {
				t.Fatalf("result displays = %v, want %q", displays, test.want)
			}
		})
	}
}
