package axis

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchAuthority stands in for one domain's sealed Link authority. The
// surface is blind to it, so a scratch authority proves the same laws a real
// factor schema does.
type scratchAuthority struct{ mounts int }

// scratchRejection stands in for one domain's own rejection evidence.
type scratchRejection uint8

const (
	scratchRejectionNone scratchRejection = iota
	scratchRejectionInput
)

// mountedSpec is one axis declaration that seals its own Link authority. The
// counter records every invocation so the table's iteration law is stated over
// observed calls rather than over the record the calls produced.
func mountedSpec(key, semantic schema.Key, order *[]schema.Key, admit bool) Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64] {
	spec := scratchSpec(key, semantic)
	spec.Mount = NewMount(func(context Mounting[scratchInputs]) (*scratchAuthority, scratchRejection, bool) {
		*order = append(*order, key)
		if !admit || !context.Inputs.ready {
			return nil, scratchRejectionInput, false
		}
		return &scratchAuthority{mounts: 1}, scratchRejectionNone, true
	})
	return spec
}

// TestMountedAxisSealsItsOwnAuthority states the mount hook's contract: the
// declared hook receives the composition's own record and its result is
// recovered at the type the owner sealed.
func TestMountedAxisSealsItsOwnAuthority(t *testing.T) {
	var order []schema.Key
	template := mustTemplate(t, mountedSpec("value", valueRole, &order, true))
	if !template.MountDeclared() {
		t.Fatalf("axis declaring a mount reports none")
	}
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if !ok || !authority.Available() || rejection.Available() {
		t.Fatalf("declared mount rejected an admissible record: ok=%v authority=%v rejection=%v", ok, authority.Available(), rejection.Available())
	}
	sealed, sealedOK := Payload[*scratchAuthority](authority)
	if !sealedOK || sealed == nil || sealed.mounts != 1 {
		t.Fatalf("mounted authority did not recover at its declared type")
	}
	if len(order) != 1 || order[0] != "value" {
		t.Fatalf("declared mount invoked %d times, want exactly once", len(order))
	}
}

// TestUndeclaredMountAdmitsEmpty states the admission law: an axis that seals
// no authority of its own mounts empty rather than failing. Its authority is
// supplied by another owner, and the phase must not read that as a rejection.
func TestUndeclaredMountAdmitsEmpty(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	if template.MountDeclared() {
		t.Fatalf("axis declaring no mount reports one")
	}
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if !ok {
		t.Fatalf("axis declaring no mount rejected the phase")
	}
	if authority.Available() || rejection.Available() {
		t.Fatalf("axis declaring no mount produced a payload: authority=%v rejection=%v", authority.Available(), rejection.Available())
	}
}

// TestRejectedMountCarriesDomainEvidence states that a rejecting mount hands
// back its own domain evidence and no authority, so the composition reports the
// rejection the domain stated rather than a generic verdict.
func TestRejectedMountCarriesDomainEvidence(t *testing.T) {
	var order []schema.Key
	template := mustTemplate(t, mountedSpec("value", valueRole, &order, false))
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if ok || authority.Available() {
		t.Fatalf("rejecting mount published an authority")
	}
	evidence, evidenceOK := Payload[scratchRejection](rejection)
	if !evidenceOK || evidence != scratchRejectionInput {
		t.Fatalf("rejecting mount lost its domain evidence: ok=%v evidence=%v", evidenceOK, evidence)
	}
}

