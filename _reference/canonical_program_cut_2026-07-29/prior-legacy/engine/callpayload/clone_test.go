package callpayload

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestCallOutcomeCloneDetachesEveryMutableLane(t *testing.T) {
	typ := reflect.TypeOf(CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		original := callOutcomeWithOneField(t, field.Name)
		cloned := original.Clone()
		originalValue := reflect.ValueOf(&original).Elem().Field(i)
		cloneValue := reflect.ValueOf(&cloned).Elem().Field(i)
		switch field.Name {
		case "NormalReturnFacts":
			cloned.NormalReturnFacts.PathRefinements = nil
			if len(original.NormalReturnFacts.PathRefinements) != 1 {
				t.Fatalf("CallOutcome.%s retained slice storage", field.Name)
			}
		case "ProtectedCallTypestate":
			// ProtectedCallTypestate owns its clone contract; its populated test
			// spelling contains scalars only.
		case "PostReturnAuthority", "SuspensionKnown", "MaySuspend":
			cloneValue.SetBool(false)
			if !originalValue.Bool() {
				t.Fatalf("CallOutcome.%s scalar mutation changed original", field.Name)
			}
		case "HeapTableObjects", "Placements":
			cloneValue.SetMapIndex(reflect.Zero(cloneValue.Type().Key()), reflect.Value{})
			if originalValue.Len() != 1 {
				t.Fatalf("CallOutcome.%s retained map storage", field.Name)
			}
		default:
			if cloneValue.Kind() != reflect.Slice {
				t.Fatalf("CallOutcome.%s has untested mutable kind %s", field.Name, cloneValue.Kind())
			}
			cloneValue.SetLen(0)
			if originalValue.Len() != 1 {
				t.Fatalf("CallOutcome.%s retained slice storage", field.Name)
			}
		}
	}
}

func TestCallOutcomeCloneDetachesNestedPaths(t *testing.T) {
	original := CallOutcome{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("before")}},
		},
		PathObligations: []CallPathObligation{{Path: pathdom.NewPlaceholder(1).Field("before")}},
	}
	cloned := original.Clone()
	cloned.NormalReturnFacts.PathRefinements[0].Path.Segments[0].Name = "after"
	cloned.PathObligations[0].Path.Segments[0].Name = "after"
	if original.NormalReturnFacts.PathRefinements[0].Path.Segments[0].Name != "before" ||
		original.PathObligations[0].Path.Segments[0].Name != "before" {
		t.Fatal("CallOutcome.Clone retained nested path segment storage")
	}
}

func TestCallOutcomeCloneDetachesEveryDirectNestedPath(t *testing.T) {
	original := CallOutcome{}
	value := reflect.ValueOf(&original).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.Kind() == reflect.Slice {
			field.Set(reflect.Append(field, reflect.New(field.Type().Elem()).Elem()))
		}
	}
	pathType := reflect.TypeOf(pathdom.Path{})
	seeded := visitCallOutcomePaths(value, pathType, func(pathdom.Path) pathdom.Path {
		return pathdom.NewPlaceholder(0).Field("before")
	})
	if seeded != 10 {
		t.Fatalf("seeded direct nested paths = %d, want 10", seeded)
	}

	cloned := original.Clone()
	changed := visitCallOutcomePaths(reflect.ValueOf(&cloned).Elem(), pathType, func(path pathdom.Path) pathdom.Path {
		path.Segments[0].Name = "after"
		return path
	})
	if changed != seeded {
		t.Fatalf("mutated direct nested paths = %d, want %d", changed, seeded)
	}
	visitCallOutcomePaths(value, pathType, func(path pathdom.Path) pathdom.Path {
		if len(path.Segments) != 1 || path.Segments[0].Name != "before" {
			t.Fatalf("CallOutcome.Clone retained nested path storage: %#v", path)
		}
		return path
	})
}

func visitCallOutcomePaths(value reflect.Value, pathType reflect.Type, visit func(pathdom.Path) pathdom.Path) int {
	if !value.IsValid() {
		return 0
	}
	if value.Type() == pathType {
		if !value.CanSet() {
			return 0
		}
		value.Set(reflect.ValueOf(visit(value.Interface().(pathdom.Path))))
		return 1
	}
	switch value.Kind() {
	case reflect.Struct:
		count := 0
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.CanSet() {
				count += visitCallOutcomePaths(field, pathType, visit)
			}
		}
		return count
	case reflect.Slice:
		count := 0
		for index := 0; index < value.Len(); index++ {
			count += visitCallOutcomePaths(value.Index(index), pathType, visit)
		}
		return count
	default:
		return 0
	}
}
