package signature

import (
	"reflect"
	"testing"
)

func TestOperationalEffectLaneRegistryCoversEveryField(t *testing.T) {
	typ := reflect.TypeOf(OperationalEffects{})
	registered := make(map[string]struct{})
	for _, lane := range operationalEffectLanes {
		if lane.fieldName == "" {
			t.Fatal("operational effect lane with empty field name")
		}
		if _, ok := registered[lane.fieldName]; ok {
			t.Fatalf("operational effect lane %s registered more than once", lane.fieldName)
		}
		field, ok := typ.FieldByName(lane.fieldName)
		if !ok {
			t.Fatalf("operational effect lane references missing field %s", lane.fieldName)
		}
		if !operationalEffectsLaneFieldKind(field.Type.Kind()) {
			t.Fatalf("operational effect lane %s references unsupported field kind %s", lane.fieldName, field.Type.Kind())
		}
		registered[lane.fieldName] = struct{}{}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !operationalEffectsLaneFieldKind(field.Type.Kind()) {
			continue
		}
		if _, ok := registered[field.Name]; !ok {
			t.Fatalf("OperationalEffects.%s has no registered lane owner", field.Name)
		}
	}
}

func operationalEffectsLaneFieldKind(kind reflect.Kind) bool {
	return kind == reflect.Bool || kind == reflect.Slice
}

func TestOperationalEffectLaneRegistryOwnsTypeSubstitution(t *testing.T) {
	wantTyped := map[string]struct{}{
		"NormalReturnTypeRefinements": {},
		"PathPresenceImplications":    {},
		"PathStaticMembers":           {},
		"PathStaticMemberDeltas":      {},
		"DynamicIndexFacts":           {},
		"ReturnAllocationTemplates":   {},
	}
	for _, lane := range operationalEffectLanes {
		_, want := wantTyped[lane.fieldName]
		got := lane.substituteTypes != nil
		if got != want {
			t.Fatalf("operational effect lane %s substituteTypes present = %v, want %v", lane.fieldName, got, want)
		}
		delete(wantTyped, lane.fieldName)
	}
	for field := range wantTyped {
		t.Fatalf("typed operational effect field %s has no lane owner", field)
	}
}
