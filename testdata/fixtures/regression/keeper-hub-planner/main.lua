local registry = require("registry")
local hub_sdk = require("hub")
local gov_consts = require("gov_consts")

type ServiceError = unknown
type Parameter = { name: string, value: string }
type RequirementTarget = { entry?: string, path?: string }
type HubRequirement = {
    name?: string,
    description?: string,
    default?: string,
    required?: boolean,
    targets?: { RequirementTarget },
}
type HubDependencyRef = {
    org?: string,
    name?: string,
    version?: string,
    version_constraint?: string,
}
type VersionPart = {
    major: number,
    minor: number,
    patch: number,
    prerelease: string,
    raw: string,
}
type VersionItem = {
    id?: string,
    version: string,
    yanked?: boolean,
    dependencies?: { HubDependencyRef },
    requirements?: { HubRequirement },
    entry_count?: number,
    entry_kinds?: { string },
    lua_modules?: { string },
    size_bytes?: number,
    digest?: string,
    protected?: boolean,
}
type DependencyEntryData = {
    component: string,
    version: string,
    parameters?: { Parameter },
}
type DependencyEntry = {
    id: string,
    kind: string,
    meta: {[string]: unknown},
    data: DependencyEntryData,
}
type GraphNode = {
    module: string,
    org: string,
    name: string,
    namespace: string,
    version: string,
    version_id?: string,
    constraint: string,
    depth: number,
    parent: string?,
    path: string,
    direct: boolean,
    dependencies: { HubDependencyRef },
    requirements: { HubRequirement },
}
type ConstraintPart = {
    op: string,
    any?: boolean,
    major?: number,
    minor?: number,
    version?: VersionPart,
}
type PlannerDeps = {
    registry: unknown?,
    catalog: unknown?,
    gov: unknown?,
}
type PlannerInstance = {
    registry: unknown,
    catalog: unknown,
    gov: unknown,
    version_cache: {[string]: {VersionItem}},
    plan_install: (PlannerInstance, unknown) -> (unknown, unknown?),
}

local M = {}

local Planner = {}
Planner.__index = Planner

M.DEFAULT_DEP_NAMESPACE = "app.deps"
M.DEFAULT_VERSION = ">=v0.0.0"
M.DEFAULT_PLAN_MAX_DEPTH = 10
M.DEFAULT_PLAN_MAX_MODULES = 200

local function trim(value: unknown): string
    return string.match(tostring(value or ""), "^%s*(.-)%s*$") or ""
end

local function shallow_copy(src)
    local out = {}
    for k, v in pairs(src or {}) do out[k] = v end
    return out
end

local ERROR_KIND_BY_CODE = {
    BAD_REQUEST = errors.INVALID,
    NOT_FOUND = errors.NOT_FOUND,
    CONFLICT = errors.CONFLICT,
    PARAMETER_TARGET_TRANSITIVE = errors.INVALID,
}

-- Error details survive a string-keyed conversion only: array parts are
-- dropped when a table crosses into errors.new details. Rows are keyed by
-- their 1-based position rendered as a string so both the rows and their
-- order survive to API responses.
function M.position_keyed(rows)
    local out = {}
    for i, row in ipairs(rows or {}) do
        out[tostring(i)] = row
    end
    return out
end

local function err(code: string, message: string, details: unknown?): unknown
    local d = {}
    if type(details) == "table" then
        for k, v in pairs(details) do d[k] = v end
    elseif details ~= nil then
        d.value = details
    end
    d.code = code
    return errors.new({
        kind = ERROR_KIND_BY_CODE[code] or errors.INTERNAL,
        message = tostring(message or "unknown error"),
        details = d,
    })
end

local function err_message(e, fallback)
    if not e then return fallback end
    local ok, method = pcall(function() return e.message end)
    if ok and type(method) == "function" then
        local called, message = pcall(method, e)
        if called then return message end
    end
    if type(e) == "table" then return e.message or e.error or fallback end
    return tostring(e)
end

local function err_details(e)
    if not e then return nil end
    local ok, method = pcall(function() return e.details end)
    if ok and type(method) == "function" then
        local called, details = pcall(method, e)
        if called then return details end
    end
    if type(e) == "table" then return e.details end
    return nil
end

