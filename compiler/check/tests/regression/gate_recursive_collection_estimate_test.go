package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

const recursiveCollectionEstimate = `
type Edge = {source_id: string, target_id: string, edge_type: string, metadata?: unknown}
local function run(edges: {Edge}, depth: number)
 local function traverse(id, dir, current_depth, visited)
  if current_depth > depth then return {outgoing = {}, incoming = {}} end
  visited = visited or {}
  if visited[id] then return {outgoing = {}, incoming = {}} end
  visited[id] = true
  local result = {outgoing = {}, incoming = {}}
  for _, edge in ipairs(edges) do
   if edge.source_id == id and (dir == "outgoing" or dir == "both") then
    local child = {target_id = edge.target_id, edge_type = edge.edge_type, metadata = edge.metadata or {}}
    if current_depth < depth then
     local sub = traverse(edge.target_id, "outgoing", current_depth + 1, visited)
     if sub.outgoing and #sub.outgoing > 0 then child.children = sub.outgoing end
    end
    table.insert(result.outgoing, child)
   end
   if edge.target_id == id and (dir == "incoming" or dir == "both") then
    local parent = {source_id = edge.source_id, edge_type = edge.edge_type, metadata = edge.metadata or {}}
    if current_depth < depth then
     local sub = traverse(edge.source_id, "incoming", current_depth + 1, visited)
     if sub.incoming and #sub.incoming > 0 then parent.parents = sub.incoming end
    end
    table.insert(result.incoming, parent)
   end
  end
  return result
 end
 local result = traverse("root", "both", 1, {})
 for _, child in ipairs(result.outgoing) do
  local id: string = child.target_id
  if child.children then
   for _, grandchild in ipairs(child.children) do print(grandchild.target_id) end
  end
 end
 return result
end
return run`

func TestRecursiveCollectionEstimateConverges(t *testing.T) {
	for _, strict := range []bool{false, true} {
		result := testutil.Check(recursiveCollectionEstimate, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
		for _, diagnostic := range result.Diagnostics {
			t.Errorf("strict=%v: %v", strict, diagnostic)
		}
	}
}

func TestRecursiveCollectionEstimateRejectsWrongLeafType(t *testing.T) {
	checkBothModes(t, strings.Replace(recursiveCollectionEstimate, "local id: string", "local id: number", 1), "cannot assign string to number")
}
