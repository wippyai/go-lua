package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestBoundedModuloTupleIndex(t *testing.T) {
	for _, source := range []string{`
local frames = {"x", "y"}
for i = 1, 5 do
    local frame: string = frames[((i - 1) % #frames) + 1]
end
`, `
local frames = {"x", "y", "z"}
for i = 1, 2 do
    local next: string = frames[i + 1]
    local reverse: string = frames[4 - i]
    local odd: string = frames[(2 * i) - 1]
end
local last: string = frames[#frames]
local first: string = frames[1]
`} {
		result := testutil.Check(source, testutil.WithStdlib())
		if result.HasError() {
			t.Fatalf("bounded tuple index must be present: %v", testutil.ErrorMessages(result.Errors))
		}
	}
}

func TestIpairsModuloTupleIndex(t *testing.T) {
	result := testutil.Check(`
local frames = {"x", "y", "z"}
local tests: {any} = {1, 2, 3}
for i, entry in ipairs(tests) do
    local frame: string = frames[((i - 1) % #frames) + 1]
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("ipairs modulo index must stay inside the tuple: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestTupleIndexOutsideBoundsStillOptional(t *testing.T) {
	for _, source := range []string{
		`local frames = {"x", "y"}; local frame: string = frames[3]`,
		`local frames = {"x", "y"}; for i = 1, 5 do local frame: string = frames[((i - 1) % #frames) + 3] end`,
		`local frames = {"x", "y", "z"}; for i = 1, 2 do local frame: string = frames[i + 2] end`,
		`local frames = {"x", "y"}; local i: number = 1.5; local frame: string = frames[(i % #frames) + 1]`,
	} {
		result := testutil.Check(source, testutil.WithStdlib())
		if !result.HasError() {
			t.Fatalf("out-of-range tuple index was accepted: %s", source)
		}
	}
}
