package sourcecontrol

import (
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/program/keyspace"
)

type vertexCatalog struct {
	paths          []keyspace.ContentID // physical sourcecontrol node -> path
	canonicalNodes []uint32             // semantic radix order -> physical node
	bodyPaths      []keyspace.ContentID // one-based Body ordinal -> BodyPath, seal-only
}

type catalogLifecycle struct {
	mu    sync.Mutex
	data  vertexCatalog
	phase uint8
	owner *Result
}

// VertexCatalogLease is the sole assembler-held release capability. It is
// opaque and owner-bound; copied Results share the lifecycle but cannot use a
// lease issued to a different Result value.
type VertexCatalogLease struct {
	state *catalogLifecycle
	owner *Result
	used  bool
}

const (
	catalogUninstalled uint8 = iota
	catalogInstalled
	catalogReleased
)

const (
	vertexRootEntryDomain        = "wippy/program/flow/vertex-root-entry"
	vertexBodyTailDomain         = "wippy/program/flow/vertex-body-tail"
	vertexBodyEmptyDomain        = "wippy/program/flow/vertex-body-empty"
	vertexDynamicDecisionDomain  = "wippy/program/flow/vertex-dynamic-loop-decision"
)

func vertexPhasePath(domain string, path keyspace.ContentID) keyspace.ContentID {
	if !path.Available() {
		return keyspace.ContentID{}
	}
	var encoded [128]byte
	offset := copy(encoded[:], domain)
	encoded[offset] = 0
	offset++
	offset += copy(encoded[offset:], path[:])
	return keyspace.ContentID(sha256.Sum256(encoded[:offset]))
}

// InstallVertexCatalog is SourceControl's sole semantic vertex issuance cut.
// It covers every structural node exactly once: a non-empty Body root entry,
// a Body tail, an empty Body, or a hidden dynamic-loop decision.  The
// existing adjacency arrays are reordered in place by the issued target
// paths, becoming the sole canonical CSR; no second graph is retained.
func (r *Result) InstallVertexCatalogLease(bodies *body.Result, receipt *semanticpath.VertexCatalogReceipt) (*VertexCatalogLease, error) {
	if r == nil || !r.ownerAvailable() || bodies == nil || receipt == nil || r.catalog == nil || r.catalog.owner != r {
		return nil, errors.New("program/flow/sourcecontrol: vertex path lease is unavailable")
	}
	lifecycle := r.catalog
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.phase != catalogUninstalled {
		return nil, errors.New("program/flow/sourcecontrol: vertex path lease is unavailable")
	}
	pathsLease, leaseOK := receipt.Consume(r.sourceID, r.flowID, r.staticID, r.moduleID)
	if !leaseOK {
		return nil, errors.New("program/flow/sourcecontrol: vertex path receipt is unavailable")
	}
	if !body.Matches(bodies, r.sourceID, r.flowID) || len(r.coordinates.bodyOffsets) == 0 || bodies.BodyCount() != len(r.coordinates.bodyOffsets)-1 ||
		r.coordinates.nodeCount == 0 || pathsLease.BodyCount() != bodies.BodyCount() || len(r.adjacency.forwardOffsets) != int(r.coordinates.nodeCount)+1 || len(r.adjacency.reverseOffsets) != int(r.coordinates.nodeCount)+1 {
		return nil, errors.New("program/flow/sourcecontrol: vertex catalog denominator disagrees with geometry")
	}
	paths := make([]keyspace.ContentID, r.coordinates.nodeCount)
	bodyPaths := make([]keyspace.ContentID, bodies.BodyCount()+1)
	for ordinal := uint32(1); ordinal <= uint32(bodies.BodyCount()); ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		rootCount, ok := bodies.RootCount(bodyTerm)
		if !ok || rootCount < 0 || int(ordinal) > pathsLease.BodyCount() {
			return nil, errors.New("program/flow/sourcecontrol: Body vertex denominator is unavailable")
		}
		start, tail := r.coordinates.bodyOffsets[ordinal-1], r.coordinates.bodyOffsets[ordinal]-1
		bodyPath, bodyPathOK := pathsLease.BodyAt(ordinal)
		if tail < start || !bodyPathOK {
			return nil, errors.New("program/flow/sourcecontrol: Body path is unavailable")
		}
		bodyPaths[ordinal] = bodyPath
		if rootCount == 0 {
			paths[start] = vertexPhasePath(vertexBodyEmptyDomain, bodyPath)
			continue
		}
		for cursor := 0; cursor < rootCount; cursor++ {
			root, rootOK := bodies.RootAt(bodyTerm, cursor)
			family, rootOrdinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if !rootOK || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || rootOrdinal == 0 {
				return nil, errors.New("program/flow/sourcecontrol: root vertex path is unavailable")
			}
			// RootEntry is body-qualified by RootPath.  The general structural
			// term plane intentionally omits that Body join for direct roots and
			// would collapse identical local root classes in sibling Bodies.
			rootPath, rootPathOK := pathsLease.RootAt(family, rootOrdinal)
			if !rootPathOK {
				return nil, errors.New("program/flow/sourcecontrol: root receipt is unavailable")
			}
			paths[start+uint32(cursor)] = vertexPhasePath(vertexRootEntryDomain, rootPath)
		}
		paths[tail] = vertexPhasePath(vertexBodyTailDomain, bodyPath)
	}
	for ordinal := uint32(1); ordinal < uint32(len(r.coordinates.loopDecision)); ordinal++ {
		node := r.coordinates.loopDecision[ordinal]
		if node == noNode {
			continue
		}
		if node >= r.coordinates.nodeCount {
			return nil, errors.New("program/flow/sourcecontrol: dynamic-loop vertex is unavailable")
		}
		rootPath, rootPathOK := pathsLease.TermAt(keyspace.FamilyLoop, ordinal)
		if !rootPathOK {
			return nil, errors.New("program/flow/sourcecontrol: dynamic-loop root receipt is unavailable")
		}
		paths[node] = vertexPhasePath(vertexDynamicDecisionDomain, rootPath)
	}
	for node, path := range paths {
		if !path.Available() {
			return nil, errors.New("program/flow/sourcecontrol: vertex phase coverage is incomplete")
		}
		if uint64(node) > uint64(^uint32(0)) {
			return nil, errors.New("program/flow/sourcecontrol: vertex ordinal overflows")
		}
	}
	canonical := make([]uint32, len(paths))
	for index := range canonical {
		canonical[index] = uint32(index)
	}
	radixVertexNodes(canonical, paths)
	for index := 1; index < len(canonical); index++ {
		if paths[canonical[index-1]] == paths[canonical[index]] {
			return nil, errors.New("program/flow/sourcecontrol: semantic vertex path collision")
		}
	}
	// Canonicalize both directions in place.  Offsets remain the same dense
	// sourcecontrol denominator, so NodeRef provenance remains exact while
	// every traversal sees one semantic CSR order.
	for node := uint32(0); node < r.coordinates.nodeCount; node++ {
		start, end := r.adjacency.forwardOffsets[node], r.adjacency.forwardOffsets[node+1]
		radixVertexNodes(r.adjacency.forwardTargets[start:end], paths)
		start, end = r.adjacency.reverseOffsets[node], r.adjacency.reverseOffsets[node+1]
		radixVertexNodes(r.adjacency.reverseTargets[start:end], paths)
	}
	lifecycle.data = vertexCatalog{paths: paths, canonicalNodes: canonical, bodyPaths: bodyPaths}
	lifecycle.phase = catalogInstalled
	return &VertexCatalogLease{state: lifecycle, owner: r}, nil
}

