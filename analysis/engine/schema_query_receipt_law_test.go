package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestSchemaExactQueryOwnsOneProjectionAndBinding(t *testing.T) {
	schema, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("exact query binding did not seal")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil || !implementation.binding.valid() || implementation.binding.factor != compositionKeyOf(coldKey(948_001)) {
		t.Fatal("exact query implementation lost its sealed factor authority")
	}
	project, projectOK := implementation.projector()
	if !projectOK || project == nil || project(OrderedCells[uint64]{}) != 0 {
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
	for _, row := range fixture.solver.runtime.queries {
		if row != nil && row.query().Key() == identity.Key() {
			typed, typedOK := row.(*receiptQueryRuntime[uint64, uint64])
			if !typedOK || typed.surface.Form != equation.SurfaceReadExact || typed.surface.Factor != surface.Factor {
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
	plane, planeOK := bindProgramPlane(fixture.graph.state, fixture.graph.graph)
	implementation := fixture.queryImplementations[0]
	if !queryOK || !planeOK || plane == nil || implementation == nil {
		t.Fatal("exact query evidence plane")
	}
	if joined, accepted := bindReceiptExactQuery[uint64, uint64](plane, implementation, query.identity); !accepted || joined == nil {
		t.Fatal("canonical exact query evidence refused")
	}
	foreign := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	if _, accepted := bindReceiptExactQuery[uint64, uint64](plane, foreign.queryImplementations[0], query.identity); accepted {
		t.Fatal("equal-shaped foreign exact query binding entered the program plane")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](plane, implementation, equation.Query{}); accepted {
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
