package diagnostic

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// ExplanationGraph is a diagnostic-only provenance graph. It may describe which
// canonical facts were observed and why an explanation chose a message, but it
// deliberately carries no Summary value and has no mutation path back into the
// canonical solver.
type ExplanationGraph struct {
	nodes []ExplanationNode
	edges []ExplanationEdge
}

type ExplanationNodeID int

type ExplanationNodeKind string

const (
	NodeSourceExpression        ExplanationNodeKind = "source-expression"
	NodeCFGPoint                ExplanationNodeKind = "cfg-point"
	NodeAbstractValueFact       ExplanationNodeKind = "abstract-value-fact"
	NodeConditionFact           ExplanationNodeKind = "condition-fact"
	NodeCallTarget              ExplanationNodeKind = "call-target"
	NodeSummaryKey              ExplanationNodeKind = "summary-key"
	NodeReturnSlot              ExplanationNodeKind = "return-slot"
	NodeProvenanceRoute         ExplanationNodeKind = "provenance-route"
	NodeParameterObligation     ExplanationNodeKind = "parameter-obligation"
	NodePostcondition           ExplanationNodeKind = "postcondition"
	NodeMissingProof            ExplanationNodeKind = "missing-proof"
	NodeWideningUnknownBoundary ExplanationNodeKind = "widening-unknown-boundary"
)

type ExplanationEdgeKind string

const (
	EdgeDerivedByTransferRule          ExplanationEdgeKind = "derived-by-transfer-rule"
	EdgeJoinedFromPredecessor          ExplanationEdgeKind = "joined-from-predecessor"
	EdgeNarrowedByBranch               ExplanationEdgeKind = "narrowed-by-branch"
	EdgeProjectedFromSummary           ExplanationEdgeKind = "projected-from-summary"
	EdgeRebasedThroughCallBoundary     ExplanationEdgeKind = "rebased-through-call-boundary"
	EdgeWeakenedByWidening             ExplanationEdgeKind = "weakened-by-widening"
	EdgeLostBecauseTop                 ExplanationEdgeKind = "lost-because-top"
	EdgeMissingBecauseUnresolvedTarget ExplanationEdgeKind = "missing-because-unresolved-target"
	EdgeRejectedBecauseUnproved        ExplanationEdgeKind = "rejected-because-obligation-unproved"
)

type ExplanationNode struct {
	ID       ExplanationNodeID
	Kind     ExplanationNodeKind
	Label    string
	Point    cfg.Point
	HasPoint bool
	Key      summary.Key
	HasKey   bool
	Value    product.AbstractValue
}

type ExplanationEdge struct {
	From ExplanationNodeID
	To   ExplanationNodeID
	Kind ExplanationEdgeKind
}

func (g *ExplanationGraph) AddNode(node ExplanationNode) ExplanationNodeID {
	id := ExplanationNodeID(len(g.nodes) + 1)
	node.ID = id
	g.nodes = append(g.nodes, node)
	return id
}

func (g *ExplanationGraph) AddEdge(from, to ExplanationNodeID, kind ExplanationEdgeKind) {
	if from == 0 || to == 0 {
		return
	}
	g.edges = append(g.edges, ExplanationEdge{From: from, To: to, Kind: kind})
}

func (g ExplanationGraph) Nodes() []ExplanationNode {
	return append([]ExplanationNode(nil), g.nodes...)
}

func (g ExplanationGraph) Edges() []ExplanationEdge {
	return append([]ExplanationEdge(nil), g.edges...)
}

func (g ExplanationGraph) evidenceLines() []string {
	if len(g.nodes) == 0 {
		return nil
	}
	nodes := make(map[ExplanationNodeID]ExplanationNode, len(g.nodes))
	for _, node := range g.nodes {
		nodes[node.ID] = node
	}
	var lines []string
	seen := map[string]bool{}
	add := func(line string) {
		if line == "" || seen[line] {
			return
		}
		seen[line] = true
		lines = append(lines, line)
	}
	for _, edge := range g.edges {
		from := nodes[edge.From]
		to := nodes[edge.To]
		switch edge.Kind {
		case EdgeDerivedByTransferRule:
			switch {
			case from.Kind == NodeProvenanceRoute:
				add("canonical evidence: provenance route observation: " + from.Label)
			case to.Kind == NodeAbstractValueFact:
				add("canonical evidence: local transfer/point observation produced " + to.Label)
			default:
				add("canonical evidence: local transfer produced " + to.Label)
			}
		case EdgeNarrowedByBranch:
			add("canonical evidence: branch narrowing produced " + to.Label)
		case EdgeProjectedFromSummary:
			add("canonical evidence: summary projection supplied " + to.Label)
		case EdgeRebasedThroughCallBoundary:
			add("canonical evidence: call-boundary rebasing mapped " + from.Label + " to " + to.Label)
		case EdgeWeakenedByWidening:
			add("precision boundary: widening weakened " + to.Label)
		case EdgeLostBecauseTop:
			add("precision boundary: top/unknown boundary lost " + to.Label)
		case EdgeMissingBecauseUnresolvedTarget:
			add("missing proof: unresolved call target prevented " + to.Label)
		case EdgeRejectedBecauseUnproved:
			add("missing proof: checker could not prove " + to.Label)
		}
	}
	for _, node := range g.nodes {
		switch node.Kind {
		case NodeMissingProof:
			add("missing proof: checker could not prove " + node.Label)
		case NodeWideningUnknownBoundary:
			add("precision boundary: " + node.Label)
		}
	}
	return lines
}

func SummaryKeyNode(key summary.Key, label string) ExplanationNode {
	return ExplanationNode{Kind: NodeSummaryKey, Label: label, Key: key, HasKey: true}
}

func PointNode(point cfg.Point, label string) ExplanationNode {
	return ExplanationNode{Kind: NodeCFGPoint, Label: label, Point: point, HasPoint: true}
}

func ValueFactNode(value product.AbstractValue, label string) ExplanationNode {
	return ExplanationNode{Kind: NodeAbstractValueFact, Label: label, Value: value}
}
