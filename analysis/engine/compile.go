package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/dependency"
	"github.com/wippyai/go-lua/analysis/engine/internal/fiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

// compiledConfluence is one exact Program occurrence's incoming local edges
// and explicit relation contributions.  It is the sole action that publishes
// the occurrence root; local reductions are separate actions over that root.
type compiledConfluence struct {
	coordinate coordinate.Coordinate
	incoming   []compiledEdge
	relations  []compiledRelation
}

// compiledAction is the one executable graph.  All rule reads are typed
// closures; all topology is already Program/Link-derived and no action stores
// an engine endpoint taxonomy.
type compiledAction struct {
	index      int
	equation   dependency.Equation
	outer      int
	coordinate coordinate.Coordinate
	logs       []ruleRead
	selectors  []activationSource
	supports   []compiledSupportOutput
	confluence *compiledConfluence
	run        func(*transaction, *compiledAction) bool
}

type compiledEdge struct {
	candidate link.Candidate
	edge      program.Edge
	input     coordinate.Coordinate
	atom      guard.Atom
	truthy    bool
	reset     []guard.Atom
	rules     []ruleDeclaration
	reuse     bodyProjection
}

// compiledRelation is a fresh zero-derived n-ary contribution.  inputs are
// ordered and may intentionally repeat.  support is one exact selector
// guard, not a range or aggregation over unrelated relation templates.
type compiledRelation struct {
	relation activeRelation
	inputs   []coordinate.Coordinate
	rule     ruleDeclaration
	support  int
	reset    []guard.Atom
}

type supportBinding struct{ relation activeRelation }

type compiledSupportOutput struct {
	relation activeRelation
	slot     int
}

type relationLocation struct {
	relation int
	action   int
	index    int
}

// ruleExecution is callback-private Product state.  origins have exactly the
// same order as Tuple inputs, so a ReadRef position cannot silently read the
// first operand of a relation.
type ruleExecution struct {
	transaction *transaction
	// epoch and rule are the private validity stamp for one Rule callback.
	// They never become a domain-visible execution vocabulary.
	epoch      uint64
	rule       *ruleIdentity
	carried    bool
	equation   dependency.Equation
	output     coordinate.Coordinate
	origins    []coordinate.Coordinate
	condition  guard.Guard
	current    bool
	outputLeaf fiber.Leaf
	inputs     fiber.Tuple
	draft      *fiber.Draft
	pruned     bool
	capture    *bodyProjectionCapture
	relation   *activeRelation
}

type termOrigin struct {
	candidate link.Candidate
	shard     link.Shard
	term      program.Term
}

func (solver *Solver) ruleSelectors(at coordinate.Coordinate, members []ruleDeclaration) ([]activationSource, bool) {
	if solver == nil || !at.Valid() {
		return nil, false
	}
	result := make([]activationSource, 0, len(members))
	seen := make(map[*ruleIdentity]struct{}, len(members))
	for _, member := range members {
		if member.identity == nil {
			return nil, false
		}
		identity := member.identity()
		if identity == nil || !identity.sealed {
			return nil, false
		}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, activationSource{rule: identity, caller: at})
	}
	sort.Slice(result, func(left, right int) bool {
		order, valid := solver.compareActivationSource(result[left], result[right])
		return valid && order < 0
	})
	return result, solver.orderedActivationSources(result)
}

func (solver *Solver) confluenceSelectors(at coordinate.Coordinate, edges []compiledEdge, relations []compiledRelation) ([]activationSource, bool) {
	members := make([]ruleDeclaration, 0)
	for _, edge := range edges {
		members = append(members, edge.rules...)
	}
	for _, relation := range relations {
		members = append(members, relation.rule)
	}
	return solver.ruleSelectors(at, members)
}

