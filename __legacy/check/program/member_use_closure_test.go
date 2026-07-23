package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFrozenMemberUseClosureRequiresCompleteAttachedTopology(t *testing.T) {
	tests := []struct {
		name   string
		extra  string
		shadow bool
		want   bool
	}{
		{name: "closed", want: true},
		{name: "member-value-escape", extra: `local escaped = methods.check`},
		{name: "table-argument-escape", extra: `consume(methods)`},
		{name: "alias-escape", extra: `local alias = methods; alias.check(instance)`},
		{name: "dynamic-read", extra: `methods[key](instance)`},
		{name: "static-member-store", extra: `methods.check = replacement`},
		{name: "dynamic-member-store", extra: `methods[key] = replacement`},
		{name: "shadowed-attachment", shadow: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prefix := ""
			if tc.shadow {
				prefix = `local setmetatable = function(value) return value end`
			}
			stmts := parseChunk(t, prefix+`
				local methods = {}
				local mt = { __index = methods }
				function methods:check()
					return self
				end
				local instance = {}
				setmetatable(instance, mt)
				methods.check(instance)
				`+tc.extra)
			config := body.Config{
				Registry: standard.Registry(), TypeValues: typevalue.NewCache(),
				Globals:    []string{"consume", "key", "replacement"},
				Signatures: signaturelookup.Source{IncludeStdlib: true},
			}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(config)})
			keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), config.Registry, config.ModuleTypes, config.ModuleExports, stmts)
			config = configWithMetatableMethodSignatureArguments(config, keys.metatableProof)
			prepared, err := prepareBoundChunkBodies(stmts, bindings, config, keys)
			if err != nil {
				t.Fatalf("prepareBoundChunkBodies: %v", err)
			}
			method := methodFunctionSymbol(t, bindings, "check")
			closure, ok := prepared.memberUseClosures[method]
			got := ok && closure.complete && closure.callSites == 1
			if got != tc.want {
				t.Fatalf("complete frozen member-use closure = %v (%#v), want %v", got, closure, tc.want)
			}
		})
	}
}

func methodFunctionSymbol(t *testing.T, bindings *bind.Result, name string) symbol.ID {
	t.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind == bind.FunctionOriginMethod && origin.Method == name {
			return origin.Symbol
		}
	}
	t.Fatalf("method %q missing", name)
	return 0
}
