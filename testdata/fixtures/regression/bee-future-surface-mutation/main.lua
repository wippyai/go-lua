-- Bee runtime-pin checker regression: bee.gateway:catalog_test:101 (additional fixture gate error)
-- Expected: A guarded Surface keeps its complete type before a later valid array-element mutation.
-- Actual: argument 1: expected Surface, got {allowed_traits?: string[], ...}.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Surface = {fixed: string, allowed_traits: {string}}
local function grant(surface: Surface): Surface?
 return {fixed = surface.fixed, allowed_traits = {"one", "two"}}
end
local function select(surface: Surface): string return surface.fixed end
local function test(surface: Surface)
 local granted = grant(surface)
 if not granted then return end
 select(granted)
 granted.allowed_traits[1] = "changed"
end
return test