// compileQueries binds only the finite backwards/forwards demanded slice.
// Relations add all ordered inputs and their selector; a root starts only if
// an explicit candidate-zero Query demands that shard Entry.
func (solver *Solver) compileQueries(active []activeRelation) bool {
	if solver == nil || solver.coordinate == nil || !solver.validateQueries() {
		return false
	}
	if len(solver.queries) == 0 {
		return true
	}
	type queryCoordinate struct {
		declaration int
		coordinate  coordinate.Coordinate
		tableSlot   int
		stateSlot   int
	}
	queryTerms := make([]termOrigin, 0, len(solver.queries))
	queryCoordinates := make([]queryCoordinate, 0, len(solver.queries))
	rootSeeds := make(map[termOrigin]struct{})
	for index, declaration := range solver.queries {
		origin := termOrigin{candidate: declaration.candidate, shard: declaration.shard, term: declaration.term}
		value, ok := solver.coordinateFor(origin)
		if !ok || declaration.bind == nil {
			return false
		}
		tableSlot, ok := solver.coordinate.Slot(value)
		if !ok || tableSlot < 0 {
			return false
		}
		queryCoordinates = append(queryCoordinates, queryCoordinate{declaration: index, coordinate: value, tableSlot: tableSlot, stateSlot: -1})
		queryTerms = append(queryTerms, origin)
		if declaration.candidate == (link.Candidate{}) {
			programValue, ok := solver.link.Program(declaration.shard)
			if !ok || programValue == nil {
				return false
			}
			entry, ok := programValue.Entry()
			if !ok {
				return false
			}
			rootSeeds[termOrigin{shard: declaration.shard, term: entry}] = struct{}{}
		}
	}
	// A completed State is a query result, not a continuation snapshot. Bind
	// every declared observation to the one compact coordinate frontier it can
	// read, sharing storage when declarations name the same Program location.
	// Table slots are private, dense coordinate identities, so ordering them is
	// exact and allocation-free at Read time without introducing a second key.
	sort.Slice(queryCoordinates, func(left, right int) bool {
		return queryCoordinates[left].tableSlot < queryCoordinates[right].tableSlot
	})
	queryRoots := make([]coordinate.Coordinate, 0, len(queryCoordinates))
	for index := range queryCoordinates {
		binding := &queryCoordinates[index]
		if len(queryRoots) == 0 || queryRoots[len(queryRoots)-1] != binding.coordinate {
			queryRoots = append(queryRoots, binding.coordinate)
		}
		binding.stateSlot = len(queryRoots) - 1
	}
	for _, binding := range queryCoordinates {
		if binding.stateSlot < 0 || binding.declaration < 0 || binding.declaration >= len(solver.queries) || !solver.queries[binding.declaration].bind(binding.coordinate, binding.stateSlot) {
			return false
		}
	}
	relationTermCount := 0
	for _, relation := range active {
		relationTermCount += len(relation.inputs) + 1
	}
	relationTerms := make([]termOrigin, 0, relationTermCount)
	selectorTerms := make([]termOrigin, 0, len(active))
	liveCandidates := make(map[link.Candidate]struct{})
	for _, relation := range active {
		if !solver.validActiveRelation(relation) {
			return false
		}
		relationTerms = append(relationTerms, relation.output)
		for _, input := range relation.inputs {
			relationTerms = append(relationTerms, input)
			if input.candidate != (link.Candidate{}) {
				liveCandidates[input.candidate] = struct{}{}
			}
		}
		if relation.output.candidate != (link.Candidate{}) {
			liveCandidates[relation.output.candidate] = struct{}{}
		}
		candidate, shard, term, ok := solver.coordinate.Semantic(relation.source.caller)
		if !ok {
			return false
		}
		selectorTerms = append(selectorTerms, termOrigin{candidate: candidate, shard: shard, term: term})
		if candidate != (link.Candidate{}) {
			liveCandidates[candidate] = struct{}{}
		}
	}
	allTerms := make([]termOrigin, 0, len(queryTerms)+len(relationTerms)+len(selectorTerms)+len(rootSeeds))
	allTerms = append(allTerms, queryTerms...)
	allTerms = append(allTerms, relationTerms...)
	allTerms = append(allTerms, selectorTerms...)
	for seed := range rootSeeds {
		allTerms = append(allTerms, seed)
	}
	all, incoming, ok := solver.bodyApplications(allTerms)
	if !ok {
		return false
	}
	outgoing := make(map[termOrigin][]termOrigin, len(all))
	for _, origin := range all {
		from := termOrigin{candidate: origin.candidate, shard: origin.shard, term: origin.edge.From()}
		to := termOrigin{candidate: origin.candidate, shard: origin.shard, term: origin.edge.To()}
		outgoing[from] = append(outgoing[from], to)
	}
	edgeRules, localRules, relationRules, ok := solver.rulesByLocation(all)
	if !ok {
		return false
	}
	relationIncoming := make(map[termOrigin][]int, len(active))
	for index, relation := range active {
		relationIncoming[relation.output] = append(relationIncoming[relation.output], index)
	}
	for target, indexes := range relationIncoming {
		sort.Slice(indexes, func(left, right int) bool {
			return solver.compareActiveRelation(active[indexes[left]], active[indexes[right]]) < 0
		})
		relationIncoming[target] = indexes
	}
	pending := make([]termOrigin, 0, len(allTerms))
	pending = append(pending, allTerms...)
	selected := make(map[termOrigin]struct{}, len(allTerms)+len(all))
	for len(pending) != 0 {
		last := len(pending) - 1
		term := pending[last]
		pending = pending[:last]
		if _, present := selected[term]; present {
			continue
		}
		selected[term] = struct{}{}
		for _, origin := range incoming[term] {
			pending = append(pending, termOrigin{candidate: origin.candidate, shard: origin.shard, term: origin.edge.From()})
		}
		for _, index := range relationIncoming[term] {
			pending = append(pending, active[index].inputs...)
		}
		forward := false
		if term.candidate == (link.Candidate{}) {
			programValue, ok := solver.link.Program(term.shard)
			if !ok || programValue == nil {
				return false
			}
			activation, ok := programValue.Activation(term.term)
			entry, entryOK := programValue.Entry()
			forward = ok && entryOK && activation == entry
			if forward {
				_, forward = rootSeeds[termOrigin{shard: term.shard, term: entry}]
			}
		} else {
			_, forward = liveCandidates[term.candidate]
		}
		if forward {
			pending = append(pending, outgoing[term]...)
		}
	}
	terms := make([]termOrigin, 0, len(selected))
	for term := range selected {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(left, right int) bool { return solver.compareTermOrigin(terms[left], terms[right]) < 0 })

	actions := make([]compiledAction, 0, len(terms)+len(solver.rules))
	confluenceAt := make(map[termOrigin]int, len(terms))
	type localNode struct {
		index int
		term  termOrigin
		reads []ruleRead
	}
	locals := make([]localNode, 0, len(solver.rules))
	producers := make(map[termOrigin]map[int]int, len(terms))
	locations := make([]relationLocation, 0, len(active))
	for _, target := range terms {
		value, ok := solver.coordinateFor(target)
		if !ok {
			return false
		}
		bundles := make([]compiledEdge, 0, len(incoming[target]))
		relations := make([]compiledRelation, 0, len(relationIncoming[target]))
		logs := make([]ruleRead, 0)
		for _, origin := range incoming[target] {
			source := termOrigin{candidate: origin.candidate, shard: origin.shard, term: origin.edge.From()}
			if _, included := selected[source]; !included {
				return false
			}
			input, ok := solver.coordinateFor(source)
			if !ok {
				return false
			}
			var atom guard.Atom
			truthy := false
			if decision, branch, conditional := origin.edge.Decision(); conditional {
				atom, ok = solver.decisionAtom(origin.candidate, origin.shard, decision)
				if !ok {
					return false
				}
				truthy = branch
			}
			members := append([]ruleDeclaration(nil), edgeRules[origin]...)
			for _, member := range members {
				reads, ok := member.reads()
				if !ok {
					return false
				}
				logs = append(logs, reads...)
			}
			reuse, ok := solver.edgeBodyProjection(origin, members)
			if !ok {
				return false
			}
			reset, ok := solver.edgeResetAtoms(origin.candidate, origin.shard, origin.edge)
			if !ok {
				return false
			}
			bundles = append(bundles, compiledEdge{candidate: origin.candidate, edge: origin.edge, input: input, atom: atom, truthy: truthy, reset: reset, rules: members, reuse: reuse})
		}
		for _, relationIndex := range relationIncoming[target] {
			relation := active[relationIndex]
			member, present := relationRules[relation.target]
			if !present {
				return false
			}
			inputs := make([]coordinate.Coordinate, len(relation.inputs))
			for index, origin := range relation.inputs {
				if _, included := selected[origin]; !included {
					return false
				}
				input, ok := solver.coordinateFor(origin)
				if !ok {
					return false
				}
				inputs[index] = input
			}
			reads, ok := member.reads()
			if !ok {
				return false
			}
			logs = append(logs, reads...)
			reset, ok := solver.relationResetAtoms(relation)
			if !ok {
				return false
			}
			relations = append(relations, compiledRelation{relation: relation, inputs: inputs, rule: member, support: -1, reset: reset})
		}
		confluence := &compiledConfluence{coordinate: value, incoming: bundles, relations: relations}
		selectors, ok := solver.confluenceSelectors(value, bundles, relations)
		if !ok {
			return false
		}
		actionIndex := len(actions)
		actions = append(actions, compiledAction{
			coordinate: value,
			logs:       distinctFactorLogs(logs),
			selectors:  selectors,
			confluence: confluence,
			run: func(transaction *transaction, action *compiledAction) bool {
				return transaction.runConfluence(action, confluence)
			},
		})
		confluenceAt[target] = actionIndex
		for relationIndex := range relations {
			locations = append(locations, relationLocation{relation: relationIncoming[target][relationIndex], action: actionIndex, index: relationIndex})
		}
		for _, member := range localRules[templateTerm{shard: target.shard, term: target.term}] {
			output, ok := member.slot()
			if !ok || output < 0 || output >= len(solver.factors) {
				return false
			}
			row := producers[target]
			if row == nil {
				row = make(map[int]int)
				producers[target] = row
			}
			if _, duplicate := row[output]; duplicate {
				return false
			}
			reads, ok := member.reads()
			if !ok {
				return false
			}
			member := member
			reuse, ok := solver.localBodyProjection(target, member)
			if !ok {
				return false
			}
			selectors, ok := solver.ruleSelectors(value, []ruleDeclaration{member})
			if !ok {
				return false
			}
			index := len(actions)
			actions = append(actions, compiledAction{
				coordinate: value,
				logs:       distinctFactorLogs(reads),
				selectors:  selectors,
				run: func(transaction *transaction, action *compiledAction) bool {
					return transaction.runLocal(action, member, reuse)
				},
			})
			row[output] = index
			locals = append(locals, localNode{index: index, term: target, reads: reads})
		}
	}
	selectorAt := make(map[activationSource]int)
	for index := range actions {
		for _, selector := range actions[index].selectors {
			if prior, duplicate := selectorAt[selector]; duplicate && prior != index {
				return false
			}
			selectorAt[selector] = index
		}
	}
	catalog, outputs, supportTargets, ok := solver.compileSupportCatalog(active, selectorAt, locations, actions)
	if !ok {
		return false
	}
	for action := range actions {
		actions[action].supports = outputs[action]
		if !solver.orderedActivationSources(actions[action].selectors) || !solver.orderedSupportOutputs(actions[action].supports) {
			return false
		}
		for _, output := range actions[action].supports {
			if output.slot >= len(catalog) {
				return false
			}
		}
	}

	type actionArc struct{ from, to int }
	orderArcs := make(map[actionArc]struct{}, len(all)+len(locals)*2+len(active)*3)
	followerArcs := make(map[actionArc]struct{}, len(all)+len(locals)*2)
	presenceArcs := make(map[actionArc]struct{}, len(active)*2)
	addOrder := func(from, to int) bool {
		if from < 0 || to < 0 || from >= len(actions) || to >= len(actions) {
			return false
		}
		orderArcs[actionArc{from: from, to: to}] = struct{}{}
		return true
	}
	addFollower := func(from, to int) bool {
		if !addOrder(from, to) {
			return false
		}
		followerArcs[actionArc{from: from, to: to}] = struct{}{}
		return true
	}
	addPresenceFollower := func(from, to int) bool {
		if !addOrder(from, to) {
			return false
		}
		presenceArcs[actionArc{from: from, to: to}] = struct{}{}
		return true
	}
	for target, destination := range confluenceAt {
		confluence := actions[destination].confluence
		if confluence == nil {
			return false
		}
		for _, bundle := range confluence.incoming {
			source := termOrigin{candidate: bundle.candidate, shard: target.shard, term: bundle.edge.From()}
			from, present := confluenceAt[source]
			if !present || !addFollower(from, destination) {
				return false
			}
			for _, local := range producers[source] {
				if !addFollower(local, destination) {
					return false
				}
			}
		}
		for _, relation := range confluence.relations {
			for _, input := range relation.relation.inputs {
				from, present := confluenceAt[input]
				if !present || !addPresenceFollower(from, destination) {
					return false
				}
				// Any local Rule may Prune its complete product terminal,
				// independently of its declared output or reads. Keep the
				// complete producer set in the one schedule graph and wake the
				// Relation only when its input support topology changes.
				for _, producer := range producers[input] {
					if !addPresenceFollower(producer, destination) {
						return false
					}
				}
			}
			reads, ok := relation.rule.reads()
			if !ok {
				return false
			}
			for _, read := range reads {
				if read.position < 0 || read.position >= len(relation.relation.inputs) {
					return false
				}
				slot, ok := read.slot()
				if !ok || slot < 0 || slot >= len(solver.factors) {
					return false
				}
				input := relation.relation.inputs[read.position]
				producer, produced := producers[input][slot]
				if !produced {
					continue
				}
				// A declaration is only a potential read. It constrains the
				// static order, while an actual ReadAt or Carry installs the
				// exact value dependency at runtime.
				if !addOrder(producer, destination) {
					return false
				}
			}
			selector, present := selectorAt[relation.relation.source]
			if !present {
				return false
			}
			if !addOrder(selector, destination) {
				return false
			}
		}
	}
	for _, local := range locals {
		confluence, present := confluenceAt[local.term]
		if !present || !addFollower(confluence, local.index) {
			return false
		}
		seen := make(map[int]struct{}, len(local.reads))
		for _, read := range local.reads {
			slot, ok := read.slot()
			if !ok || slot < 0 || slot >= len(solver.factors) {
				return false
			}
			from, produced := producers[local.term][slot]
			if !produced {
				continue
			}
			if _, duplicate := seen[from]; duplicate {
				continue
			}
			seen[from] = struct{}{}
			// As with Relations, declaration establishes scheduling order;
			// the typed dependency log owns actual value invalidation.
			if !addOrder(from, local.index) {
				return false
			}
		}
	}
	edges := make([]schedule.Edge, 0, len(orderArcs))
	followers := make([][]int, len(actions))
	presenceFollowers := make([][]int, len(actions))
	for arc := range orderArcs {
		edges = append(edges, schedule.Edge{From: schedule.Node(arc.from), To: schedule.Node(arc.to)})
	}
	for arc := range followerArcs {
		followers[arc.from] = append(followers[arc.from], arc.to)
	}
	for arc := range presenceArcs {
		presenceFollowers[arc.from] = append(presenceFollowers[arc.from], arc.to)
	}
	for index := range followers {
		sort.Ints(followers[index])
		followers[index] = compactInts(followers[index])
		sort.Ints(presenceFollowers[index])
		presenceFollowers[index] = compactInts(presenceFollowers[index])
	}
	order := make([]uint64, len(actions))
	for index := range order {
		order[index] = uint64(index + 1)
	}
	prepared, err := schedule.Prepare(len(actions), edges, order)
	if err != nil {
		return false
	}
	type outerTuple struct {
		slots   []int
		members []int
	}
	componentTuples := make([]outerTuple, prepared.ComponentCount())
	cyclicTupleRanked := false
	for action := range actions {
		component, cyclic, present := prepared.ComponentOf(schedule.Node(action))
		if !present || component < 0 || component >= len(componentTuples) {
			return false
		}
		if !cyclic {
			continue
		}
		if !cyclicTupleRanked {
			// A cyclic equation sweeps the complete correlated Fiber tuple. Every
			// sealed Factor column is therefore a member of every Mu tuple,
			// including columns untouched by this component's present Rule outputs.
			// Widen has a termination proof only for ranked columns, so preflight
			// the fixed schema once before constructing any Mu schedule. Acyclic
			// components never take this path and may contain unranked Factors.
			for _, declaration := range solver.factors {
				if declaration.widen == nil {
					return false
				}
			}
			cyclicTupleRanked = true
		}
		slot, valid := solver.coordinate.Slot(actions[action].coordinate)
		if !valid || slot < 0 {
			return false
		}
		componentTuples[component].members = append(componentTuples[component].members, action)
		componentTuples[component].slots = append(componentTuples[component].slots, slot)
	}
	tuples := make([]outerTuple, len(actions))
	tuplePresent := make([]bool, len(actions))
	mu := make([]schedule.Node, 0)
	for index := 0; index < prepared.ComponentCount(); index++ {
		minimum, cyclic, ok := prepared.ComponentAt(index)
		if !ok {
			return false
		}
		if !cyclic {
			continue
		}
		head := int(minimum)
		tuple := componentTuples[index]
		if len(tuple.members) == 0 {
			return false
		}
		sort.Ints(tuple.slots)
		tuple.slots = compactInts(tuple.slots)
		if head < 0 || head >= len(tuples) || tuplePresent[head] {
			return false
		}
		tuples[head], tuplePresent[head] = tuple, true
		mu = append(mu, minimum)
	}
	scheduled, err := prepared.Build(mu)
	if err != nil || scheduled == nil {
		return false
	}
	regions := make([]compiledRegion, scheduled.RegionCount())
	for index := range regions {
		region, valid := scheduled.RegionAt(index)
		if !valid || region.Head < 0 || int(region.Head) >= len(actions) {
			return false
		}
		regions[index].head, regions[index].outer = int(region.Head), region.Parent == schedule.NoRegion
		if regions[index].outer {
			if !tuplePresent[regions[index].head] {
				return false
			}
			tuple := tuples[regions[index].head]
			if len(tuple.slots) == 0 || len(tuple.members) == 0 {
				return false
			}
			regions[index].slots, regions[index].members, regions[index].narrow = tuple.slots, tuple.members, true
			for _, declaration := range solver.factors {
				if declaration.narrow == nil || declaration.lessOrEq == nil {
					regions[index].narrow = false
					break
				}
			}
		}
	}
	for index := range actions {
		actions[index].outer, actions[index].index, actions[index].equation = -1, index, dependency.NewEquation(uint32(index))
	}
	for index, region := range regions {
		if !region.outer {
			continue
		}
		for _, member := range region.members {
			if member < 0 || member >= len(actions) || actions[member].outer >= 0 {
				return false
			}
			actions[member].outer = index
		}
	}
	roots := make([]coordinate.Coordinate, solver.coordinate.Count())
	for index := 0; index < solver.coordinate.Count(); index++ {
		value, ok := solver.coordinate.OrderedAt(index)
		if !ok {
			return false
		}
		slot, ok := solver.coordinate.Slot(value)
		if !ok || slot < 0 || slot >= len(roots) || roots[slot].Valid() {
			return false
		}
		roots[slot] = value
	}
	entrySeeds := make(map[coordinate.Coordinate]struct{}, len(rootSeeds))
	for origin := range rootSeeds {
		value, ok := solver.coordinateFor(origin)
		if !ok {
			return false
		}
		entrySeeds[value] = struct{}{}
	}
	for slot, target := range supportTargets {
		if target < 0 || target >= len(actions) {
			return false
		}
		outer := actions[target].outer
		if outer >= 0 {
			if outer >= len(regions) || !regions[outer].outer {
				return false
			}
			regions[outer].supports = append(regions[outer].supports, slot)
		}
	}
	for index := range regions {
		if regions[index].outer {
			sort.Ints(regions[index].supports)
			regions[index].supports = compactInts(regions[index].supports)
		}
	}
	solver.actions, solver.schedule, solver.regions = actions, scheduled, regions
	solver.followers, solver.presenceFollowers, solver.roots, solver.queryRoots = followers, presenceFollowers, roots, queryRoots
	solver.supportCatalog, solver.supportTargets, solver.entrySeeds = catalog, supportTargets, entrySeeds
	return true
}

