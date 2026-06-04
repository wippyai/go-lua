package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCanonicalDynamicIndexWriteAdmitsLaterSameKeyRead(t *testing.T) {
	src := `
type Entry = { value: number }
type Store = { entries: {[string]: Entry} }

local function install(self: Store, id: string): number
    if id == "" then
        return 0
    end

    self.entries[id] = { value = 42 }
    local current = self.entries[id]
    return current.value
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected dynamic write to admit later same-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsFixtureShapeNestedRead(t *testing.T) {
	src := `
type DataTargets = {[string]: string}
type NodeConfig = { data_targets: DataTargets }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function ensure_node(self: Store, id: string): DataTargets
    if id == "" then
        return {}
    end

    self.nodes[id] = { config = { data_targets = {} } }
    local prev = self.nodes[id]
    local targets = prev.config.data_targets
    targets[id] = "present"
    return targets
end

return ensure_node
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected fixture-shaped dynamic write to admit nested same-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalGuardedStaticMemberInstallAdmitsLaterValueUse(t *testing.T) {
	src := `
local function install(id: string)
    local store = { nodes = {} }
    store.nodes[id] = { config = { kind = "first" } }
    local prev = store.nodes[id]
    if not prev.config.data_targets then
        prev.config.data_targets = {}
    end
    table.insert(prev.config.data_targets, "next")
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected guarded static member install to admit later value use, got diagnostics: %v", msgs)
	}
}

func TestCanonicalLoopCarriedGuardedStaticMemberInstallAdmitsLaterValueUse(t *testing.T) {
	src := `
type Op = { kind: string, id: string }

local function chain(ops: {Op})
    local store = { nodes = {} }
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = store.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        if op.kind == "agent" then
            store.nodes[node_id] = { config = { agent = op.id } }
        else
            store.nodes[node_id] = { config = { func_id = op.id } }
        end
        last_id = node_id
    end
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected loop-carried guarded static member install to admit later value use, got diagnostics: %v", msgs)
	}
}

func TestCanonicalUnannotatedMethodLoopCarriesPreviousDynamicKey(t *testing.T) {
	src := `
local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = self.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        if op.kind == "agent" then
            self.nodes[node_id] = { config = { agent = op.id } }
        else
            self.nodes[node_id] = { config = { func_id = op.id } }
        end
        last_id = node_id
    end
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected unannotated method loop to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsLoopCarriedPreviousKey(t *testing.T) {
	src := `
type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function chain(self: Store, ids: {string}): NodeConfig?
    local last_id = nil
    for _, id in ipairs(ids) do
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[id] = { config = { data_targets = {} } }
        last_id = id
    end
    return nil
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsBranchedLoopCarriedPreviousKey(t *testing.T) {
	src := `
type Op = { kind: string, id: string }
type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function chain(self: Store, ops: {Op}): NodeConfig?
    local last_id = nil
    for _, op in ipairs(ops) do
        if op.kind == "a" then
            local node_id = op.id .. "-a"
            if last_id then
                local prev = self.nodes[last_id]
                return prev.config
            end
            self.nodes[node_id] = { config = { data_targets = {} } }
            last_id = node_id
        elseif op.kind == "b" then
            local node_id = op.id .. "-b"
            if last_id then
                local prev = self.nodes[last_id]
                return prev.config
            end
            self.nodes[node_id] = { config = { data_targets = {} } }
            last_id = node_id
        end
    end
    return nil
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected branched loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueSamePathKey(t *testing.T) {
	src := `
local uuid = require("uuid")

type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function install(self: Store): NodeConfig
    local node_id = uuid.v7()
    self.nodes[node_id] = { config = { data_targets = {} } }
    local prev = self.nodes[node_id]
    return prev.config
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected path-backed dynamic write key to admit same-path read, got diagnostics: %v", msgs)
	}
}