func radixVertexNodes(nodes []uint32, paths []keyspace.ContentID) {
	if len(nodes) < 2 {
		return
	}
	work := make([]uint32, len(nodes))
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, node := range nodes {
			counts[paths[node][byteIndex]]++
		}
		at := 0
		for index := range counts {
			at, counts[index] = at+counts[index], at
		}
		for _, node := range nodes {
			value := paths[node][byteIndex]
			work[counts[value]] = node
			counts[value]++
		}
		copy(nodes, work)
	}
}

func (r *Result) VertexCatalogAvailable() bool {
	// Do not call Result.available here: availability composes this phase
	// predicate, and a mutual call would make every post-catalog query recurse.
	if r == nil || !r.ownerAvailable() || r.catalog == nil {
		return false
	}
	state := r.catalog
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == catalogInstalled && r.coordinates.nodeCount != 0 &&
		len(state.data.paths) == int(r.coordinates.nodeCount) && len(state.data.canonicalNodes) == int(r.coordinates.nodeCount)
}

// VertexPathAt exposes only an already-issued semantic vertex path.  Dense
// coordinates remain an internal lease capability and never enter a path.
func (r *Result) VertexPathAt(node uint32) (keyspace.ContentID, bool) {
	if r == nil || !r.ownerAvailable() || r.catalog == nil {
		return keyspace.ContentID{}, false
	}
	state := r.catalog
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != catalogInstalled || node >= r.coordinates.nodeCount || len(state.data.paths) != int(r.coordinates.nodeCount) {
		return keyspace.ContentID{}, false
	}
	path := state.data.paths[node]
	return path, path.Available()
}

func (r *Result) VertexPath(ref NodeRef) (keyspace.ContentID, bool) {
	node, ok := r.ResolveNodeRef(ref)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return r.VertexPathAt(node)
}

// BodyEntryPath returns the exact phase receipt for a Body's entry vertex.
// For an empty Body this is the BodyEmpty phase; for a non-empty Body it is
// the first RootEntry phase.  It is intentionally a typed Body projection,
// not a generic node/Term path mapper.
func (r *Result) BodyEntryPath(body keyspace.Term) (keyspace.ContentID, bool) {
	node, ok := r.Cursor(body, 0)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return r.VertexPathAt(node)
}

// BodyTailPath returns the exact BodyTail (or BodyEmpty) phase receipt.  It
// stays available while Causal attaches terminal Outcome Sites even if no
// final route names that endpoint.
func (r *Result) BodyTailPath(body keyspace.Term) (keyspace.ContentID, bool) {
	node, ok := r.Tail(body)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return r.VertexPathAt(node)
}

// CanonicalNodeAt supplies the semantic radix permutation for full-graph
// recurrence traversal.  It never filters by Plan/executable liveness.
func (r *Result) CanonicalNodeAt(index int) (uint32, bool) {
	if r == nil || !r.ownerAvailable() || r.catalog == nil {
		return 0, false
	}
	state := r.catalog
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != catalogInstalled || index < 0 || index >= len(state.data.canonicalNodes) {
		return 0, false
	}
	node := state.data.canonicalNodes[index]
	return node, node < r.coordinates.nodeCount
}

// ReleaseVertexCatalog clears the transient semantic paths/permutation after
// all consumers have copied their exact point receipts.  SourceControl's Arc
// witnesses remain available for the rest of the assembly, but no topology
// or path lease can escape Flow publication.
func (r *Result) ReleaseVertexCatalog(lease *VertexCatalogLease) bool {
	if r == nil || lease == nil || lease.owner != r || lease.state == nil || r.catalog != lease.state {
		return false
	}
	state := lease.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != catalogInstalled || lease.used {
		return false
	}
	state.data.paths, state.data.canonicalNodes, state.data.bodyPaths = nil, nil, nil
	state.phase = catalogReleased
	lease.used = true
	return true
}
