package program

import "testing"

func TestBodyViewIsIssuedAndOwnedByProgram(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-body-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	body, ok := published.BodyAt(0)
	if !ok || !body.Available() {
		t.Fatal("BodyAt(0) did not return the published body")
	}
	if published.BodyCount() != 1 || !published.OwnsBody(body) {
		t.Fatalf("body ownership = count %d, owns %v; want one owned body", published.BodyCount(), published.OwnsBody(body))
	}
	if body.ProgramID() != published.ContentID() || !body.ContextID().Available() {
		t.Fatal("Body did not retain its exact Program and boundary identities")
	}
	if _, ok := published.BodyAt(-1); ok {
		t.Fatal("negative BodyAt index was accepted")
	}
	if published.OwnsBody(Body{}) {
		t.Fatal("zero Body passed the ownership fence")
	}
}
