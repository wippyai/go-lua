package cofiber

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

// lowerRegion is the only concrete Region-to-support lowering operation. It
// walks the owner-issued immutable DAG with an explicit stack, resolves every
// neutral atom through the sealed lookup, and publishes one support Mask.
// The state vector makes malformed cyclic transport fail closed even though a
// sealed Region normally proves acyclicity at its own boundary.
func lowerRegion(value region.Region, lookup Lookup, physicalByNeutral map[identity.ContentID]guard.Atom) (support.Mask, bool) {
	if !value.Available() || !lookup.Available() {
		return support.Mask{}, false
	}
	manager := lookup.manager()
	work := support.New(manager)
	if work == nil {
		return support.Mask{}, false
	}
	nodes := value.Nodes()
	root, rootOK := value.Root()
	if !rootOK {
		work.Discard()
		return support.Mask{}, false
	}
	values := make([]support.Mask, len(nodes)+2)
	values[0], values[1] = work.False(), work.True()
	if !values[0].Valid() || !values[1].Valid() {
		work.Discard()
		return support.Mask{}, false
	}
	if value.IsFalse() {
		if !work.Seal() {
			work.Discard()
			return support.Mask{}, false
		}
		return values[0], true
	}
	if value.IsTrue() {
		if !work.Seal() {
			work.Discard()
			return support.Mask{}, false
		}
		return values[1], true
	}
	if len(nodes) == 0 {
		work.Discard()
		return support.Mask{}, false
	}

	type frame struct {
		reference uint32
		ready     bool
	}
	state := make([]uint8, len(nodes)+2) // 0 unseen, 1 visiting, 2 done
	// Terminal references are implicit rows in the Region transport and are
	// already reduced values before the explicit DAG walk starts.
	state[0], state[1] = 2, 2
	stack := []frame{{reference: root}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.reference < 2 || current.reference > uint32(len(nodes))+1 {
			work.Discard()
			return support.Mask{}, false
		}
		index := current.reference - 2
		if current.ready {
			if state[current.reference] != 1 {
				work.Discard()
				return support.Mask{}, false
			}
			node := nodes[index]
			if node.Low > uint32(len(nodes))+1 || node.High > uint32(len(nodes))+1 || node.Low == node.High || state[node.Low] != 2 || state[node.High] != 2 {
				work.Discard()
				return support.Mask{}, false
			}
			physical, physicalOK := physicalByNeutral[node.Atom.ID()]
			if !physicalOK {
				work.Discard()
				return support.Mask{}, false
			}
			literal, literalOK := work.Literal(physical, true)
			negative, negativeOK := work.Not(literal)
			high, highOK := work.And(literal, values[node.High])
			low, lowOK := work.And(negative, values[node.Low])
			result, resultOK := work.Or(low, high)
			if !literalOK || !negativeOK || !highOK || !lowOK || !resultOK || !work.Valid(result) {
				work.Discard()
				return support.Mask{}, false
			}
			values[current.reference] = result
			state[current.reference] = 2
			continue
		}
		switch state[current.reference] {
		case 2:
			continue
		case 1:
			work.Discard()
			return support.Mask{}, false
		}
		node := nodes[index]
		if !node.Atom.Available() || node.Low > uint32(len(nodes))+1 || node.High > uint32(len(nodes))+1 || node.Low == node.High {
			work.Discard()
			return support.Mask{}, false
		}
		state[current.reference] = 1
		stack = append(stack, frame{reference: current.reference, ready: true})
		if node.High >= 2 {
			if state[node.High] == 1 {
				work.Discard()
				return support.Mask{}, false
			}
			stack = append(stack, frame{reference: node.High})
		}
		if node.Low >= 2 {
			if state[node.Low] == 1 {
				work.Discard()
				return support.Mask{}, false
			}
			stack = append(stack, frame{reference: node.Low})
		}
	}
	for reference := uint32(2); reference <= uint32(len(nodes))+1; reference++ {
		if state[reference] != 2 {
			work.Discard()
			return support.Mask{}, false
		}
	}
	if root < 2 || root > uint32(len(nodes))+1 {
		work.Discard()
		return support.Mask{}, false
	}
	result := values[root]
	if !work.Seal() || !result.Valid() {
		work.Discard()
		return support.Mask{}, false
	}
	return result, true
}
