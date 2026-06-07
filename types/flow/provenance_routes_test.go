package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestPointFactsProvenanceRoutesComposeIteratorAppendFieldSources(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(301), "entry")
	records := constraint.NewPath(cfg.SymbolID(302), "records")
	payload := constraint.NewPath(cfg.SymbolID(303), "payload")
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, entry),
			testStableAddressPath(t, records),
			ValueOriginIndexedIterator,
			1,
		),
		KeyPresence: KeyPresenceFacts{}.
			WithAppendHistoryBaseAddress(testStableAddressPath(t, records)).
			WithAppendElementFieldOriginFromAddresses(
				testStableAddressPath(t, records),
				field,
				testStableAddressPath(t, payload),
				nil,
			),
	}

	routes := PointFactsOf(state).ProvenanceRoutes(entry.Field("id"))
	if len(routes) != 2 {
		t.Fatalf("ProvenanceRoutes(entry.id) got %d routes, want append source + iterator source", len(routes))
	}
	if routes[0].Kind != ProvenanceRouteAppendElementField {
		t.Fatalf("first route kind = %v, want append-field source", routes[0].Kind)
	}
	if !routes[0].Source.Equal(payload) {
		t.Fatalf("append-field source = %v, want %v", routes[0].Source, payload)
	}
	if len(routes[0].FieldRemainder) != 0 {
		t.Fatalf("append-field remainder = %#v, want none", routes[0].FieldRemainder)
	}
	if routes[1].Kind != ProvenanceRouteIndexedIterator || routes[1].VarIndex != 1 {
		t.Fatalf("second route = %#v, want indexed iterator value route", routes[1])
	}
	if !routes[1].Source.Equal(records) {
		t.Fatalf("iterator source = %v, want %v", routes[1].Source, records)
	}
	if len(routes[1].Remainder) != 1 || routes[1].Remainder[0].Name != "id" {
		t.Fatalf("iterator remainder = %#v, want .id", routes[1].Remainder)
	}
}

func TestPointFactsProvenanceRoutesForDescendantAssignmentAliases(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(311), "target")
	pathAliasSource := constraint.NewPath(cfg.SymbolID(312), "path_source")
	assignmentSource := constraint.NewPath(cfg.SymbolID(313), "assignment_source")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, pathAliasSource),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, assignmentSource),
			ValueOriginAssignmentAlias,
			0,
		),
	}

	exact := PointFactsOf(state).ProvenanceRoutesFor(ProvenanceRouteQuery{
		Path:                target,
		IdentityAliases:     true,
		IdentityAliasPolicy: IdentityAliasDescendantOriginPolicy,
	})
	if len(exact) != 0 {
		t.Fatalf("exact descendant routes got %d, want none", len(exact))
	}
	descendant := PointFactsOf(state).ProvenanceRoutesFor(ProvenanceRouteQuery{
		Path:                target.Field("id"),
		IdentityAliases:     true,
		IdentityAliasPolicy: IdentityAliasDescendantOriginPolicy,
	})
	if len(descendant) != 1 {
		t.Fatalf("descendant routes got %d, want assignment alias only", len(descendant))
	}
	if descendant[0].Kind != ProvenanceRouteIdentityAlias {
		t.Fatalf("route kind = %v, want identity alias", descendant[0].Kind)
	}
	if !descendant[0].Source.Equal(assignmentSource.Field("id")) {
		t.Fatalf("route source = %v, want %v", descendant[0].Source, assignmentSource.Field("id"))
	}
}
