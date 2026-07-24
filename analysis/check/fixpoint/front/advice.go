package front

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// adviceControlDiagnostics projects only finite WIR relations whose complete
// source topology is already admitted by the front.  It deliberately uses
// symbol/path identities and CFG reachability, never source spelling.
func adviceControlDiagnostics(body *wir.Body, graph cfg.Graph) []ControlDiagnostic {
	if body == nil || graph == nil {
		return nil
	}
	var out []ControlDiagnostic
	for i := 0; i < body.Len(); i++ {
		birth := body.Instr(i)
		if birth.Op != wir.OpMakeTable || birth.Dst.Kind != wir.OperandPath || len(body.TableEntries(birth.TableEntries)) != 0 {
			continue
		}
		root := body.Path(wir.PathRef(birth.Dst.Ref))
		if root.Symbol == 0 || len(root.Segments) != 0 {
			continue
		}
		writes := adviceStaticWrites(body, root)
		if len(writes) == 0 || adviceDynamicWrite(body, root) {
			continue
		}
		if item, ok := adviceSplitBirth(body, graph, birth, root, writes); ok {
			out = append(out, item)
		}
		if item, ok := adviceShape(body, graph, birth, root, writes); ok {
			out = append(out, item)
		}
	}
	out = append(out, adviceRedundantGuards(body, graph)...)
	out = append(out, adviceInvariantLoopReads(body, graph)...)
	return out
}

// advicePolicyDiagnostics publishes finite lexical lint facts from the same
// lowered body and CFG that authorize the existing advice family.  These facts
// remain optional at the lint policy boundary; this pass only records proofs
// that are complete in the source-owned graph.
func advicePolicyDiagnostics(body *wir.Body, graph cfg.Graph) []ControlDiagnostic {
	if body == nil || graph == nil || graphHasCycle(graph) {
		return nil
	}
	var out []ControlDiagnostic
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		path, ok := adviceLocalDeclaration(body, inst)
		if !ok {
			continue
		}
		if !adviceRootReadAfter(body, graph, path, inst.Point) {
			name := body.SymbolName(path.Symbol)
			if name == "" {
				continue
			}
			out = append(out, ControlDiagnostic{
				Key:      fmt.Sprintf("lint.unused.local/%d/%d", inst.TargetSpan.StartLine, inst.Point),
				Code:     "lint.unused.local",
				Message:  "local \"" + name + "\" is never read",
				Span:     inst.TargetSpan,
				Evidence: []ControlDiagnosticEvidence{{Span: inst.TargetSpan, Kind: "abstract fact", Trust: "proven", Message: "no read of local \"" + name + "\" was found in this scope"}},
				Labels:   []ControlDiagnosticLabel{{Span: inst.TargetSpan, Message: "unused local"}},
				Help:     "Remove it, use it, or rename it with a leading _ when intentionally unused.",
			})
		}
		if item, ok := adviceDeadLocalAssignment(body, graph, path, inst); ok {
			out = append(out, item)
		}
	}
	return out
}

func adviceLocalDeclaration(body *wir.Body, inst wir.Instruction) (pathdom.Path, bool) {
	if inst.Op != wir.OpAssign || inst.Assign != wir.AssignLocalDeclaration || inst.Dst.Kind != wir.OperandPath {
		return pathdom.Path{}, false
	}
	path := body.Path(wir.PathRef(inst.Dst.Ref))
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return pathdom.Path{}, false
	}
	kind, ok := body.SymbolKind(path.Symbol)
	return path, ok && kind == wir.SymbolLocal
}

func adviceRootReadAfter(body *wir.Body, graph cfg.Graph, root pathdom.Path, after cfg.Point) bool {
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Point != after && graphReachable(graph, after, inst.Point) && instructionReadsRoot(body, inst, root) {
			return true
		}
	}
	return false
}

