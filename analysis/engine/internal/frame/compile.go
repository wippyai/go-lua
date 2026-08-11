package frame

// Compile validates and seals one frame-access declaration. It first takes
// the equality quotient, then computes the least forward-reachable read and
// write sets independently. Work is O(R + E + P) up to union-find, where R is
// the number of roots, E the equality/follow declarations, and P projection
// entries. The returned closure retains only immutable root-membership
// bitsets; no per-root map or graph survives compilation.
func Compile(spec Spec) (*Closure, bool) {
	if !validRootCount(spec.Roots) || !validateRelations(spec) {
		return nil, false
	}
	unknown, projectionsOK := validateProjections(spec)
	if !projectionsOK {
		return nil, false
	}
	if unknown {
		return &Closure{roots: spec.Roots, valid: true}, true
	}

	sets := newDisjointSet(spec.Roots)
	for _, pair := range spec.Equalities {
		sets.union(rootIndex(pair.Left), rootIndex(pair.Right))
	}
	readSeeds, writeSeeds := projectionSeeds(spec, sets)
	graph := collapsedGraph(spec, sets)
	readClasses := forwardClosure(graph, readSeeds)
	writeClasses := forwardClosure(graph, writeSeeds)
	return &Closure{
		roots: spec.Roots,
		read:  expandClasses(sets, readClasses),
		write: expandClasses(sets, writeClasses),
		valid: true,
		known: true,
	}, true
}

func validRootCount(roots int) bool {
	// Root is uint32 and ordinal zero is reserved, so a larger cardinality
	// cannot be represented by this algebra.
	return roots >= 0 && uint64(roots) <= uint64(^uint32(0))
}

func validateRelations(spec Spec) bool {
	for _, pair := range spec.Equalities {
		if !validRoot(pair.Left, spec.Roots) || !validRoot(pair.Right, spec.Roots) {
			return false
		}
	}
	for _, edge := range spec.Follows {
		if !validRoot(edge.From, spec.Roots) || !validRoot(edge.To, spec.Roots) {
			return false
		}
	}
	return true
}

// validateProjections intentionally does not inspect an unknown row's lists:
// an unknown mapping has no bounded root interpretation, and treating its
// spelling as an authority would accidentally turn unknown into known.
func validateProjections(spec Spec) (unknown bool, valid bool) {
	for _, projection := range spec.Projections {
		if !projection.Known {
			unknown = true
			continue
		}
		for _, root := range projection.MayRead {
			if !validRoot(root, spec.Roots) {
				return false, false
			}
		}
		for _, root := range projection.MayWrite {
			if !validRoot(root, spec.Roots) {
				return false, false
			}
		}
	}
	return unknown, true
}

func rootIndex(root Root) int { return int(root - 1) }

func projectionSeeds(spec Spec, sets *disjointSet) (read []bool, write []bool) {
	read, write = make([]bool, spec.Roots), make([]bool, spec.Roots)
	for _, projection := range spec.Projections {
		// Compile has already rejected an unknown projection before this point.
		for _, root := range projection.MayRead {
			read[sets.find(rootIndex(root))] = true
		}
		for _, root := range projection.MayWrite {
			write[sets.find(rootIndex(root))] = true
		}
	}
	return read, write
}

// collapsedGraph builds the quotient graph directly in disjoint-set root
// space. The stamp row de-duplicates each directed adjacency row in linear
// time without a map or a declaration-order sort.
func collapsedGraph(spec Spec, sets *disjointSet) [][]int {
	graph := make([][]int, spec.Roots)
	for _, edge := range spec.Follows {
		from, to := sets.find(rootIndex(edge.From)), sets.find(rootIndex(edge.To))
		graph[from] = append(graph[from], to)
	}
	stamps := make([]uint32, spec.Roots)
	for from, row := range graph {
		if len(row) < 2 {
			continue
		}
		stamp := uint32(from + 1)
		out := row[:0]
		for _, to := range row {
			if stamps[to] == stamp {
				continue
			}
			stamps[to] = stamp
			out = append(out, to)
		}
		graph[from] = out
	}
	return graph
}

func forwardClosure(graph [][]int, seeds []bool) []bool {
	reached := make([]bool, len(graph))
	queue := make([]int, 0)
	for root, seed := range seeds {
		if seed {
			reached[root] = true
			queue = append(queue, root)
		}
	}
	for head := 0; head < len(queue); head++ {
		for _, next := range graph[queue[head]] {
			if reached[next] {
				continue
			}
			reached[next] = true
			queue = append(queue, next)
		}
	}
	return reached
}

func expandClasses(sets *disjointSet, classes []bool) []uint64 {
	bits := make([]uint64, wordCount(len(classes)))
	for root := range classes {
		if classes[sets.find(root)] {
			bits[root>>6] |= uint64(1) << uint(root&63)
		}
	}
	return bits
}

func wordCount(roots int) int {
	if roots == 0 {
		return 0
	}
	return (roots-1)/64 + 1
}
