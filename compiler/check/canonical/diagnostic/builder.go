package diagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// Builder renders diagnostic-only explanations from frozen canonical
// observation surfaces. It records why an existing diagnostic was emitted; it
// never asks the solver for new summaries, replays boundary facts, or creates
// substitute facts.
type Builder struct {
	observer observation.Projector
}

func NewBuilder(observer observation.Projector) Builder {
	return Builder{observer: observer}
}

func (b Builder) ExplainAssignmentMismatch(source ast.Expr, point cfg.Point, actual, expected typ.Type) string {
	var g ExplanationGraph
	expr := g.AddNode(ExplanationNode{Kind: NodeSourceExpression, Label: "assignment source"})
	fact := g.AddNode(ValueFactNode(product.FromType(actual), "observed source type "+formatType(actual)))
	missing := g.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "required assignment proof for " + formatType(expected)})
	g.AddEdge(expr, fact, EdgeDerivedByTransferRule)
	g.AddEdge(fact, missing, EdgeRejectedBecauseUnproved)
	b.addPathEvidence(&g, point, source, fact)
	b.addUnknownBoundary(&g, fact, actual)
	return b.render(g, point, source, []string{
		"source expression was observed as " + formatType(actual),
		"assignment target requires " + formatType(expected),
		"no canonical assignment-source proof made the observed value satisfy the target type",
	})
}

func (b Builder) ExplainCallArgumentMismatch(info *cfg.CallInfo, argIdx int, point cfg.Point, actual, expected typ.Type, obligation callobligation.Obligation) string {
	var arg ast.Expr
	if info != nil && argIdx > 0 && argIdx <= len(info.Args) {
		arg = info.Args[argIdx-1]
	}
	var g ExplanationGraph
	call := g.AddNode(ExplanationNode{Kind: NodeCallTarget, Label: "selected call contract"})
	argNode := g.AddNode(ExplanationNode{Kind: NodeSourceExpression, Label: "argument " + intString(argIdx)})
	fact := g.AddNode(ValueFactNode(product.FromType(actual), "observed argument type "+formatType(actual)))
	obligationNode := g.AddNode(ExplanationNode{Kind: NodeParameterObligation, Label: "parameter " + intString(argIdx) + " obligation " + formatType(expected)})
	missing := g.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "argument " + intString(argIdx) + " proof for " + formatType(expected)})
	g.AddEdge(call, obligationNode, EdgeProjectedFromSummary)
	g.AddEdge(obligationNode, argNode, EdgeRebasedThroughCallBoundary)
	g.AddEdge(argNode, fact, EdgeDerivedByTransferRule)
	g.AddEdge(fact, missing, EdgeRejectedBecauseUnproved)
	b.addPathEvidence(&g, point, arg, fact)
	b.addUnknownBoundary(&g, fact, actual)

	lines := []string{
		"argument " + intString(argIdx) + " was observed as " + formatType(actual),
		"the solved call contract requires " + formatType(expected),
	}
	if obligation.Informative() {
		lines = append(lines, "the obligation came from the canonical call boundary")
	}
	lines = append(lines,
		"no canonical argument proof or postcondition satisfied the selected obligation",
	)
	return b.render(g, point, arg, lines)
}

func (b Builder) ExplainOptionalIndex(expr *ast.AttrGetExpr, point cfg.Point, objType typ.Type) string {
	var g ExplanationGraph
	receiver := g.AddNode(ExplanationNode{Kind: NodeSourceExpression, Label: "indexed receiver"})
	fact := g.AddNode(ValueFactNode(product.FromType(objType), "receiver type "+formatType(objType)))
	missing := g.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "non-nil receiver proof"})
	g.AddEdge(receiver, fact, EdgeDerivedByTransferRule)
	g.AddEdge(fact, missing, EdgeRejectedBecauseUnproved)
	b.addUnknownBoundary(&g, fact, objType)

	var obj ast.Expr
	if expr != nil {
		obj = expr.Object
	}
	b.addPathEvidence(&g, point, obj, fact)
	return b.render(g, point, obj, []string{
		"receiver was observed as " + formatType(objType),
		"indexing requires a non-nil container at this point",
		"no canonical branch or product proof made the receiver definitely present",
	})
}