// adviceDeadLocalAssignment proves that every path after a local declaration
// reaches a replacement write or return before a read of that exact symbol.
// An open exit, loop, call, or read leaves the fact unpublished.
func adviceDeadLocalAssignment(body *wir.Body, graph cfg.Graph, root pathdom.Path, declaration wir.Instruction) (ControlDiagnostic, bool) {
	type terminal struct {
		write      wir.Span
		returnSpan wir.Span
	}
	var terminals []terminal
	seen := make(map[cfg.Point]bool)
	queue := append([]cfg.Point(nil), graph.Successors(declaration.Point)...)
	for len(queue) != 0 {
		point := queue[0]
		queue = queue[1:]
		if seen[point] {
			continue
		}
		seen[point] = true
		if point == graph.Exit() {
			return ControlDiagnostic{}, false
		}
		var stop *terminal
		for i := 0; i < body.Len(); i++ {
			inst := body.Instr(i)
			if inst.Point != point {
				continue
			}
			if instructionReadsRoot(body, inst, root) {
				return ControlDiagnostic{}, false
			}
			if inst.WritesAssignmentPoint() && inst.Dst.Kind == wir.OperandPath && adviceSamePath(body.Path(wir.PathRef(inst.Dst.Ref)), root) {
				candidate := terminal{write: inst.TargetSpan}
				stop = &candidate
				break
			}
			if inst.Op == wir.OpReturn {
				candidate := terminal{returnSpan: inst.ExprSpan}
				stop = &candidate
				break
			}
			if inst.Op == wir.OpCall {
				return ControlDiagnostic{}, false
			}
		}
		if stop != nil {
			terminals = append(terminals, *stop)
			continue
		}
		queue = append(queue, graph.Successors(point)...)
	}
	if len(terminals) == 0 {
		return ControlDiagnostic{}, false
	}
	name := body.SymbolName(root.Symbol)
	if name == "" {
		return ControlDiagnostic{}, false
	}
	var overwrite, exit wir.Span
	for _, item := range terminals {
		if !overwrite.Valid() && item.write.Valid() {
			overwrite = item.write
		}
		if !exit.Valid() && item.returnSpan.Valid() {
			exit = item.returnSpan
		}
	}
	evidence := make([]ControlDiagnosticEvidence, 0, 2)
	labels := []ControlDiagnosticLabel{{Span: declaration.TargetSpan, Message: "dead assignment"}}
	message := "assignment to \"" + name + "\" is overwritten before it is read"
	help := "Remove this assignment, or read `" + name + "` before the later overwrite."
	if exit.Valid() {
		message = "assignment to \"" + name + "\" is discarded before it is read"
		help = "Remove this assignment, or read `" + name + "` before every later overwrite or exit."
		evidence = append(evidence, ControlDiagnosticEvidence{Span: exit, Kind: "abstract fact", Trust: "proven", Message: "control can leave before \"" + name + "\" is read"})
		labels = append(labels, ControlDiagnosticLabel{Span: exit, Message: "exit before read"})
	}
	if overwrite.Valid() {
		evidence = append(evidence, ControlDiagnosticEvidence{Span: overwrite, Kind: "abstract fact", Trust: "proven", Message: "later assignment replaces \"" + name + "\" before the earlier value is read"})
		labels = append(labels, ControlDiagnosticLabel{Span: overwrite, Message: "overwriting assignment"})
	}
	return ControlDiagnostic{Key: fmt.Sprintf("lint.dead.assignment/%d/%d", declaration.TargetSpan.StartLine, declaration.Point), Code: "lint.dead.assignment", Message: message, Span: declaration.TargetSpan, Evidence: evidence, Labels: labels, Help: help}, true
}

type adviceWrite struct {
	inst      wir.Instruction
	path      string
	literal   string
	isLiteral bool
}

func adviceStaticWrites(body *wir.Body, root pathdom.Path) []adviceWrite {
	rootName := root.String()
	var out []adviceWrite
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpStaticMemberWrite || inst.Dst.Kind != wir.OperandPath {
			continue
		}
		p := body.Path(wir.PathRef(inst.Dst.Ref))
		if len(p.Segments) != 1 || p.Root != rootName || p.Segments[0].Name == "" {
			continue
		}
		item := adviceWrite{inst: inst, path: p.String()}
		if inst.A.Kind == wir.OperandConst {
			c := body.Const(wir.ConstRef(inst.A.Ref))
			item.literal, item.isLiteral = c.Str, c.Kind == wir.ConstString
		}
		out = append(out, item)
	}
	return out
}

func adviceDynamicWrite(body *wir.Body, root pathdom.Path) bool {
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpDynamicIndexWrite && inst.Dst.Kind == wir.OperandPath && body.Path(wir.PathRef(inst.Dst.Ref)).Root == root.String() {
			return true
		}
	}
	return false
}

