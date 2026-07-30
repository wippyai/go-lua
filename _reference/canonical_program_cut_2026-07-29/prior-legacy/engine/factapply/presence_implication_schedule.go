package factapply

import (
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// presenceSemanticLocation is an actual storage location, not a path spelling.
// Segment-free resolver/stable/unversioned spellings normalize to one Values
// slot; only local structural paths remain keyspace locations.
type presenceSemanticLocation struct {
	root bool
	slot statekey.ValueDependency
	path keyspace.Key
}

type presenceImplicationFootprint struct {
	reads, writes []presenceSemanticLocation
	regions       []statekey.ValueDependency // descendant-mutation semantic roots
}

func presenceLocation(keys *keyspace.KeySpace, path keyspace.Key) presenceSemanticLocation {
	if slot, ok := rootValueDependencyForKey(keys, path); ok {
		return presenceSemanticLocation{root: true, slot: slot}
	}
	return presenceSemanticLocation{path: path}
}

func appendPresenceLocation(out []presenceSemanticLocation, location presenceSemanticLocation) []presenceSemanticLocation {
	for _, prior := range out {
		if prior == location {
			return out
		}
	}
	return append(out, location)
}

// presenceImplicationFootprints expands exactly the storage writes performed
// by the canonical target kernel, including already-observed equality aliases.
// This certificate is consumed by both SCC scheduling and guarded block
// planning so execution and dependency math cannot drift.
func presenceImplicationFootprints(
	storage presenceImplicationStorage,
	keys *keyspace.KeySpace,
	rows []pathevidence.PathPresenceImplication,
) ([]presenceImplicationFootprint, bool) {
	if storage == nil || keys == nil || !keys.Valid() {
		return nil, false
	}
	out := make([]presenceImplicationFootprint, len(rows))
	for index, row := range rows {
		fp := &out[index]
		fp.reads = appendPresenceLocation(fp.reads, presenceLocation(keys, row.Trigger))
		if row.HasTriggerPathEqual {
			fp.reads = appendPresenceLocation(fp.reads, presenceLocation(keys, row.TriggerOther))
		}
		targets := []keyspace.Key{row.Target}
		if _, root := rootValueDependencyForKey(keys, row.Target); !root {
			equivalent, ok := storage.EquivalentKeys(row.Target)
			if !ok {
				return nil, false
			}
			targets = append(targets, equivalent...)
		}
		for _, target := range targets {
			location := presenceLocation(keys, target)
			fp.reads = appendPresenceLocation(fp.reads, location)
			fp.writes = appendPresenceLocation(fp.writes, location)
		}
		if presenceImplicationTargetInvalidatesDescendants(row) {
			for _, target := range targets {
				if slot, root := rootValueDependencyForKey(keys, target); root {
					fp.regions = appendUniqueValueDependency(fp.regions, slot)
				}
			}
		}
	}
	return out, true
}

func presenceLocationsConflict(keys *keyspace.KeySpace, writer presenceImplicationFootprint, reader presenceImplicationFootprint) bool {
	equal := func(left, right presenceSemanticLocation) bool { return left == right }
	for _, write := range writer.writes {
		for _, read := range reader.reads {
			if equal(write, read) {
				return true
			}
		}
		for _, otherWrite := range reader.writes {
			if equal(write, otherWrite) {
				return true
			}
		}
	}
	for _, region := range writer.regions {
		for _, location := range append(append([]presenceSemanticLocation(nil), reader.reads...), reader.writes...) {
			if location.root {
				continue
			}
			// Stable/unversioned roots and resolver-version descendants share a
			// semantic Values root even though KeySpace prefix intentionally does
			// not equate their raw spellings.
			root, rooted := pathevidence.PathValueDependency(keys, location.path)
			if rooted && root == region && location.path.Segs != 0 {
				return true
			}
		}
	}
	return false
}

// presenceImplicationSCCs builds the exact directional RAW/WAW schedule for a
// sealed implication inventory. Shared read-only triggers create no edge.
func presenceImplicationSCCs(
	storage presenceImplicationStorage,
	keys *keyspace.KeySpace,
	rows []pathevidence.PathPresenceImplication,
) ([][]int, bool) {
	if storage == nil || keys == nil || !keys.Valid() {
		return nil, false
	}
	footprints, ok := presenceImplicationFootprints(storage, keys, rows)
	if !ok {
		return nil, false
	}
	return presenceDependencyComponents(len(rows), func(writer, reader int) bool {
		return presenceLocationsConflict(keys, footprints[writer], footprints[reader])
	})
}

// presenceDependencyComponents is the single deterministic SCC scheduler for
// both concrete footprints and family-certified guarded dependencies.  The
// caller supplies only the directional RAW/WAW relation; shared reads never
// create an edge.
func presenceDependencyComponents(size int, conflicts func(writer, reader int) bool) ([][]int, bool) {
	if size < 0 || conflicts == nil {
		return nil, false
	}
	graph := make([][]int, size)
	reverse := make([][]int, size)
	for writer := 0; writer < size; writer++ {
		for reader := 0; reader < size; reader++ {
			if !conflicts(writer, reader) {
				continue
			}
			graph[writer] = append(graph[writer], reader)
			reverse[reader] = append(reverse[reader], writer)
		}
	}

	type frame struct{ node, next int }
	seen := make([]bool, size)
	order := make([]int, 0, size)
	for start := 0; start < size; start++ {
		if seen[start] {
			continue
		}
		seen[start] = true
		stack := []frame{{node: start}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < len(graph[top.node]) {
				next := graph[top.node][top.next]
				top.next++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, frame{node: next})
				}
				continue
			}
			order = append(order, top.node)
			stack = stack[:len(stack)-1]
		}
	}
	componentOf := make([]int, size)
	for index := range componentOf {
		componentOf[index] = -1
	}
	components := make([][]int, 0)
	for oi := len(order) - 1; oi >= 0; oi-- {
		start := order[oi]
		if componentOf[start] >= 0 {
			continue
		}
		id := len(components)
		component := []int(nil)
		stack := []int{start}
		componentOf[start] = id
		for len(stack) != 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, node)
			for _, next := range reverse[node] {
				if componentOf[next] < 0 {
					componentOf[next] = id
					stack = append(stack, next)
				}
			}
		}
		sort.Ints(component)
		components = append(components, component)
	}

	condensed := make([][]int, len(components))
	indegree := make([]int, len(components))
	for source, edges := range graph {
		from := componentOf[source]
		for _, target := range edges {
			to := componentOf[target]
			if from == to {
				continue
			}
			duplicate := false
			for _, prior := range condensed[from] {
				duplicate = duplicate || prior == to
			}
			if !duplicate {
				condensed[from] = append(condensed[from], to)
				indegree[to]++
			}
		}
	}
	ready := make([]int, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	lessComponent := func(left, right int) bool { return components[left][0] < components[right][0] }
	sort.Slice(ready, func(i, j int) bool { return lessComponent(ready[i], ready[j]) })
	out := make([][]int, 0, len(components))
	for len(ready) != 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, components[id])
		for _, next := range condensed[id] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
				sort.Slice(ready, func(i, j int) bool { return lessComponent(ready[i], ready[j]) })
			}
		}
	}
	return out, len(out) == len(components)
}
