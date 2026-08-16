package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// bodyOrigin is exactly one Program-owned lexical Body in one Link shard.
// It is Solver-private compilation provenance, never an additional Program
// identity, State coordinate, or serializable handle.
type bodyOrigin struct {
	shard link.Shard
	body  program.Term
}

// activationOrigin selects the existing executable activation that owns a
// group of lexical Body equations: the shard Entry or a Function Body. The
// index only avoids rediscovering lexical ownership while compiling demand.
type activationOrigin struct {
	shard      link.Shard
	activation program.Term
}

// bodyIngress and bodyOutcomes are the complete existing Program boundary of
// one lexical body.  They deliberately retain Program Terms rather than
// minting an engine-side boundary vocabulary: domains will later attach their
// own formal entry/outcome Rules to these Terms.
type bodyIngress struct {
	entry program.Term
	first program.Term
}

type bodyOutcomes struct {
	normal   program.Term
	returned program.Term // absent when the body cannot Return.
	thrown   program.Term
	yielded  program.Term
	canceled program.Term
}

// bodyEdgeRange indexes a contiguous run in an edge-index vector.  The
// vector is sorted by the owning Program edge's construction order, so a
// demanded slice remains deterministic without rebuilding topology.
type bodyEdgeRange struct {
	term       program.Term
	start, end uint32
}

// bodyRecurrence retains Program's existing Mu provenance beside the edge
// that crosses it.  It is intentionally structural: the engine still asks
// the normal Program/Guard path to translate those terms for a live
// activation.  No scheduler SCC or candidate identity enters this row.
type bodyRecurrence struct {
	edge      uint32
	head      program.Term
	decisions []program.Term
}

// activationBodyRef identifies one lexical body in the outer activation
// catalog. It carries no live candidate or caller state. The activation index
// maps a demanded Term to only its owning bodies; each body then uses its own
// exact edge slice. This avoids duplicating every Edge in the activation
// index while still costing O(log T + degree) per demand.
type activationBodyRef uint32

// activationIngress is the existing BodyEntry -> BodyFirst relation for one
// lexical body. It is a Program relation, not a synthetic causal Edge.
type activationIngress struct {
	entry program.Term
	first program.Term
}

// compiledActivation is a private immutable index over the body-local
// templates owned by one existing activation. It is not another IR or graph:
// each entry points back to exactly one Program Body edge/ingress. Its only
// purpose is to prevent a lexical-body scan while satisfying a term demand.
type compiledActivation struct {
	bodies    []bodyOrigin
	ingresses []activationIngress
	incoming  []bodyEdgeRange
	inBodies  []activationBodyRef
	outgoing  []bodyEdgeRange
	outBodies []activationBodyRef
}

// compiledBody is the immutable, entry-independent equation template for one
// canonical Program Body. It contains only Program-owned identities and
// owner-bound Edge capabilities. In particular Candidate, caller, Link
// application, Factor value, and State identities cannot enter this type.
//
// incoming/outgoing are lazy demanded reverse/forward indexes. They are an
// index over the same Edges, not a second graph and not a candidate-expanded
// body inventory.
type compiledBody struct {
	body       program.Term
	activation program.Term
	ingress    bodyIngress
	outcomes   bodyOutcomes
	terms      []program.Term
	edges      []program.Edge
	incoming   []bodyEdgeRange
	inEdges    []uint32
	outgoing   []bodyEdgeRange
	outEdges   []uint32
	recurrence []bodyRecurrence
}