func adviceSplitBirth(body *wir.Body, graph cfg.Graph, birth wir.Instruction, root pathdom.Path, writes []adviceWrite) (ControlDiagnostic, bool) {
	for _, tag := range writes {
		if !tag.isLiteral {
			continue
		}
		for i := 0; i < body.Len(); i++ {
			use := body.Instr(i)
			if use.Op != wir.OpBranch || use.Check == 0 || !graphReachable(graph, tag.inst.Point, use.Point) {
				continue
			}
			check := body.Check(use.Check)
			if check.Kind != wir.CheckLiteralEqual || check.Path.String() != tag.path {
				continue
			}
			var payload *adviceWrite
			for j := range writes {
				if writes[j].path != tag.path && graphReachable(graph, birth.Point, writes[j].inst.Point) {
					payload = &writes[j]
					break
				}
			}
			if payload == nil {
				continue
			}
			return ControlDiagnostic{Key: "advice.split_birth_discriminant", Code: "advice.split_birth_discriminant", Message: tag.path + " is assigned apart from its payload", Span: tag.inst.TargetSpan,
				Evidence: []ControlDiagnosticEvidence{{Span: birth.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: root.String() + " is born as a table here"}, {Span: tag.inst.TargetSpan, Kind: "abstract fact", Trust: "proven", Message: tag.path + " is assigned literal \"" + tag.literal + "\" here"}, {Span: payload.inst.TargetSpan, Kind: "abstract fact", Trust: "proven", Message: payload.path + " is assigned separately"}, {Span: use.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: tag.path + " is used as a discriminant here"}},
				Labels:   []ControlDiagnosticLabel{{Span: tag.inst.TargetSpan, Message: "tag write"}, {Span: birth.ExprSpan, Message: "table birth"}, {Span: payload.inst.TargetSpan, Message: "payload write"}, {Span: use.ExprSpan, Message: "discriminant use"}}, Help: "Construct the variant in one table literal so the tag and payload are born atomically."}, true
		}
	}
	return ControlDiagnostic{}, false
}

func adviceShape(body *wir.Body, graph cfg.Graph, birth wir.Instruction, root pathdom.Path, writes []adviceWrite) (ControlDiagnostic, bool) {
	var use wir.Instruction
	found := false
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpReturn {
			for _, value := range body.Operands(candidate.List) {
				if value.Kind == wir.OperandPath && body.Path(wir.PathRef(value.Ref)).String() == root.String() {
					use = candidate
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}
	if !found {
		return ControlDiagnostic{}, false
	}
	if meta := body.ReturnValueMeta(use.ReturnValues); len(meta) != 0 && meta[0].Span.Valid() {
		use.ExprSpan = meta[0].Span
	}
	fields := make([]adviceWrite, 0, len(writes))
	for _, write := range writes {
		if graphReachable(graph, birth.Point, write.inst.Point) && graphReachable(graph, write.inst.Point, use.Point) {
			fields = append(fields, write)
		}
	}
	if len(fields) < 2 {
		return ControlDiagnostic{}, false
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].path < fields[j].path })
	evidence := []ControlDiagnosticEvidence{{Span: birth.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: root.String() + " is born as a table here"}}
	labels := []ControlDiagnosticLabel{{Span: use.ExprSpan, Message: "shape-relevant use"}, {Span: birth.ExprSpan, Message: "table birth"}}
	for _, field := range fields {
		evidence = append(evidence, ControlDiagnosticEvidence{Span: field.inst.TargetSpan, Kind: "abstract fact", Trust: "proven", Message: field.path + " is added only on some paths"})
		labels = append(labels, ControlDiagnosticLabel{Span: field.inst.TargetSpan, Message: "conditionally present field"})
	}
	evidence = append(evidence, ControlDiagnosticEvidence{Span: use.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: "StableShape is refused because " + root.String() + " has a non-uniform field set"}, ControlDiagnosticEvidence{Span: use.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: root.String() + " is used where a fixed shape matters"})
	return ControlDiagnostic{Key: "advice.shape.polymorphic", Code: "advice.shape.polymorphic", Message: root.String() + " has a path-dependent field shape", Span: use.ExprSpan, Evidence: evidence, Labels: labels, Help: "Construct all variants with one fixed-shape constructor (all fields present, absent ones nil/default)."}, true
}

