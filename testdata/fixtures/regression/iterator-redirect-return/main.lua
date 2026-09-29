-- Registry source excerpt from userspace.dataflow.node.parallel:iterator_test.
local test = require("test")
local consts = require("consts")
local iterator = require("iterator")
local config = {
    data_targets = {
        { data_type = consts.DATA_TYPE.NODE_OUTPUT },
        { data_type = consts.DATA_TYPE.NODE_OUTPUT },
    },
    error_targets = {
        { data_type = consts.DATA_TYPE.NODE_OUTPUT, key = "primary-error" },
        { data_type = consts.DATA_TYPE.NODE_OUTPUT, key = "audit-error" },
    },
}
local redirected = iterator.redirect_terminals_to_parent(
    config, "parallel-parent", 3, "source-node", "attempt-1"
)
test.eq(redirected.data_targets[1].key, "source-node:terminal:1")
test.eq(redirected.data_targets[2].key, "source-node:terminal:2")
test.eq(redirected.error_targets[1].key, "source-node:terminal:1")
test.eq(redirected.error_targets[2].key, "source-node:terminal:2")
