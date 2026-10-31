-- From kickside.sso.connectors:providers resolve() and app:providers_registry_test.
local function resolve(engine: string, endpoints: table?)
    local base = { engine = engine }
    if engine == "github" then
        return base
    end
    if endpoints then
        base.endpoints = endpoints
        return base
    end
    base.endpoints = { authorize = "https://okta.test/oauth2/v1/authorize" }
    return base
end

local desc = resolve("oidc", { authorize = "https://okta.test/oauth2/v1/authorize" })
if desc == nil then error("missing descriptor") end
if desc.engine ~= "oidc" then error("wrong engine") end
local authorize = desc.endpoints.authorize
return authorize