func adviceRedundantGuards(body *wir.Body, graph cfg.Graph) []ControlDiagnostic {
	var out []ControlDiagnostic
	for i := 0; i < body.Len(); i++ {
		inner := body.Instr(i)
		if inner.Op != wir.OpBranch || inner.Check == 0 {
			continue
		}
		check := body.Check(inner.Check)
		for j := 0; j < body.Len(); j++ {
			if j == i {
				continue
			}
			outer := body.Instr(j)
			if outer.Op != wir.OpBranch || outer.Check == 0 {
				continue
			}
			prior := body.Check(outer.Check)
			implies := adviceImplies(prior, check) && adviceTrueEdgeDominates(graph, outer.Point, inner.Point) && !advicePathMutatedBetween(body, graph, outer.Point, inner.Point, check.Path)
			redundant, always := adviceRedundantNilGuard(body, graph, outer, prior, inner, check)
			if !implies && !redundant {
				continue
			}
			if implies {
				out = append(out, ControlDiagnostic{Key: fmt.Sprintf("advice.always_true_guard/%d/%d", inner.ExprSpan.StartLine, inner.Point), Code: "advice.always_true_guard", Message: "condition is proven always true", Span: inner.ExprSpan, Evidence: []ControlDiagnosticEvidence{{Span: inner.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: "condition is proven to be true on every reachable path"}}, Labels: []ControlDiagnosticLabel{{Span: inner.ExprSpan, Message: "constant guard"}}, Help: "Remove the guard or move the guarded code out of the branch."})
			}
			if redundant {
				message, help := "condition is always false here", "Remove this unreachable branch, or change the prior guard if this path should still run."
				if always {
					message, help = "condition is always true here", "Remove this repeated check, or move any needed work into the branch already guarded above."
				}
				current := check.Path.String() + " ~= nil"
				priorMessage := check.Path.String() + " is nil"
				if prior.Kind == wir.CheckNotNil {
					priorMessage = check.Path.String() + " is not nil"
				}
				out = append(out, ControlDiagnostic{Key: fmt.Sprintf("lint.condition.redundant/%d/%d", inner.ExprSpan.StartLine, inner.Point), Code: "lint.condition.redundant", Message: message, Span: inner.ExprSpan,
					Evidence: []ControlDiagnosticEvidence{{Span: inner.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: "current check: " + current}, {Span: outer.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: "prior guard established " + priorMessage}, {Span: inner.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: check.Path.String() + " is unchanged between the prior guard and this check"}},
					Labels:   []ControlDiagnosticLabel{{Span: inner.ExprSpan, Message: "current check"}, {Span: outer.ExprSpan, Message: "prior guard"}}, Help: help})
			}
			break
		}
	}
	return out
}

// adviceRedundantNilGuard admits a source-local lint fact only when the true
// edge of an earlier exact nil predicate dominates the later predicate and no
// write or opaque call can intervene.  The proof is entirely WIR/CFG-owned;
// it does not expose a child refinement to its caller.
func adviceRedundantNilGuard(body *wir.Body, graph cfg.Graph, outer wir.Instruction, prior wir.Check, inner wir.Instruction, current wir.Check) (bool, bool) {
	if current.Kind != wir.CheckNotNil || !adviceSamePath(prior.Path, current.Path) {
		return false, false
	}
	if prior.Kind != wir.CheckNotNil && prior.Kind != wir.CheckNil {
		return false, false
	}
	if !adviceTrueEdgeDominates(graph, outer.Point, inner.Point) || advicePathMutatedBetween(body, graph, outer.Point, inner.Point, current.Path) {
		return false, false
	}
	return true, prior.Kind == wir.CheckNotNil
}

