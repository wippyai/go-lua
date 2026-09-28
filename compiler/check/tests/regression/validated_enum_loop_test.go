package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestValidatedEnumThroughLoop(t *testing.T) {
	const source = `
        local function send(field: "created_at" | "updated_at" | "next_run_at") end
        local function run(order_by: string)
            local valid_order_fields = {"created_at", "updated_at", "next_run_at"}
            local valid_order_field = false
            for _, field in ipairs(valid_order_fields) do
                if order_by == field then
                    valid_order_field = true
                    break
                end
            end
            if not valid_order_field then return end
            send(order_by)
        end
    `
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("validated field rejected: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