// compileBodies constructs the one Body-local equation inventory once during
// Solver seal. It visits each sealed Body and each Body-owned causal Edge once
// (plus fixed terminal handles), so construction is O(V + E). Runtime
// evaluation consumes these rows and never scans Program source/control data.
func (solver *Solver) compileBodies() bool {
	if solver == nil || solver.link == nil || solver.bodies != nil || solver.activationBodies != nil {
		return false
	}
	// Optional decoded sections are admitted before any body is installed. A
	// malformed, stale, or composition-mismatched section contributes no rows;
	// the ordinary Program compilation below then reconstructs its complete
	// shard. There is no mixed partial cache path.
	if !solver.prepareEquationCaches() {
		return false
	}
	bodies := make(map[bodyOrigin]compiledBody)
	activationOrigins := make(map[activationOrigin][]bodyOrigin)
	for shardIndex := 0; shardIndex < solver.link.ShardCount(); shardIndex++ {
		shard, ok := solver.link.ShardAt(shardIndex)
		if !ok {
			return false
		}
		p, ok := solver.link.Program(shard)
		if !ok || p == nil {
			return false
		}
		for bodyIndex := 0; bodyIndex < p.BodyCount(); bodyIndex++ {
			body, ok := p.BodyAt(bodyIndex)
			if !ok {
				return false
			}
			activation, ok := p.Activation(body)
			if !ok || activation == 0 {
				return false
			}
			row, cached := solver.cacheBodies[bodyOrigin{shard: shard, body: body}]
			if !cached {
				row, ok = compileProgramBody(p, body, activation)
				if !ok {
					return false
				}
			}
			if row.body != body || row.activation != activation {
				return false
			}
			origin := bodyOrigin{shard: shard, body: body}
			if _, duplicate := bodies[origin]; duplicate {
				return false
			}
			bodies[origin] = row
			key := activationOrigin{shard: shard, activation: activation}
			activationOrigins[key] = append(activationOrigins[key], origin)
		}
	}
	byActivation := make(map[activationOrigin]compiledActivation, len(activationOrigins))
	for key, origins := range activationOrigins {
		sort.Slice(origins, func(left, right int) bool { return origins[left].body < origins[right].body })
		index, ok := compileActivationIndex(bodies, origins)
		if !ok {
			return false
		}
		byActivation[key] = index
	}
	solver.bodies, solver.activationBodies = bodies, byActivation
	return true
}

func compileProgramBody(p *program.Program, body, activation program.Term) (compiledBody, bool) {
	if p == nil || body == 0 || activation == 0 {
		return compiledBody{}, false
	}
	entry, entryOK := p.BodyEntry(body)
	normal, normalOK := p.BodyNormalExit(body)
	thrown, throwOK := p.BodyThrowExit(body)
	yielded, yieldOK := p.BodyYieldExit(body)
	canceled, cancelOK := p.BodyCancelExit(body)
	if !entryOK || !normalOK || !throwOK || !yieldOK || !cancelOK {
		return compiledBody{}, false
	}
	ingress := bodyIngress{entry: entry}
	if first, ok := p.BodyFirst(body); ok {
		ingress.first = first
	}
	outcomes := bodyOutcomes{normal: normal, thrown: thrown, yielded: yielded, canceled: canceled}
	if returned, ok := p.BodyReturnExit(body); ok {
		outcomes.returned = returned
	}
	terms := make([]program.Term, 0, 8)
	add := func(term program.Term, ok bool) bool {
		if !ok || term == 0 || !p.Valid(term) {
			return false
		}
		terms = append(terms, term)
		return true
	}
	if !add(ingress.entry, true) || !add(outcomes.normal, true) ||
		!add(outcomes.thrown, true) || !add(outcomes.yielded, true) ||
		!add(outcomes.canceled, true) {
		return compiledBody{}, false
	}
	// Empty Bodies have no first occurrence, and Return exists only when the
	// Program has a real lexical return path. Both are absent coordinates, not
	// missing equation structure.
	if ingress.first != 0 && !add(ingress.first, true) {
		return compiledBody{}, false
	}
	if outcomes.returned != 0 && !add(outcomes.returned, true) {
		return compiledBody{}, false
	}
	count, ok := p.BodyEdgeCount(body)
	if !ok || count < 0 {
		return compiledBody{}, false
	}
	edges := make([]program.Edge, 0, count)
	for index := 0; index < count; index++ {
		edge, ok := p.BodyEdgeAt(body, index)
		if !ok || !p.ValidEdge(edge) || !add(edge.From(), true) || !add(edge.To(), true) {
			return compiledBody{}, false
		}
		edges = append(edges, edge)
	}
	sort.Slice(terms, func(left, right int) bool { return terms[left] < terms[right] })
	terms = compactTerms(terms)
	row := compiledBody{
		body:       body,
		activation: activation,
		ingress:    ingress,
		outcomes:   outcomes,
		terms:      terms,
		edges:      edges,
	}
	return row.withIndexes(p)
}

