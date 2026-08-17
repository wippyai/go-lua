package guard

import (
	"context"
	"crypto/sha256"

	"github.com/wippyai/go-lua/internal/canonical"
)

// FormulaID is the fixed, collision-resistant canonical identity of one
// sealed Boolean formula. It is suitable for evidence rows because it is
// independent of BDD page addresses, Work generations, and construction
// history.
type FormulaID [sha256.Size]byte

// Available reports whether id was issued by Identity.
func (id FormulaID) Available() bool { return id != FormulaID{} }

const (
	formulaIdentityDomain  = "analysis/engine/guard/formula"
	formulaIdentityVersion = 1

	formulaFalseReference uint64 = iota
	formulaTrueReference
	formulaFirstNodeReference

	formulaDecisionRecord = 1
)

type formulaNode struct {
	atom      Atom
	low, high uint64
}

type formulaVisit uint8

const (
	formulaUnseen formulaVisit = iota
	formulaVisiting
	formulaDone
)

type formulaFrame struct {
	guard Guard
	phase uint8
}

// Identity derives a canonical SHA-256 identity for one sealed Guard. It is
// the no-cancellation convenience for cold/read-only callers; runtime code
// that holds an epoch must use IdentityWithCheckpoint.
func Identity(root Guard) (FormulaID, bool) {
	return IdentityWithCheckpoint(root, nil)
}

// IdentityWithCheckpoint derives a canonical SHA-256 identity for one sealed
// Guard. It emits a manager-independent reduced-BDD DAG: terminals have fixed
// references, and every decision records its Atom spelling and the canonical
// references of its low/high successors. The traversal and encoding loops
// poll checkpoint on every reachable-node step. A failed checkpoint returns no
// identity; no partial digest is observable or retained. The traversal is
// iterative and visits each reachable node once. It retains neither a cache
// nor a history in Manager; all traversal state dies with this call.
func IdentityWithCheckpoint(root Guard, checkpoint func() bool) (FormulaID, bool) {
	if !identityLive(checkpoint) {
		return FormulaID{}, false
	}
	manager := root.manager
	if manager == nil || !manager.validSealed(root) {
		return FormulaID{}, false
	}

	references := make(map[Guard]uint64)
	visits := make(map[Guard]formulaVisit)
	nodes := make([]formulaNode, 0)
	stack := make([]formulaFrame, 0, 16)

	childReference := func(child Guard) (uint64, bool) {
		if isTerminal(child) {
			if terminalValue(child) {
				return formulaTrueReference, true
			}
			return formulaFalseReference, true
		}
		reference, found := references[child]
		return reference, found && reference >= formulaFirstNodeReference
	}
	push := func(value Guard) bool {
		if !manager.validSealed(value) {
			return false
		}
		if isTerminal(value) {
			return true
		}
		switch visits[value] {
		case formulaDone:
			return true
		case formulaVisiting:
			return false
		default:
			visits[value] = formulaVisiting
			stack = append(stack, formulaFrame{guard: value})
			return true
		}
	}
	if !push(root) {
		return FormulaID{}, false
	}
	for len(stack) != 0 {
		if !identityLive(checkpoint) {
			return FormulaID{}, false
		}
		last := len(stack) - 1
		frame := &stack[last]
		n := manager.node(frame.guard)
		if n.rank >= uint64(len(manager.order)) {
			return FormulaID{}, false
		}
		switch frame.phase {
		case 0:
			frame.phase = 1
			if !push(n.low) {
				return FormulaID{}, false
			}
		case 1:
			frame.phase = 2
			if !push(n.high) {
				return FormulaID{}, false
			}
		default:
			low, lowOK := childReference(n.low)
			high, highOK := childReference(n.high)
			if !lowOK || !highOK {
				return FormulaID{}, false
			}
			reference := formulaFirstNodeReference + uint64(len(nodes))
			references[frame.guard] = reference
			visits[frame.guard] = formulaDone
			nodes = append(nodes, formulaNode{atom: manager.atom(n.rank), low: low, high: high})
			stack = stack[:last]
		}
	}

	rootReference, rootOK := childReference(root)
	if !identityLive(checkpoint) || !rootOK {
		return FormulaID{}, false
	}
	if !isTerminal(root) && (len(nodes) == 0 || rootReference != formulaFirstNodeReference+uint64(len(nodes))-1) {
		return FormulaID{}, false
	}
	hash := sha256.New()
	var writer canonical.Writer
	if !identityLive(checkpoint) || writer.Reset(context.Background(), hash, formulaIdentityDomain, formulaIdentityVersion) != nil ||
		writer.Uint(rootReference) != nil || writer.Count(uint64(len(nodes))) != nil {
		return FormulaID{}, false
	}
	for _, node := range nodes {
		if !identityLive(checkpoint) || writer.Record(formulaDecisionRecord) != nil || writer.Uint(uint64(node.atom)) != nil || writer.Uint(node.low) != nil || writer.Uint(node.high) != nil {
			return FormulaID{}, false
		}
	}
	if !identityLive(checkpoint) || writer.Finish() != nil || !identityLive(checkpoint) {
		return FormulaID{}, false
	}
	digest := hash.Sum(nil)
	if len(digest) != len(FormulaID{}) {
		return FormulaID{}, false
	}
	var result FormulaID
	copy(result[:], digest)
	return result, result.Available()
}

func identityLive(checkpoint func() bool) bool {
	return checkpoint == nil || checkpoint()
}
