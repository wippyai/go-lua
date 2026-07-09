package checktest

import "testing"

func TestCallArgumentNonNilAssertionNarrowsOptionalMember(t *testing.T) {
	result := Check(`
local function decode(src: string): ()
end

local function consume(msg: {data: string?}): ()
    decode(msg.data!)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want non-nil assertion to satisfy call argument", result.Diagnostics)
	}
}
