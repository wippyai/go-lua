package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

// TestTopFixtureBodyCensusUsesExecutableSourceRoots fixes the production
// denominator at the real edge-matrix boundary. Source order also contains
// the five static TypeAlias rows, while BodyRootAt is the owner-issued
// runtime-root projection and includes every empty executable arm as a Body.
func TestTopFixtureBodyCensusUsesExecutableSourceRoots(t *testing.T) {
	project, err := testfixture.FrozenCorpusProject("semantic/type-engine-edge-matrix")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}

	bodies, censusOK := mountedProgramBodies(linked)
	if !censusOK || len(bodies) != 225 {
		t.Fatalf("mounted executable Body census = %d/%t, want 225/true", len(bodies), censusOK)
	}
	program := bodies[0].program
	if got := program.Flow().Executable().FamilyCount(keyspace.FamilyBody); got != 225 {
		t.Fatalf("Flow executable Body denominator = %d, want 225", got)
	}

	order := program.Source().Order()
	index := program.Source().Index()
	firstAuthored, authoredOK := order.BodyAt(bodies[0].term, 0)
	firstRoot, rootOK := index.BodyRootAt(bodies[0].term, 0)
	if !authoredOK || !rootOK || keyspace.TermFamily(firstAuthored) != keyspace.FamilyTypeAlias ||
		keyspace.TermFamily(firstRoot) == keyspace.FamilyTypeAlias || !program.Flow().Executable().Contains(firstRoot) {
		t.Fatalf("Source/Flow root split = authored:%v/%t root:%v/%t", firstAuthored, authoredOK, firstRoot, rootOK)
	}

	empty := 0
	staticAliases := 0
	for ordinal := 1; ordinal <= program.Source().Identity().FamilyCount(keyspace.FamilyTypeAlias); ordinal++ {
		alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, uint32(ordinal))
		if !program.Flow().Executable().Contains(alias) {
			staticAliases++
		}
	}
	for _, body := range bodies {
		rootCount, rootCountOK := body.program.Source().Index().BodyRootLen(body.term)
		if !rootCountOK {
			t.Fatalf("BodyRootLen(%v) unavailable", body.term)
		}
		if rootCount == 0 {
			empty++
		}
		for offset := 0; offset < rootCount; offset++ {
			root, rootOK := body.program.Source().Index().BodyRootAt(body.term, offset)
			if !rootOK || keyspace.TermFamily(root) == keyspace.FamilyTypeAlias || !body.program.Flow().Executable().Contains(root) {
				t.Fatalf("BodyRootAt(%v,%d) = %v/%t; want executable runtime root", body.term, offset, root, rootOK)
			}
		}
	}
	if empty != 78 || staticAliases != 5 {
		t.Fatalf("top fixture empty/runtime denominator = empty:%d aliases:%d, want 78/5", empty, staticAliases)
	}

	declared, declaredOK := newProgramAnalysis(linked)
	if !declaredOK || declared == nil || len(declared.bodies) != 225 || declared.queries.value == nil || declared.queries.effect == nil {
		t.Fatalf("production body/query families = declared:%t bodies:%d value:%t effect:%t, want 225/1/1", declaredOK, len(declared.bodies), declared != nil && declared.queries.value != nil, declared != nil && declared.queries.effect != nil)
	}
}
