package mounted

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
)

// ExecutionRoot is one independent entry into mounted execution: the entry
// point of a non-callable body at one mount. A callable body's interior is not
// a root, because it enters execution only through a selected call, a callback,
// or an explicit observation interface; an uncalled library function therefore
// exists in the denominator without being a root.
type ExecutionRoot struct {
	Mount identity.ContentID
	Body  identity.ContentID
	Entry identity.ContentID
}

func (root ExecutionRoot) Available() bool {
	return root.Mount.Available() && root.Body.Available() && root.Entry.Available()
}

// CompareExecutionRoot is the canonical order of the root key: mount bytes,
// then body bytes, then entry bytes.
func CompareExecutionRoot(left, right ExecutionRoot) int {
	if order := bytes.Compare(left.Mount[:], right.Mount[:]); order != 0 {
		return order
	}
	if order := bytes.Compare(left.Body[:], right.Body[:]); order != 0 {
		return order
	}
	return bytes.Compare(left.Entry[:], right.Entry[:])
}

// ExecutionRoots is the independent execution-root set of a sealed Link,
// together with the execution points those roots seed.
//
// Both are derived from the placed artifacts alone. Which body is a root is a
// property of the sealed program; which points its roots seed is a property of
// the semantic occurrences that program carries. Neither consults a query
// family, so what is executed stops being a function of who wants to read it.
type ExecutionRoots struct {
	roots  []ExecutionRoot
	seeds  ExecutionPoints
	sealed bool
}

func (set ExecutionRoots) Available() bool {
	return set.sealed && len(set.roots) != 0 && set.seeds.Available()
}

func (set ExecutionRoots) Count() int {
	if !set.Available() {
		return 0
	}
	return len(set.roots)
}

func (set ExecutionRoots) At(index int) (ExecutionRoot, bool) {
	if !set.Available() || index < 0 || index >= len(set.roots) {
		return ExecutionRoot{}, false
	}
	return set.roots[index], true
}

// Seeds is the execution-point population the roots demand: the points of
// every semantic occurrence inside a root body, plus the entry points of a root
// body no occurrence of which contributes a placed point. A root with nothing
// to observe still needs one anchor, and it is the program-issued entry
// attachment of that root, never an arbitrary point borrowed from a callable
// body.
func (set ExecutionRoots) Seeds() ExecutionPoints {
	if !set.Available() {
		return ExecutionPoints{}
	}
	return set.seeds
}

// SealExecutionRoots derives the root set and its seeds from the placed
// artifacts. A mount with no non-callable body, a root body with no entry
// attachment, or a Link that seeds nothing is a program the demand graph cannot
// be rooted in, so the whole population fails closed.
func SealExecutionRoots(mounts []Mount) (ExecutionRoots, bool) {
	if !mountsAvailable(mounts) {
		return ExecutionRoots{}, false
	}
	roots := make([]ExecutionRoot, 0)
	seeds := make([]ExecutionPoint, 0)
	for _, mount := range mounts {
		entriesByBody, entriesOK := rootBodyEntries(mount)
		if !entriesOK {
			return ExecutionRoots{}, false
		}
		for body, entries := range entriesByBody {
			for _, entry := range entries {
				roots = append(roots, ExecutionRoot{Mount: mount.ModuleKey, Body: body, Entry: entry})
			}
		}
		demanded, demandedOK := rootDemandedPoints(mount, entriesByBody)
		if !demandedOK {
			return ExecutionRoots{}, false
		}
		for index := 0; index < mount.Artifact.PointCount(); index++ {
			point, ok := mount.Artifact.PointAt(index)
			if !ok || !point.Available() || !point.ID().Available() {
				return ExecutionRoots{}, false
			}
			if _, seeded := demanded[point.ID()]; !seeded {
				continue
			}
			seeds = append(seeds, ExecutionPoint{Mount: mount.ModuleKey, Point: point.ID()})
		}
	}
	column, columnOK := sealExecutionPointColumn(seeds)
	if !columnOK || len(roots) == 0 {
		return ExecutionRoots{}, false
	}
	sort.Slice(roots, func(left, right int) bool {
		return CompareExecutionRoot(roots[left], roots[right]) < 0
	})
	for index, root := range roots {
		if !root.Available() || index != 0 && CompareExecutionRoot(roots[index-1], root) >= 0 {
			return ExecutionRoots{}, false
		}
	}
	return ExecutionRoots{roots: roots, seeds: column, sealed: true}, true
}

// rootBodyEntries collects the entry attachments of every non-callable body of
// one mount. A root that carries no entry attachment cannot be anchored, and a
// mount that carries no root at all is not an executable placement.
func rootBodyEntries(mount Mount) (map[identity.ContentID][]identity.ContentID, bool) {
	entriesByBody := make(map[identity.ContentID][]identity.ContentID)
	for index := 0; index < mount.Artifact.BodyCount(); index++ {
		body, bodyOK := mount.Artifact.BodyAt(index)
		if !bodyOK || !body.Available() || !body.ID().Available() {
			return nil, false
		}
		if body.Callable() {
			continue
		}
		entries := make([]identity.ContentID, body.EntryPointCount())
		for entryIndex := range entries {
			entry, entryOK := body.EntryPointAt(entryIndex)
			if !entryOK || !entry.Available() {
				return nil, false
			}
			entries[entryIndex] = entry
		}
		if len(entries) == 0 {
			return nil, false
		}
		if _, duplicate := entriesByBody[body.ID()]; duplicate {
			return nil, false
		}
		entriesByBody[body.ID()] = entries
	}
	if len(entriesByBody) == 0 {
		return nil, false
	}
	return entriesByBody, true
}

// rootDemandedPoints resolves what one mount's roots demand. Occurrences owned
// by a callable body are skipped: the runtime cut is that a callable interior is
// reached through a call rather than by existing in a mounted program.
//
// A root body counts as occupied only once one of its occurrences contributes
// an addressable point row. An occurrence that names a root body without
// carrying such a point leaves the root with nothing to observe, so the entry
// anchor below still owes it one seed.
func rootDemandedPoints(mount Mount, entriesByBody map[identity.ContentID][]identity.ContentID) (map[identity.ContentID]struct{}, bool) {
	placed := make(map[identity.ContentID]struct{}, mount.Artifact.PointCount())
	for index := 0; index < mount.Artifact.PointCount(); index++ {
		point, ok := mount.Artifact.PointAt(index)
		if !ok || !point.Available() || !point.ID().Available() {
			return nil, false
		}
		placed[point.ID()] = struct{}{}
	}
	demanded := make(map[identity.ContentID]struct{})
	occupied := make(map[identity.ContentID]struct{}, len(entriesByBody))
	for index := 0; index < mount.Artifact.OccurrenceCount(); index++ {
		occurrence, occurrenceOK := mount.Artifact.OccurrenceAt(index)
		if !occurrenceOK || !occurrence.Available() {
			return nil, false
		}
		body, bodyOK := occurrence.BodyID()
		if !bodyOK {
			continue
		}
		if _, root := entriesByBody[body]; !root {
			continue
		}
		for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
			point, pointOK := occurrence.PointAt(pointIndex)
			if !pointOK || !point.Available() {
				return nil, false
			}
			if _, present := placed[point]; !present {
				continue
			}
			demanded[point] = struct{}{}
			occupied[body] = struct{}{}
		}
	}
	for body, entries := range entriesByBody {
		if _, present := occupied[body]; present {
			continue
		}
		for _, entry := range entries {
			demanded[entry] = struct{}{}
		}
	}
	return demanded, true
}
