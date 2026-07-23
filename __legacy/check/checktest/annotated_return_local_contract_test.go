package checktest

import "testing"

func TestAnnotatedReturnLocalBranchAssignmentsKeepDeclaredContract(t *testing.T) {
	result := Check(`
type RenderOutput = {
    kind: "rendered",
    body: string,
    label: string?,
}

type AuditOutput = {
    kind: "audited",
    note: string,
    retry_after: number?,
}

type Output = RenderOutput | AuditOutput

type Receipt<T> = {
    plugin: string,
    output: T,
}

type OutputReceipt = Receipt<Output>
type DispatchResult = {ok: true, value: OutputReceipt?} | {ok: false, error: string}

local function dispatch(kind: string): DispatchResult
    local receipt: OutputReceipt
    if kind == "render" then
        receipt = {
            plugin = "render",
            output = {
                kind = "rendered",
                body = "body",
                label = nil,
            },
        }
    else
        receipt = {
            plugin = "audit",
            output = {
                kind = "audited",
                note = "note",
                retry_after = nil,
            },
        }
    end
    return {ok = true, value = receipt}
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want branch-assigned annotated local to satisfy declared return contract", result.Diagnostics)
	}
}
