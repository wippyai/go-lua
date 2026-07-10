package checktest

import "testing"

func TestRuntimeCastRecordMemberReadUsesValidatedExpression(t *testing.T) {
	result := Check(`
type Payload = {
    name: string,
}

local function f(raw: any): string
    return (raw :: Payload).name
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want runtime record cast to validate member read", result.Diagnostics)
	}
}