// TestMountPhaseRunsEveryDeclaredMountOnceInCatalogOrder states the generic
// iteration law: a table walk invokes each declared mount exactly once, in the
// catalog's own order, and passes over the axes that declare none.
func TestMountPhaseRunsEveryDeclaredMountOnceInCatalogOrder(t *testing.T) {
	var order []schema.Key
	templates := []*Template[scratchInputs]{
		mustTemplate(t, mountedSpec("heap", heapRole, &order, true)),
		mustTemplate(t, scratchSpec("pack", packRole)),
		mustTemplate(t, mountedSpec("value", valueRole, &order, true)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("complete mounted inventory rejected: %v", failure)
	}
	mounted := 0
	for _, template := range templates {
		authority, _, ok := template.Mount(scratchInputs{ready: true})
		if !ok {
			t.Fatalf("axis %q rejected the mount phase", template.Key())
		}
		if authority.Available() != template.MountDeclared() {
			t.Fatalf("axis %q published an authority it did not declare a mount for", template.Key())
		}
		if authority.Available() {
			mounted++
		}
	}
	if mounted != 2 {
		t.Fatalf("mount phase sealed %d authorities, want one per declared mount", mounted)
	}
	if len(order) != 2 || order[0] != "heap" || order[1] != "value" {
		t.Fatalf("mount phase ran %v, want the catalog order [heap value]", order)
	}
}

// TestDeclaringAMountDoesNotMoveEntryContent states the content boundary: which
// owner seals an axis's Link authority is wiring, not declared content, so
// moving a domain onto its own mount leaves the declaration digest exactly
// where it was. Only a changed coordinate space may move it.
func TestDeclaringAMountDoesNotMoveEntryContent(t *testing.T) {
	var order []schema.Key
	plain := mustTemplate(t, scratchSpec("value", valueRole))
	mounting := mustTemplate(t, mountedSpec("value", valueRole, &order, true))
	if !mounting.MountDeclared() || plain.MountDeclared() {
		t.Fatalf("mount declaration fixture is not the pair the law is about")
	}
	if entryContentBytes(t, plain) != entryContentBytes(t, mounting) {
		t.Fatalf("declaring a mount moved the axis entry's content")
	}
	if len(order) != 0 {
		t.Fatalf("writing entry content invoked the mount hook")
	}
}

// dependentSpec is one axis declaration that seals over a peer's authority.
func dependentSpec(key, semantic schema.Key, dependencies ...schema.Key) Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64] {
	spec := scratchSpec(key, semantic)
	spec.Dependencies = dependencies
	return spec
}

// TestDependencyOrderPlacesEveryAxisAfterItsDependencies states the derivation
// a dependency-respecting phase walks: an axis follows the axes it declared an
// edge to, whatever the catalog order was.
func TestDependencyOrderPlacesEveryAxisAfterItsDependencies(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, dependentSpec("value", valueRole, "heap")),
		mustTemplate(t, dependentSpec("pack", packRole)),
		mustTemplate(t, dependentSpec("heap", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("acyclic inventory rejected: %v", failure)
	}
	ordered, ok := DependencyOrder(templates)
	if !ok || len(ordered) != len(templates) {
		t.Fatalf("dependency order rejected an acyclic inventory: ok=%v placed=%d", ok, len(ordered))
	}
	positions := make(map[schema.Key]int, len(ordered))
	for index, template := range ordered {
		positions[template.Key()] = index
	}
	for _, template := range templates {
		for index := 0; index < template.DependencyCount(); index++ {
			dependency, _ := template.DependencyAt(index)
			if positions[dependency] >= positions[template.Key()] {
				t.Fatalf("axis %q is ordered before its dependency %q", template.Key(), dependency)
			}
		}
	}
	// The order is stable: axes no edge separates keep the catalog's own order.
	if positions["pack"] <= positions["heap"] && positions["heap"] < positions["value"] {
		return
	}
	t.Fatalf("dependency order did not keep the unconstrained catalog order: %v", positions)
}

// TestDeclaredCycleIsRejectedAtSeal states that a cycle is a declaration error
// rather than a walk that never begins: the table refuses to seal, and it names
// an axis the cycle blocked.
func TestDeclaredCycleIsRejectedAtSeal(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, dependentSpec("value", valueRole, "heap")),
		mustTemplate(t, dependentSpec("heap", heapRole, "value")),
	}
	failure := sealTemplates(t, templates)
	if !failure.Available() || failure.Law != LawDependencyAcyclic {
		t.Fatalf("cyclic inventory sealed: %v", failure)
	}
	if ordered, ok := DependencyOrder(templates); ok || len(ordered) != 0 {
		t.Fatalf("dependency order admitted a cycle: ok=%v placed=%d", ok, len(ordered))
	}
}

func entryContentBytes(t *testing.T, template *Template[scratchInputs]) string {
	t.Helper()
	var sink bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&sink, "analysis/axis-entry-content-law/v1", 1); err != nil {
		t.Fatalf("axis %q content stream: %v", template.Key(), err)
	}
	if err := template.EntryContent(&writer); err != nil {
		t.Fatalf("axis %q entry content: %v", template.Key(), err)
	}
	return sink.String()
}

// TestNilTemplateRejectsMount keeps the surface total: an absent template is a
// rejection rather than a panic.
func TestNilTemplateRejectsMount(t *testing.T) {
	var template *Template[scratchInputs]
	if _, _, ok := template.Mount(scratchInputs{ready: true}); ok {
		t.Fatalf("absent template admitted a mount")
	}
	if template.MountDeclared() {
		t.Fatalf("absent template reports a declared mount")
	}
}
