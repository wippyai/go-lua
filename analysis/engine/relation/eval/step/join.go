package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeJoin interprets the oriented two-child correspondence. Each pair
// of sealed cofiber ranges is passed to the tuple kernel in authored
// left-then-right order. Geometry is the exact mounted cofiber authority for
// scope conjunction; no reader is fabricated merely to perform scope algebra.
func (session Session) executeJoin(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Join()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children := node.Children()
	if len(children) != 2 {
		return nodeValue{}, false
	}
	left, leftOK := session.executeNode(children[0])
	right, rightOK := session.executeNode(children[1])
	if !leftOK || !rightOK || !left.available() || !right.available() || !relationKind(left.kind) || !relationKind(right.kind) {
		return nodeValue{}, false
	}
	outputs := make([]tuple.Batch, 0)
	for _, leftBatch := range left.batches {
		for _, rightBatch := range right.batches {
			value, executeOK := join.Join(binding, session.mounted, session.geometry, leftBatch, rightBatch)
			if !executeOK || !value.Available() {
				return nodeValue{}, false
			}
			outputs = append(outputs, value)
		}
	}
	return relationNode(node.Digest(), algebra.KindJoin, outputs)
}
