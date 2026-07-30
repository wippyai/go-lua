package checktest

import "testing"

func TestCheckFreshObjectLiteralMethodArgumentSatisfiesImportedRequestContract(t *testing.T) {
	status := CheckAndExport(`
type HttpMethod = "GET" | "POST"

type Request = {
    method: HttpMethod,
    path: string,
    body: any?,
    headers: {[string]: string},
}

type Response = {
    status: number,
    body: any?,
    headers: {[string]: string},
}

local M = {}
M.Request = Request
M.Response = Response
return M
`, "status")
	if len(status.Errors) != 0 {
		t.Fatalf("status diagnostics = %#v, want none", status.Errors)
	}

	handler := CheckAndExport(`
local status = require("status")

type Router = {
    handle: (self: Router, req: status.Request) -> status.Response,
}

local M = {}

function M.new(): Router
    local router: Router = {
        handle = function(self: Router, req: status.Request): status.Response
            return {status = 200, body = req.body, headers = {}}
        end,
    }
    return router
end

return M
`, "handler", WithModule("status", status))
	if len(handler.Errors) != 0 {
		t.Fatalf("handler diagnostics = %#v, want none", handler.Errors)
	}

	result := Check(`
local handler = require("handler")

local router = handler.new()
local response = router:handle({
    method = "GET",
    path = "/users",
    headers = {},
})
local status_code: number = response.status
	`, WithModule("status", status), WithModule("handler", handler))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for fresh request literal", result.Diagnostics)
	}
}
