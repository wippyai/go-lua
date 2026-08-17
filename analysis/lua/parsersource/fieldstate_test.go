package parsersource

import "testing"

// TestEveryDeclaredFieldFormHasAStateSpace states that the state axis is total
// over the forms the AST actually declares. A form whose states are unmodelled
// returns nothing, and a denominator built on it would silently contain no rows
// for that carrier rather than failing, so the absence is checked here against
// the shipped schema instead of being assumed by every consumer.
func TestEveryDeclaredFieldFormHasAStateSpace(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Constructors) == 0 {
		t.Fatal("the parser constructs no AST forms")
	}
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			states := field.Form.States()
			if len(states) == 0 {
				t.Fatalf("ast.%s field %s has form %d with no state space", constructor.Name, field.Name, field.Form)
			}
			seen := make(map[FieldState]bool, len(states))
			for _, state := range states {
				if state == FieldStateInvalid {
					t.Fatalf("ast.%s field %s admits the invalid state", constructor.Name, field.Name)
				}
				if seen[state] {
					t.Fatalf("ast.%s field %s admits %s twice", constructor.Name, field.Name, state)
				}
				seen[state] = true
			}
		}
	}
}

// TestFieldFormStateSpacesAreDisjoint states that the states themselves keep the
// forms apart. Two forms sharing a state spelling would let a row key name a
// state its carrier cannot be in, which is exactly the confusion a closed state
// vocabulary exists to prevent.
func TestFieldFormStateSpacesAreDisjoint(t *testing.T) {
	forms := []FieldForm{
		FieldFormScalar, FieldFormBool, FieldFormString, FieldFormOptional,
		FieldFormSequence, FieldFormMapping, FieldFormInterface, FieldFormNamed,
	}
	spelling := make(map[string]FieldState)
	for _, form := range forms {
		states := form.States()
		if len(states) != 2 {
			t.Fatalf("form %d admits %d states, every modelled form is binary", form, len(states))
		}
		for _, state := range states {
			text := state.String()
			if text == "" || text == "invalid" {
				t.Fatalf("state %d has no spelling", state)
			}
			if existing, taken := spelling[text]; taken && existing != state {
				t.Fatalf("states %d and %d share the spelling %s", existing, state, text)
			}
			spelling[text] = state
		}
	}
	if len(spelling) != 8 {
		t.Fatalf("the modelled forms reach %d distinct states, want 8", len(spelling))
	}
}
