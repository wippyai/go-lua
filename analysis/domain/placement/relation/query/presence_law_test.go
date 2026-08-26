package query_test

import (
	"reflect"
	"testing"
	"unsafe"

	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	fixtureplacement "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/allocationbirth"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	projection "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestPlacementFactRowsKeepPresenceSeparateFromThePlacementLattice exercises
// every canonical cell status through the owner query boundary.  The fixture
// is solved once; each subtest only rebuilds the immutable canonical snapshot
// with the same declaration axes and denominator members.
func TestPlacementFactRowsKeepPresenceSeparateFromThePlacementLattice(t *testing.T) {
	fixture := fixtureplacement.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("placement solve")
	}
	published, ok := projection.Publish(result, fixture.View())
	if !ok || !published.Available() {
		t.Fatal("placement canonical projection")
	}
	ids := fixture.IDs()
	column, ok := placementquery.NewFactColumn(ids.OutputPayload, fixture.PlacementCodec())
	if !ok {
		t.Fatal("placement fact column")
	}
	keys := published.Keys(ids.OutputPayload)
	if len(keys) != 1 || !keys[0].Available() || keys[0].Relation != ids.Output || !keys[0].Scope.Available() {
		t.Fatalf("output denominator keys = %#v, want one scoped member", keys)
	}
	key := keys[0]

	original, status := published.Read(ids.OutputPayload, key)
	if status != canonical.ReadHit || !original.Available() || !original.Value.Available() || !original.Lineage.Available() {
		t.Fatalf("original output cell status=%s available=%v value=%v lineage=%v", status, original.Available(), original.Value.Available(), original.Lineage.Available())
	}
	decoded, ok := fixture.PlacementCodec().Decode(original.Value)
	if !ok || !placementdomain.EqualFact(decoded, placementdomain.DefaultFact()) {
		t.Fatalf("owner Placement decode = %#v/%v, want DefaultFact", decoded, ok)
	}

	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("present presence")
	}
	present := original
	present.Presence = presence

	opaque, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		t.Fatal("opaque presence")
	}
	authenticatedOpaque := original
	authenticatedOpaque.Presence = opaque

	missing, ok := model.NewPresence(model.UnprovenMissing)
	if !ok {
		t.Fatal("missing presence")
	}
	unprovenMissing := original
	unprovenMissing.Presence = missing
	// A missing cell is explicitly value-less.  Keep the owner lineage and
	// logical address; only the semantic value is unavailable.
	unprovenMissing.Value = zeroValueToken()

	refusalContent, ok := identity.DeriveContentID("placement/query/refused-presence")
	if !ok {
		t.Fatal("refusal content")
	}
	refusal, ok := model.IssueRefusalID(ids.PlacementOwner, refusalContent)
	if !ok {
		t.Fatal("refusal identity")
	}
	refusedPresence, ok := model.NewRefused(refusal)
	if !ok {
		t.Fatal("refused presence")
	}
	refused := original
	refused.Presence = refusedPresence
	refused.Value = zeroValueToken()

	tests := []struct {
		name        string
		cell        *projection.Cell
		wantStatus  canonical.ReadStatus
		wantKind    model.PresenceKind
		wantFact    bool
		wantLineage bool
	}{
		{name: "present default fact", cell: &present, wantStatus: canonical.ReadHit, wantKind: model.Present, wantFact: true, wantLineage: true},
		{name: "sparse proven absence", cell: nil, wantStatus: canonical.ReadProvenAbsent, wantKind: model.ProvenAbsent, wantFact: false, wantLineage: false},
		{name: "authenticated opaque", cell: &authenticatedOpaque, wantStatus: canonical.ReadHit, wantKind: model.AuthenticatedOpaque, wantFact: true, wantLineage: true},
		{name: "unproven missing", cell: &unprovenMissing, wantStatus: canonical.ReadHit, wantKind: model.UnprovenMissing, wantFact: false, wantLineage: true},
		{name: "refused", cell: &refused, wantStatus: canonical.ReadHit, wantKind: model.Refused, wantFact: false, wantLineage: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := canonicalContent(t, published, ids.OutputPayload, key, test.cell)
			rebuilt := rebuildProjection(t, published, ids.OutputPayload, content)
			members := rebuilt.Keys(ids.OutputPayload)
			if len(members) != 1 || members[0] != key || !members[0].Scope.Same(key.Scope) {
				t.Fatalf("rebuilt denominator members = %#v, want exact scoped key %#v", members, key)
			}
			row, gotStatus, ok := placementquery.ReadOne(rebuilt, column, key)
			if !ok || gotStatus != test.wantStatus || !row.Available() {
				t.Fatalf("row = ok:%v status:%s available:%v, want ok/status/available=%v/%s/true", ok, gotStatus, row.Available(), true, test.wantStatus)
			}
			if row.Key() != key || !row.Key().Scope.Same(key.Scope) || row.Key().Relation != ids.Output {
				t.Fatalf("row key = %#v, want exact output key %#v", row.Key(), key)
			}
			if !row.Presence().Is(test.wantKind) || row.HasLineage() != test.wantLineage {
				t.Fatalf("presence/lineage = %s/%v, want %s/%v", row.Presence().Kind(), row.HasLineage(), test.wantKind, test.wantLineage)
			}
			fact, hasFact := row.Fact()
			if hasFact != test.wantFact {
				t.Fatalf("Fact presence = %v, want %v (fact=%#v)", hasFact, test.wantFact, fact)
			}
			if test.wantFact {
				if !placementdomain.EqualFact(fact, placementdomain.DefaultFact()) {
					t.Fatalf("decoded fact = %#v, want DefaultFact", fact)
				}
			} else if fact != (placementdomain.Fact{}) {
				// The zero value is returned with ok=false.  In particular, a
				// zero Fact may have the same field bits as BottomFact, but it
				// is not an admitted semantic result without the presence bit.
				t.Fatalf("absence/missing/refusal returned non-zero fact %#v", fact)
			}
		})
	}
}

