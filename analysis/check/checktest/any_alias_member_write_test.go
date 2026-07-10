package checktest

import "testing"

func TestAnyAliasMemberWritePreservesOriginalTableKind(t *testing.T) {
	result := Check(`
local function build_error(kind: string, msg: string, details: table): table
	return details
end

local x = "value"
local merged_details = {}
local md = merged_details :: any
md.some_field = x

build_error("kind", "message", merged_details)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: original table kind must survive writes through an any alias", result.Diagnostics)
	}
}