func (b Builder) ExplainIndexFailure(expr *ast.AttrGetExpr, point cfg.Point, objType, keyType typ.Type) string {
	var g ExplanationGraph
	receiver := g.AddNode(ExplanationNode{Kind: NodeSourceExpression, Label: "indexed receiver"})
	fact := g.AddNode(ValueFactNode(product.FromType(objType), "receiver type "+formatType(objType)))
	missing := g.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "runtime index/key read proof"})
	g.AddEdge(receiver, fact, EdgeDerivedByTransferRule)
	g.AddEdge(fact, missing, EdgeRejectedBecauseUnproved)
	b.addUnknownBoundary(&g, fact, objType)

	var obj ast.Expr
	if expr != nil {
		obj = expr.Object
	}
	b.addPathEvidence(&g, point, obj, fact)
	lines := []string{
		"receiver was observed as " + formatType(objType),
		"index key was observed as " + formatType(keyType),
		"no canonical indexed-read proof or runtime index projection proved a readable value",
	}
	return b.render(g, point, obj, lines)
}

func (b Builder) render(g ExplanationGraph, point cfg.Point, expr ast.Expr, lines []string) string {
	if point != 0 {
		p := g.AddNode(PointNode(point, "diagnostic point "+intString(int(point))))
		if len(g.nodes) > 0 {
			g.AddEdge(p, g.nodes[0].ID, EdgeJoinedFromPredecessor)
		}
	}
	if expr != nil && expr.Line() > 0 {
		lines = append([]string{"at line " + intString(expr.Line()) + ", column " + intString(expr.Column())}, lines...)
	}
	lines = append(lines, g.evidenceLines()...)
	return renderLines(lines)
}

func (b Builder) addPathEvidence(g *ExplanationGraph, point cfg.Point, expr ast.Expr, observed ExplanationNodeID) {
	path := b.observer.PathOfExpr(expr, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return
	}
	routes := b.observer.ProvenanceRoutesAt(point, path)
	if len(routes) == 0 {
		if query, ok := flow.AppendElementFieldRouteQueryForPath(path); ok {
			if appendRoutes := b.observer.AppendElementFieldSourceRoutesAt(point, query); len(appendRoutes) > 0 {
				route := g.AddNode(ExplanationNode{Kind: NodeProvenanceRoute, Label: appendRoutes[0].EvidenceLabel(path)})
				g.AddEdge(route, observed, EdgeDerivedByTransferRule)
				return
			}
		}
		pv := b.observer.ProductValueAtPath(point, path)
		if pv.State == flow.StateResolved && !pv.Value.IsZero() {
			pointFact := g.AddNode(ValueFactNode(pv.Value, "point-state product evidence for "+path.String()))
			g.AddEdge(pointFact, observed, EdgeDerivedByTransferRule)
			return
		}
		missing := g.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "provenance route or product evidence for " + path.String()})
		g.AddEdge(observed, missing, EdgeRejectedBecauseUnproved)
		return
	}
	route := g.AddNode(ExplanationNode{Kind: NodeProvenanceRoute, Label: routes[0].EvidenceLabel(path)})
	g.AddEdge(route, observed, EdgeDerivedByTransferRule)
}

func (b Builder) addUnknownBoundary(g *ExplanationGraph, fact ExplanationNodeID, t typ.Type) {
	if !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t) {
		return
	}
	label := "observed fact is " + formatType(t) + ", so stronger precision was not available from canonical evidence"
	node := g.AddNode(ExplanationNode{Kind: NodeWideningUnknownBoundary, Label: label})
	g.AddEdge(fact, node, EdgeLostBecauseTop)
}

func renderLines(lines []string) string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func formatType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return typ.FormatShort(t)
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