func (solver *Solver) compileSupportCatalog(active []activeRelation, selectorAt map[activationSource]int, locations []relationLocation, actions []compiledAction) ([]supportBinding, [][]compiledSupportOutput, []int, bool) {
	if solver == nil || len(actions) == 0 && len(active) != 0 {
		return nil, nil, nil, false
	}
	// relationLocation already names the dense active-relation position that
	// created it. Validate that one-to-one correspondence once, rather than
	// recovering it by comparing every canonical relation against every
	// location. The compact table is compiler-private topology, not another
	// relation representation.
	byRelation := make([]relationLocation, len(active))
	present := make([]bool, len(active))
	for _, location := range locations {
		if location.relation < 0 || location.relation >= len(active) || present[location.relation] || location.action < 0 || location.action >= len(actions) || actions[location.action].confluence == nil || location.index < 0 || location.index >= len(actions[location.action].confluence.relations) {
			return nil, nil, nil, false
		}
		compiled := actions[location.action].confluence.relations[location.index]
		if compiled.support >= 0 || solver.compareActiveRelation(compiled.relation, active[location.relation]) != 0 {
			return nil, nil, nil, false
		}
		byRelation[location.relation], present[location.relation] = location, true
	}
	for index := range active {
		if !present[index] || !solver.validActiveRelation(active[index]) {
			return nil, nil, nil, false
		}
	}

	// An epoch normally supplies an already strict active sequence. The
	// compiler keeps this boundary self-contained: sort its dense positions
	// once, then use that one canonical order for catalog slots and every
	// selector projection. No run-time relation lookup is introduced.
	ordered := make([]int, len(active))
	for index := range ordered {
		ordered[index] = index
	}
	sort.Slice(ordered, func(left, right int) bool {
		return solver.compareActiveRelation(active[ordered[left]], active[ordered[right]]) < 0
	})
	for index := 1; index < len(ordered); index++ {
		if solver.compareActiveRelation(active[ordered[index-1]], active[ordered[index]]) == 0 {
			return nil, nil, nil, false
		}
	}
	catalog := make([]supportBinding, len(ordered))
	outputs := make([][]compiledSupportOutput, len(actions))
	targets := make([]int, len(ordered))
	for slot, relationIndex := range ordered {
		relation, location := active[relationIndex], byRelation[relationIndex]
		catalog[slot], targets[slot] = supportBinding{relation: relation}, location.action
		selector, present := selectorAt[relation.source]
		if !present || selector < 0 || selector >= len(actions) {
			return nil, nil, nil, false
		}
		if targets[slot] < 0 || targets[slot] >= len(actions) {
			return nil, nil, nil, false
		}
		actions[location.action].confluence.relations[location.index].support = slot
		// Iterating the complete canonical order makes every selector subset
		// canonical too; there is no second per-action sort.
		outputs[selector] = append(outputs[selector], compiledSupportOutput{relation: relation, slot: slot})
	}
	return catalog, outputs, targets, true
}

