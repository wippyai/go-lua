package canonical_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestCanonicalIndexPresenceFillIfAbsentKeepsExactMemberPrecision(t *testing.T) {
	src := `
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

if not messages["root"] then
    messages["root"] = {
        _topic = "installed",
        topic = function(self: Message): string
            return self._topic
        end,
    }
end

local installed: string = messages["root"]:topic()
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		logIndexPresenceFacts(t, res)
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
}

func TestCanonicalSingleFixpointBranchLocalStaticIndexInstallIsOptionalAtUse(t *testing.T) {
	src := `
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}
local function dynamic_cond(): boolean
    return math.random() > 0.5
end
local cond = dynamic_cond()
if cond then
    messages["root"] = {
        _topic = "a",
        topic = function(self: Message): string
            return self._topic
        end,
    }
end

local topic: string = messages["root"]:topic()
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if res == nil || res.Session == nil || res.Session.RootResult == nil || res.Session.RootResult.Graph == nil {
		t.Fatal("missing canonical root result")
	}
	root := res.Session.RootResult
	bindings := root.Graph.Bindings()
	if bindings == nil {
		t.Fatal("missing graph bindings")
	}
	syms := bindings.SymbolsByName("messages")
	if len(syms) == 0 {
		t.Fatal("missing messages symbol")
	}
	pathFacts, ok := root.Facts.(flow.PathFacts)
	if !ok {
		t.Fatal("canonical facts do not expose path facts")
	}
	path := constraint.NewPath(syms[0], "messages").IndexStr("root")
	var sawTopicCall bool
	root.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil || info.Call.Method != "topic" {
			return
		}
		sawTopicCall = true
		tv := pathFacts.RefinedPathAt(p, path)
		if tv.State != flow.StateResolved || tv.Type == nil {
			t.Fatalf("messages[\"root\"] at topic call is unresolved: %#v", tv)
		}
		if !unwrap.IsOptionalLike(tv.Type) {
			logIndexPresenceFacts(t, res)
			for _, pred := range root.Graph.Predecessors(p) {
				logIndexPresencePoint(t, root, pred, path)
				for _, predPred := range root.Graph.Predecessors(pred) {
					logIndexPresencePoint(t, root, predPred, path)
				}
			}
			t.Fatalf("messages[\"root\"] at branch-local topic call = %v, want optional after one-branch install; call point=%v preds=%s", tv.Type, p, describePointPredecessors(root.Graph, p))
		}
	})
	if !sawTopicCall {
		t.Fatal("did not find topic call site")
	}
}

func logIndexPresencePoint(t *testing.T, root *api.FuncResult, p cfg.Point, path constraint.Path) {
	t.Helper()
	if root == nil || root.Graph == nil {
		return
	}
	pathFacts, ok := root.Facts.(flow.PathFacts)
	if !ok {
		return
	}
	postFacts, _ := root.Facts.(interface {
		PostRefinedPathAt(cfg.Point, constraint.Path) flow.TypedValue
	})
	tv := pathFacts.RefinedPathAt(p, path)
	var post flow.TypedValue
	if postFacts != nil {
		post = postFacts.PostRefinedPathAt(p, path)
	}
	t.Logf("messages[\"root\"] explicit point %v (%s/%T): in=%v post=%v preds=%s succs=%s", p, describeNodeKind(root.Graph.Node(p)), root.Graph.Info(p), tv.Type, post.Type, describePointPredecessors(root.Graph, p), describePointSuccessors(root.Graph, p))
}

func logIndexPresenceFacts(t *testing.T, res *testutil.Result) {
	t.Helper()
	if res == nil || res.Session == nil || res.Session.RootResult == nil || res.Session.RootResult.Graph == nil {
		return
	}
	root := res.Session.RootResult
	bindings := root.Graph.Bindings()
	if bindings == nil {
		return
	}
	syms := bindings.SymbolsByName("messages")
	if len(syms) == 0 {
		return
	}
	path := constraint.NewPath(syms[0], "messages").IndexStr("root")
	pathFacts, ok := root.Facts.(flow.PathFacts)
	if !ok {
		return
	}
	postFacts, _ := root.Facts.(interface {
		PostRefinedPathAt(cfg.Point, constraint.Path) flow.TypedValue
	})
	root.Graph.EachNode(func(p cfg.Point, info cfg.NodeInfo) {
		tv := pathFacts.RefinedPathAt(p, path)
		var post flow.TypedValue
		if postFacts != nil {
			post = postFacts.PostRefinedPathAt(p, path)
		}
		if (tv.State == flow.StateResolved && tv.Type != nil) || (post.State == flow.StateResolved && post.Type != nil) {
			t.Logf("messages[\"root\"] at point %v (%s/%T): in=%v post=%v preds=%s", p, describeNodeKind(root.Graph.Node(p)), info, tv.Type, post.Type, describePointPredecessors(root.Graph, p))
		}
	})
}

func describePointPredecessors(g *cfg.Graph, p cfg.Point) string {
	if g == nil {
		return "<nil graph>"
	}
	preds := g.Predecessors(p)
	if len(preds) == 0 {
		return "<none>"
	}
	out := ""
	for i, pred := range preds {
		if i > 0 {
			out += "; "
		}
		out += fmt.Sprintf("%v=%s/%s", pred, describeNodeKind(g.Node(pred)), describeNodeInfo(g.Info(pred)))
	}
	return out
}

func describePointSuccessors(g *cfg.Graph, p cfg.Point) string {
	if g == nil {
		return "<nil graph>"
	}
	succs := g.Successors(p)
	if len(succs) == 0 {
		return "<none>"
	}
	out := ""
	for i, succ := range succs {
		if i > 0 {
			out += "; "
		}
		cond, known := g.EdgeCond(p, succ)
		edge := "?"
		if known {
			edge = fmt.Sprintf("%v", cond)
		}
		out += fmt.Sprintf("%v[%s]=%s/%s", succ, edge, describeNodeKind(g.Node(succ)), describeNodeInfo(g.Info(succ)))
	}
	return out
}

func describeNodeKind(n *cfg.Node) string {
	if n == nil {
		return "<nil node>"
	}
	switch n.Kind {
	case cfg.NodeEntry:
		return "entry"
	case cfg.NodeExit:
		return "exit"
	case cfg.NodeAssign:
		return "assign"
	case cfg.NodeCall:
		return "call"
	case cfg.NodeBranch:
		return "branch"
	case cfg.NodeJoin:
		return "join"
	case cfg.NodeReturn:
		return "return"
	case cfg.NodeScopeEnter:
		return "scope-enter"
	case cfg.NodeScopeExit:
		return "scope-exit"
	case cfg.NodeTypeDef:
		return "type-def"
	default:
		return fmt.Sprintf("kind-%d", n.Kind)
	}
}

func describeNodeInfo(info cfg.NodeInfo) string {
	switch n := info.(type) {
	case *cfg.AssignInfo:
		return fmt.Sprintf("assign(targets=%d,sources=%d)", len(n.Targets), len(n.Sources))
	case *cfg.BranchInfo:
		return "branch(" + n.CondVar + ")"
	case *cfg.CallInfo:
		if n.Call != nil {
			return "call(" + n.Call.Method + ")"
		}
		return "call"
	case *cfg.ReturnInfo:
		return "return"
	default:
		if info == nil {
			return "<no node info>"
		}
		return "node"
	}
}