func compactTerms(terms []program.Term) []program.Term {
	if len(terms) < 2 {
		return terms
	}
	write := 1
	for _, term := range terms[1:] {
		if terms[write-1] == term {
			continue
		}
		terms[write] = term
		write++
	}
	return terms[:write]
}

// withIndexes seals the Body-local indexes once. The two index vectors have
// exactly one member for every edge each; they are not copied per caller.
func (body compiledBody) withIndexes(p *program.Program) (compiledBody, bool) {
	if p == nil || body.body == 0 || body.activation == 0 ||
		body.ingress.entry == 0 || body.outcomes.normal == 0 || body.outcomes.thrown == 0 ||
		body.outcomes.yielded == 0 || body.outcomes.canceled == 0 {
		return compiledBody{}, false
	}
	type edgeSlot struct {
		term program.Term
		edge uint32
	}
	in := make([]edgeSlot, len(body.edges))
	out := make([]edgeSlot, len(body.edges))
	recurrence := make([]bodyRecurrence, 0)
	for index, edge := range body.edges {
		if !p.ValidEdge(edge) || !containsBodyTerm(body.terms, edge.From()) || !containsBodyTerm(body.terms, edge.To()) {
			return compiledBody{}, false
		}
		in[index] = edgeSlot{term: edge.To(), edge: uint32(index)}
		out[index] = edgeSlot{term: edge.From(), edge: uint32(index)}
		if head, recurring := edge.Mu(); recurring {
			count, ok := edge.MuDecisionCount()
			if !ok || count < 0 {
				return compiledBody{}, false
			}
			decisions := make([]program.Term, count)
			for decisionIndex := range decisions {
				decision, ok := edge.MuDecisionAt(decisionIndex)
				if !ok || decision == 0 {
					return compiledBody{}, false
				}
				decisions[decisionIndex] = decision
			}
			recurrence = append(recurrence, bodyRecurrence{edge: uint32(index), head: head, decisions: decisions})
		}
	}
	build := func(slots []edgeSlot) ([]bodyEdgeRange, []uint32) {
		sort.Slice(slots, func(left, right int) bool {
			if slots[left].term != slots[right].term {
				return slots[left].term < slots[right].term
			}
			return slots[left].edge < slots[right].edge
		})
		indexes := make([]uint32, len(slots))
		ranges := make([]bodyEdgeRange, 0, len(slots))
		for start := 0; start < len(slots); {
			end := start + 1
			for end < len(slots) && slots[end].term == slots[start].term {
				end++
			}
			ranges = append(ranges, bodyEdgeRange{term: slots[start].term, start: uint32(start), end: uint32(end)})
			for index := start; index < end; index++ {
				indexes[index] = slots[index].edge
			}
			start = end
		}
		return ranges, indexes
	}
	body.incoming, body.inEdges = build(in)
	body.outgoing, body.outEdges = build(out)
	body.recurrence = recurrence
	return body, true
}

