-- dataflow/src/node/func/func_child_output_test.lua: a mock reader built from
-- a state table records its queries into that table through closures; the
-- test reads the recorded fields afterwards.
local function make_reader(state)
    return {
        with_data = function(self: any, data_ids: any)
            state.queried_data_ids = data_ids
            return self
        end,
        with_nodes = function(self: any, node_ids: any)
            state.queried_nodes = node_ids
            state.queried_data_ids = nil
            return self
        end,
        all = function()
            state.output_query_count = state.output_query_count + 1
            return {}
        end
    }
end

local function make_state(output_available)
    return {
        commands = {},
        output_query_count = 0,
        output_available = output_available
    }
end

local state = make_state(function() return false end)
local reader = make_reader(state)
reader:with_data({ "child-output-1" })
reader:all()

local first = state.queried_data_ids
local nodes = state.queried_nodes
local count: integer = state.output_query_count
return { first, nodes, count }