// orderedActivationSources is the compile boundary proof for the immutable
// selector table. Dispatch borrows this table and performs exact binary
// search; no transaction-local selector copy or lookup structure exists.
func (solver *Solver) orderedActivationSources(values []activationSource) bool {
	if solver == nil {
		return false
	}
	for index := range values {
		if values[index].rule == nil || !values[index].caller.Valid() {
			return false
		}
		if _, valid := solver.compareActivationSource(values[index], values[index]); !valid {
			return false
		}
		if index == 0 {
			continue
		}
		order, valid := solver.compareActivationSource(values[index-1], values[index])
		if !valid || order >= 0 {
			return false
		}
	}
	return true
}

// orderedSupportOutputs is the compile boundary proof for the immutable
// support projection. Its order is the canonical active-relation order, so a
// running action can locate a known relation without a second relation map.
func (solver *Solver) orderedSupportOutputs(values []compiledSupportOutput) bool {
	if solver == nil {
		return false
	}
	for index := range values {
		if values[index].slot < 0 || !solver.validActiveRelation(values[index].relation) {
			return false
		}
		if index > 0 && solver.compareActiveRelation(values[index-1].relation, values[index].relation) >= 0 {
			return false
		}
	}
	return true
}

func (solver *Solver) rulesByLocation(all []edgeOrigin) (map[edgeOrigin][]ruleDeclaration, map[templateTerm][]ruleDeclaration, map[*ruleIdentity]ruleDeclaration, bool) {
	if solver == nil {
		return nil, nil, nil, false
	}
	byEdge := make(map[templateEdge][]ruleDeclaration)
	local := make(map[templateTerm][]ruleDeclaration)
	relations := make(map[*ruleIdentity]ruleDeclaration)
	for _, declaration := range solver.rules {
		if declaration.base == nil || declaration.identity == nil || declaration.slot == nil || declaration.formal == nil || declaration.reads == nil || declaration.apply == nil {
			return nil, nil, nil, false
		}
		identity := declaration.identity()
		if identity == nil || !identity.sealed || !solver.validRuleAnchor(declaration.base.anchor) {
			return nil, nil, nil, false
		}
		if _, ok := declaration.slot(); !ok {
			return nil, nil, nil, false
		}
		switch declaration.base.anchor.form {
		case ruleFrom:
			key := templateEdge{shard: declaration.base.anchor.shard, edge: declaration.base.anchor.edge}
			byEdge[key] = append(byEdge[key], declaration)
		case ruleAt:
			key := templateTerm{shard: declaration.base.anchor.shard, term: declaration.base.anchor.term}
			local[key] = append(local[key], declaration)
		case ruleRelation:
			if _, duplicate := relations[identity]; duplicate {
				return nil, nil, nil, false
			}
			relations[identity] = declaration
		default:
			return nil, nil, nil, false
		}
	}
	for key, members := range byEdge {
		if !solver.sortRuleDeclarations(members) {
			return nil, nil, nil, false
		}
		byEdge[key] = members
	}
	for key, members := range local {
		if !solver.sortRuleDeclarations(members) {
			return nil, nil, nil, false
		}
		local[key] = members
	}
	result := make(map[edgeOrigin][]ruleDeclaration, len(all))
	for _, origin := range all {
		if members := byEdge[templateEdge{shard: origin.shard, edge: origin.edge}]; len(members) != 0 {
			result[origin] = members
		}
	}
	return result, local, relations, true
}

