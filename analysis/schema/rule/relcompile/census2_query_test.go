package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// The four registered query families and the selected-point denominator they
// are all keyed by.
//
// domain/composite/query_table.go registers exactly four families: value's
// coordinatewise summary, effect's exact read, placement's heterogeneous
// summary and call's callee-set observation. Each declaration states a family
// identity, the subject axes it reads, the Artifact population it is asked at
// and the fold contract its answers compose under; none of them states a
// relational plan, and the population itself is recomputed by a hand-staged
// fixpoint in domain/composite/query_sites.go.
//
// The plans below are those declarations read as relations. Two statements the
// declarations make are load-bearing here and are stated relationally rather
// than as engine behaviour:
//
//   - a family is asked at a selected point, so every family's candidate is the
//     one query_site relation and never a population of its own;
//   - a family's Fold contract is its delivery contract, so a distributive
//     coordinatewise fold is a complete-span input keyed by the grouping the
//     family reduces under, and an exact family is a scalar input.
//
// A join names both sides' declared address columns. The address column is the
// key vector the relation's owner declares its rows are identified by, so a
// correspondence, parent or occurrence read is this same equijoin over ordinary
// relation data.

// census2QuerySitePlan is A0: the selected-point denominator every family is
// keyed by. Today it is a hand-staged worklist fixpoint with a hand digest and
// a hand cardinality check; here it is six ordinary derivations over two
// recursive relations and one closed product.
//
// The two recurrences are the whole of the staging: a call's target body is
// selected because its owner body is, and a region member's point is observed
// because the region's point is. Both are positive, so they are SCC heads in
// the dependency graph and never staged phases of an executor.
//
// The two predicates the hand fixpoint spells as field tests - a body that is
// not callable, and a selected body that holds no occurrence - are joins onto
// the fact relations that state them. A predicate over declared facts is an
// equijoin; it is not a filter callback.
func census2QuerySitePlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)

	body := plane.composite("composite/body")
	nonCallableBody := plane.composite("composite/non-callable-body")
	call := plane.composite("composite/call")
	moduleEntryRoot := plane.composite("composite/module-entry-root-function")
	callableFunction := plane.composite("composite/callable-function")
	occurrence := plane.composite("composite/occurrence")
	regionMember := plane.composite("composite/region-member")
	bodyWithoutOccurrence := plane.composite("composite/body-without-occurrence")
	bodyEntry := plane.composite("composite/body-entry")
	eligibleContext := plane.composite("composite/eligible-context")
	queryFamily := plane.query("query/family")

	selectedBody := plane.composite("composite/selected-body")
	observedPoint := plane.composite("composite/observed-point")
	querySite := plane.query("query/query-site")

	plane.add(census2Step{
		Key: "query-site/selected-body-root", Candidate: body,
		Joins:   []relcompile.JoinSpec{plane.join(body, nonCallableBody)},
		Publish: selectedBody,
	})
	plane.add(census2Step{
		Key: "query-site/selected-body-call-target", Candidate: call,
		Joins:   []relcompile.JoinSpec{plane.join(call, selectedBody)},
		Publish: selectedBody,
	})
	plane.add(census2Step{
		Key: "query-site/selected-body-module-entry", Candidate: moduleEntryRoot,
		Joins:   []relcompile.JoinSpec{plane.join(moduleEntryRoot, callableFunction)},
		Publish: selectedBody,
	})
	plane.add(census2Step{
		Key: "query-site/observed-point-occurrence", Candidate: occurrence,
		Joins:   []relcompile.JoinSpec{plane.join(occurrence, selectedBody)},
		Publish: observedPoint,
	})
	plane.add(census2Step{
		Key: "query-site/observed-point-region-member", Candidate: regionMember,
		Joins:   []relcompile.JoinSpec{plane.join(regionMember, observedPoint)},
		Publish: observedPoint,
	})
	plane.add(census2Step{
		Key: "query-site/observed-point-body-entry", Candidate: selectedBody,
		Joins: []relcompile.JoinSpec{
			plane.join(selectedBody, bodyWithoutOccurrence),
			plane.join(selectedBody, bodyEntry)},
		Publish: observedPoint,
	})
	// The denominator is the product of every registered family and every
	// observed point of an eligible context: each read closes over its own
	// authenticated denominator, so the closed product is the join of two
	// complete spans and never a cardinality arithmetic the executor repeats.
	plane.add(census2Step{
		Key: "query-site/site", Candidate: eligibleContext,
		Joins: []relcompile.JoinSpec{
			plane.completedJoin(eligibleContext, observedPoint),
			plane.completedJoin(eligibleContext, queryFamily)},
		Publish: querySite,
	})
	return plane
}

// census2QuerySiteOf declares the denominator relation and the family
// registration row one family's plan is asked over. The family row is the
// query.Registration itself: selecting the sites of one family is an equijoin
// onto the registration, not a family tag the engine tests.
func census2QuerySiteOf(plane *census2Plane, family schema.Key) (site relcompile.Name, registration relcompile.Name) {
	return plane.query("query/query-site"), plane.query(family)
}

