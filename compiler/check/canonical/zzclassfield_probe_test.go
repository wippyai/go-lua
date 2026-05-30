package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZClassFieldProbe drives the split-pattern OOP class (local methods = {};
// mt = {__index = methods}; instance literal + setmetatable) through the canonical
// flow. The method-receiver `self` must carry the instance's DATA fields, not just
// the prototype's method surface. Debug probe for class-instance data-field tracking.
func TestZZClassFieldProbe(t *testing.T) {
	cases := map[string]string{
		"split-pattern": `
local node = {}
local methods = {}
local mt = { __index = methods }

function node.new(node_id, dataflow_id)
    local instance = {
        node_id = node_id,
        dataflow_id = dataflow_id,
        _config = {},
        _queued_commands = {},
    }
    return setmetatable(instance, mt)
end

function methods:config()
    return self._config
end

function methods:ident()
    return self.node_id
end

function methods:enqueue(cmd)
    table.insert(self._queued_commands, cmd)
    return self
end

return node
`,
		"inline-literal-setmetatable": `
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = {},
        node_order = {},
        last_id = nil,
    }, flow_graph_mt)
end

function FlowGraph:add(op)
    table.insert(self.operations, op)
    self.last_id = op
    return self
end

function FlowGraph:order()
    return self.node_order
end

return FlowGraph
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("DIAG: %s", m)
			}
		})
	}
}
