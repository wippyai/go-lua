package callpayload

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestCallOutcomeEmptyAndPostReturnEvidenceCoverEveryLane(t *testing.T) {
	typ := reflect.TypeOf(CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		lane, ok := callOutcomeLaneByField(field.Name)
		if !ok {
			t.Fatalf("CallOutcome.%s has no registered lane", field.Name)
		}
		outcome := callOutcomeWithOneField(t, field.Name)
		if !lane.has(outcome) {
			t.Fatalf("CallOutcome lane %s ignored its field", field.Name)
		}
		if outcome.Empty() {
			t.Fatalf("CallOutcome.Empty ignored field %s", field.Name)
		}

		wantPostReturn := lane.postReturn
		if got := outcome.HasPostReturnEvidence(); got != wantPostReturn {
			t.Fatalf("CallOutcome.HasPostReturnEvidence(%s) = %v, want %v", field.Name, got, wantPostReturn)
		}
	}
}

func TestCallOutcomeLaneRegistryCoversEveryFieldOnce(t *testing.T) {
	fields := make(map[string]struct{})
	typ := reflect.TypeOf(CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		fields[typ.Field(i).Name] = struct{}{}
	}

	registered := make(map[string]struct{})
	for _, lane := range callOutcomeLanes {
		if lane.fieldName == "" {
			t.Fatal("call outcome lane with empty field name")
		}
		if lane.has == nil {
			t.Fatalf("call outcome lane %s has nil presence predicate", lane.fieldName)
		}
		if _, ok := registered[lane.fieldName]; ok {
			t.Fatalf("call outcome lane %s registered more than once", lane.fieldName)
		}
		if _, ok := fields[lane.fieldName]; !ok {
			t.Fatalf("call outcome lane references missing field %s", lane.fieldName)
		}
		registered[lane.fieldName] = struct{}{}
	}
	for field := range fields {
		if _, ok := registered[field]; !ok {
			t.Fatalf("CallOutcome.%s has no registered lane", field)
		}
	}
}

func TestCallOutcomeSupplementalFactRolesExcludeResultAndAuthority(t *testing.T) {
	roles := CallOutcomeSupplementalFactRoles()
	if len(roles) != len(callOutcomeLanes)-2 {
		t.Fatalf("supplemental fact roles = %d, want call outcome lanes minus result/authority = %d", len(roles), len(callOutcomeLanes)-2)
	}
	for _, role := range roles {
		switch role.FieldName {
		case "Results", "PostReturnAuthority":
			t.Fatalf("%s must be handled outside supplemental fact roles", role.FieldName)
		}
	}
}

func TestBindCallOutcomeSupplementalFactRolesUsesRoleOrder(t *testing.T) {
	handlers := callOutcomeSupplementalFactRoleTestHandlers()
	bindings := BindCallOutcomeSupplementalFactRoles("test", handlers, func(v int) bool { return v > 0 })
	roles := CallOutcomeSupplementalFactRoles()
	if len(bindings) != len(roles) {
		t.Fatalf("bindings = %d, want roles = %d", len(bindings), len(roles))
	}
	for i, binding := range bindings {
		if binding.Role != roles[i] {
			t.Fatalf("binding %d role = %#v, want %#v", i, binding.Role, roles[i])
		}
		if binding.Value != i+1 {
			t.Fatalf("binding %d value = %d, want %d", i, binding.Value, i+1)
		}
	}
}

func TestBindCallOutcomeSupplementalFactRolesRejectsMissingInvalidAndOrphanHandlers(t *testing.T) {
	missing := callOutcomeSupplementalFactRoleTestHandlers()
	delete(missing, "NormalReturnFacts")
	requirePanic(t, func() {
		_ = BindCallOutcomeSupplementalFactRoles("test", missing, func(v int) bool { return v > 0 })
	})

	invalid := callOutcomeSupplementalFactRoleTestHandlers()
	invalid["NormalReturnFacts"] = 0
	requirePanic(t, func() {
		_ = BindCallOutcomeSupplementalFactRoles("test", invalid, func(v int) bool { return v > 0 })
	})

	orphan := callOutcomeSupplementalFactRoleTestHandlers()
	orphan["NotAField"] = 99
	requirePanic(t, func() {
		_ = BindCallOutcomeSupplementalFactRoles("test", orphan, func(v int) bool { return v > 0 })
	})
}

func callOutcomeSupplementalFactRoleTestHandlers() map[string]int {
	out := make(map[string]int)
	for i, role := range CallOutcomeSupplementalFactRoles() {
		out[role.FieldName] = i + 1
	}
	return out
}

func callOutcomeLaneByField(fieldName string) (callOutcomeLane, bool) {
	for _, lane := range callOutcomeLanes {
		if lane.fieldName == fieldName {
			return lane, true
		}
	}
	return callOutcomeLane{}, false
}

func callOutcomeWithOneField(t *testing.T, fieldName string) CallOutcome {
	t.Helper()
	out := CallOutcome{}
	if fieldName == "NormalReturnFacts" {
		out.NormalReturnFacts = callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{{}},
		}
		return out
	}
	value := reflect.ValueOf(&out).Elem().FieldByName(fieldName)
	if !value.IsValid() {
		t.Fatalf("CallOutcome.%s does not exist", fieldName)
	}
	switch value.Kind() {
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Map:
		m := reflect.MakeMapWithSize(value.Type(), 1)
		m.SetMapIndex(reflect.Zero(value.Type().Key()), reflect.Zero(value.Type().Elem()))
		value.Set(m)
	case reflect.Slice:
		value.Set(reflect.Append(value, reflect.Zero(value.Type().Elem())))
	default:
		t.Fatalf("CallOutcome.%s has unsupported test kind %s", fieldName, value.Kind())
	}
	return out
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