func (solver *Solver) sortRuleDeclarations(members []ruleDeclaration) bool {
	if solver == nil {
		return false
	}
	for _, member := range members {
		if member.identity == nil {
			return false
		}
		identity := member.identity()
		if identity == nil || !identity.sealed {
			return false
		}
		if _, ok := member.slot(); !ok {
			return false
		}
	}
	sort.Slice(members, func(left, right int) bool {
		leftSlot, _ := members[left].slot()
		rightSlot, _ := members[right].slot()
		if leftSlot != rightSlot {
			return leftSlot < rightSlot
		}
		return solver.compareRuleIdentity(members[left].identity(), members[right].identity()) < 0
	})
	for index := 1; index < len(members); index++ {
		prior, priorOK := members[index-1].slot()
		current, currentOK := members[index].slot()
		if !priorOK || !currentOK || prior == current {
			return false
		}
	}
	return true
}

func (solver *Solver) coordinateFor(origin termOrigin) (coordinate.Coordinate, bool) {
	if solver == nil || solver.coordinate == nil {
		return coordinate.Coordinate{}, false
	}
	if origin.candidate == (link.Candidate{}) {
		return solver.coordinate.InternRoot(origin.shard, origin.term)
	}
	return solver.coordinate.InternCandidate(origin.candidate, origin.shard, origin.term)
}

