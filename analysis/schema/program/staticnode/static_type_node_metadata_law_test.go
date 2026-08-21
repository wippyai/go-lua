package staticnode

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The three metadata families carry a name key beside their typed child, and
// the name law is not the same for all three. A record field and an interface
// member are addressed by their name, so a row without one addresses nothing.
// A type-function parameter is addressed by its position: `(number) -> number`
// declares a parameter type whose name is absent in the source, and the zero
// key is that absence written down. The law below pins both directions, so a
// later unification of the three constructors can neither restore the blanket
// requirement nor drop it for the two families that hold it.
func TestStaticTypeNodeMetadataNameLawIsPerFamily(t *testing.T) {
	parent, child := identity.ContentID{1}, identity.ContentID{2}

	parameter, parameterOK := NewStaticTypeNodeTypeFunctionParameter(parent, child, 0, "", 0)
	if !parameterOK || !parameter.Available() {
		t.Fatal("a type-function parameter with no name was refused; an anonymous parameter type is written without one")
	}
	if parameter.ParentID() != parent || parameter.ChildID() != child || parameter.Key() != 0 || parameter.Text() != "" || parameter.Position() != 0 {
		t.Fatalf("an anonymous type-function parameter did not round-trip: parent=%v child=%v key=%d text=%q position=%d",
			parameter.ParentID(), parameter.ChildID(), parameter.Key(), parameter.Text(), parameter.Position())
	}

	named, namedOK := NewStaticTypeNodeTypeFunctionParameter(parent, child, 7, "count", 1)
	if !namedOK || !named.Available() || named.Key() != 7 || named.Text() != "count" || named.Position() != 1 {
		t.Fatalf("a named type-function parameter did not round-trip: ok=%v key=%d text=%q position=%d", namedOK, named.Key(), named.Text(), named.Position())
	}

	field, fieldOK := NewStaticTypeNodeRecordField(parent, child, 0, "", false, false, 0)
	if fieldOK || field.Available() {
		t.Fatal("a record field with no name was admitted; a field is addressed by its name")
	}

	member, memberOK := NewStaticTypeNodeInterfaceMember(parent, child, 0, "", false, false, 0, 0)
	if memberOK || member.Available() {
		t.Fatal("an interface member with no name was admitted; a member is addressed by its name")
	}
}
