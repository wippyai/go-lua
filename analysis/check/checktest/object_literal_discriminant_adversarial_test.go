package checktest

import "testing"

func TestObjectLiteralDotFieldDiscriminantSatisfiesUnionArm(t *testing.T) {
	result := Check(`
type Start = {kind: "start", payload: string}
type Stop = {kind: "stop", code: number}
type Event = Start | Stop

local event: Event = {kind = "stop", code = 1}
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for dot-field discriminant", result.Diagnostics)
	}
}

func TestObjectLiteralBracketStringDiscriminantSatisfiesDotFieldUnion(t *testing.T) {
	src := `
type Start = {kind: "start", payload: string}
type Stop = {kind: "stop", code: number}
type Event = Start | Stop

local event: Event = {["kind"] = "stop", code = 1}
`
	result := Check(src)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want bracket string key to satisfy dot field", result.Diagnostics)
	}
}
