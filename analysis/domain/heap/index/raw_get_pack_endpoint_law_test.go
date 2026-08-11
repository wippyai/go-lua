package index

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestRawGetPayloadsUseExecutableOpenTailOffsets closes the real consumer
// boundary for Pack's offset denominator.  The Target contract has no
// formals, but the executable third field assignment selects offset two from
// an open call-result Pack.  Heap must consume that sealed selection without
// asking Target to predict Program syntax.
func TestRawGetPayloadsUseExecutableOpenTailOffsets(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "raw_get_open_tail_payload_law.lua", Text: []byte(`
local function many(...) return ... end
local record = {}
record.first, record.second, record.third = many()
return record.third
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "raw_get_open_tail_payload_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := staticdomain.Seal(linked, types)
	heap, heapOK := heapdomain.Seal(linked)
	values, valuesOK := valuedomain.Seal(linked, heap)
	calls, callsOK := calldomain.New(linked)
	packs, packsOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heap, values, calls)
	if !typesOK || staticErr != nil || !heapOK || !valuesOK || !callsOK || !packsOK || !topologyOK {
		t.Fatal("open-tail Heap/Pack denominator")
	}

	var third heapdomain.RawPayloadTag
	complete := heap.VisitRawPayloadTags(func(tag heapdomain.RawPayloadTag, payload heapdomain.Payload) bool {
		_, _, offset, source := payload.Source()
		if source && offset == 2 {
			third = tag
		}
		return true
	})
	if !complete || third == 0 {
		t.Fatal("executable third field assignment did not expose open-tail offset two")
	}
	payloads, _, ok := buildRawPayloads(topology, packs)
	row, found := payloadAt(payloads, third)
	root, rootOK := row.payload.Root()
	_, selectionOK := row.payload.Selection()
	if !ok || !found || row.kind != rawPayloadTail || !rootOK || !selectionOK {
		t.Fatalf("Heap did not retain Pack's sealed open-tail offset-two payload: build=%t tag=%d rows=%d found=%t kind=%d root=%t selection=%t", ok, third, len(payloads), found, row.kind, rootOK, selectionOK)
	}
	if _, ok := packs.Builder(root); !ok {
		t.Fatal("Heap payload root is not owned by the canonical Pack relation")
	}
}

// TestRawGetPackEndpointUsesValueAuthority proves that a selected exact Pack
// endpoint reaches RawGet through the existing Boundary Value selector. Pack
// carries only its sealed endpoint relation; it neither embeds nor reconstructs
// a Value fact.
func TestRawGetPackEndpointUsesValueAuthority(t *testing.T) {
	linked, heap, valuesSchema, _, packsSchema, _, _, _ := rawGetFieldFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(241), rawGetKey(242), valuesSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(243), packsSchema)
	if !valuesOK || !packsOK {
		t.Fatal("declare Pack and Value authorities")
	}
	source, sourceOK := linked.Boundary().Values().At(0)
	endpoint, endpointOK := packsSchema.Endpoint(source)
	root, rootOK := packsSchema.RootAt(0)
	builder, builderOK := packsSchema.Builder(root)
	scalar, scalarOK := builder.Endpoint(endpoint)
	if !sourceOK || !endpointOK || !rootOK || !builderOK || !scalarOK {
		t.Fatal("Pack endpoint scalar denominator")
	}
	stringAtom, atomOK := valuesSchema.OpaqueKind(runtimekind.String)
	input, inputOK := valuesSchema.Singleton(stringAtom)
	none, containmentOK := heap.ContainmentNone()
	if !atomOK || !inputOK || !containmentOK {
		t.Fatal("Value source fact denominator")
	}
	const tag rawSourceTag = 1
	payload := rawPayload{kind: rawPayloadTail, byValue: map[linkboundary.Value]rawSourceTag{source: tag}}
	// This is the exact source descriptor produced by buildRawPayloads for a
	// payload whose selected Pack expression names source.  The law isolates
	// the consumer boundary: no Pack fact is smuggled into RawGet.
	rule := &RawGetRule{values: values, packs: packs, sources: []rawSource{{value: source}}}
	resolved, resolvedOK := packsSchema.ScalarSource(scalar)
	if !resolvedOK || resolved != source {
		t.Fatal("Pack endpoint did not resolve to its canonical Link source")
	}
	view := rawGetView{source: func(want rawSourceTag) rawSelected[valuedomain.Value] {
		if want != tag {
			return rawSelected[valuedomain.Value]{valid: true}
		}
		return rawSelected[valuedomain.Value]{value: input, present: true, found: true, valid: true}
	}}
	result, present := valuesSchema.Bottom(), false
	applied := rule.applyScalar(view, payload, scalar, none, &result, &present)
	if !applied || !present || !valuesSchema.Equal(result, input) {
		t.Fatalf("exact Pack endpoint did not retain its Value-authority fact: applied=%t present=%t equal=%t", applied, present, valuesSchema.Equal(result, input))
	}
	missing := rawPayload{kind: rawPayloadTail, byValue: map[linkboundary.Value]rawSourceTag{}}
	if rule.applyScalar(view, missing, scalar, none, &result, &present) {
		t.Fatal("endpoint without its exact Value-selector route was approximated")
	}
}