func (solver *Solver) activationFor(origin termOrigin) (program.Term, bool) {
	if solver == nil || solver.link == nil || origin.shard == 0 || origin.term == 0 {
		return 0, false
	}
	programValue, ok := solver.link.Program(origin.shard)
	if !ok || programValue == nil {
		return 0, false
	}
	activation, ok := programValue.Activation(origin.term)
	if !ok || activation == 0 {
		return 0, false
	}
	if origin.candidate == (link.Candidate{}) {
		entry, ok := programValue.Entry()
		return activation, ok && activation == entry
	}
	selectedShard, body, ok := solver.link.CandidateBody(origin.candidate)
	return activation, ok && selectedShard == origin.shard && activation == body
}

// edgeResetAtoms compiles the exact Program-owned decision interface of one
// real recurrence Edge under that Edge's existing activation provenance. The
// scheduler's components are intentionally irrelevant: an ordinary Edge, or
// a recurrence Edge with an empty Program interface, carries no reset.
func (solver *Solver) edgeResetAtoms(candidate link.Candidate, shard link.Shard, edge program.Edge) ([]guard.Atom, bool) {
	if solver == nil || shard == 0 || edge.From() == 0 || edge.To() == 0 {
		return nil, false
	}
	if _, recurring := edge.Mu(); !recurring {
		return nil, true
	}
	count, ok := edge.MuDecisionCount()
	if !ok {
		return nil, false
	}
	return solver.resetAtoms(candidate, shard, count, edge.MuDecisionAt)
}