local function starts_with(s: string, prefix: string): boolean
    return string.sub(tostring(s or ""), 1, #prefix) == prefix
end

local function is_array(t): boolean
    if type(t) ~= "table" then return false end
    local n = 0
    for k in pairs(t) do
        if type(k) ~= "number" or k < 1 or k % 1 ~= 0 then return false end
        if k > n then n = k end
    end
    for i = 1, n do
        if t[i] == nil then return false end
    end
    return true
end

function M.parse_component(component)
    component = tostring(trim(component))
    if component == "" then
        return nil, err("BAD_REQUEST", "component is required")
    end
    local org, name = string.match(component, "^([%w_.-]+)/([%w_.-]+)$")
    if not org or not name then
        return nil, err("BAD_REQUEST", "component must be in org/module form")
    end
    if org == "" or name == "" then
        return nil, err("BAD_REQUEST", "component must include both org and module")
    end
    return { org = org, module = name, component = org .. "/" .. name }, nil
end

function M.module_namespace(component: string): string?
    local parsed = M.parse_component(component)
    if not parsed then return nil end
    return parsed.org .. "." .. parsed.module
end

function M.sanitize_dependency_name(name)
    name = string.gsub(string.lower(trim(name)), "[^%w_.-]", "_")
    name = string.gsub(name, "^[_%.%-]+", "")
    name = string.gsub(name, "[_%.%-]+$", "")
    if name == "" then return nil end
    return name
end

function M.validate_namespace(namespace)
    namespace = trim(namespace)
    if namespace == "" then
        return nil, err("BAD_REQUEST", "namespace is required")
    end
    if string.find(namespace, "..", 1, true) or string.sub(namespace, 1, 1) == "." or string.sub(namespace, -1) == "." then
        return nil, err("BAD_REQUEST", "namespace must be dot-separated identifiers")
    end
    for part in string.gmatch(namespace, "[^.]+") do
        if not string.match(part, "^[A-Za-z][A-Za-z0-9_]*$") then
            return nil, err("BAD_REQUEST", "namespace must be dot-separated identifiers")
        end
    end
    if not string.match(namespace, "^[A-Za-z0-9_.]+$") then
        return nil, err("BAD_REQUEST", "namespace must be dot-separated identifiers")
    end
    return namespace, nil
end

function M.resolve_dependency_id(args)
    args = args or {}
    local explicit = tostring(trim(args.id))
    if explicit ~= "" then
        local ns, name = string.match(explicit, "^([^:]+):(.+)$")
        if not ns or not name or name == "" then
            return nil, err("BAD_REQUEST", "id must be namespace:name")
        end
        local ok_ns, ns_err = M.validate_namespace(ns)
        if not ok_ns then return nil, ns_err end
        return explicit, nil
    end

    local parsed, comp_err = M.parse_component(args.component)
    if not parsed then return nil, comp_err end

    local ns, ns_err = M.validate_namespace(args.namespace or M.DEFAULT_DEP_NAMESPACE)
    if not ns then return nil, ns_err end

    local name: string? = trim(args.name)
    if name == "" then
        name = M.sanitize_dependency_name(parsed.module)
    else
        name = M.sanitize_dependency_name(name)
    end
    if not name then
        return nil, err("BAD_REQUEST", "dependency name cannot be empty")
    end
    return ns .. ":" .. name, nil
end

local function entry_namespace(id: unknown): string?
    return tostring(id or ""):match("^([^:]+):")
end

local function entry_name(id: unknown): string?
    return tostring(id or ""):match("^[^:]+:(.+)$")
end

local function requirements_from_entries(entries)
    local out = {}
    for _, entry in ipairs(entries or {}) do
        if type(entry) == "table" and entry.kind == "ns.requirement" then
            local id = trim(entry.id)
            local data = type(entry.data) == "table" and entry.data or entry
            local meta = shallow_copy(type(data.meta) == "table" and data.meta or {})
            for key, value in pairs(type(entry.meta) == "table" and entry.meta or {}) do
                meta[key] = value
            end
            local name = trim(data.name)
            if name == "" then name = trim(entry_name(id)) end
            if name ~= "" then
                table.insert(out, {
                    id = id,
                    namespace = entry_namespace(id),
                    name = name,
                    meta = meta,
                    description = data.description or meta.description or meta.comment,
                    default = data.default,
                    targets = data.targets or {},
                })
            end
        end
    end
    return out
end

local function dependency_component(entry): string
    local data = entry and entry.data or {}
    return trim(data.component)
end

local function namespace_score(namespace: string, count: number): number
    local lower = string.lower(namespace)
    local score = count * 100
    if lower == M.DEFAULT_DEP_NAMESPACE then score = score + 40 end
    if lower == "deps" or lower:match("%.deps$") then score = score + 30 end
    if lower == "dependencies" or lower:match("%.dependencies$") then score = score + 20 end
    if lower == "app" or lower:match("^app%.") then score = score + 10 end
    return score
end

local function managed_namespaces_from(gov): { string }
    local getter = type(gov) == "table" and gov.get_managed_namespaces or nil
    if type(getter) == "function" then
        local ok, namespaces = pcall(getter)
        if ok and type(namespaces) == "table" then
            local out = {}
            for _, ns in ipairs(namespaces) do
                ns = trim(ns)
                if ns ~= "" then table.insert(out, ns) end
            end
            if #out > 0 then return out end
        end
    end
    return { "app", "keeper" }
end

local function namespace_is_managed_by(gov, namespace: string): boolean
    namespace = trim(namespace)
    if namespace == "" then return false end

    local checker = type(gov) == "table" and gov.is_namespace_managed or nil
    if type(checker) == "function" then
        local ok, managed = pcall(checker, namespace)
        if ok and managed == true then return true end
    end

    for _, root in ipairs(managed_namespaces_from(gov)) do
        if namespace == root or namespace:sub(1, #root + 1) == root .. "." then
            return true
        end
    end
    return false
end

-- Placement heuristics must honor an explicit governance answer. The broader
-- namespace_is_managed_by helper retains the legacy app/keeper fallback for a
-- deployment that has not configured governance yet, but that fallback must
-- not make an existing arbitrary cluster look intentionally managed.
local function namespace_is_explicitly_managed_by(gov, namespace: string): boolean
    namespace = trim(namespace)
    if namespace == "" then return false end

    local checker = type(gov) == "table" and gov.is_namespace_managed or nil
    if type(checker) == "function" then
        local ok, managed = pcall(checker, namespace)
        if ok then return managed == true end
    end

    local getter = type(gov) == "table" and gov.get_managed_namespaces or nil
    if type(getter) ~= "function" then return false end
    local ok, roots = pcall(getter)
    if not ok or type(roots) ~= "table" then return false end
    for _, root in ipairs(roots) do
        root = trim(root)
        if root ~= "" and (namespace == root or namespace:sub(1, #root + 1) == root .. ".") then
            return true
        end
    end
    return false
end

local function default_dependency_namespace_for(gov): string
    if namespace_is_managed_by(gov, M.DEFAULT_DEP_NAMESPACE) then return M.DEFAULT_DEP_NAMESPACE end

    local roots = managed_namespaces_from(gov)
    for _, root in ipairs(roots) do
        if root ~= "keeper" then return root .. ".deps" end
    end
    return (roots[1] or "app") .. ".deps"
end

function M.normalize_parameters(input): ({ Parameter }?, unknown?)
    if input == nil then return {}, nil end
    if type(input) ~= "table" then
        return nil, err("BAD_REQUEST", "parameters must be an array of {name,value} or an object map") :: unknown?
    end

    local out: { Parameter } = {}
    local seen = {}

    local function add(name, value)
        name = trim(name)
        if name == "" then
            return err("BAD_REQUEST", "parameter name is required")
        end
        if seen[name] then
            return err("BAD_REQUEST", "duplicate parameter: " .. name)
        end
        seen[name] = true
        if value == nil then value = "" end
        table.insert(out, { name = name, value = tostring(value) })
        return nil
    end

    if is_array(input) then
        for i, item in ipairs(input) do
            if type(item) ~= "table" then
                return nil, err("BAD_REQUEST", "parameters[" .. i .. "] must be an object") :: unknown?
            end
            local add_err = add(item.name, item.value)
            if add_err then return nil, add_err end
        end
    else
        local keys = {}
        for k in pairs(input) do table.insert(keys, k) end
        table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
        for _, k in ipairs(keys) do
            local add_err = add(k, input[k])
            if add_err then return nil, add_err end
        end
    end

    return out, nil
end

function M.build_dependency_entry(args)
    args = args or {}
    local parsed, comp_err = M.parse_component(args.component)
    if not parsed then return nil, comp_err end

    local id, id_err = M.resolve_dependency_id(args)
    if not id then return nil, id_err end

    local parameters, param_err = M.normalize_parameters(args.parameters)
    if not parameters then return nil, param_err end

    local data = {
        component = parsed.component,
        version = trim(args.version) ~= "" and trim(args.version) or M.DEFAULT_VERSION,
    }
    if #parameters > 0 then data.parameters = parameters end

    return {
        id = id,
        kind = "ns.dependency",
        dependency_root = true,
        meta = shallow_copy(args.meta or {}),
        data = data,
    }, nil
end

function M.dependency_summary(entry)
    local data = entry and entry.data or {}
    return {
        id = entry.id,
        namespace = (entry.id and string.match(tostring(entry.id), "^([^:]+):")) or nil,
        name = (entry.id and string.match(tostring(entry.id), "^[^:]+:(.+)$")) or nil,
        kind = entry.kind,
        component = data.component,
        version = data.version,
        parameters = data.parameters or {},
        meta = entry.meta or {},
    }
end

local function version_parts(version): VersionPart?
    version = tostring(trim(version)):gsub("^v", "")
    local maj, min, patch, pre = string.match(version, "^(%d+)%.(%d+)%.(%d+)%-(.+)$")
    if not maj then
        maj, min, patch = string.match(version, "^(%d+)%.(%d+)%.(%d+)$")
    end
    if not maj then return nil end
    return {
        major = tonumber(maj) or 0,
        minor = tonumber(min) or 0,
        patch = tonumber(patch) or 0,
        prerelease = pre or "",
        raw = version,
    }
end

local function compare_versions(a: unknown, b: unknown): number
    local left = type(a) == "table" and (a :: VersionPart) or version_parts(a)
    local right = type(b) == "table" and (b :: VersionPart) or version_parts(b)
    if not left and not right then return 0 end
    if not left then return -1 end
    if not right then return 1 end
    for _, k in ipairs({ "major", "minor", "patch" }) do
        if left[k] ~= right[k] then return left[k] < right[k] and -1 or 1 end
    end
    if left.prerelease == right.prerelease then return 0 end
    if left.prerelease == "" then return 1 end
    if right.prerelease == "" then return -1 end
    return left.prerelease < right.prerelease and -1 or 1
end

local function best_version(items)
    if type(items) ~= "table" then return nil end
    local arr = items :: {VersionItem}
    local best
    for _, item in ipairs(arr) do
        if not item.yanked and trim(item.version) ~= "" then
            if not best or compare_versions(item.version, best.version) > 0 then
                best = item
            end
        end
    end
    return best or arr[1] or nil
end

local function split_constraint(constraint): { string }
    constraint = tostring(trim(constraint)):gsub(",", " ")
    local parts: { string } = {}
    for p in string.gmatch(constraint, "%S+") do table.insert(parts, p) end
    return parts
end

local function parse_constraint_part(part): ConstraintPart?
    part = tostring(part or "")
    if part == "*" or part == "x" or part == "X" then
        return { op = "*", any = true }
    end
    local op = "="
    local ver = part
    for _, candidate in ipairs({ ">=", "<=", "!=", "^", "~", ">", "<", "=" }) do
        if string.sub(part, 1, #candidate) == candidate then
            op = candidate
            ver = string.sub(part, #candidate + 1)
            break
        end
    end
    ver = tostring(ver or "")
    local wildcard_major = string.match(ver, "^(%d+)%.([xX%*])$")
    if wildcard_major then
        return { op = "wildcard_minor", major = tonumber(wildcard_major) }
    end
    local major, minor, wildcard_patch = string.match(ver, "^(%d+)%.(%d+)%.([xX%*])$")
    if major and wildcard_patch then
        return { op = "wildcard_patch", major = tonumber(major), minor = tonumber(minor) }
    end
    local parsed = version_parts(ver)
    if not parsed then return nil end
    return { op = op, version = parsed }
end

local function satisfies_part(version: VersionPart, part: ConstraintPart): boolean
    if part.any then return true end
    if part.op == "wildcard_minor" then return version.major == part.major end
    if part.op == "wildcard_patch" then
        return version.major == part.major and version.minor == part.minor
    end

    local rhs = part.version
    if not rhs then return false end
    local cmp = compare_versions(version, rhs)
    if part.op == "=" then return cmp == 0 end
    if part.op == "!=" then return cmp ~= 0 end
    if part.op == ">" then return cmp > 0 end
    if part.op == ">=" then return cmp >= 0 end
    if part.op == "<" then return cmp < 0 end
    if part.op == "<=" then return cmp <= 0 end
    if part.op == "^" then
        if cmp < 0 then return false end
        local base = part.version
        if base.major > 0 then return version.major == base.major end
        if base.minor > 0 then return version.major == 0 and version.minor == base.minor end
        return version.major == 0 and version.minor == 0 and version.patch == base.patch
    end
    if part.op == "~" then
        return cmp >= 0 and version.major == part.version.major and version.minor == part.version.minor
    end
    return false
end

local function is_semver_constraint(constraint: string): boolean
    constraint = trim(constraint)
    if constraint == "" then return false end
    if starts_with(constraint, "@") then return false end
    if string.find(constraint, "[,<>=~%^!*xX%s]") then return true end
    return false
end

local function version_satisfies(version, constraint): boolean
    local parsed = version_parts(version)
    if not parsed then return false end
    local parts = split_constraint(constraint)
    if #parts == 0 then return true end
    for _, raw in ipairs(parts) do
        local part = parse_constraint_part(raw)
        if not part or not satisfies_part(parsed, part) then return false end
    end
    return true
end

-- Whether one resolved version admits a single recorded constraint. Labels are
-- resolved separately by select_version_for_constraints because they require a
-- catalog lookup; this predicate remains useful for installed offline edges.
local function constraint_allows(version, constraint)
    constraint = trim(constraint)
    if constraint == "" or starts_with(constraint, "@") then return true end
    if is_semver_constraint(constraint) then return version_satisfies(version, constraint) end
    local pinned = version_parts(constraint)
    if not pinned then return true end
    return compare_versions(version, pinned) == 0
end

local function module_ref_from_dep(dep: unknown): string?
    if type(dep) ~= "table" then return nil end
    local dep_map = dep :: {[string]: unknown}
    local org = trim(dep_map.org)
    local name = trim(dep_map.name)
    if org == "" or name == "" then return nil end
    return org .. "/" .. name
end

local function has_entry_kind(version, kind: string): boolean
    for _, entry_kind in ipairs((version and version.entry_kinds) or {}) do
        if tostring(entry_kind) == kind then return true end
    end
    return false
end

local function requirement_required(req): boolean
    if req.required == false then return false end
    if req.default ~= nil and trim(req.default) == "" then return false end
    return true
end

local REQUIREMENT_VALUE_KIND_BY_TARGET_PATH = {
    ["router"] = "http.router",
    [".router"] = "http.router",
    ["meta.router"] = "http.router",
    [".meta.router"] = "http.router",
    ["storage"] = "env.storage",
    [".storage"] = "env.storage",
    ["env_storage"] = "env.storage",
    [".env_storage"] = "env.storage",
}

local REQUIREMENT_VALUE_KIND_BY_NAME = {
    ["router"] = "http.router",
    ["webhook_router"] = "http.router",
    ["env_storage"] = "env.storage",
    ["storage"] = "env.storage",
}

local function requirement_value_kind(req): string?
    local explicit = trim(req and req.meta and req.meta.value_kind)
    if explicit ~= "" then return explicit end
    for _, target in ipairs((req and req.targets) or {}) do
        local path = trim(target.path)
        local kind = REQUIREMENT_VALUE_KIND_BY_TARGET_PATH[path]
        if kind then return kind end
    end
    return REQUIREMENT_VALUE_KIND_BY_NAME[string.lower(trim(req and req.name))]
end

local KIND_PREFIX_SEARCH = {
    ["env.storage"] = true,
}

local function kind_matches(expected: string, actual: unknown): boolean
    expected = trim(expected)
    local kind = trim(actual)
    if expected == "" or kind == "" then return false end
    if kind == expected then return true end
    return KIND_PREFIX_SEARCH[expected] == true and kind:sub(1, #expected + 1) == expected .. "."
end

function M.new(deps: PlannerDeps?)
    deps = deps or {}
    return setmetatable({
        registry = deps.registry or registry,
        catalog = deps.catalog or hub_sdk,
        gov = deps.gov or gov_consts,
        version_cache = {},
    }, Planner) :: PlannerInstance
end

function Planner:find_entries(criteria)
    local rows, find_err = self.registry.find(criteria or {})
    if find_err then
        return nil, err("INTERNAL", "registry.find failed: " .. tostring(find_err))
    end
    return rows or {}, nil
end

function Planner:dependency_entries()
    local rows, rows_err = self:find_entries({ [".kind"] = "ns.dependency" })
    if not rows then return nil, rows_err end
    -- Sort a copy: registry.find may hand back a backing slice, and sorting it
    -- in place mutates registry-owned state shared with later readers.
    local out = {}
    for _, entry in ipairs(rows) do table.insert(out, entry) end
    table.sort(out, function(a, b) return tostring(a.id) < tostring(b.id) end)
    return out, nil
end

-- Deployment roots are dependency directives in namespaces managed by the
-- consuming application. Package dependency edges outside that boundary still
-- participate in graph resolution, but are never install/update destinations.
function Planner:deployment_dependency_entries()
    local rows, rows_err = self:dependency_entries()
    if not rows then return nil, rows_err end
    local out = {}
    for _, entry in ipairs(rows) do
        local ns = entry_namespace(entry.id)
        if ns and namespace_is_managed_by(self.gov, ns) then table.insert(out, entry) end
    end
    return out, nil
end

-- Precomputes the two per-node install-graph flags from live registry state:
--   installed_names: modules represented by module-owned registry entries
--   shared_names: modules already reachable from an EXISTING deployment root
--                 other than the one being installed, walking the same offline
--                 installed edges the closure resolver uses. A node in this set
--                 is reused (shared) rather than newly added.
function Planner:install_graph_context(component)
    local exclude = trim(component)

    local installed_names = {}
    local definitions = self:find_entries({ [".kind"] = "ns.definition" })
    for _, row in ipairs(definitions or {}) do
        local name = trim(row.meta and row.meta.module)
        if name ~= "" then installed_names[name] = true end
    end

    local shared_names = {}
    local deps = self:deployment_dependency_entries()
    if deps then
        local roots = {}
        for _, dep in ipairs(deps) do
            local comp = dependency_component(dep)
            if comp ~= "" and comp ~= exclude then
                table.insert(roots, comp)
            end
        end
        local keep = self:resolve_dependency_closure({ roots = roots })
        if keep then shared_names = keep end
    end

    return { installed_names = installed_names, shared_names = shared_names }
end

function Planner:existing_dependency_for_component(component)
    local wanted = trim(component)
    local deps, deps_err = self:deployment_dependency_entries()
    if not deps then return nil, deps_err end
    local matches = {}
    for _, dep in ipairs(deps) do
        if dependency_component(dep) == wanted then
            table.insert(matches, dep)
        end
    end
    if #matches == 0 then return nil, nil end
    if #matches == 1 then return matches[1], nil end

    local duplicate_ids = {}
    for i, dep in ipairs(matches) do duplicate_ids[tostring(i)] = tostring(dep.id) end
    return nil, err("CONFLICT",
        "multiple application dependency roots declare " .. wanted
            .. "; remove the duplicate roots before installing or updating this component",
        { component = wanted, dependency_ids = duplicate_ids })
end

function Planner:preferred_dependency_namespace(): (string?, unknown?)
    local deps, deps_err = self:deployment_dependency_entries()
    if not deps then return nil, deps_err end

    local counts = {}
    for _, dep in ipairs(deps) do
        local ns = entry_namespace(dep.id)
        if ns and ns ~= "" and namespace_is_explicitly_managed_by(self.gov, ns) then
            counts[ns] = (counts[ns] or 0) + 1
        end
    end

    local best = default_dependency_namespace_for(self.gov)
    local best_score = namespace_score(best, 0)
    for ns, count in pairs(counts) do
        local score = namespace_score(ns, count)
        if score > best_score or (score == best_score and ns < best) then
            best = ns
            best_score = score
        end
    end
    return best, nil
end

function Planner:resolve_dependency_destination_args(args): (unknown?, unknown?)
    local out = shallow_copy(args or {})
    local parsed, comp_err = M.parse_component(out.component)
    if not parsed then return nil, comp_err end

    local explicit_id = trim(out.id)
    local has_explicit_name = trim(out.name) ~= ""
    local has_explicit_namespace = trim(out.namespace) ~= ""
    local existing_root, existing_root_err = self:existing_dependency_for_component(parsed.component)
    if existing_root_err then return nil, existing_root_err end
    if explicit_id == "" and not has_explicit_name and not has_explicit_namespace then
        if existing_root and existing_root.id then
            out.id = existing_root.id
            explicit_id = tostring(existing_root.id)
        end
    end

    if explicit_id == "" and not has_explicit_namespace then
        local namespace, namespace_err = self:preferred_dependency_namespace()
        if not namespace then return nil, namespace_err end
        out.namespace = namespace
    end

    local destination_id, id_err = M.resolve_dependency_id(out)
    if not destination_id then return nil, id_err end
    if existing_root and tostring(existing_root.id) ~= destination_id then
        return nil, err("CONFLICT", parsed.component .. " already has an application dependency root at "
            .. tostring(existing_root.id) .. "; update that root instead of creating " .. destination_id,
            {
                component = parsed.component,
                existing_dependency_id = tostring(existing_root.id),
                requested_dependency_id = destination_id,
            }) :: unknown?
    end
    local destination_namespace = entry_namespace(destination_id)
    local existing, get_err = self.registry.get(destination_id)
    if get_err and not string.find(string.lower(tostring(get_err)), "not found", 1, true) then
        return nil, err("INTERNAL", "failed to inspect dependency destination "
            .. destination_id .. ": " .. tostring(get_err)) :: unknown?
    end
    if existing then
        if existing.kind ~= "ns.dependency" then
            return nil, err("CONFLICT", "dependency destination is already occupied by "
                .. tostring(existing.kind) .. ": " .. destination_id,
                { id = destination_id, existing_kind = existing.kind, component = parsed.component }) :: unknown?
        end
        if not destination_namespace or not namespace_is_managed_by(self.gov, destination_namespace) then
            return nil, err("BAD_REQUEST", "dependency destination is outside the governance-managed dependency namespaces: "
                .. destination_id,
                { id = destination_id, namespace = destination_namespace, component = parsed.component }) :: unknown?
        end
        local existing_component = dependency_component(existing)
        if existing_component ~= "" and existing_component ~= parsed.component then
            -- The default short name can collide across organizations. Pick a
            -- deterministic fully-qualified fallback when the caller did not
            -- explicitly choose the destination name/id.
            if explicit_id == "" and not has_explicit_name then
                out.name = M.sanitize_dependency_name(parsed.org .. "_" .. parsed.module)
                destination_id, id_err = M.resolve_dependency_id(out)
                if not destination_id then return nil, id_err end
                local fallback, fallback_err = self.registry.get(destination_id)
                if fallback_err and not string.find(string.lower(tostring(fallback_err)), "not found", 1, true) then
                    return nil, err("INTERNAL", "failed to inspect dependency destination "
                        .. destination_id .. ": " .. tostring(fallback_err)) :: unknown?
                end
                if fallback then
                    local fallback_component = dependency_component(fallback)
                    local fallback_namespace = entry_namespace(destination_id)
                    if fallback.kind ~= "ns.dependency"
                        or not fallback_namespace
                        or not namespace_is_managed_by(self.gov, fallback_namespace)
                        or fallback_component ~= parsed.component then
                        return nil, err("CONFLICT", "dependency destination is already occupied: "
                            .. destination_id,
                            { id = destination_id, existing_component = fallback_component, component = parsed.component }) :: unknown?
                    end
                end
            else
                return nil, err("CONFLICT", "dependency destination " .. destination_id
                    .. " already belongs to " .. existing_component,
                    { id = destination_id, existing_component = existing_component, component = parsed.component }) :: unknown?
            end
        end
    end
    if not destination_namespace or not namespace_is_managed_by(self.gov, destination_namespace) then
        return nil, err("BAD_REQUEST",
            "dependency destination namespace is not managed by governance: "
                .. tostring(destination_namespace or ""),
            { id = destination_id, namespace = destination_namespace, component = parsed.component }) :: unknown?
    end
    return out, nil
end

function Planner:list_all_versions(component)
    component = trim(component)
    if component == "" then
        return nil, err("BAD_REQUEST", "component is required")
    end
    if self.version_cache[component] then
        return self.version_cache[component], nil
    end
    if not self.catalog or not self.catalog.versions or not self.catalog.versions.list then
        return nil, err("INTERNAL", "hub catalog versions API unavailable")
    end

    local out = {}
    local page = 1
    while true do
        local result, call_err = self.catalog.versions.list(component, { page = page, page_size = 100 })
        if not result then
            return nil, err("INTERNAL", "hub versions lookup failed for " .. component .. ": " .. tostring(call_err))
        end
        for _, item in ipairs(result.items or {}) do table.insert(out, item) end
        local total = tonumber(result.total or 0) or 0
        local page_size = tonumber(result.page_size or 100) or 100
        if #out >= total or #(result.items or {}) < page_size then break end
        page = page + 1
        if page > 50 then
            return nil, err("INTERNAL", "hub versions lookup exceeded pagination guard for " .. component)
        end
    end
    table.sort(out, function(a, b)
        return compare_versions(a.version, b.version) > 0
    end)
    self.version_cache[component] = out
    return out, nil
end

function Planner:version_details(component, selected)
    if not selected or not self.catalog or not self.catalog.versions or not self.catalog.versions.get then
        local with_deps, dep_err = self:dependency_details(component, selected)
        if not with_deps then return nil, dep_err end
        return self:artifact_requirement_details(component, with_deps)
    end

    local ref
    if trim(selected.id) ~= "" then
        ref = { id = selected.id }
    elseif trim(selected.version) ~= "" then
        ref = { version = selected.version }
    else
        return selected, nil
    end

    local detailed, detail_err = self.catalog.versions.get(component, ref)
    if detailed then
        local with_deps, dep_err = self:dependency_details(component, detailed)
        if not with_deps then return nil, dep_err end
        return self:artifact_requirement_details(component, with_deps)
    end

    -- Some Hub implementations return full records from list and do not support
    -- get-by-id/version. Planning must still work from the list payload.
    local with_deps, dep_err = self:dependency_details(component, selected)
    if not with_deps then return nil, dep_err end
    return self:artifact_requirement_details(component, with_deps)
end

function Planner:artifact_requirement_details(component, selected)
    if not selected then return selected, nil end
    local selected_item = selected :: VersionItem
    if not has_entry_kind(selected_item, "ns.requirement") then
        return selected, nil
    end
    local inspected, inspect_err = self:inspect_artifact(component, selected_item)
    if not inspected then
        if #(selected_item.requirements or {}) > 0 then
            return selected, nil
        end
        return nil, err("INTERNAL", "hub artifact inspection failed for " .. component .. ": " .. tostring(inspect_err))
    end

    local inspected_artifact = inspected :: any
    local merged = shallow_copy(selected_item)
    local entry_requirements = requirements_from_entries(inspected_artifact.entries)
    if #entry_requirements > 0 then
        merged.requirements = entry_requirements
    else
        merged.requirements = inspected_artifact.requirements or {}
    end
    merged.entry_count = inspected_artifact.entry_count or merged.entry_count
    merged.entry_kinds = inspected_artifact.entry_kinds or merged.entry_kinds
    merged.size_bytes = inspected_artifact.size_bytes or merged.size_bytes
    merged.digest = inspected_artifact.digest or merged.digest
    merged.protected = inspected_artifact.protected == true or merged.protected == true
    return merged, nil
end

function Planner:inspect_artifact(component, selected)
    component = trim(component)
    local selected_item = selected :: VersionItem
    if component == "" then
        return nil, err("BAD_REQUEST", "component is required")
    end

    local ref
    if trim(selected_item.version) ~= "" then
        ref = { version = selected_item.version }
    elseif trim(selected_item.id) ~= "" then
        ref = { id = selected_item.id }
    else
        return nil, err("INTERNAL", "cannot inspect Hub artifact without version id or version")
    end

    local versions = self.catalog and self.catalog.versions
    local inspect = versions and versions.inspect
    if versions and type(versions.open) == "function" then
        local pkg, open_err = versions.open(component, ref)
        if pkg then
            local entries, entries_err = pkg:entries({ include_data = true })
            local close_ok, close_err = pcall(function() return pkg:close() end)
            if not close_ok then close_err = tostring(close_err) end
            if type(entries) == "table" then
                return {
                    entries = entries,
                    entry_count = #entries,
                    version = trim(pkg.version) ~= "" and tostring(pkg.version) or selected_item.version,
                    digest = trim(pkg.digest) ~= "" and tostring(pkg.digest) or selected_item.digest,
                    close_error = close_err,
                }, nil
            end
            if type(inspect) ~= "function" then
                return nil, entries_err or open_err or "hub artifact entries unavailable"
            end
        elseif type(inspect) ~= "function" then
            return nil, open_err or "hub artifact open failed"
        end
    end

    if type(inspect) ~= "function" then
        return nil, err("INTERNAL", "hub artifact inspection API unavailable for " .. component)
    end
    return inspect(component, ref)
end

function Planner:dependency_details(component, selected)
    if not selected then return selected, nil end
    local selected_item = selected :: VersionItem
    if #(selected_item.dependencies or {}) > 0 or not has_entry_kind(selected_item, "ns.dependency") then
        return selected, nil
    end
    if not self.catalog or not self.catalog.dependencies or not self.catalog.dependencies.get then
        return nil, err("INTERNAL", "hub dependency metadata API unavailable for " .. component)
    end

    local version = trim(selected_item.version)
    local version_ref = version ~= "" and version or nil
    if not version_ref and trim(selected_item.id) ~= "" then
        version_ref = { id = selected_item.id }
    end
    if not version_ref then
        return nil, err("INTERNAL", "cannot resolve Hub dependencies without version id or version")
    end

    local result, dependency_err = self.catalog.dependencies.get(component, version_ref)
    if not result then
        return nil, err("INTERNAL", "hub dependency lookup failed for " .. component .. ": " .. tostring(dependency_err))
    end

    local merged = shallow_copy(selected_item)
    merged.dependencies = result.items or result.dependencies or {}
    return merged, nil
end

function Planner:select_version(component, constraint)
    constraint = trim(constraint)
    local versions, versions_err = self:list_all_versions(component)
    if not versions then return nil, versions_err end
    if #versions == 0 then
        return nil, err("NOT_FOUND", "no versions available for " .. component)
    end

    if constraint == "" then
        local selected = best_version(versions)
        return self:version_details(component, selected)
    end

    if starts_with(constraint, "@") then
        if self.catalog.versions.get then
            local label = string.sub(constraint, 2)
            local version, get_err = self.catalog.versions.get(component, { label = label })
            if version then return version, nil end
            return nil, err("NOT_FOUND", "label " .. constraint .. " not found for " .. component .. ": " .. tostring(get_err))
        end
        return nil, err("BAD_REQUEST", "hub catalog does not support label resolution")
    end

    if not is_semver_constraint(constraint) then
        for _, item in ipairs(versions) do
            if item.version == constraint or item.version == ("v" .. constraint) then
                return self:version_details(component, item)
            end
        end
        return nil, err("NOT_FOUND", "version " .. constraint .. " not found for " .. component)
    end

    local selected
    for _, item in ipairs(versions) do
        if not item.yanked and version_satisfies(item.version, constraint) then
            if not selected or compare_versions(item.version, selected.version) > 0 then
                selected = item
            end
        end
    end
    if not selected then
        return nil, err("NOT_FOUND", "no version of " .. component .. " satisfies " .. constraint)
    end
    return self:version_details(component, selected)
end

-- Selects the highest published version in the intersection of every incoming
-- edge constraint. A dependency graph is not a tree: diamonds and cycles can
-- address the same module more than once, and choosing from only the first edge
-- makes the plan depend on traversal order.
function Planner:select_version_for_constraints(component, incoming, preferred, keep_preferred)
    incoming = incoming or {}
    if #incoming == 0 then
        incoming = { { constraint = M.DEFAULT_VERSION, required_by = "root", path = component } }
    end

    local versions, versions_err = self:list_all_versions(component)
    if not versions then return nil, versions_err end
    if #versions == 0 then
        return nil, err("NOT_FOUND", "no versions available for " .. component)
    end

    local labels = {}
    for _, edge in ipairs(incoming) do
        local constraint = trim(edge.constraint)
        if starts_with(constraint, "@") and labels[constraint] == nil then
            local selected, label_err = self:select_version(component, constraint)
            if not selected then return nil, label_err end
            labels[constraint] = tostring(selected.version or "")
        end
    end

    local function admits(item)
        for _, edge in ipairs(incoming) do
            local constraint = trim(edge.constraint)
            if constraint == "" then
                if item.yanked then return false end
            elseif starts_with(constraint, "@") then
                if compare_versions(item.version, labels[constraint]) ~= 0 then return false end
            elseif is_semver_constraint(constraint) then
                if item.yanked or not version_satisfies(item.version, constraint) then return false end
            else
                local pinned = version_parts(constraint)
                if pinned then
                    if compare_versions(item.version, pinned) ~= 0 then return false end
                elseif tostring(item.version) ~= constraint and tostring(item.version) ~= "v" .. constraint then
                    return false
                end
            end
        end
        return true
    end

    -- During traversal, preserve the previous pass's choice while it satisfies
    -- the constraints seen so far. The complete incoming set is intersected
    -- after the pass; eagerly upgrading from a partial set would oscillate in a
    -- compatible diamond (broad edge -> latest, full intersection -> older).
    if keep_preferred and preferred and admits(preferred) then
        return preferred, nil
    end

    local selected
    for _, item in ipairs(versions) do
        if admits(item) and (not selected or compare_versions(item.version, selected.version) > 0) then
            selected = item
        end
    end
    if not selected then
        local summaries = {}
        local rows = {}
        for _, edge in ipairs(incoming) do
            local constraint = trim(edge.constraint)
            if constraint == "" then constraint = M.DEFAULT_VERSION end
            local required_by = trim(edge.required_by)
            if required_by == "" then required_by = "unknown" end
            table.insert(summaries, constraint .. " (required by " .. required_by .. ")")
            table.insert(rows, {
                constraint = constraint,
                required_by = required_by,
                path = edge.path,
            })
        end
        return nil, err("CONFLICT", "conflicting version constraints for " .. component .. ": "
            .. table.concat(summaries, ", "), {
                component = component,
                constraints = M.position_keyed(rows),
            })
    end

    if preferred and compare_versions(preferred.version, selected.version) == 0 then
        return preferred, nil
    end
    return self:version_details(component, selected)
end

-- Shared worklist graph walker. Starts from a set of roots and, for each
-- module, calls expand(ref, constraint, depth, parent, path) which returns a
-- node to record (or nil) and its child edges as { ref, constraint } items.
-- Dedup, depth/count limits, and error accumulation are handled here so both
-- the Hub install graph and the installed-closure resolver share one walk.
function Planner:walk_graph(roots, expand, opts)
    opts = type(opts) == "table" and (opts :: {[string]: unknown}) or {}
    local max_depth = tonumber(opts.max_depth or M.DEFAULT_PLAN_MAX_DEPTH) or M.DEFAULT_PLAN_MAX_DEPTH
    local max_modules = tonumber(opts.max_modules or M.DEFAULT_PLAN_MAX_MODULES) or M.DEFAULT_PLAN_MAX_MODULES
    local nodes = {}
    local seen = {}
    local resolution_errors = {}

    local function visit(ref, constraint, depth, parent, path)
        if #nodes >= max_modules then
            table.insert(resolution_errors, { module = ref, constraint = constraint, message = "maximum module count exceeded" })
            return
        end
        if depth >= max_depth then
            table.insert(resolution_errors, { module = ref, constraint = constraint, message = "maximum dependency depth exceeded" })
            return
        end
        if seen[ref] then return end
        seen[ref] = true

        local node, children, expand_err = expand(ref, constraint, depth, parent, path)
        if expand_err then
            table.insert(resolution_errors, {
                module = ref,
                constraint = constraint,
                message = err_message(expand_err, "dependency resolution failed"),
                details = err_details(expand_err),
            })
            return
        end
        if node then table.insert(nodes, node) end

        for _, child in ipairs(children or {}) do
            local child_ref = child.ref
            if child_ref and child_ref ~= "" then
                visit(child_ref, child.constraint or "", depth + 1, ref, (path or ref) .. " > " .. child_ref)
            end
        end
    end

    for _, root in ipairs(roots or {}) do
        visit(root.ref, root.constraint or M.DEFAULT_VERSION, 0, nil, root.path or root.ref)
    end

    if #resolution_errors > 0 then
        local summaries = {}
        for _, row in ipairs(resolution_errors) do
            table.insert(summaries, tostring(row.module) .. " (" .. tostring(row.constraint) .. "): " .. tostring(row.message))
        end
        return nil, err("CONFLICT", "dependency resolution failed: " .. table.concat(summaries, "; "),
            { errors = M.position_keyed(resolution_errors) })
    end
    return nodes, nil
end

function Planner:resolve_install_graph(component, constraint, opts)
    opts = type(opts) == "table" and (opts :: {[string]: unknown}) or {}
    local parsed, comp_err = M.parse_component(component)
    if not parsed then return nil, comp_err end

    local ctx = self:install_graph_context(parsed.component)
    local installed_constraints, installed_constraints_err = self:installed_constraints(parsed.component)
    if not installed_constraints then return nil, installed_constraints_err end
    local max_depth = tonumber(opts.max_depth or M.DEFAULT_PLAN_MAX_DEPTH) or M.DEFAULT_PLAN_MAX_DEPTH
    local max_modules = tonumber(opts.max_modules or M.DEFAULT_PLAN_MAX_MODULES) or M.DEFAULT_PLAN_MAX_MODULES
    local preferred = {}
    local max_passes = math.max(8, max_modules * 2)

    -- A pass traverses breadth-first so each node's displayed parent/depth/path
    -- is its shortest discovered route. It then intersects all incoming
    -- constraints. If an intersection changes a selected version, another pass
    -- rebuilds edges from that version; dependencies removed by reselection do
    -- not remain as stale phantom constraints.
    for _ = 1, max_passes do
        local queue = {
            {
                ref = parsed.component,
                constraint = constraint or M.DEFAULT_VERSION,
                depth = 0,
                parent = nil,
                path = parsed.component,
                required_by = "root",
            },
        }
        local head = 1
        local nodes = {}
        local node_by_ref = {}
        local incoming_by_ref = {}
        local resolution_errors = {}

        while head <= #queue do
            local work = queue[head]
            head = head + 1
            local ref = trim(work.ref)
            if ref ~= "" then
                incoming_by_ref[ref] = incoming_by_ref[ref] or {}
                table.insert(incoming_by_ref[ref], {
                    constraint = work.constraint or "",
                    required_by = work.required_by or work.parent or "root",
                    path = work.path,
                })

                if not node_by_ref[ref] then
                    if work.depth >= max_depth then
                        table.insert(resolution_errors, {
                            module = ref,
                            constraint = work.constraint,
                            message = "maximum dependency depth exceeded",
                        })
                    elseif #nodes >= max_modules then
                        table.insert(resolution_errors, {
                            module = ref,
                            constraint = work.constraint,
                            message = "maximum module count exceeded",
                        })
                    else
                        local selected, select_err = self:select_version_for_constraints(
                            ref, incoming_by_ref[ref], preferred[ref], true)
                        if not selected then
                            table.insert(resolution_errors, {
                                module = ref,
                                constraint = work.constraint,
                                message = err_message(select_err, "dependency resolution failed"),
                                details = err_details(select_err),
                            })
                        else
                            local selected_item = selected :: VersionItem
                            local ref_parsed = M.parse_component(ref)
                            local node = {
                                module = ref,
                                org = ref_parsed and ref_parsed.org or (string.match(ref, "^([^/]+)/") or ""),
                                name = ref_parsed and ref_parsed.module or (string.match(ref, "/(.+)$") or ""),
                                namespace = M.module_namespace(ref) or ref,
                                version = selected_item.version or "",
                                version_id = selected_item.id,
                                constraint = work.constraint or "",
                                constraints = incoming_by_ref[ref],
                                depth = work.depth,
                                path = work.path or ref,
                                direct = work.depth == 0,
                                dependencies = selected_item.dependencies or {},
                                requirements = selected_item.requirements or {},
                                __selected = selected_item,
                            }
                            if work.parent then node.parent = work.parent end
                            node.entry_count = selected_item.entry_count
                            node.entry_kinds = selected_item.entry_kinds or {}
                            node.lua_modules = selected_item.lua_modules or {}
                            node.size_bytes = selected_item.size_bytes
                            node.digest = selected_item.digest
                            node.yanked = selected_item.yanked == true
                            node.protected = selected_item.protected == true

                            local installed = ctx.installed_names[ref] == true
                            if not installed then installed = self:module_installed(ref) == true end
                            node.installed = installed
                            node.shared = ctx.shared_names[ref] == true

                            node_by_ref[ref] = node
                            table.insert(nodes, node)
                            for _, dep in ipairs(selected_item.dependencies or {}) do
                                local child_ref = module_ref_from_dep(dep)
                                if child_ref then
                                    table.insert(queue, {
                                        ref = child_ref,
                                        constraint = dep.version_constraint or dep.version or "",
                                        depth = work.depth + 1,
                                        parent = ref,
                                        required_by = ref,
                                        path = (work.path or ref) .. " > " .. child_ref,
                                    })
                                end
                            end
                        end
                    end
                else
                    node_by_ref[ref].constraints = incoming_by_ref[ref]
                end
            end
        end

        local next_preferred = {}
        local changed = false
        local conflicts = {}
        for _, node in ipairs(nodes) do
            local effective_constraints = {}
            for _, edge in ipairs(incoming_by_ref[node.module] or {}) do
                table.insert(effective_constraints, edge)
            end
            for _, existing in ipairs(installed_constraints) do
                -- An installed edge owned by a module in this graph will be
                -- replaced by the install and must not constrain its new
                -- version. Remaining deployment roots and their reachable
                -- module edges still participate in the same intersection.
                if existing.component == node.module
                    and (existing.owner == nil or node_by_ref[existing.owner] == nil) then
                    table.insert(effective_constraints, {
                        constraint = existing.constraint,
                        required_by = existing.required_by,
                        path = existing.entry_id,
                    })
                end
            end
            node.constraints = effective_constraints
            local selected, select_err = self:select_version_for_constraints(
                node.module, effective_constraints, node.__selected)
            if not selected then
                table.insert(conflicts, select_err)
            else
                next_preferred[node.module] = selected
                if compare_versions(selected.version, node.version) ~= 0 then changed = true end
            end
        end

        -- Lookup/depth failures can belong to an outgoing edge from a version
        -- that this same pass just narrowed. Rebuild first; only report failures
        -- after selections and therefore the reachable edge set are stable.
        if changed then
            preferred = next_preferred
        elseif #resolution_errors > 0 then
            local summaries = {}
            for _, row in ipairs(resolution_errors) do
                table.insert(summaries, tostring(row.module) .. " (" .. tostring(row.constraint) .. "): "
                    .. tostring(row.message))
            end
            return nil, err("CONFLICT", "dependency resolution failed: " .. table.concat(summaries, "; "),
                { errors = M.position_keyed(resolution_errors) })
        elseif #conflicts > 0 then
            return nil, conflicts[1]
        else
            for _, node in ipairs(nodes) do node.__selected = nil end
            return nodes, nil
        end
    end

    return nil, err("CONFLICT", "dependency resolution did not converge for " .. parsed.component,
        { component = parsed.component, max_passes = max_passes })
end

-- True when the module has any registry entry stamped with meta.module, i.e. it
-- is installed locally and its dependency edges live in the registry.
function Planner:module_installed(component)
    local rows, rows_err = self:find_entries({ ["meta.module"] = component })
    if rows_err then return false, rows_err end
    return rows ~= nil and #rows > 0, nil
end

-- Child components of a locally installed module, read from its module-owned
-- ns.dependency entries (meta.module == component -> data.component).
function Planner:installed_child_components(component)
    local rows, rows_err = self:find_entries({ ["meta.module"] = component, [".kind"] = "ns.dependency" })
    if not rows then return nil, rows_err end
    local out = {}
    local seen = {}
    for _, row in ipairs(rows) do
        local data = row.data or {}
        local child = trim(data.component)
        if child ~= "" and not seen[child] then
            seen[child] = true
            table.insert(out, { ref = child, constraint = trim(data.version) })
        end
    end
    return out, nil
end

-- Computes the installed dependency closure of a set of deployment roots by
-- walking local registry edges only (module-owned ns.dependency entries:
-- meta.module == component -> data.component). Resolution is fully offline: a
-- ref with no local installation is treated as a leaf and contributes only its
-- own name. Uninstall must never re-resolve against the Hub, because brownfield
-- apps carry roots whose modules are unpublished or never installed (e.g. a
-- source-declared or stale root); a network re-resolve would fail the whole
-- prune on those.
--
-- Returns (keep, unresolved): keep is the set of module names in the closure;
-- unresolved is the set of reached refs that have no local installation and thus
-- unknowable dependency edges. The caller may surface that uncertainty in a
-- preview, but live installed state remains the registry itself.
--
-- Soundness contract: this offline closure is correct because governance install
-- writes each module's complete edge set atomically -- a module's ns.definition
-- and every module-owned ns.dependency row (meta.module == it) land in ONE
-- all-or-nothing governance changeset. So an installed module always exposes its
-- FULL edge set here; a partial edge set (some edges present, one missing) implies
-- registry corruption, which is out of scope. Where such corruption manifests as
-- an edgeless remaining root it is surfaced as unresolved rather than inferred.
function Planner:resolve_dependency_closure(args)
    args = args or {}

    local roots = {}
    for _, root in ipairs(args.roots or {}) do
        local component
        if type(root) == "table" then
            component = dependency_component(root)
            if component == "" then component = trim((root :: {[string]: unknown}).component) end
        else
            component = trim(root)
        end
        if component ~= "" then
            table.insert(roots, { ref = component })
        end
    end

    local unresolved = {}
    local function expand(ref, constraint, depth, parent, path)
        local installed, inst_err = self:module_installed(ref)
        if inst_err then return nil, nil, inst_err end
        if not installed then
            unresolved[ref] = true
            return { module = ref }, {}, nil
        end
        local children, edges_err = self:installed_child_components(ref)
        if not children then return nil, nil, edges_err end
        return { module = ref }, children, nil
    end

    local nodes, walk_err = self:walk_graph(roots, expand, {
        max_depth = args.max_depth,
        max_modules = args.max_modules,
    })
    if not nodes then return nil, nil, walk_err end

    local keep = {}
    for _, node in ipairs(nodes) do keep[node.module] = true end
    return keep, unresolved, nil
end

-- Supplied fully-qualified parameters can only configure the root module.
-- The install pipeline attaches parameters to the root's dependency entry, so
-- a full id addressing a transitive module's requirement would be recorded but
-- never applied; refuse it loudly instead of installing into the conflict the
-- parameter was meant to avoid. Bare names keep their fan-out semantics.
local function validate_parameter_targets(graph, parameters)
    local node_by_namespace = {}
    for _, node in ipairs(graph or {}) do
        node_by_namespace[tostring(node.namespace)] = node
    end
    for _, param in ipairs(parameters or {}) do
        local ns = string.match(tostring(param.name or ""), "^([^:]+):")
        local node = ns and node_by_namespace[ns] or nil
        if node and node.depth > 0 then
            return err("PARAMETER_TARGET_TRANSITIVE",
                "parameter " .. tostring(param.name) .. " targets a requirement of the transitive module "
                    .. tostring(node.module) .. "; parameters apply only to the module being installed. Install "
                    .. tostring(node.module) .. " as an explicit root with this parameter instead",
                { parameter = param.name, module = node.module, dependency_path = node.path })
        end
    end
    return nil
end

-- Version constraints the registry already records offline: deployment roots
-- pin their component and installed modules pin their children through
-- module-owned ns.dependency edges -- the same rows the closure resolver
-- walks. exclude_component drops the root entry being installed or updated,
-- whose recorded constraint this plan replaces.
function Planner:installed_constraints(exclude_component)
    local roots, roots_err = self:deployment_dependency_entries()
    if not roots then return nil, roots_err end
    local remaining_roots = {}
    for _, root in ipairs(roots) do
        if dependency_component(root) ~= exclude_component then
            table.insert(remaining_roots, root)
        end
    end
    local reachable, _, closure_err = self:resolve_dependency_closure({ roots = remaining_roots })
    if not reachable then return nil, closure_err end

    local rows, rows_err = self:dependency_entries()
    if not rows then return nil, rows_err end
    local out = {}
    for _, entry in ipairs(rows) do
        local component = dependency_component(entry)
        local constraint = trim(entry.data and entry.data.version)
        local owner = trim(entry.meta and entry.meta.module)
        if component ~= "" and constraint ~= "" then
            if owner ~= "" then
                if reachable[owner] == true then
                    table.insert(out, { component = component, constraint = constraint, required_by = owner, owner = owner })
                end
            elseif component ~= exclude_component then
                table.insert(out, { component = component, constraint = constraint, required_by = "root", entry_id = tostring(entry.id) })
            end
        end
    end
    return out, nil
end

-- Plan==install fidelity: every resolved module version must satisfy the
-- constraints existing installed roots and edges already record, because the
-- install apply enforces them. A mismatch fails the plan with the same
-- conflict the install would raise. Edges owned by a module that is itself in
-- the resolved graph do not bind: this install re-resolves that module and
-- rewrites its edges.
function Planner:validate_graph_constraints(graph, component)
    local constraints, constraints_err = self:installed_constraints(component)
    if not constraints then return constraints_err end

    local node_by_module = {}
    for _, node in ipairs(graph or {}) do node_by_module[node.module] = node end

    local conflicts = {}
    local summaries = {}
    for _, c in ipairs(constraints) do
        local node = node_by_module[c.component]
        local owner_in_graph = c.owner ~= nil and node_by_module[c.owner] ~= nil
        if node and not owner_in_graph and not constraint_allows(node.version, c.constraint) then
            local resolved_required_by = tostring(node.parent or component)
            local summary = "conflicting version constraints for " .. c.component .. ": "
                .. c.constraint .. " (required by " .. c.required_by .. "), "
                .. node.version .. " (required by " .. resolved_required_by .. ")"
            table.insert(conflicts, {
                module = c.component,
                installed_constraint = c.constraint,
                required_by = c.required_by,
                resolved_version = node.version,
                resolved_required_by = resolved_required_by,
                message = summary,
            })
            table.insert(summaries, summary)
        end
    end
    if #conflicts > 0 then
        return err("CONFLICT", table.concat(summaries, "; "), { conflicts = M.position_keyed(conflicts) })
    end
    return nil
end

function Planner:existing_parameter_values()
    local deps, deps_err = self:deployment_dependency_entries()
    if not deps then return nil, deps_err end
    local out = {}
    for _, dep in ipairs(deps) do
        local data = dep.data or {}
        for _, param in ipairs(data.parameters or {}) do
            local name = trim(param.name)
            if name ~= "" then
                table.insert(out, {
                    name = name,
                    value = tostring(param.value or ""),
                    dependency_id = tostring(dep.id),
                    component = data.component,
                })
            end
        end
    end
    return out, nil
end

function Planner:plan_requirements(graph, supplied_parameters)
    supplied_parameters = supplied_parameters or {}

    local function find_supplied(full_id, name, direct)
        for _, param in ipairs(supplied_parameters) do
            local param_name = trim(param.name)
            if param_name == full_id then
                return tostring(param.value or ""), "provided"
            end
            if direct and param_name == name then
                return tostring(param.value or ""), "provided_bare"
            end
        end
        return nil, nil
    end

    local existing, existing_err = self:existing_parameter_values()
    if not existing then return nil, existing_err end

    local function unique_existing_values(name, component)
        local out = {}
        local seen = {}
        for _, param in ipairs(existing or {}) do
            local value = trim(param.value)
            if value ~= "" and param.name == name and (component == nil or param.component == component) and not seen[value] then
                seen[value] = true
                table.insert(out, {
                    value = value,
                    dependency_id = param.dependency_id,
                    component = param.component,
                })
            end
        end
        return out
    end

    local function registry_values_for_kind(kind)
        kind = trim(kind)
        if kind == "" then return {}, nil end
        if kind == "security.scope" then
            local out = {}
            local seen = {}

            local function add(value, actual_kind, label, description)
                value = trim(value)
                if value == "" or seen[value] then return end
                seen[value] = true
                table.insert(out, {
                    value = value,
                    kind = actual_kind or kind,
                    label = trim(label) ~= "" and trim(label) or value,
                    description = trim(description),
                })
            end

            local scope_rows, scope_err = self:find_entries({ [".kind"] = "security.scope" })
            if not scope_rows then return nil, scope_err end
            for _, row in ipairs(scope_rows or {}) do
                local meta = type(row.meta) == "table" and row.meta or {}
                add(row.id, row.kind or kind, meta.title, meta.comment or meta.description)
            end

            local policy_rows, policy_err = self:find_entries({ [".kind"] = "security.policy" })
            if not policy_rows then return nil, policy_err end
            for _, row in ipairs(policy_rows or {}) do
                local data = type(row.data) == "table" and row.data or row
                for _, group in ipairs(type(data.groups) == "table" and data.groups or {}) do
                    add(group, kind)
                end
            end

            local descriptor_rows, descriptor_err = self:find_entries({ [".kind"] = "registry.entry" })
            if not descriptor_rows then return nil, descriptor_err end
            for _, row in ipairs(descriptor_rows or {}) do
                local meta = type(row.meta) == "table" and row.meta or {}
                local data = type(row.data) == "table" and row.data or row
                if trim(meta.type) == "kickside.security.role" then
                    add(data.role_id, kind, meta.title, meta.comment or meta.description)
                end
            end

            table.sort(out, function(a, b) return tostring(a.value) < tostring(b.value) end)
            return out, nil
        end

        local criteria = KIND_PREFIX_SEARCH[kind] == true and {} or { [".kind"] = kind }
        local rows, rows_err = self:find_entries(criteria)
        if not rows then return nil, rows_err end
        table.sort(rows, function(a, b) return tostring(a.id) < tostring(b.id) end)

        local out = {}
        local seen = {}
        for _, row in ipairs(rows or {}) do
            local value = trim(row.id)
            if value ~= "" and kind_matches(kind, row.kind) and not seen[value] then
                seen[value] = true
                table.insert(out, { value = value, kind = row.kind or kind })
            end
        end
        return out, nil
    end

    local function candidate_set(candidates)
        local out = {}
        for _, candidate in ipairs(candidates or {}) do
            out[trim(candidate.value)] = true
        end
        return out
    end

    local function add_suggestion(suggestions, seen, value, label, source, dependency_id, kind, description)
        value = trim(value)
        if value == "" or seen[value] then return end
        seen[value] = true
        table.insert(suggestions, {
            value = value,
            label = label or value,
            source = source,
            dependency_id = dependency_id,
            kind = kind,
            description = trim(description),
        })
    end

    local out = {}
    local values = {}
    local missing = {}

    for _, node in ipairs(graph or {}) do
        for _, req in ipairs(node.requirements or {}) do
            local name = trim(req.name)
            if name ~= "" then
                local requirement_namespace = trim(req.namespace)
                if requirement_namespace == "" then
                    requirement_namespace = tostring(node.namespace or M.module_namespace(node.module) or node.module)
                end
                local full_id = requirement_namespace .. ":" .. name
                local value, source = find_supplied(full_id, name, node.direct)
                local suggestions = {}
                local suggestion_seen = {}
                local expected_kind = requirement_value_kind(req)
                local registry_candidates = {}
                local registry_candidate_set = {}
                if expected_kind then
                    local candidates, candidates_err = registry_values_for_kind(expected_kind)
                    if not candidates then return nil, candidates_err end
                    registry_candidates = candidates
                    registry_candidate_set = candidate_set(candidates)
                end

                local function compatible_value(v)
                    v = trim(v)
                    if v == "" then return false end
                    if not expected_kind then return true end
                    return registry_candidate_set[v] == true
                end

                local exact_existing = unique_existing_values(full_id)
                for _, match in ipairs(exact_existing) do
                    if compatible_value(match.value) then
                        add_suggestion(
                            suggestions,
                            suggestion_seen,
                            match.value,
                            match.value .. " from " .. tostring(match.dependency_id),
                            "existing",
                            match.dependency_id,
                            expected_kind
                        )
                    end
                end

                -- Bare names are application profile keys. Reuse them across
                -- managed deployment roots only when every declaration agrees.
                -- existing_parameter_values() already excludes package-owned
                -- transitive edges, so this cannot import package-private state.
                local bare_existing = unique_existing_values(name)
                for _, match in ipairs(bare_existing) do
                    if compatible_value(match.value) then
                        add_suggestion(
                            suggestions,
                            suggestion_seen,
                            match.value,
                            match.value .. " from " .. tostring(match.dependency_id),
                            "existing_bare",
                            match.dependency_id,
                            expected_kind
                        )
                    end
                end

                local default_value = trim(req.default)
                local default_compatible = compatible_value(default_value)
                local invalid_value = false
                local invalid_reason = nil
                if value ~= nil and trim(value) ~= "" and not compatible_value(value) then
                    invalid_value = true
                    invalid_reason = "value must reference an existing " .. tostring(expected_kind)
                    source = tostring(source or "provided") .. "_invalid"
                end
                if default_value ~= "" and default_compatible then
                    add_suggestion(
                        suggestions,
                        suggestion_seen,
                        default_value,
                        "package default: " .. default_value,
                        "default",
                        nil,
                        expected_kind
                    )
                end

                for _, candidate in ipairs(registry_candidates) do
                    local registry_candidate = candidate :: any
                    add_suggestion(
                        suggestions,
                        suggestion_seen,
                        registry_candidate.value,
                        registry_candidate.label or registry_candidate.value,
                        "registry",
                        nil,
                        registry_candidate.kind,
                        registry_candidate.description
                    )
                end

                if value == nil then
                    local compatible_existing = {}
                    for _, match in ipairs(exact_existing) do
                        if compatible_value(match.value) then table.insert(compatible_existing, match) end
                    end
                    if #compatible_existing == 1 then
                        value = compatible_existing[1].value
                        source = "existing"
                    elseif #compatible_existing > 1 then
                        source = "conflict"
                    end
                end
                if value == nil and source ~= "conflict" then
                    local compatible_existing = {}
                    for _, match in ipairs(bare_existing) do
                        if compatible_value(match.value) then table.insert(compatible_existing, match) end
                    end
                    if #compatible_existing == 1 then
                        value = compatible_existing[1].value
                        source = "existing_bare"
                    elseif #compatible_existing > 1 then
                        source = "conflict"
                    end
                end
                if value == nil and source ~= "conflict" and default_value ~= "" and default_compatible then
                    value = default_value
                    source = "default"
                end
                value = value or ""
                values[full_id] = value

                local row = {
                    name = name,
                    short_name = name,
                    parameter_name = full_id,
                    full_id = full_id,
                    module = node.module,
                    namespace = node.namespace,
                    version = node.version,
                    depth = node.depth,
                    dependency_path = node.path,
                    description = req.description,
                    default = req.default,
                    targets = req.targets or {},
                    expected_kind = expected_kind,
                    required = requirement_required(req),
                    value = value,
                    value_source = source or "empty",
                    invalid = invalid_value,
                    invalid_reason = invalid_reason,
                    suggestions = suggestions,
                    transitive = node.depth > 0,
                }
                row.missing = row.required and (trim(value) == "" or row.invalid == true)
                table.insert(out, row)
            end
        end
    end

    table.sort(out, function(a, b)
        if a.depth ~= b.depth then return a.depth < b.depth end
        if a.module ~= b.module then return tostring(a.module) < tostring(b.module) end
        return tostring(a.name) < tostring(b.name)
    end)

    local parameters = {}
    for _, row in ipairs(out) do
        if row.missing then table.insert(missing, row.parameter_name) end
        if row.transitive ~= true and trim(row.value) ~= "" and row.invalid ~= true then
            table.insert(parameters, { name = row.parameter_name, value = row.value })
        end
    end

    return {
        requirements = out,
        values = values,
        parameters = parameters,
        missing = missing,
        count = #out,
    }, nil
end

function Planner:plan_install(args)
    args = args or {}
    local planned_args, dest_err = self:resolve_dependency_destination_args(args)
    if not planned_args then return nil, dest_err end

    local entry, entry_err = M.build_dependency_entry(planned_args)
    if not entry then return nil, entry_err end
    local data = entry.data :: DependencyEntryData

    local graph, graph_err = self:resolve_install_graph(data.component, data.version, {
        max_depth = planned_args.max_depth,
        max_modules = planned_args.max_modules,
    })
    if not graph then return nil, graph_err end

    local target_err = validate_parameter_targets(graph, data.parameters or {})
    if target_err then return nil, target_err end

    local constraint_err = self:validate_graph_constraints(graph, data.component)
    if constraint_err then return nil, constraint_err end

    local req_plan, req_err = self:plan_requirements(graph, data.parameters or {})
    if not req_plan then return nil, req_err end

    return {
        dependency = M.dependency_summary(entry),
        graph = graph,
        module_count = #graph,
        requirements = req_plan.requirements,
        requirement_count = req_plan.count,
        missing_requirements = req_plan.missing,
        parameter_values = req_plan.values,
        recommended_parameters = req_plan.parameters,
        migration_policy = planned_args.migration_policy or (planned_args.run_migrations == true and "up" or "none"),
        install_payload = {
            id = entry.id,
            namespace = M.dependency_summary(entry).namespace,
            component = data.component,
            version = data.version,
            parameters = req_plan.parameters,
            migration_policy = planned_args.migration_policy or (planned_args.run_migrations == true and "up" or "none"),
        },
    }, nil
end

function M.plan_install(args)
    return M.new():plan_install(args)
end

return M