// compileActivationIndex fuses lexical Body indexes once at Seal/decode time.
// Runtime demand then performs a binary search over the requested Term and
// visits only its exact edge degree. No candidate-qualified copy of this
// index exists.
func compileActivationIndex(bodies map[bodyOrigin]compiledBody, origins []bodyOrigin) (compiledActivation, bool) {
	if len(origins) == 0 {
		return compiledActivation{}, false
	}
	type edgeSlot struct {
		term program.Term
		body activationBodyRef
	}
	activation := compiledActivation{bodies: append([]bodyOrigin(nil), origins...)}
	in := make([]edgeSlot, 0)
	out := make([]edgeSlot, 0)
	activation.ingresses = make([]activationIngress, 0, len(origins))
	for bodyIndex, origin := range activation.bodies {
		body, ok := bodies[origin]
		if !ok || body.body != origin.body || body.activation == 0 || uint64(bodyIndex) > uint64(^uint32(0)) {
			return compiledActivation{}, false
		}
		if body.ingress.first != 0 {
			activation.ingresses = append(activation.ingresses, activationIngress{entry: body.ingress.entry, first: body.ingress.first})
		}
		for _, edge := range body.edges {
			if edge.From() == 0 || edge.To() == 0 {
				return compiledActivation{}, false
			}
			ref := activationBodyRef(bodyIndex)
			in = append(in, edgeSlot{term: edge.To(), body: ref})
			out = append(out, edgeSlot{term: edge.From(), body: ref})
		}
	}
	sort.Slice(activation.ingresses, func(left, right int) bool {
		return activation.ingresses[left].entry < activation.ingresses[right].entry
	})
	for index := 1; index < len(activation.ingresses); index++ {
		if activation.ingresses[index-1].entry == activation.ingresses[index].entry {
			return compiledActivation{}, false
		}
	}
	build := func(slots []edgeSlot) ([]bodyEdgeRange, []activationBodyRef) {
		sort.Slice(slots, func(left, right int) bool {
			if slots[left].term != slots[right].term {
				return slots[left].term < slots[right].term
			}
			return slots[left].body < slots[right].body
		})
		refs := make([]activationBodyRef, 0, len(slots))
		ranges := make([]bodyEdgeRange, 0, len(slots))
		for start := 0; start < len(slots); {
			end := start + 1
			for end < len(slots) && slots[end].term == slots[start].term {
				end++
			}
			refStart := len(refs)
			for index := start; index < end; index++ {
				if index == start || slots[index-1].body != slots[index].body {
					refs = append(refs, slots[index].body)
				}
			}
			ranges = append(ranges, bodyEdgeRange{term: slots[start].term, start: uint32(refStart), end: uint32(len(refs))})
			start = end
		}
		return ranges, refs
	}
	activation.incoming, activation.inBodies = build(in)
	activation.outgoing, activation.outBodies = build(out)
	return activation, true
}

func (activation compiledActivation) visitIncoming(bodies map[bodyOrigin]compiledBody, term program.Term, visit func(program.Edge) bool) bool {
	return activation.visitIndexed(bodies, activation.incoming, activation.inBodies, term, false, visit)
}

func (activation compiledActivation) visitOutgoing(bodies map[bodyOrigin]compiledBody, term program.Term, visit func(program.Edge) bool) bool {
	return activation.visitIndexed(bodies, activation.outgoing, activation.outBodies, term, true, visit)
}

func (activation compiledActivation) visitIndexed(bodies map[bodyOrigin]compiledBody, ranges []bodyEdgeRange, refs []activationBodyRef, term program.Term, forward bool, visit func(program.Edge) bool) bool {
	if term == 0 || visit == nil {
		return false
	}
	index := sort.Search(len(ranges), func(index int) bool { return ranges[index].term >= term })
	if index == len(ranges) || ranges[index].term != term {
		return true
	}
	rangeValue := ranges[index]
	if rangeValue.start > rangeValue.end || int(rangeValue.end) > len(refs) {
		return false
	}
	for _, ref := range refs[rangeValue.start:rangeValue.end] {
		if int(ref) >= len(activation.bodies) {
			return false
		}
		body, ok := bodies[activation.bodies[ref]]
		if !ok {
			return false
		}
		if forward {
			if !body.visitOutgoing(term, visit) {
				return false
			}
		} else if !body.visitIncoming(term, visit) {
			return false
		}
	}
	return true
}

