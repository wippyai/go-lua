package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func confinementManifest(network typ.Type) *io.Manifest {
	fs := typ.NewRecord().
		OptField("read", typ.NewArray(typ.String)).
		OptField("write", typ.NewArray(typ.String)).
		OptField("exec", typ.NewArray(typ.String)).Build()
	env := typ.NewRecord().OptField("allow", typ.NewArray(typ.String)).Build()
	limits := typ.NewRecord().OptField("mem_mb", typ.Integer).
		OptField("pids", typ.Integer).OptField("wall_s", typ.Integer).Build()
	tree := typ.NewRecord().OptField("kill_on_owner_exit", typ.Boolean).Build()
	patch := typ.NewRecord().OptField("fs", fs).OptField("env", env).
		OptField("network", network).OptField("limits", limits).OptField("tree", tree).Build()
	mount := typ.NewRecord().Field("source", typ.String).Field("target", typ.String).
		OptField("read_only", typ.Boolean).Build()
	pty := typ.NewRecord().OptField("width", typ.Integer).OptField("height", typ.Integer).
		OptField("term", typ.String).Build()
	options := typ.NewRecord().OptField("work_dir", typ.String).
		OptField("env", typ.NewMap(typ.String, typ.String)).OptField("pty", pty).
		OptField("process_group", typ.Boolean).OptField("mounts", typ.NewArray(mount)).
		OptField("confine", patch).Build()
	manifest := io.NewManifest("exec")
	manifest.DefineType("ConfinementPatch", patch)
	manifest.DefineType("ProcessOptions", options)
	manifest.SetExport(typ.NewRecord().Build())
	return manifest
}

func TestAnnotatedConfinementCapture(t *testing.T) {
	const declaration = `
local exec = require("exec")
local confine: exec.ConfinementPatch = {
    fs = {read = {"/workspace", "{tmp}"}, write = {"{tmp}"}, exec = {}},
    env = {allow = {"PATH"}},
    network = "none",
    limits = {mem_mb = 128, pids = 16, wall_s = 30},
    tree = {kill_on_owner_exit = true},
}
`
	const closure = `
local function options(): exec.ProcessOptions
    return {confine = confine}
end
`
	type testCase struct {
		name   string
		source string
		bad    bool
	}
	cases := []testCase{
		{"direct", declaration + `local options: exec.ProcessOptions = {confine = confine}; return options`, false},
		{"direct_return", `local exec = require("exec")
local function options(): exec.ProcessOptions
` + declaration + `return {confine = confine}
end
return options`, false},
		{"captured", declaration + closure + `return options`, false},
		{"mutated_after_capture", declaration + closure + `confine.network = "none"; return options`, false},
		{"mutated_in_closure", declaration + `
local function options(): exec.ProcessOptions
    confine.network = "none"
    return {confine = confine}
end
return options`, false},
		{"nested_capture", declaration + `
local function outer()
    local function options(): exec.ProcessOptions
        return {confine = confine}
    end
    confine.network = "none"
    return options
end
return outer`, false},
		{"guarded_capture_mutation", declaration + `
if confine.network then
    local function options(): exec.ProcessOptions
        confine.network = nil
        return {confine = confine}
    end
    return options
end`, false},
		{"invalid_initializer", strings.Replace(declaration, `network = "none"`, `network = "bogus"`, 1) + closure + `return options`, true},
		{"invalid_narrowing_option", `local exec = require("exec")
local options: exec.ProcessOptions = {confine = {network = "host"}}
return options`, true},
		{"write_nil", declaration + closure + `confine.network = nil; return options`, false},
	}
	for _, union := range []bool{false, true} {
		network := typ.Type(typ.LiteralString("none"))
		modeCases := cases
		if union {
			network = typ.NewUnion(network, typ.LiteralString("isolated"))
			modeCases = append(append([]testCase(nil), cases...),
				testCase{"write_isolated", declaration + closure + `confine.network = "isolated"; return options`, false},
				testCase{"write_isolated_in_closure", declaration + `
local function options(): exec.ProcessOptions
    confine.network = "isolated"
    return {confine = confine}
end
return options`, false})
		}
		for _, strict := range []bool{false, true} {
			for _, tc := range modeCases {
				t.Run(fmt.Sprintf("union=%v/strict=%v/%s", union, strict, tc.name), func(t *testing.T) {
					result := testutil.Check(tc.source, testutil.WithStdlib(),
						testutil.WithManifest("exec", confinementManifest(network)),
						testutil.WithCheckOptions(check.Options{Strict: strict}))
					if result.HasError() != tc.bad {
						t.Fatalf("want errors=%v, got %v", tc.bad, testutil.ErrorMessages(result.Errors))
					}
					if tc.bad {
						messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
						invalid := "bogus"
						if tc.name == "invalid_narrowing_option" {
							invalid = "host"
						}
						if !strings.Contains(messages, invalid) {
							t.Fatalf("want diagnostic for %q, got %s", invalid, messages)
						}
					}
				})
			}
		}
	}
}
