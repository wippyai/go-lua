package factor_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect/factor"
)

// A directory row is a fact that crosses to readers holding neither Effect's
// algebra nor Pack's mounted inputs, so it carries no pointer and no owner-
// fenced capability of either. A row that carried one would publish a live
// handle under the name of a sealed fact.
//
// All three published rows answer under this law, not the receipt row alone:
// the call row and the member row are published in their own columns and are
// read by the same holders, so a slice or handle added to either would cross
// the same boundary the receipt row is kept clean of.
func TestDirectoryRowCarriesNoLiveCapability(t *testing.T) {
	for _, published := range []any{
		factor.PublicationRow{},
		factor.PublicationCallRow{},
		factor.PublicationMemberRow{},
	} {
		row := reflect.TypeOf(published)
		for index := 0; index < row.NumField(); index++ {
			field := row.Field(index)
			switch field.Type.Kind() {
			case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
				t.Fatalf("%s.%s is %s", row.Name(), field.Name, field.Type)
			}
			for _, forbidden := range []string{"factor.", "packtransfer", "transfer.MountedInput", "value.Coordinate"} {
				if strings.Contains(field.Type.String(), forbidden) {
					t.Fatalf("%s.%s is %s", row.Name(), field.Name, field.Type)
				}
			}
		}
	}
}
