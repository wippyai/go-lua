package callboundary

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func TestNormalReturnFactsCloneDetachesEveryLaneAndNestedPath(t *testing.T) {
	original := NormalReturnFacts{}
	originalValue := reflect.ValueOf(&original).Elem()
	for index := 0; index < originalValue.NumField(); index++ {
		lane := originalValue.Field(index)
		if lane.Kind() != reflect.Slice {
			t.Fatalf("NormalReturnFacts.%s is not a slice lane", originalValue.Type().Field(index).Name)
		}
		lane.Set(reflect.Append(lane, reflect.New(lane.Type().Elem()).Elem()))
	}
	pathCount := visitMutablePaths(originalValue, func(path pathdom.Path) pathdom.Path {
		return pathdom.NewPlaceholder(0).Field("before")
	})
	if pathCount != 31 {
		t.Fatalf("seeded nested paths = %d, want 31", pathCount)
	}

	cloned := original.Clone()
	cloneValue := reflect.ValueOf(&cloned).Elem()
	for index := 0; index < cloneValue.NumField(); index++ {
		cloneLane := cloneValue.Field(index)
		cloneLane.SetLen(0)
		if originalValue.Field(index).Len() != 1 {
			t.Fatalf("NormalReturnFacts.%s retained slice storage", cloneValue.Type().Field(index).Name)
		}
	}

	// Re-clone after the storage check and mutate every path-bearing leaf.
	cloned = original.Clone()
	changed := visitMutablePaths(reflect.ValueOf(&cloned).Elem(), func(path pathdom.Path) pathdom.Path {
		path.Segments[0].Name = "after"
		return path
	})
	if changed != pathCount {
		t.Fatalf("mutated nested paths = %d, want %d", changed, pathCount)
	}
	visitMutablePaths(originalValue, func(path pathdom.Path) pathdom.Path {
		if len(path.Segments) != 1 || path.Segments[0].Name != "before" {
			t.Fatalf("NormalReturnFacts.Clone retained nested path storage: %#v", path)
		}
		return path
	})
}

func TestProtectedCallTypestateCloneDetachesStores(t *testing.T) {
	resource := typestate.Resource{ID: "resource", Protocol: "protocol"}
	original := ProtectedCallTypestate{
		Normal:    typestate.Empty().Acquire(resource, "open", typestate.Obligation{Final: "closed"}),
		HasNormal: true,
	}
	cloned := original.Clone()
	cloned.Normal = cloned.Normal.Transition(resource, "open", "closed")

	originalSlot, ok := original.Normal.Lookup(resource)
	if !ok || originalSlot.Current != "open" {
		t.Fatalf("original normal store = %#v/%v, want open", originalSlot, ok)
	}
	clonedSlot, ok := cloned.Normal.Lookup(resource)
	if !ok || clonedSlot.Current != "closed" {
		t.Fatalf("cloned normal store = %#v/%v, want closed", clonedSlot, ok)
	}
}

var mutablePathType = reflect.TypeOf(pathdom.Path{})

func visitMutablePaths(value reflect.Value, visit func(pathdom.Path) pathdom.Path) int {
	if !value.IsValid() {
		return 0
	}
	if value.Type() == mutablePathType {
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
				count += visitMutablePaths(field, visit)
			}
		}
		return count
	case reflect.Slice:
		count := 0
		for index := 0; index < value.Len(); index++ {
			count += visitMutablePaths(value.Index(index), visit)
		}
		return count
	default:
		return 0
	}
}
