package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestSchemaExactQueryOwnsOneProjectionAndBinding(t *testing.T) {
	schema, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("exact query binding did not seal")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	row, rowOK := implementation.sealedRow()
	projection, projectionOK := schema.queryProjectionShapeAt(row.ordinal, 0)
	if !ok || implementation == nil || !rowOK || !projectionOK || projection.Factor != compositionKeyOf(coldKey(948_001)) {
		t.Fatal("exact query implementation lost its sealed factor authority")
	}
	project := row.projection.project
	if project == nil || project(OrderedCells[uint64]{}) != 0 {
		t.Fatal("exact query implementation did not retain one projector")
	}
}

func TestSchemaExactQueryRejectsForeignEqualBinding(t *testing.T) {
	schema, factor, _ := exactQuerySchemaFixture(t)
	foreignSchema, _, foreignQuery := exactQuerySchemaFixture(t)
	if schema.ID() != foreignSchema.ID() {
		t.Fatal("equal exact schemas did not canonicalize")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || BindExactQuery(binding, foreignQuery, factor, hotExactQuerySpec()) || !binding.Poisoned() {
		t.Fatal("foreign equal-schema query crossed the binding authority")
	}
}

func TestSchemaExactQueryRejectsSummaryAndSupportRows(t *testing.T) {
	schema, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Poisoned() {
		t.Fatal("duplicate exact query projection was admitted")
	}
	summarySchema, summaryFactor, summaryForm, summaryQuery := summaryQueryLawSchema(t)
	summaryBinding := NewSchemaBinding(summarySchema)
	if summaryBinding == nil || !BindFactor(summaryBinding, summaryFactor, hotUintFactorSpec()) || BindExactQuery(summaryBinding, summaryQuery, summaryFactor, hotExactQuerySpec()) || !summaryBinding.Poisoned() || summaryForm.Kind() == SchemaFormReadExact {
		t.Fatal("summary projection crossed the exact query boundary")
	}
}

// TestCommittedExactQueryPublishesOneEvidenceSurface maps the old graph
// evidence join onto the sealed program query table: the implementation's
// exact factor/form authority is the same surface the committed Query row
// publishes, with no alternate factor or support projection.
func TestCommittedExactQueryPublishesOneEvidenceSurface(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	query, queryOK := fixture.graph.Query(programMatrixID(110))
	if !queryOK {
		t.Fatal("committed exact query handle")
	}
	locator, locatorOK := fixture.graph.directory.query(programMatrixID(110))
	identity, identityOK := locator.Resolve(fixture.graph.graph)
	surfaces := identity.Surfaces()
	if !locatorOK || !identityOK || !fixture.graph.graph.OwnsQuery(identity) || identity.Family() != compositionKeyOf(coldKey(953_000)) || len(surfaces) != 1 {
		t.Fatal("committed exact query evidence row")
	}
	surface := surfaces[0]
	if surface.Factor != compositionKeyOf(coldKey(951_000)) || surface.Form != equation.SurfaceReadExact || surface.Local != 1 || surface.Semantic.Available() || surface.Normalizer.Available() {
		t.Fatal("committed exact query published the wrong surface")
	}
	key, keyed := query.PublicationKey()
	if !keyed || !key.Available() {
		t.Fatal("committed exact query publication key")
	}
	program := fixture.solver.runtime.program
	for index := 0; index < program.queryCount(); index++ {
		row, rowOK := program.queryAt(index)
		graphQuery, graphQueryOK := fixture.graph.graph.QueryAt(index)
		if rowOK && graphQueryOK && graphQuery.Key() == identity.Key() {
			projection, projectionOK := fixture.graph.state.schema.queryProjectionShapeAt(row.queryOrdinal, 0)
			if !projectionOK || projection.Kind != composition.QueryFactorExact || projection.Factor != surface.Factor || row.factorOrdinal >= uint64(program.factorCount()) {
				t.Fatal("runtime exact query evidence diverged from committed surface")
			}
			return
		}
	}
	t.Fatal("committed exact query runtime row")
}

// TestProgramExactQueryEvidenceRejectsForeignBindingAndGraph maps the old
// compiler join refusal onto the current sealed Factor plane.  The exact
// implementation and the graph Query must share both immutable authorities;
// an equal-shaped foreign seal or an unavailable Query cannot enter the row.
func TestProgramExactQueryEvidenceRejectsForeignBindingAndGraph(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	query, queryOK := fixture.graph.Query(programMatrixID(110))
	plane, _, planeOK := bindProgramPlane(fixture.graph.state, fixture.graph.graph)
	implementation := fixture.queryImplementations[0]
	if !queryOK || !planeOK || plane == nil || implementation == nil || !plane.attachQueryContext(fixture.graph) {
		t.Fatal("exact query evidence plane")
	}
	if joined, accepted := implementation.bindProgramQuery(plane, query.identity); !accepted || !joined.valid() {
		t.Fatal("canonical exact query evidence refused")
	}
	foreign := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	if _, accepted := foreign.queryImplementations[0].bindProgramQuery(plane, query.identity); accepted {
		t.Fatal("equal-shaped foreign exact query binding entered the program plane")
	}
	if _, accepted := implementation.bindProgramQuery(plane, equation.Query{}); accepted {
		t.Fatal("unavailable/foreign exact query identity entered the program plane")
	}
}

func hotSummaryQueryLawSpec() HotSummaryQuerySpec[uint64, uint64] {
	return HotSummaryQuerySpec[uint64, uint64]{
		Project: func(cells OrderedCells[uint64]) uint64 { return uint64(cells.Count()) },
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(953_100),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
			Present:     func(uint64) bool { return true },
		},
	}
}
