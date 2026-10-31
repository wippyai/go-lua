package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestMapReadBooleanDiscriminantGuard(t *testing.T) {
	const source = `
type Direction = "outbound" | "inbound"
type LinkState = {connected: false} | {connected: true, direction: Direction, remote_address: string}
type LinkStates = {[string]: LinkState}
type LinkFields = {connected: false, direction: nil, remote_address: nil}
    | {connected: true, direction: Direction, remote_address: string}
local function link_fields(node_id: string, links: LinkStates): LinkFields
    local state = links[node_id]
    if not state or not state.connected then
        return {connected = false, direction = nil, remote_address = nil}
    end
    return {connected = true, direction = state.direction, remote_address = state.remote_address}
end
return link_fields
`
	cases := []struct {
		name   string
		source string
		bad    bool
	}{
		{"guarded", source, false},
		{"without_discriminant", strings.Replace(source, "not state or not state.connected", "not state", 1), true},
		{"false_discriminant", strings.Replace(source, "not state or not state.connected", "not state or state.connected", 1), true},
		{"explicit_true", strings.Replace(source, "not state or not state.connected", "not state or state.connected ~= true", 1), false},
		{"separate_guards", strings.Replace(source, "if not state or not state.connected then", `if not state then return {connected = false, direction = nil, remote_address = nil} end
    if not state.connected then`, 1), false},
	}
	for _, strict := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("strict=%v/%s", strict, tc.name), func(t *testing.T) {
				result := testutil.Check(tc.source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
				messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
				if result.HasError() != tc.bad {
					t.Fatalf("want errors=%v, got %q", tc.bad, messages)
				}
				if tc.bad && !strings.Contains(messages, "expected LinkFields") {
					t.Fatalf("want return type diagnostic, got %q", messages)
				}
			})
		}
	}
}
