package testfixture

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func mustPresence(t TB) model.Presence {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	return presence
}

func content(label string) identity.ContentID {
	value, _ := identity.DeriveContentID(fixtureDomain, []byte(label))
	return value
}

func mustIssue[T any](t TB, label string, issue func(identity.ContentID) (T, bool)) T {
	t.Helper()
	value, ok := issue(content(label))
	if !ok {
		t.Fatalf("issue %s", label)
	}
	return value
}

func mustValue(t TB, mounted witness.Mounted, typeID model.TypeID, label string) binding.ValueToken {
	t.Helper()
	value, ok := mounted.IssueValue(typeID, content(label))
	if !ok {
		t.Fatal("value")
	}
	return value
}

func issueValues(t TB, mounted witness.Mounted, typeID model.TypeID, prefix string) [2][4]binding.ValueToken {
	t.Helper()
	var values [2][4]binding.ValueToken
	for row := range values {
		for column := range values[row] {
			values[row][column] = mustValue(t, mounted, typeID, prefix+"/seed/"+string(rune('a'+row*4+column)))
		}
	}
	return values
}

func mustLayout(t TB, mounted witness.Mounted, access arrangement.Access) arrangement.Layout {
	t.Helper()
	if !access.Available() {
		t.Fatalf("unavailable layout access: %v", access)
	}
	// Resolve is intentionally ambiguous when one logical vector owns more
	// than one physical coordinate. Fixture readers consume the ordinary
	// neutral coordinate (or the declared key coordinate for keyed Access),
	// never whichever variant happens to be first in the sealed table.
	if layout, ok := mounted.Arrangement().Resolve(access); ok && layout.Available() {
		return layout
	}
	wantClass := arrangement.CoordinateClassNone
	if access.Key().Available() {
		wantClass = arrangement.CoordinateClassDeclaredKey
	}
	var selected arrangement.Layout
	for _, layout := range mounted.Arrangement().Layouts() {
		if !layout.Available() || !layout.Access().Equal(access) || layout.CoordinateClass() != wantClass {
			continue
		}
		if selected.Available() {
			t.Fatalf("ambiguous canonical fixture layout: class=%v access=%v", wantClass, access)
		}
		selected = layout
	}
	if selected.Available() {
		return selected
	}
	t.Fatalf("missing layout access: relation=%v key=%v columns=%v", access.Relation(), access.Key(), access.Columns())
	return arrangement.Layout{}
}

func mustRelationLayout(t TB, mounted witness.Mounted, relation model.RelationID) arrangement.Layout {
	t.Helper()
	access, ok := arrangement.NewRelationAccess(relation)
	if !ok {
		t.Fatalf("relation layout access: %v", relation)
	}
	return mustLayout(t, mounted, access)
}

func mustKeyLayout(t TB, mounted witness.Mounted, key model.KeyID) arrangement.Layout {
	t.Helper()
	access, ok := arrangement.NewKeyAccess(key)
	if !ok {
		t.Fatalf("key layout access: %v", key)
	}
	return mustLayout(t, mounted, access)
}

func mustVectorLayout(t TB, mounted witness.Mounted, relation model.RelationID, columns []model.ColumnID) arrangement.Layout {
	t.Helper()
	access, ok := arrangement.NewVectorAccess(relation, columns)
	if !ok {
		t.Fatalf("vector layout access: relation=%v columns=%v", relation, columns)
	}
	return mustLayout(t, mounted, access)
}
