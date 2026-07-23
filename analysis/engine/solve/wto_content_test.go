package solve

import "testing"

func TestWTOPlanInfluencesUseCanonicalPlanOrder(t *testing.T) {
	plan, err := FreezeWTOPlan([]string{"a", "b", "c"}, []WTOElement[string]{{Vertex: "a"}, {Vertex: "b"}, {Vertex: "c"}}, []WTOInfluence[string]{{From: "b", To: "c"}, {From: "a", To: "c"}, {From: "a", To: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Influences()
	want := []WTOInfluence[string]{{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "b", To: "c"}}
	if len(got) != len(want) {
		t.Fatalf("influences = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("influence[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}