func adviceTrueEdgeDominates(graph cfg.Graph, branch, target cfg.Point) bool {
	var trueSuccessors []cfg.Point
	for _, next := range graph.Successors(branch) {
		if truth, ok := graph.EdgeCond(branch, next); ok && truth {
			trueSuccessors = append(trueSuccessors, next)
		}
	}
	if len(trueSuccessors) != 1 || !graphReachable(graph, trueSuccessors[0], target) {
		return false
	}
	seen := map[cfg.Point]bool{graph.Entry(): true}
	queue := []cfg.Point{graph.Entry()}
	for len(queue) != 0 {
		point := queue[0]
		queue = queue[1:]
		for _, next := range graph.Successors(point) {
			if point == branch && next == trueSuccessors[0] {
				continue
			}
			if next == target {
				return false
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return true
}

func advicePathMutatedBetween(body *wir.Body, graph cfg.Graph, from, to cfg.Point, path pathdom.Path) bool {
	root := path.Root
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if !graphReachable(graph, from, inst.Point) || !graphReachable(graph, inst.Point, to) {
			continue
		}
		switch inst.Op {
		case wir.OpCall:
			return true
		case wir.OpAssign, wir.OpStaticMemberWrite:
			if inst.Dst.Kind == wir.OperandPath && adviceSamePath(body.Path(wir.PathRef(inst.Dst.Ref)), path) {
				return true
			}
		case wir.OpDynamicIndexWrite:
			if inst.Dst.Kind == wir.OperandPath && body.Path(wir.PathRef(inst.Dst.Ref)).Root == root {
				return true
			}
		}
	}
	return false
}

func adviceInvariantLoopReads(body *wir.Body, graph cfg.Graph) []ControlDiagnostic {
	var out []ControlDiagnostic
	for i := 0; i < body.Len(); i++ {
		read := body.Instr(i)
		if read.Op != wir.OpAssign || read.A.Kind != wir.OperandPath || !advicePointInCycle(graph, read.Point) {
			continue
		}
		path := body.Path(wir.PathRef(read.A.Ref))
		if path.Symbol == 0 || len(path.Segments) != 1 {
			continue
		}
		aliases := map[uint32]bool{uint32(path.Symbol): true}
		changed := true
		for changed {
			changed = false
			for j := 0; j < body.Len(); j++ {
				inst := body.Instr(j)
				if inst.Op == wir.OpAssign && inst.Dst.Kind == wir.OperandPath && inst.A.Kind == wir.OperandPath {
					dst, src := body.Path(wir.PathRef(inst.Dst.Ref)), body.Path(wir.PathRef(inst.A.Ref))
					if len(dst.Segments) == 0 && len(src.Segments) == 0 && aliases[uint32(src.Symbol)] && !aliases[uint32(dst.Symbol)] {
						aliases[uint32(dst.Symbol)] = true
						changed = true
					}
				}
			}
		}
		mutated := false
		for j := 0; j < body.Len(); j++ {
			inst := body.Instr(j)
			if inst.Op == wir.OpStaticMemberWrite && inst.Dst.Kind == wir.OperandPath {
				target := body.Path(wir.PathRef(inst.Dst.Ref))
				if len(target.Segments) != 0 && aliases[uint32(target.Symbol)] {
					mutated = true
					break
				}
			}
		}
		if mutated {
			continue
		}
		loop := adviceLoopHead(body, graph, read.Point)
		out = append(out, ControlDiagnostic{Key: "advice.invariant_loop_read", Code: "advice.invariant_loop_read", Message: path.String() + " is loop-invariant and can be hoisted", Span: read.ExprSpan, Evidence: []ControlDiagnosticEvidence{{Span: read.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: path.String() + " is not written by the loop body"}, {Span: read.ExprSpan, Kind: "abstract fact", Trust: "proven", Message: path.Root + " is non-nil on all loop paths"}}, Labels: []ControlDiagnosticLabel{{Span: read.ExprSpan, Message: "loop read"}, {Span: loop, Message: "loop head"}}, Help: "Read `" + path.String() + "` once before the loop when that makes the code clearer or cheaper."})
	}
	return out
}

func advicePointInCycle(graph cfg.Graph, point cfg.Point) bool {
	for _, next := range graph.Successors(point) {
		if graphReachable(graph, next, point) {
			return true
		}
	}
	for _, prior := range graph.Predecessors(point) {
		if graphReachable(graph, point, prior) {
			return true
		}
	}
	return false
}
func adviceLoopHead(body *wir.Body, graph cfg.Graph, point cfg.Point) wir.Span {
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpBranch && advicePointInCycle(graph, inst.Point) && graphReachable(graph, inst.Point, point) {
			return wir.Span{StartLine: inst.ExprSpan.StartLine, StartCol: 1, EndLine: inst.ExprSpan.EndLine, EndCol: inst.ExprSpan.EndCol}
		}
	}
	return wir.Span{}
}

func adviceImplies(prior, current wir.Check) bool {
	if prior.Kind == wir.CheckNotNil && current.Kind == wir.CheckNotNil {
		return adviceSamePath(prior.Path, current.Path) || (prior.Path.Root == current.Path.Root && prior.Path.Root != "")
	}
	if !adviceSamePath(prior.Path, current.Path) {
		return false
	}
	return prior.Kind == wir.CheckTypeEqual && current.Kind == wir.CheckTypeNot && prior.TypeName != "" && current.TypeName != "" && prior.TypeName != current.TypeName
}

func adviceSamePath(left, right pathdom.Path) bool {
	if left.Symbol != right.Symbol || left.Root != right.Root || len(left.Segments) != len(right.Segments) {
		return false
	}
	for i := range left.Segments {
		if left.Segments[i] != right.Segments[i] {
			return false
		}
	}
	return true
}
func graphReachable(graph cfg.Graph, from, to cfg.Point) bool {
	if from == to {
		return true
	}
	seen := map[cfg.Point]bool{from: true}
	queue := []cfg.Point{from}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, next := range graph.Successors(p) {
			if next == to {
				return true
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}