func (activation compiledActivation) firstAfterIngress(term program.Term) (program.Term, bool) {
	if term == 0 {
		return 0, false
	}
	index := sort.Search(len(activation.ingresses), func(index int) bool { return activation.ingresses[index].entry >= term })
	if index == len(activation.ingresses) || activation.ingresses[index].entry != term {
		return 0, false
	}
	return activation.ingresses[index].first, true
}

func (body compiledBody) visitIncoming(term program.Term, visit func(program.Edge) bool) bool {
	return body.visitIndexed(body.incoming, body.inEdges, term, visit)
}

func (body compiledBody) visitOutgoing(term program.Term, visit func(program.Edge) bool) bool {
	return body.visitIndexed(body.outgoing, body.outEdges, term, visit)
}

// firstAfterIngress exposes Program's existing BodyEntry -> BodyFirst
// relation without inventing a causal Edge. Empty bodies have no first
// occurrence.  This is necessary because a Body entry is an activation
// boundary, not necessarily the From endpoint of a causal edge.
func (body compiledBody) firstAfterIngress(term program.Term) (program.Term, bool) {
	if term == 0 || term != body.ingress.entry || body.ingress.first == 0 {
		return 0, false
	}
	return body.ingress.first, true
}

func (body compiledBody) visitIndexed(ranges []bodyEdgeRange, indexes []uint32, term program.Term, visit func(program.Edge) bool) bool {
	if term == 0 || visit == nil {
		return false
	}
	index := sort.Search(len(ranges), func(index int) bool { return ranges[index].term >= term })
	if index == len(ranges) || ranges[index].term != term {
		return true
	}
	rangeValue := ranges[index]
	if rangeValue.start > rangeValue.end || int(rangeValue.end) > len(indexes) {
		return false
	}
	for _, edgeIndex := range indexes[rangeValue.start:rangeValue.end] {
		if int(edgeIndex) >= len(body.edges) || !visit(body.edges[edgeIndex]) {
			return false
		}
	}
	return true
}

// visitActivationEdges projects only the demanded reverse or forward body
// slice for one live activation. Candidate identity is introduced here, at
// the disposable execution boundary, never in compiledBody.
func (solver *Solver) visitActivationEdges(candidate link.Candidate, shard link.Shard, activation, term program.Term, reverse bool, visit func(edgeOrigin) bool) bool {
	if solver == nil || solver.bodies == nil || solver.activationBodies == nil || shard == 0 || activation == 0 {
		return false
	}
	row, ok := solver.activationBodies[activationOrigin{shard: shard, activation: activation}]
	if !ok || len(row.bodies) == 0 {
		return false
	}
	use := func(edge program.Edge) bool {
		if edge.From() == 0 || edge.To() == 0 {
			return false
		}
		return visit(edgeOrigin{candidate: candidate, shard: shard, edge: edge})
	}
	if reverse {
		return row.visitIncoming(solver.bodies, term, use)
	}
	return row.visitOutgoing(solver.bodies, term, use)
}

// visitActivationIngresses projects only existing lexical entry-to-first
// correspondences for a demanded activation term. It transports no fact and
// creates no edge identity; the normal confluence at BodyFirst remains the
// only executable equation.
func (solver *Solver) visitActivationIngresses(shard link.Shard, activation, term program.Term, visit func(program.Term) bool) bool {
	if solver == nil || solver.bodies == nil || solver.activationBodies == nil || shard == 0 || activation == 0 || term == 0 || visit == nil {
		return false
	}
	row, ok := solver.activationBodies[activationOrigin{shard: shard, activation: activation}]
	if !ok || len(row.bodies) == 0 {
		return false
	}
	if first, present := row.firstAfterIngress(term); present && !visit(first) {
		return false
	}
	return true
}

func containsBodyTerm(terms []program.Term, wanted program.Term) bool {
	index := sort.Search(len(terms), func(index int) bool { return terms[index] >= wanted })
	return index < len(terms) && terms[index] == wanted
}
