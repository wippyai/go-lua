package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestConditionalTableFieldsDeterministic(t *testing.T) {
	const source = `
        local function send(value: {filters: {status: string?, class: string?, enabled: boolean?}, ordering: {field: "created_at" | "updated_at"}}) end
        local function run(status: string?, class: string?, enabled: boolean?, order_by: string)
            local request = {filters = {}, ordering = {field = order_by}}
            if status and status ~= "" then request.filters.status = status end
            if enabled ~= nil then request.filters.enabled = enabled end
            if class and class ~= "" then request.filters.class = class end
            send(request)
        end
    `
	var first string
	for i := 0; i < 100; i++ {
		result := testutil.Check(source)
		messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n")
		for _, want := range []string{"class?: string", "enabled?: boolean", "status?: string"} {
			if !strings.Contains(messages, want) {
				t.Fatalf("check %d lost optional field %q: %s", i, want, messages)
			}
		}
		if i == 0 {
			first = messages
		} else if messages != first {
			t.Fatalf("check %d differs:\nfirst: %s\nnow: %s", i, first, messages)
		}
	}
}
