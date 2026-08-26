package region

import (
	"bytes"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

const formulaDigestDomain = "analysis/relation/schema/region/formula/v1"

const (
	falseReference uint32 = iota
	trueReference
	firstNodeReference
)

var (
	falseDigest = terminalDigest(falseReference)
	trueDigest  = terminalDigest(trueReference)
)

// Region is one sealed immutable reduced ordered Boolean DAG.  A zero Region
// is unavailable.  The private storage and settled valid bit ensure every
// exported accessor is a read of an immutable value and no caller can mutate
// the canonical graph through a returned slice.
type Region struct {
	nodes  []node
	root   uint32
	digest identity.ContentID
	valid  bool
}

type node struct {
	atom Atom
	low  uint32
	high uint32
}

// False returns the explicit false terminal.
func False() Region { return Region{digest: falseDigest, valid: true} }

// True returns the explicit true terminal.
func True() Region { return Region{root: trueReference, digest: trueDigest, valid: true} }

// FromAtom constructs the one-atom proposition Atom?true:false.
func FromAtom(atom Atom) (Region, bool) {
	if !atom.Available() {
		return Region{}, false
	}
	return sealTransport([]Node{{Atom: atom, Low: falseReference, High: trueReference}}, firstNodeReference)
}

// NewRegion seals transport rows into one immutable reduced ordered Boolean
// DAG. References use 0=false, 1=true, and 2+ for row ordinals.
func NewRegion(nodes []Node, root uint32) (Region, bool) {
	return sealTransport(nodes, root)
}

// Available reports whether the Region was successfully sealed.  Seal proves
// all graph invariants before setting valid; this method intentionally does
// not re-walk the graph.
func (region Region) Available() bool { return region.valid }

// IsFalse reports whether this is the explicit false terminal.
func (region Region) IsFalse() bool {
	return region.Available() && region.root == falseReference
}

// IsTrue reports whether this is the explicit true terminal.
func (region Region) IsTrue() bool {
	return region.Available() && region.root == trueReference
}

// Identity returns the canonical Boolean-function identity.  Unavailable
// Regions return the zero identity.
func (region Region) Identity() identity.ContentID {
	if !region.Available() {
		return identity.ContentID{}
	}
	return region.digest
}

// Nodes returns all canonical rows in immutable postorder.  The returned
// slice is a defensive copy.
func (region Region) Nodes() []Node {
	if !region.Available() || len(region.nodes) == 0 {
		return nil
	}
	result := make([]Node, len(region.nodes))
	for index, value := range region.nodes {
		result[index] = Node{Atom: value.atom, Low: value.low, High: value.high}
	}
	return result
}

func sealTransport(rows []Node, root uint32) (Region, bool) {
	if root > uint32(len(rows))+1 {
		return Region{}, false
	}
	// Validate and canonicalize only after the complete source graph has been
	// checked. The source ordinals are transport detail; canonical ordinals are
	// assigned by the deterministic low-then-high postorder below.
	state := make([]uint8, len(rows)+2) // 0 unseen, 1 visiting, 2 done
	ordinal := make([]uint32, len(rows)+2)
	ordinal[falseReference], ordinal[trueReference] = falseReference, trueReference
	nodes := make([]node, 0, len(rows))
	unique := make(map[nodeKey]struct{}, len(rows))

	type frame struct {
		id    uint32
		ready bool
	}
	stack := []frame{{id: root}}
	reachable := 0
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.id < firstNodeReference {
			continue
		}
		if current.id > uint32(len(rows))+1 {
			return Region{}, false
		}
		index := current.id - firstNodeReference
		row := rows[index]
		if current.ready {
			if state[current.id] != 1 {
				return Region{}, false
			}
			if !validTransportNode(row, rows) {
				return Region{}, false
			}
			for _, child := range []uint32{row.Low, row.High} {
				if child < firstNodeReference {
					continue
				}
				if state[child] != 2 || !atomLess(row.Atom, rows[child-firstNodeReference].Atom) {
					return Region{}, false
				}
			}
			low, high := ordinal[row.Low], ordinal[row.High]
			if low == high {
				return Region{}, false
			}
			key := nodeKey{atom: row.Atom, low: low, high: high}
			if _, found := unique[key]; found {
				return Region{}, false
			}
			ordinal[current.id] = uint32(len(nodes)) + firstNodeReference
			nodes = append(nodes, node{atom: row.Atom, low: low, high: high})
			unique[key] = struct{}{}
			state[current.id] = 2
			continue
		}
		switch state[current.id] {
		case 2:
			continue
		case 1:
			return Region{}, false // cycle
		}
		if !validTransportNode(row, rows) {
			return Region{}, false
		}
		state[current.id] = 1
		reachable++
		// Push the ready frame first, then high and low so low is processed
		// before high by the LIFO stack.
		stack = append(stack, frame{id: current.id, ready: true}, frame{id: row.High}, frame{id: row.Low})
	}
	if reachable != len(rows) {
		return Region{}, false
	}
	canonicalRoot := ordinal[root]
	result := Region{nodes: nodes, root: canonicalRoot}
	result.digest = digestRegion(result)
	if !result.digest.Available() {
		return Region{}, false
	}
	result.valid = true
	return result, true
}

func validTransportNode(row Node, rows []Node) bool {
	return row.Atom.Available() && row.Low != row.High &&
		row.Low <= uint32(len(rows))+1 && row.High <= uint32(len(rows))+1
}

type nodeKey struct {
	atom      Atom
	low, high uint32
}

func atomLess(left, right Atom) bool {
	if !left.Available() || !right.Available() {
		return false
	}
	return bytes.Compare(left.id[:], right.id[:]) < 0
}

func atomEqual(left, right Atom) bool {
	return left.Available() && right.Available() && left.id == right.id
}

func digestRegion(region Region) identity.ContentID {
	if !region.rootValid() {
		return identity.ContentID{}
	}
	parts := make([][]byte, 0, 2+3*len(region.nodes))
	parts = append(parts, uint32Bytes(region.root), uint64Bytes(uint64(len(region.nodes))))
	for _, value := range region.nodes {
		parts = append(parts, value.atom.id[:], uint32Bytes(value.low), uint32Bytes(value.high))
	}
	digest, ok := identity.DeriveContentID(formulaDigestDomain, parts...)
	if !ok {
		return identity.ContentID{}
	}
	return digest
}

func terminalDigest(root uint32) identity.ContentID {
	return digestRegion(Region{root: root})
}

func (region Region) rootValid() bool {
	return region.root <= uint32(len(region.nodes))+1
}

func uint32Bytes(value uint32) []byte {
	var result [4]byte
	binary.BigEndian.PutUint32(result[:], value)
	return result[:]
}

func uint64Bytes(value uint64) []byte {
	var result [8]byte
	binary.BigEndian.PutUint64(result[:], value)
	return result[:]
}
