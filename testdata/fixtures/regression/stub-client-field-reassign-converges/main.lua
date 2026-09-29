type Map = {[string]: any}

local runs = require("runs")

local function it(name: string, fn: () -> ())
    fn()
end

local function describe(name: string, fn: () -> ())
    fn()
end

local function stub_client(sink: Map): (any?, string?)
    return {
        create_workflow = function(_self: any, commands: any, options: any): (any?, string?)
            sink.created = sink.created + 1
            return tostring((options :: Map).dataflow_id or "df"), nil
        end,
        start = function(_self: any, dataflow_id: any, options: any): (any?, string?)
            return dataflow_id, nil
        end,
    }, nil
end

describe("runs", function()
    it("stubs", function()
        local sink: Map = { created = 0 }
        runs._client_new = function() return stub_client(sink) end
        runs.launch("a")
    end)

    it("passivates", function()
        local created = 0
        runs._client_new = function()
            return {
                create_workflow = function(_self: any, _commands: any, options: Map)
                    created = created + 1
                    return tostring(options.dataflow_id), nil
                end,
                execute = function(_self: any, dataflow_id: string)
                    return { success = true, dataflow_id = dataflow_id, pending = true }, nil
                end,
            }, nil
        end
        runs.launch("b")
    end)

    it("refuses", function()
        local sink: Map = { created = 0 }
        runs._client_new = function()
            return {
                create_workflow = function(_self: any, _commands: any, options: Map): (any?, string?)
                    sink.created = sink.created + 1
                    return tostring(options.dataflow_id), nil
                end,
                start = function(_self: any, _dataflow_id: any, _options: any): (any?, string?)
                    return nil, "activation refused"
                end,
            }, nil
        end
        runs.launch("c")
        runs._client_new = function(): (any?, string?) return nil, "engine unavailable" end
        runs.launch("d")
    end)
end)
