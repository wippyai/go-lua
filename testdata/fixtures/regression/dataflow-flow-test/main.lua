local test = require("test")
local flow = require("flow")
local compiler = require("compiler")

local function define_tests()
    describe("flow builder", function()
        it("compiles rolling parallel scheduling", function()
            local template = (flow.template() :: any):func("example.process")
            local builder = (flow.create() :: any)
                :with_input({ items = { "a", "b" } })
                :parallel({
                    source_array_key = "items",
                    batch_size = 2,
                    scheduling = "rolling",
                    template = template,
                })
            local result, err = compiler.compile(builder.operations, {})

            test.is_nil(err)
            local parallel_node = nil
            for _, node in pairs(result.graph.nodes) do
                if node.node_type == "userspace.dataflow.node.parallel:parallel" then
                    parallel_node = node
                    break
                end
            end
            parallel_node = test.not_nil(parallel_node)
            test.eq(parallel_node.config.batch_size, 2)
            test.eq(parallel_node.config.scheduling, "rolling")
        end)
    end)
end

return test.run_cases(define_tests)
