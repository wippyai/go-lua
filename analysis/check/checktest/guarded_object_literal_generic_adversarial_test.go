package checktest

import (
	"fmt"
	"testing"
)

func TestGuardedAnyFieldsFlowIntoGenericObjectLiteralArgument(t *testing.T) {
	resultMod := CheckFile(`
type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local M = {}

function M.ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

function M.err<T>(error: string): Result<T>
    return {ok = false, error = error}
end

return M
`, "result.lua")
	resultManifest := moduleResultFromCheck("result", resultMod)
	if len(resultManifest.Errors) != 0 {
		t.Fatalf("result diagnostics = %#v", resultManifest.Errors)
	}

	result := Check(`
local result = require("result")

type User = {id: string, retries: number}
type UserResult = {ok: true, value: User} | {ok: false, error: string}

local function decode(raw: any): UserResult
    if type(raw) ~= "table" then
        return result.err("root")
    end
    if type(raw.id) ~= "string" then
        return result.err("id")
    end
    if type(raw.retries) ~= "number" then
        return result.err("retries")
    end
    return result.ok({id = raw.id, retries = raw.retries})
end
`, WithModule("result", resultManifest))

	if len(result.Diagnostics) != 0 {
		debug := "<no checked result>"
		if result.checked != nil {
			root := result.checked.RootResult()
			debug = callOutcomeDebug(root)
			for _, fn := range root.FunctionResults() {
				debug += "\nchild: " + callOutcomeDebug(fn)
				if fn.Graph() != nil {
					for _, point := range fn.Graph().RPO() {
						site, ok := fn.CallSite(point)
						if !ok {
							continue
						}
						source, ok := site.ArgumentSourceAt(0)
						if !ok {
							continue
						}
						if t, ok := fn.SignatureArgumentTypeAtBoundary(point, source); ok {
							debug += fmt.Sprintf("\narg0@%d: %s", point, t)
						}
					}
				}
			}
		}
		t.Fatalf("diagnostics = %#v, want guarded any fields to type the object literal argument\ncalls: %s", result.Diagnostics, debug)
	}
}

func TestGuardedAnyFieldReadAfterEarlyReturn(t *testing.T) {
	result := Check(`
local function decode(raw: any): ()
    if type(raw) ~= "table" then
        return
    end
    if type(raw.id) ~= "string" then
        return
    end
    local id: string = raw.id
end
`)

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want early-return type guard to refine raw.id", result.Diagnostics)
	}
}
