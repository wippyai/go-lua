package static

import (
	"reflect"
	"testing"
)

// The namespace plane is Link substitution only. This gate prevents a future
// convenience query from turning it back into a second Program static model.
func TestNamespacePlaneRetainsOnlyScalarIdentity(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(Component{}), reflect.TypeOf(Cold{})} {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if field.Name == "fence" && field.Type == reflect.TypeOf((*draftFence)(nil)) {
				continue // lifecycle invalidation only; never a semantic authority.
			}
			if field.Type.Kind() == reflect.Ptr || field.Type.Kind() == reflect.Interface {
				t.Fatalf("%s.%s retains non-scalar authority %s", typ.Name(), field.Name, field.Type)
			}
		}
	}
}
