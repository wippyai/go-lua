package factor_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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

// A subject member's tag is stable, scoped to the member it names, and never
// zero. It is what a selection over a publication's subject pairs its cells
// by, so two members sharing a tag would let one member's fact answer for
// another, and a zero tag would name no member at all.
func TestPublicationMemberTagIsStableAndMemberScoped(t *testing.T) {
	first := identity.ContentID([32]byte{1})
	second := identity.ContentID([32]byte{2})

	left, leftOK := factor.PublicationMemberTag(first)
	repeat, repeatOK := factor.PublicationMemberTag(first)
	right, rightOK := factor.PublicationMemberTag(second)
	if !leftOK || !repeatOK || !rightOK {
		t.Fatal("member tags refused an available identity")
	}
	if left != repeat {
		t.Fatalf("one member folded two tags: %d and %d", left, repeat)
	}
	if left == right {
		t.Fatalf("two members share tag %d", left)
	}
	if left == 0 || right == 0 {
		t.Fatal("a member folded the zero tag")
	}
	if _, ok := factor.PublicationMemberTag(identity.ContentID{}); ok {
		t.Fatal("an unavailable identity acquired a tag")
	}
}

// Two publications naming one semantic member are two members, so their tags
// differ: the tag folds the member's own identity, which the directory derives
// from the row it belongs to and its position in that row's pack.
func TestOneSemanticMemberUnderTwoPublicationsCarriesTwoTags(t *testing.T) {
	left, leftOK := factor.PublicationMemberID(identity.ContentID([32]byte{1}), 0)
	right, rightOK := factor.PublicationMemberID(identity.ContentID([32]byte{2}), 0)
	if !leftOK || !rightOK || left == right {
		t.Fatal("one member identity served two publications")
	}
	leftTag, leftTagOK := factor.PublicationMemberTag(left)
	rightTag, rightTagOK := factor.PublicationMemberTag(right)
	if !leftTagOK || !rightTagOK || leftTag == rightTag {
		t.Fatalf("two publications share member tag %d", leftTag)
	}
}
