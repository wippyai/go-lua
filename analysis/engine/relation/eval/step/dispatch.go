package step

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// Evaluate redeems one dependency-owned physical root. Callers obtain entry
// from fixpoint.Queue.Entry(work); this package deliberately accepts the
// already-sealed entry instead of importing the queue, which keeps the queue
// -> evaluator composition acyclic and prevents evaluator-side scheduling.
//
// The switch is closed over the logical algebra. A kind whose typed kernel is
// not yet admitted refuses at this boundary; it never falls back to a legacy
// form, callback, or runtime plan reconstruction.
func (session Session) Evaluate(entry arrangement.ScheduleEntry) (Result, bool) {
	node, ok := session.redeem(entry)
	if !ok {
		return Result{}, false
	}
	value, ok := session.execute(entry, node)
	if !ok {
		return Result{}, false
	}
	return sealNodeResult(entry.Dependency(), entry.Expression(), value)
}

func (session Session) redeem(entry arrangement.ScheduleEntry) (arrangement.Node, bool) {
	if !session.Available() || !entry.Available() {
		return arrangement.Node{}, false
	}
	execution := session.root.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	canonical, ok := execution.Dependency(entry.Dependency())
	if !ok || !canonical.Available() || canonical.Dependency() != entry.Dependency() || canonical.Expression() != entry.Expression() || canonical.Component() != entry.Component() {
		return arrangement.Node{}, false
	}
	node := entry.Node()
	canonicalNode := canonical.Node()
	if !node.Available() || !canonicalNode.Available() || node.Digest() != canonicalNode.Digest() || node.Kind() != canonicalNode.Kind() {
		return arrangement.Node{}, false
	}
	return node, true
}

func (session Session) execute(entry arrangement.ScheduleEntry, node arrangement.Node) (nodeValue, bool) {
	if !session.Available() || !entry.Available() || !node.Available() {
		return nodeValue{}, false
	}
	// Each arm is intentionally named. Adding a new logical kind requires an
	// evaluator decision and a typed kernel; it cannot silently become an
	// untyped default path.
	switch node.Kind() {
	case algebra.KindInput:
		return session.executeInput(node)
	case algebra.KindSelect:
		return session.executeSelect(node)
	case algebra.KindProject:
		return session.executeProject(node)
	case algebra.KindColumnProject:
		return session.executeColumnProject(node)
	case algebra.KindJoin:
		return session.executeJoin(node)
	case algebra.KindMerge:
		return session.executeMerge(node)
	case algebra.KindGroup:
		return session.executeGroup(node)
	case algebra.KindComplete:
		return session.executeComplete(node)
	case algebra.KindExpand:
		return session.executeExpand(node)
	case algebra.KindApply:
		return session.executeApply(node)
	case algebra.KindPublish:
		return session.executePublish(entry, node)
	default:
		return nodeValue{}, false
	}
}

// executeNode recursively redeems a child from the already-sealed physical
// tree. Child nodes intentionally have no schedule identity; only the root
// work entry is named by the dependency schedule. This is a direct
// interpretation, not a result cache or an opportunity to rebuild a plan.
func (session Session) executeNode(node arrangement.Node) (nodeValue, bool) {
	if !session.Available() || !node.Available() {
		return nodeValue{}, false
	}
	switch node.Kind() {
	case algebra.KindInput:
		return session.executeInput(node)
	case algebra.KindSelect:
		return session.executeSelect(node)
	case algebra.KindProject:
		return session.executeProject(node)
	case algebra.KindColumnProject:
		return session.executeColumnProject(node)
	case algebra.KindJoin:
		return session.executeJoin(node)
	case algebra.KindMerge:
		return session.executeMerge(node)
	case algebra.KindGroup:
		return session.executeGroup(node)
	case algebra.KindComplete:
		return session.executeComplete(node)
	case algebra.KindExpand:
		return session.executeExpand(node)
	case algebra.KindApply:
		return session.executeApply(node)
	case algebra.KindPublish:
		return session.executePublish(arrangement.ScheduleEntry{}, node)
	default:
		return nodeValue{}, false
	}
}

func (session Session) children(node arrangement.Node) ([]nodeValue, bool) {
	if !session.Available() || !node.Available() {
		return nil, false
	}
	children := node.Children()
	if len(children) == 0 {
		return nil, false
	}
	values := make([]nodeValue, len(children))
	for index, child := range children {
		value, ok := session.executeNode(child)
		if !ok || !value.available() {
			return nil, false
		}
		values[index] = value
	}
	return values, true
}
