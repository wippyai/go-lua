-- From app:migration_lifecycle_test in crm/test.
local function with_db(fn)
    local db: any = {}
    local ok, result = pcall(fn, db)
    if not ok then error(result) end
    return result
end

local function table_exists(name: string)
    return with_db(function(db)
        return name
    end)
end

local function projection_state(): table
    return with_db(function(db)
        local row = { body = "saved", state = "valid" }
        return row
    end)
end

-- A protected call's second return is the error object when it fails.
local function unguarded(fn)
    local ok, result = pcall(fn)
    return result
end
local function a() return unguarded(function() return "text" end) end
local function b(): table
    return unguarded(function() return { value = 1 } end) -- expect-error: cannot return
end

-- A local error function returns normally, even though its name is familiar.
local error = function(_value) end
local function shadowed_error(fn)
    local ok, result = pcall(fn)
    if not ok then error(result) end
    return result
end
local function c() return shadowed_error(function() return "text" end) end
local function d(): table
    return shadowed_error(function() return { value = 2 } end) -- expect-error: cannot return
end


table_exists("projection")
return projection_state()
