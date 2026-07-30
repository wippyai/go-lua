package checktest

import "testing"

func TestExportedTypeContractValueMethodIsPresentAcrossRequire(t *testing.T) {
	errorsMod := CheckAndExport(`
type AppError = {
    code: string,
    message: string,
}

local M = {}
M.AppError = AppError
return M
`, "errors")
	if len(errorsMod.Errors) != 0 {
		t.Fatalf("errors diagnostics = %#v", errorsMod.Errors)
	}

	result := Check(`
local errors = require("errors")

local raw = { code = "TEST", message = "hello" }
local validated, err = errors.AppError:is(raw)
if err == nil and validated then
    local code: string = validated.code
end
`, WithModule("errors", errorsMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want exported type contract value to expose :is", result.Diagnostics)
	}
}
