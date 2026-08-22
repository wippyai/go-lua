package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestSealMountedProgramsAdmitsGenericStaticGraphs is the C6 binding
// regression anchor. Generic aliases, formals, and instantiated applications
// are ordinary Program static rows and must seal through the Link type
// authority into one Static authority.
func TestSealMountedProgramsAdmitsGenericStaticGraphs(t *testing.T) {
	input, err := lower.Lower(lower.Source{Name: "generic-static-binding.lua", Text: []byte(`
type Result<T, E> = {ok: true, value: T} | {ok: false, error: E}
local function map<T, U, E>(r: Result<T, E>, f: (T) -> U): Result<U, E>
    if r.ok then
        return {ok = true, value = f(r.value)}
    end
    return {ok = false, error = r.error}
end
local r: Result<number, string> = {ok = true, value = 2}
return map(r, function(x: number): number return x * 2 end)
`)})
	if err != nil || input == nil || !input.Available() {
		t.Fatalf("lower generic Static fixture: %v", err)
	}
	executionSchema, executionSchemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !executionSchemaOK {
		t.Fatal("generic Static execution schema")
	}
	artifact, failure := artifactcompiler.CompileDetailed(input, executionSchema, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile generic Static fixture: %s", failure.Error())
	}
	program := artifact.Program()
	linkID := identity.ContentID{3}
	types, err := typeauthority.SealProgramRows(linkID, []programschema.Program{program})
	if err != nil || types == nil {
		t.Fatalf("seal generic Link type authority: %v", err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil || target == nil {
		t.Fatalf("seal canonical target: %v", err)
	}
	moduleID := identity.ContentID{4}
	authority, _, err := SealMountedPrograms(MountContext{LinkID: linkID, Target: target}, types, []MountedProgram{{Program: program, ModuleID: moduleID, NamespaceID: moduleID}})
	if err != nil || authority == nil {
		t.Fatalf("seal generic mounted Static authority: %v", err)
	}
}
