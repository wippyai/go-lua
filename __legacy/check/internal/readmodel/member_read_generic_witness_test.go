package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
)

func TestForEachMissingMemberReadUsesGenericWitnessVariantOriginAsBroadReceiver(t *testing.T) {
	stmts := parseChunk(t, `
type Type<T> = { decode: (any) -> T }
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

local function decode<T>(data: string, witness: Type<T>): T
	return witness.decode(data)
end

local function tree_type(): Type<TreeNode>
	return {
		decode = function(raw: any): TreeNode
			return { kind = "text", value = tostring(raw) }
		end,
	}
end

local tree = decode("{}", tree_type())
if tree.kind == "text" then
	local children = tree.children
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var reads []MissingMemberRead
	root := checked.RootResult()
	New(root).ForEachMissingMemberRead(func(read MissingMemberRead) bool {
		reads = append(reads, read)
		return true
	})
	if len(reads) != 1 {
		t.Fatalf("missing member reads = %#v, want one generic-witness union-arm miss", reads)
	}
	if reads[0].ReadLabel != "tree.children" || reads[0].MemberName != "children" {
		t.Fatalf("missing member read = %#v, want tree.children", reads[0])
	}
}