// relationResetAtoms recognizes the one source-level activation boundary a
// resolved relation can prove without inventing a Link or scheduler
// correspondence: its primary Body Candidate is also its exact output at the
// selected body's Program entry. Other relation shapes retain every existing
// decision; in particular, an explicitly supplied Body of another Candidate
// is not a reset boundary.
func (solver *Solver) relationResetAtoms(relation activeRelation) ([]guard.Atom, bool) {
	if solver == nil || solver.link == nil {
		return nil, true
	}
	if relation.output.candidate != relation.candidate {
		// Generic Relation.Bind has no continuation correspondence. A Resume
		// cannot re-enter a different body activation until Program/Link names
		// that suspended-continuation boundary explicitly. This is an exact
		// typed application check, not a scheduling or recurrence inference.
		if relation.target != nil {
			if _, _, _, _, resume := solver.link.ResumeApplication(relation.target.anchor.application); resume {
				return nil, false
			}
		}
		return nil, true
	}
	shard, body, bodyOK := solver.link.CandidateBody(relation.candidate)
	if !bodyOK || relation.output.shard != shard {
		return nil, true
	}
	programValue, programOK := solver.link.Program(shard)
	if !programOK || programValue == nil {
		return nil, false
	}
	entry, entryOK := programValue.BodyEntry(body)
	if !entryOK {
		return nil, false
	}
	if relation.output.term != entry {
		return nil, true
	}
	count, countOK := programValue.ActivationDecisionCount(body)
	if !countOK {
		return nil, false
	}
	return solver.resetAtoms(relation.candidate, shard, count, func(index int) (program.Term, bool) {
		return programValue.ActivationDecisionAt(body, index)
	})
}

// resetAtoms translates an already-proven finite Program decision interface
// once at compilation. The strict packed order is both the compile boundary
// proof and the direct Fiber.Discharge input; evaluation never reconstructs,
// sorts, or allocates this interface.
func (solver *Solver) resetAtoms(candidate link.Candidate, shard link.Shard, count int, at func(int) (program.Term, bool)) ([]guard.Atom, bool) {
	if solver == nil || shard == 0 || count < 0 || at == nil {
		return nil, false
	}
	if count == 0 {
		return nil, true
	}
	atoms := make([]guard.Atom, count)
	var prior guard.Atom
	for index := range atoms {
		decision, ok := at(index)
		if !ok {
			return nil, false
		}
		atom, ok := solver.decisionAtom(candidate, shard, decision)
		if !ok || index != 0 && atom <= prior {
			return nil, false
		}
		atoms[index] = atom
		prior = atom
	}
	return atoms, true
}

func (solver *Solver) compareTermOrigin(left, right termOrigin) int {
	leftRoot := left.candidate == (link.Candidate{})
	rightRoot := right.candidate == (link.Candidate{})
	if leftRoot != rightRoot {
		if leftRoot {
			return -1
		}
		return 1
	}
	if !leftRoot {
		order, ok := solver.link.CompareCandidate(left.candidate, right.candidate)
		if !ok {
			return 0
		}
		if order != 0 {
			return order
		}
	}
	if left.shard < right.shard {
		return -1
	}
	if left.shard > right.shard {
		return 1
	}
	if left.term < right.term {
		return -1
	}
	if left.term > right.term {
		return 1
	}
	return 0
}

func (solver *Solver) compareActiveRelation(left, right activeRelation) int {
	if solver == nil || left.target == nil || right.target == nil {
		return 0
	}
	if order := solver.compareRuleIdentity(left.target, right.target); order != 0 {
		return order
	}
	if order, ok := solver.compareActivationSource(left.source, right.source); !ok || order != 0 {
		return order
	}
	if order, ok := solver.link.CompareCandidate(left.candidate, right.candidate); !ok || order != 0 {
		return order
	}
	if len(left.inputs) < len(right.inputs) {
		return -1
	}
	if len(left.inputs) > len(right.inputs) {
		return 1
	}
	for index := range left.inputs {
		if order := solver.compareTermOrigin(left.inputs[index], right.inputs[index]); order != 0 {
			return order
		}
	}
	return solver.compareTermOrigin(left.output, right.output)
}

func (solver *Solver) compareActivationSource(left, right activationSource) (int, bool) {
	if solver == nil || left.rule == nil || right.rule == nil || !left.caller.Valid() || !right.caller.Valid() {
		return 0, false
	}
	if order := solver.compareRuleIdentity(left.rule, right.rule); order != 0 {
		return order, true
	}
	leftCandidate, leftShard, leftTerm, leftOK := solver.coordinate.Semantic(left.caller)
	rightCandidate, rightShard, rightTerm, rightOK := solver.coordinate.Semantic(right.caller)
	if !leftOK || !rightOK {
		return 0, false
	}
	return solver.compareTermOrigin(termOrigin{candidate: leftCandidate, shard: leftShard, term: leftTerm}, termOrigin{candidate: rightCandidate, shard: rightShard, term: rightTerm}), true
}