// canonicalContent returns one replacement content for the target axis. A
// nil cell intentionally leaves the denominator member without a row, which
// is the canonical proof of ProvenAbsent rather than a fabricated value cell.
func canonicalContent(t *testing.T, published projection.Projection, id model.ColumnID, key projection.RowKey, cell *projection.Cell) canonical.Content[projection.RowKey, projection.Cell] {
	t.Helper()
	column, ok := published.Column(id)
	if !ok {
		t.Fatal("target projection column")
	}
	rows := make(map[projection.RowKey]projection.Cell)
	if cell != nil {
		if !cell.Available() {
			t.Fatalf("replacement cell unavailable: %#v", *cell)
		}
		rows[key] = *cell
	}
	return canonical.Content[projection.RowKey, projection.Cell]{Rows: rows, Denominator: column.DenominatorID, Members: []projection.RowKey{key}}
}

// rebuildProjection copies every declared axis into one same-anchor canonical
// publication and replaces only the target content. The returned projection
// still owns exactly one immutable canonical snapshot; the reflection bridge
// is confined to this law because Projection intentionally has no mutation or
// alternate-snapshot production API.
func rebuildProjection(t *testing.T, published projection.Projection, target model.ColumnID, replacement canonical.Content[projection.RowKey, projection.Cell]) projection.Projection {
	t.Helper()
	builder := canonical.NewBuilder(published.Schema(), published.Store(), published.Generation())
	for _, column := range published.Columns() {
		content := canonical.Content[projection.RowKey, projection.Cell]{
			Rows:        make(map[projection.RowKey]projection.Cell),
			Denominator: column.DenominatorID,
			Members:     published.Keys(column.ID),
		}
		if column.ID == target {
			content = replacement
		}
		if column.ID != target {
			for _, member := range content.Members {
				cell, status := published.Read(column.ID, member)
				if status == canonical.ReadHit && cell.Available() {
					content.Rows[member] = cell
				}
			}
		}
		if err := canonical.PutColumn(&builder, column.Axis(), content); err != nil {
			t.Fatalf("rebuild column %v: %v", column.ID, err)
		}
		if err := builder.Publish(column.PublicationID, column.Axis().Slot); err != nil {
			t.Fatalf("rebuild publication %v: %v", column.ID, err)
		}
	}
	sealed, err := builder.Seal()
	if err != nil || !sealed.Published() {
		t.Fatalf("rebuild seal: %v", err)
	}

	result := published
	field := reflect.ValueOf(&result).Elem().FieldByName("published")
	if !field.IsValid() || !field.CanAddr() {
		t.Fatal("projection canonical field unavailable")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(sealed))
	if !result.Available() {
		t.Fatal("rebuilt projection unavailable")
	}
	return result
}

// ValueToken has no exported constructor for an unavailable token; the zero
// value is the canonical no-value representation required by sparse cells.
func zeroValueToken() binding.ValueToken { return binding.ValueToken{} }
