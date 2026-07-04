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
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
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
	requireAssignmentDiagnosticWithEvidence(t, result, "direct imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "provider.meta(...) has type {name: string}")
	requireEvidenceMessage(t, result.Diagnostics[0], "n is declared as number")
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
	requireAssignmentDiagnosticWithEvidence(t, result, "container-injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta(...) has type {name: string}")
	requireEvidenceMessage(t, result.Diagnostics[0], "n is declared as number")
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
	requireAssignmentDiagnosticWithEvidence(t, result, "constructor-returned injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta(...) has type {name: string}")
	requireEvidenceMessage(t, result.Diagnostics[0], "n is declared as number")
}

func TestRequireCheckImportedBuilderFactoryReturnKeepsReceiverMethods(t *testing.T) {
	storeMod := CheckAndExport(`
		type Store = {
			state: {flags: {[string]: boolean}},
			lookup_projection: (self: Store, id: string) -> string?,
		}
		local Store = {}
		Store.__index = Store
		local M = {}
		M.Store = Store
		function M.new(): Store
			local self: Store = {
				state = {flags = {}},
				lookup_projection = Store.lookup_projection,
			}
			setmetatable(self, Store)
			return self
		end
		function Store:lookup_projection(id: string): string?
			return nil
		end
		return M
	`, "store")
	if len(storeMod.Errors) != 0 {
		t.Fatalf("store module errors = %#v, want none", storeMod.Errors)
	}

	busMod := CheckAndExport(`
		local store = require("store")
		type Bus = {
			register: (self: Bus, topic: string) -> Bus,
			new_store: (self: Bus) -> store.Store,
			replay: (self: Bus, target: store.Store) -> (),
		}
		local Bus = {}
		Bus.__index = Bus
		local M = {}
		M.Bus = Bus
		function M.new(): Bus
			local self: Bus = {
				register = Bus.register,
				new_store = Bus.new_store,
				replay = Bus.replay,
			}
			setmetatable(self, Bus)
			return self
		end
		function Bus:register(topic: string): Bus
			return self
		end
		function Bus:new_store(): store.Store
			return store.new()
		end
		function Bus:replay(target: store.Store)
			target.state.flags["replayed"] = true
		end
		return M
	`, "bus", WithStdlib(), WithModule("store", storeMod))
	if len(busMod.Errors) != 0 {
		t.Fatalf("bus module errors = %#v, want none", busMod.Errors)
	}

	result := Check(`
		local bus = require("bus")
		local app = bus.new():register("tasks")
		local store = app:new_store()
		app:replay(store)
		local projection = store:lookup_projection("job-1")
	`, WithStdlib(), WithModule("store", storeMod), WithModule("bus", busMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported builder factory receiver methods retained", result.Diagnostics)
	}
}

func TestRequireCheckImportedFluentBuilderKeepsPresentReceiverAcrossChain(t *testing.T) {
	builderMod := CheckAndExport(`
		type Builder = {
			with_name: (self: Builder, name: string) -> Builder,
			with_context: (self: Builder, context: {any}) -> Builder,
			call: (self: Builder, input: string) -> (string?, string?),
		}
		local M = {}
		M.Builder = Builder
		function M.new(): Builder
			local builder: Builder
			builder = {
				with_name = function(self: Builder, name: string): Builder
					return self
				end,
				with_context = function(self: Builder, context: {any}): Builder
					return self
				end,
				call = function(self: Builder, input: string): (string?, string?)
					return input, nil
				end,
			}
			return builder
		end
		return M
	`, "builder")
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder module errors = %#v, want none", builderMod.Errors)
	}

	result := Check(`
		local builder = require("builder")
		local out, err = builder.new()
			:with_name("jobs")
			:with_context({})
			:call("run")
	`, WithStdlib(), WithModule("builder", builderMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported fluent builder receivers to stay present across chain", result.Diagnostics)
	}
}

func TestRequireCheckImportedFluentBuilderWithImportedParameterTypesKeepsReceiverPresent(t *testing.T) {
	securityMod := CheckAndExport(`
		type ActorMeta = {
			security_groups: {string},
		}
		type Actor = {
			id: (self: Actor) -> string,
			meta: (self: Actor) -> ActorMeta,
		}
		type Scope = {
			name: string,
		}
		local M = {}
		M.ActorMeta = ActorMeta
		M.Actor = Actor
		M.Scope = Scope
		function M.new_actor(id: string, meta: ActorMeta?): Actor
			local actor_meta = meta or {security_groups = {}}
			local actor: Actor = {
				id = function(self: Actor): string
					return id
				end,
				meta = function(self: Actor): ActorMeta
					return actor_meta
				end,
			}
			return actor
		end
		function M.named_scope(name: string): Scope
			return { name = name }
		end
		return M
	`, "security")
	if len(securityMod.Errors) != 0 {
		t.Fatalf("security module errors = %#v, want none", securityMod.Errors)
	}

	builderMod := CheckAndExport(`
		local security = require("security")
		type Builder = {
			actor: security.Actor?,
			scope: security.Scope?,
			context: {any}?,
			with_actor: (self: Builder, actor: security.Actor) -> Builder,
			with_scope: (self: Builder, scope: security.Scope) -> Builder,
			with_context: (self: Builder, context: {any}) -> Builder,
			call: (self: Builder, input: string) -> (string?, string?),
		}
		local M = {}
		M.Builder = Builder
		function M.new(): Builder
			local builder: Builder
			builder = {
				actor = nil,
				scope = nil,
				context = nil,
				with_actor = function(self: Builder, actor: security.Actor): Builder
					self.actor = actor
					return self
				end,
				with_scope = function(self: Builder, scope: security.Scope): Builder
					self.scope = scope
					return self
				end,
				with_context = function(self: Builder, context: {any}): Builder
					self.context = context
					return self
				end,
				call = function(self: Builder, input: string): (string?, string?)
					return input, nil
				end,
			}
			return builder
		end
		return M
	`, "builder", WithStdlib(), WithModule("security", securityMod))
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder module errors = %#v, want none", builderMod.Errors)
	}

	result := Check(`
		local builder = require("builder")
		local security = require("security")
		local out, err = builder.new()
			:with_actor(security.new_actor("u-1", {security_groups = {"jobs"}}))
			:with_scope(security.named_scope("jobs"))
			:with_context({})
			:call("run")
	`, WithStdlib(), WithModule("builder", builderMod), WithModule("security", securityMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported fluent builder with imported parameter types to keep receiver present", result.Diagnostics)
	}

	result = Check(`
		local builder = require("builder")
		local security = require("security")
		local function dispatch(input: string): (string?, string?)
			return builder.new()
				:with_actor(security.new_actor("u-1", {security_groups = {"jobs"}}))
				:with_scope(security.named_scope("jobs"))
				:with_context({})
				:call(input)
		end
		local out, err = dispatch("run")
	`, WithStdlib(), WithModule("builder", builderMod), WithModule("security", securityMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported fluent builder return chain to keep receiver present", result.Diagnostics)
	}
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
	if len(result.Diagnostics) != 1 {
		debug := "<no checked result>"
		if result.checked != nil && result.checked.RootResult() != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %d, want one incompatible replacement assignment diagnostic: %#v\ncalls: %s", len(result.Diagnostics), result.Diagnostics, debug)
	}
	requireAssignmentDiagnosticWithEvidence(t, result, "incompatible replacement should be rejected at assignment")
	requireEvidenceMessage(t, result.Diagnostics[0], "replacement has type {meta: fun() -> number}")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client is declared as {meta: fun() -> {name: string}}")
}

func TestRequireCheckLocalDottedMethodDeclarationKeepsResultEvidence(t *testing.T) {
	result := Check(`
		local replacement = {}
		function replacement.meta(): number
			return 1
		end
		local n: number = replacement.meta()
	`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local dotted method declaration to prove number result", result.Diagnostics)
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
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %d, want replacement assignment plus call assignment diagnostics: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "replacement has type {meta: fun() -> string}")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client is declared as {meta: fun() -> {name: string}}")
	requireEvidenceMessage(t, result.Diagnostics[1], "container.client.meta(...) has type string")
	requireEvidenceMessage(t, result.Diagnostics[1], "n is declared as number")
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
	if len(result.Diagnostics) != 2 {
		debug := "<no checked result>"
		if result.checked != nil && result.checked.RootResult() != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %d, want replacement assignment plus nested factory DI diagnostic: %#v\ncalls: %s", len(result.Diagnostics), result.Diagnostics, debug)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "replacement has type {meta: fun() -> number}")
	requireEvidenceMessage(t, result.Diagnostics[0], "root.api.primary is declared as {meta: fun() -> {name: string}}")
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 2,
		Line:            23,
		Column:          23,
		Span:            diagnostic.Span{StartLine: 23, StartCol: 23, EndLine: 23, EndCol: 42},
		MessageContains: []string{"root.api.backup.meta(...)", "not number"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"root.api.backup.meta(...) has type {name: string}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"bad is declared as number"},
			},
		},
		LabelMin:     2,
		HelpContains: []string{"Use a value compatible with the expected type"},
		Sources:      diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]",
			"↓ declared type",
			"local bad: number = root.api.backup.meta()",
			"↑ assigned value",
			"1. proven: root.api.backup.meta(...) has type {name: string}",
			"2. claimed: bad is declared as number",
		},
		RenderNotContains: []string{
			"provider.meta returns",
			"root.api.primary.meta returns",
			"^~",
		},
	})
	requireAssignmentDiagnosticWithEvidence(t, Result{Diagnostics: []diagnostic.Diagnostic{result.Diagnostics[1]}}, "nested factory DI keeps sibling imported member evidence")
	requireEvidenceMessage(t, result.Diagnostics[1], "root.api.backup.meta(...) has type {name: string}")
	requireEvidenceMessage(t, result.Diagnostics[1], "bad is declared as number")
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