func (solver *Solver) compareRuleIdentity(left, right *ruleIdentity) int {
	if left == nil || right == nil {
		return 0
	}
	if order := compareSemanticKey(left.semantic, right.semantic); order != 0 {
		return order
	}
	if order := compareSemanticKey(left.output, right.output); order != 0 {
		return order
	}
	if order, ok := solver.compareRuleAnchor(left.anchor, right.anchor); !ok || order != 0 {
		return order
	}
	if len(left.reads) < len(right.reads) {
		return -1
	}
	if len(left.reads) > len(right.reads) {
		return 1
	}
	for index := range left.reads {
		if left.reads[index].position < right.reads[index].position {
			return -1
		}
		if left.reads[index].position > right.reads[index].position {
			return 1
		}
		if order := compareSemanticKey(left.reads[index].factor, right.reads[index].factor); order != 0 {
			return order
		}
	}
	return 0
}

func (solver *Solver) compareRuleAnchor(left, right ruleAnchor) (int, bool) {
	if solver == nil || solver.link == nil {
		return 0, false
	}
	if left.form < right.form {
		return -1, true
	}
	if left.form > right.form {
		return 1, true
	}
	if left.inputArity < right.inputArity {
		return -1, true
	}
	if left.inputArity > right.inputArity {
		return 1, true
	}
	if left.form == ruleRelation {
		return solver.link.CompareApplication(left.application, right.application)
	}
	leftModule, rightModule := solver.link.ModuleKey(left.shard), solver.link.ModuleKey(right.shard)
	if !leftModule.Available() || !rightModule.Available() {
		return 0, false
	}
	if order := compareContentID(leftModule, rightModule); order != 0 {
		return order, true
	}
	if left.term < right.term {
		return -1, true
	}
	if left.term > right.term {
		return 1, true
	}
	if left.form != ruleFrom {
		return 0, true
	}
	if left.edge.From() < right.edge.From() {
		return -1, true
	}
	if left.edge.From() > right.edge.From() {
		return 1, true
	}
	if left.edge.To() < right.edge.To() {
		return -1, true
	}
	if left.edge.To() > right.edge.To() {
		return 1, true
	}
	leftDecision, leftTruthy, leftConditional := left.edge.Decision()
	rightDecision, rightTruthy, rightConditional := right.edge.Decision()
	if leftConditional != rightConditional {
		if !leftConditional {
			return -1, true
		}
		return 1, true
	}
	if leftDecision < rightDecision {
		return -1, true
	}
	if leftDecision > rightDecision {
		return 1, true
	}
	if leftTruthy != rightTruthy {
		if !leftTruthy {
			return -1, true
		}
		return 1, true
	}
	return 0, true
}

func (solver *Solver) sameApplication(left, right link.Application) bool {
	if solver == nil || solver.link == nil {
		return false
	}
	order, ok := solver.link.CompareApplication(left, right)
	return ok && order == 0
}

func compareSemanticKey(left, right SemanticKey) int {
	if order := compareContentID(left.ID, right.ID); order != 0 {
		return order
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func compareContentID(left, right program.ContentID) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func compactInts(values []int) []int {
	if len(values) < 2 {
		return values
	}
	write := 1
	for _, value := range values[1:] {
		if values[write-1] == value {
			continue
		}
		values[write] = value
		write++
	}
	return values[:write]
}

// bodyApplications projects only demanded Program activation equations; it
// never enumerates a project-wide Candidate product.
func (solver *Solver) bodyApplications(queries []termOrigin) ([]edgeOrigin, map[termOrigin][]edgeOrigin, bool) {
	if solver == nil || solver.link == nil || solver.bodies == nil || solver.activationBodies == nil {
		return nil, nil, false
	}
	seen := make(map[activationInstance]struct{}, len(queries))
	all := make([]edgeOrigin, 0)
	incoming := make(map[termOrigin][]edgeOrigin)
	for _, query := range queries {
		activation, ok := solver.activationFor(query)
		if !ok {
			return nil, nil, false
		}
		instance := activationInstance{candidate: query.candidate, shard: query.shard, activation: activation}
		if _, duplicate := seen[instance]; duplicate {
			continue
		}
		seen[instance] = struct{}{}
		edges, ok := solver.activationEdges(query.candidate, query.shard, activation)
		if !ok {
			return nil, nil, false
		}
		for _, origin := range edges {
			if origin.shard != query.shard || origin.candidate != query.candidate || origin.edge.From() == 0 || origin.edge.To() == 0 {
				return nil, nil, false
			}
			all = append(all, origin)
			target := termOrigin{candidate: origin.candidate, shard: origin.shard, term: origin.edge.To()}
			incoming[target] = append(incoming[target], origin)
		}
	}
	return all, incoming, true
}

type activationInstance struct {
	candidate  link.Candidate
	shard      link.Shard
	activation program.Term
}

type templateEdge struct {
	shard link.Shard
	edge  program.Edge
}

type templateTerm struct {
	shard link.Shard
	term  program.Term
}

func distinctFactorLogs(reads []ruleRead) []ruleRead {
	if len(reads) < 2 {
		return reads
	}
	result := make([]ruleRead, 0, len(reads))
	seen := make(map[int]struct{}, len(reads))
	for _, read := range reads {
		slot, ok := read.slot()
		if !ok {
			return nil
		}
		if _, duplicate := seen[slot]; duplicate {
			continue
		}
		seen[slot] = struct{}{}
		result = append(result, read)
	}
	return result
}

func (action *compiledAction) open(transaction *transaction) bool {
	if action == nil || transaction == nil {
		return false
	}
	for _, read := range action.logs {
		if read.begin == nil || !read.begin(transaction, action.equation) {
			return false
		}
	}
	return true
}

func (action *compiledAction) close(transaction *transaction) bool {
	if action == nil || transaction == nil {
		return false
	}
	for _, read := range action.logs {
		if read.seal == nil || !read.seal(transaction, action.equation) {
			return false
		}
	}
	return true
}
