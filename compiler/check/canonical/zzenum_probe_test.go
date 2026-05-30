package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZEnumProbe reproduces the typed-enum-constants handle loop guard so the
// compound-narrowing effect on the return type is visible. Debug probe.
func TestZZEnumProbe(t *testing.T) {
	src := `
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH"
type StatusCode = 200 | 201 | 400 | 404 | 500
type Request = {
    method: HttpMethod,
    path: string,
}
type Response = { status: StatusCode }
type Route = {
    method: HttpMethod,
    path: string,
    handler: (req: Request) -> Response,
}
type Router = {
    _routes: {Route},
    handle: (self: Router, req: Request) -> Response,
}
local function mkok(): Response return { status = 200 } end
local function mkerr(): Response return { status = 404 } end
local function handle(self: Router, req: Request): Response
    for _, route in ipairs(self._routes) do
        if route.method == req.method and route.path == req.path then
            return route.handler(req)
        end
    end
    return mkerr()
end
local rt: Router = { _routes = {}, handle = handle }
local resp = rt:handle({ method = "GET", path = "/x" })
local s: number = resp.status
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("DIAG: %s", m)
	}
}
