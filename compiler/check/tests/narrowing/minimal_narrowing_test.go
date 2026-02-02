package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestMinimalNarrowing_Equality(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", value: number}
		local r: A | B = {tag="a", value="x"}

		if r.tag == "a" then
			local s: string = r.value
		else
			local n: number = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic: %s", d.Message)
	}
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestMinimalNarrowing_ElseBranchWrongType(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", value: number}
		local r: A | B = {tag="a", value="x"}

		if r.tag == "a" then
		else
			local s: string = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic: %s", d.Message)
	}
	if !result.HasError() {
		t.Errorf("expected error (assigning number to string), got none")
	}
}

func TestMinimalNarrowing_BooleanDiscriminant(t *testing.T) {
	source := `
		type OK = {ok: true, value: string}
		type ERR = {ok: false, value: number}
		local r: OK | ERR = {ok=true, value="x"}

		if r.ok then
			local s: string = r.value
		else
			local n: number = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic: %s", d.Message)
	}
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
