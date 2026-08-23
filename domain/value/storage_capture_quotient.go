package value

import "github.com/wippyai/go-lua/analysis/identity"

// storageCaptureQuotient is the seal-time equivalence relation for one
// mounted Program's captured storage Cells.  A FunctionCapture has two
// authored Cell identities because the child and parent bodies own different
// lexical interfaces, but both identities denote one mutable Lua upvalue at
// runtime.  The quotient keeps those portable IDs distinct while giving every
// mounted semantic lookup the one coordinate of their equivalence class.
//
// The map stores only captured IDs. A missing key means that the semantic ID
// is not a captured storage Cell and must retain its ordinary coordinate. A
// non-root entry points at the Cell in the enclosing body; a root points to
// itself. Thus the representative is the outermost owner, not an arbitrary
// hash-order choice.
type storageCaptureQuotient map[identity.ContentID]identity.ContentID

func (quotient storageCaptureQuotient) add(id identity.ContentID) bool {
	if quotient == nil || !id.Available() {
		return false
	}
	if _, present := quotient[id]; !present {
		quotient[id] = id
	}
	return true
}

func (quotient storageCaptureQuotient) find(id identity.ContentID) (identity.ContentID, bool) {
	if quotient == nil {
		return identity.ContentID{}, false
	}
	path := make([]identity.ContentID, 0, 4)
	seen := make(map[identity.ContentID]struct{}, 4)
	current := id
	for {
		if _, duplicate := seen[current]; duplicate {
			return identity.ContentID{}, false
		}
		seen[current] = struct{}{}
		parent, present := quotient[current]
		if !present || !parent.Available() {
			return identity.ContentID{}, false
		}
		if parent == current {
			for _, item := range path {
				quotient[item] = current
			}
			return current, true
		}
		path = append(path, current)
		current = parent
	}
}

// link records one lexical capture edge. The child Cell must point at its
// enclosing Cell; assigning a second parent or creating a cycle is malformed
// capture topology and is refused before any coordinate is published.
func (quotient storageCaptureQuotient) link(inner, outer identity.ContentID) bool {
	if quotient == nil || !inner.Available() || !outer.Available() || inner == outer {
		return false
	}
	if _, present := quotient[inner]; !present {
		if !quotient.add(inner) {
			return false
		}
	}
	if _, present := quotient[outer]; !present {
		if !quotient.add(outer) {
			return false
		}
	}
	if parent := quotient[inner]; parent != inner {
		return false
	}
	quotient[inner] = outer
	if _, ok := quotient.find(inner); ok {
		return true
	}
	quotient[inner] = inner
	return false
}

func (quotient storageCaptureQuotient) canonical(id identity.ContentID) (identity.ContentID, bool) {
	if quotient == nil {
		return identity.ContentID{}, false
	}
	return quotient.find(id)
}

// storageCaptureQuotientForModule seals the capture relation of one mounted
// Program.  Capture rows are already Program-owned and body-authenticated;
// this pass adds the storage-coordinate checks needed before Value can use
// them as a mounted quotient.  In particular, one authored storage Cell may
// participate in several lexical edges, but it must always retain one owner
// Body identity.  Any malformed or non-transitive bridge fails closed.
func (schema *valueBuilder) storageCaptureQuotientForModule(module identity.ContentID) (storageCaptureQuotient, bool) {
	if schema == nil || schema.Schema == nil || schema.artifacts == nil || schema.sealBoundary() == nil || !module.Available() {
		return nil, false
	}
	mount, mountOK := schema.artifacts[module]
	if !mountOK || !mount.Available() {
		return nil, false
	}
	program := mount.Program
	captureCount, countOK := program.FunctionCaptureCount()
	if !countOK {
		return nil, false
	}
	values := schema.sealBoundary().Values()
	quotient := make(storageCaptureQuotient, captureCount*2)
	// A storage identity is rooted in one authored Cell and therefore has one
	// body owner.  Checking that invariant here catches a malformed capture
	// bridge before it can silently merge unrelated mutable state.
	ownerBody := make(map[identity.ContentID]identity.ContentID, captureCount*2)
	type edge struct {
		inner identity.ContentID
		outer identity.ContentID
	}
	seenEdges := make(map[edge]struct{}, captureCount)
	for index := 0; index < captureCount; index++ {
		capture, captureOK := program.FunctionCaptureAt(index)
		inner := capture.InnerStorageCellID()
		outer := capture.OuterStorageCellID()
		innerBody := capture.InnerBodyID()
		outerBody := capture.OuterBodyID()
		if !captureOK || !capture.Available() || !inner.Available() || !outer.Available() || !innerBody.Available() || !outerBody.Available() || inner == outer || innerBody == outerBody {
			return nil, false
		}
		innerValue, innerValueOK := values.ForMountedSemantic(module, inner)
		outerValue, outerValueOK := values.ForMountedSemantic(module, outer)
		if !innerValueOK || !outerValueOK {
			return nil, false
		}
		if _, innerCoordinateOK := schema.coordinateForCold(innerValue); !innerCoordinateOK {
			return nil, false
		}
		if _, outerCoordinateOK := schema.coordinateForCold(outerValue); !outerCoordinateOK {
			return nil, false
		}
		current := edge{inner: inner, outer: outer}
		if _, duplicate := seenEdges[current]; duplicate {
			return nil, false
		}
		seenEdges[current] = struct{}{}
		for id, body := range map[identity.ContentID]identity.ContentID{inner: innerBody, outer: outerBody} {
			if prior, present := ownerBody[id]; present && prior != body {
				return nil, false
			}
			ownerBody[id] = body
			if !quotient.add(id) {
				return nil, false
			}
		}
		if !quotient.link(inner, outer) {
			return nil, false
		}
	}
	// Resolve every parent pointer once so published lookups are read-only and
	// canonical representatives are stable after sealing.
	for id := range quotient {
		if _, ok := quotient.canonical(id); !ok {
			return nil, false
		}
	}
	return quotient, true
}
