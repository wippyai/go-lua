-- Bee runtime-pin checker regression: bee.apps:catalog_test:383
-- Expected vs actual: Both callbacks return Selection; actual first return becomes unresolved | Selection.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Selection = {revision: string, items: {string}}
local test = {}
function test.it(name: string, run: () -> ()) run() end
local catalog = {}
function catalog.read(): Selection return {revision = "1", items = {}} end
function catalog.resolve_open(refresh: () -> Selection?): Selection? return refresh() end
local function run(): ()
    test.it("refresh", function()
        local visible = catalog.read() :: Selection
        local stale: Selection = {revision = visible.revision, items = {}}
        local refreshes = 0
        catalog.resolve_open(function()
            refreshes = refreshes + 1
            return refreshes == 1 and stale or visible
        end)
        refreshes = 0
        catalog.resolve_open(function()
            refreshes = refreshes + 1
            return visible
        end)
    end)
end
return run
