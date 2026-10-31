-- From app:association_pull_test and hubspot source/association_pull.lua.
type Conn = { component_id: string? }
type ClientResult = { success: boolean, data: any?, error: string?, status_code: integer? }
type Transport = {
    connect: (string?) -> (Conn?, string?),
    list_objects: (Conn, string, any) -> ClientResult,
    batch_read_associations: (Conn, string, string, { any }) -> any,
}
type Deps = { transport: Transport? }

local function pull(deps: Deps?): boolean
    return deps ~= nil
end

local function fake_transport(): any
    local fake: any = { lists = {}, batches = {} }
    function fake.connect(component_id: string?): (any, string?)
        return { component_id = component_id }, nil
    end
    function fake.list_objects(_conn: any, object_type: string, query: any): any
        return { success = true, data = { results = {} } }
    end
    function fake.batch_read_associations(_conn: any, from_type: string, to_type: string, inputs: any): any
        return { success = true, data = { results = {} } }
    end
    return fake
end

local fake = fake_transport()
function fake.list_objects(_conn: any, _object_type: string, _query: any): any
    return { success = true, data = { results = { { id = 10 } } } }
end
function fake.batch_read_associations(_conn: any, _from: string, _to: string, _inputs: any): any
    return { success = true, data = { results = {} } }
end
local first = pull({ transport = fake })

local partial = fake_transport()
function partial.batch_read_associations(_conn: any, _from: string, _to: string, _inputs: any): any
    return { success = true, status_code = 207, data = { results = {} } }
end
local second = pull({ transport = partial })

local missing = fake_transport()
function missing.batch_read_associations(_conn: any, _from: string, _to: string, _inputs: any): any
    return { success = true, status_code = 207, data = { results = {} } }
end
local third = pull({ transport = missing })

return first and second and third