// census2ValueSummaryPlan is A1: the only widenable family.
//
// value/owner declares FoldDistributive under the coordinatewise summary
// contract, over the single "value" subject. Distributive is the statement that
// the family may be answered over disjoint fragments of its subject and joined,
// which is exactly a grouped reduction: the fold's input is the complete span
// of the value cells one query site selects, delivered under the grouping key,
// and its output is closed against the value schema's coordinate plane.
func census2ValueSummaryPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	site, registration := census2QuerySiteOf(plane, "query/value-summary")
	cells := plane.axis("value", "value/summary-cells")
	coordinates := plane.axis("value", "value/summary-coordinates")
	answer := plane.query("query/value-summary/answer")
	fold := plane.foldOperation(plane.axis("value", "value/summary-coordinatewise"), answer, cells, cells)

	plane.add(census2Step{
		Key: "query/value-summary", Candidate: site,
		Joins: []relcompile.JoinSpec{
			plane.join(site, registration),
			plane.completedJoin(site, cells)},
		Complete: census2Ref(coordinates),
		Apply:    fold,
		ApplySlots: []relcompile.ReadOccurrence{
			relcompile.JoinOccurrence(1), // complete value-cell read
		},
		Publish: answer,
	})
	return plane
}

// census2EffectExactPlan is A2: the exact family.
//
// effect/owner declares FoldGeneral and admits no split of its subject: the
// answer is produced from the one cell the site selects. The runtime guard that
// refuses a read of any other cardinality is that statement made twice; here it
// is the sealed signature's scalar delivery and exactly-one cardinality, and
// the answer relation is closed against the site denominator so one site has
// one answer.
func census2EffectExactPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	site, registration := census2QuerySiteOf(plane, "query/effect-exact")
	cells := plane.axis("effect", "effect/cells")
	answer := plane.query("query/effect-exact/answer")
	accumulate := plane.exactOperation(plane.axis("effect", "effect/exact-accumulate"), answer, cells)

	plane.add(census2Step{
		Key: "query/effect-exact", Candidate: site,
		Joins: []relcompile.JoinSpec{
			plane.join(site, registration),
			plane.join(site, cells)},
		Complete: census2Ref(site),
		Apply:    accumulate,
		ApplySlots: []relcompile.ReadOccurrence{
			relcompile.JoinOccurrence(1), // effect-cell read
		},
		Publish: answer,
	})
	return plane
}

// census2PlacementSummaryPlan is A3: the one correspondence join.
//
// placement/query declares three subjects - the placement class, the complete
// heap vector and suspension evidence - and binds them only after proving
// placementSchema.Heap() == heapSchema and placementSchema == evidenceSchema.
// That precondition is a correspondence between three owners' schemas, and a
// correspondence is an ordinary relation joined by the ordinary equijoin. The
// complete heap vector its containment evidence depends on is the heap read's
// own authenticated denominator.
//
// The family answers a parent row and one typed child row per allocation root,
// so it is two derivations: the child closed against the allocation-root
// denominator, and the parent closed against the site.
func census2PlacementSummaryPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	site, registration := census2QuerySiteOf(plane, "query/placement-summary")
	placementCells := plane.axis("placement", "placement/cells")
	heapCells := plane.axis("heap", "heap/cells")
	evidenceCells := plane.axis("placement-suspension-evidence", "placement/suspension-evidence-cells")
	placementHeap := plane.composite("corr/placement-heap")
	placementEvidence := plane.composite("corr/placement-evidence")
	allocationRoots := plane.axis("placement", "placement/allocation-roots")
	child := plane.query("query/placement-summary/allocation")
	parent := plane.query("query/placement-summary/answer")
	containment := plane.foldOperation(plane.axis("placement", "placement/summary-containment"), child, heapCells, heapCells)
	summary := plane.exactOperation(plane.axis("placement", "placement/summary-parent"), parent, child)

	plane.add(census2Step{
		Key: "query/placement-summary-allocation", Candidate: site,
		Joins: []relcompile.JoinSpec{
			plane.join(site, registration),
			plane.join(site, placementCells),
			plane.completedJoin(site, heapCells),
			plane.join(heapCells, placementHeap),
			plane.join(placementCells, placementEvidence),
			plane.join(site, evidenceCells)},
		Complete: census2Ref(allocationRoots),
		Apply:    containment,
		ApplySlots: []relcompile.ReadOccurrence{
			relcompile.JoinOccurrence(2), // complete heap-cell read
		},
		Publish: child,
	})
	plane.add(census2Step{
		Key: "query/placement-summary", Candidate: site,
		Joins: []relcompile.JoinSpec{
			plane.join(site, registration),
			plane.completedJoin(site, child)},
		Complete: census2Ref(site),
		Apply:    summary,
		ApplySlots: []relcompile.ReadOccurrence{
			relcompile.JoinOccurrence(1), // allocation-summary child
		},
		Publish: parent,
	})
	return plane
}

// census2CalleeSetPlan is A4: the observation population.
//
// call/query declares PopulationObservation: the family is attached only where
// a caller asks for the observation and it publishes no selected-point answer.
// Its plan is therefore the read-only shape - it ends in the relation its
// consumer reads from the converged snapshot and proposes nothing - and its
// population is the observation site rather than the query site the other three
// families share.
func census2CalleeSetPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	site := plane.observation("observation/call-callee-set-site")
	cells := plane.axis("call", "call/cells")
	classify := plane.exactOperation(plane.axis("call", "call/dispatch-classify-callee-set"), site, cells)

	plane.add(census2Step{
		Key: "observation/call-callee-set", Candidate: site,
		Joins:    []relcompile.JoinSpec{plane.join(site, cells)},
		Complete: census2Ref(site),
		Apply:    classify,
		ApplySlots: []relcompile.ReadOccurrence{
			relcompile.JoinOccurrence(0), // call-cell read
		},
	})
	return plane
}
