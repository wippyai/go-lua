package programmount

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// TestMountedArtifactDoesNotCarryProgramArtifact states the mount-phase
// fence: contributors receive the sealed ingress snapshot, not the owner.
func TestMountedArtifactDoesNotCarryProgramArtifact(t *testing.T) {
	row := reflect.TypeOf(MountedArtifact{})
	for index := 0; index < row.NumField(); index++ {
		field := row.Field(index)
		if strings.Contains(field.Type.String(), "programartifact") {
			t.Fatalf("MountedArtifact.%s is %s", field.Name, field.Type)
		}
	}
	if _, ok := row.FieldByName("Snapshot"); !ok {
		t.Fatal("MountedArtifact has no Snapshot")
	}
}

func mountLawIdentity(t *testing.T, tag string) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("program-mount-law/"+tag, nil)
	if !derived {
		t.Fatalf("identity %s", tag)
	}
	return id
}

func mountLawProgram(t *testing.T, module string) Program {
	t.Helper()
	schema := mountLawIdentity(t, "runtime-schema")
	catalog, derived := programschema.CatalogID(schema)
	if !derived {
		t.Fatal("cold catalog")
	}
	content, sealed := programschema.CallTargetFamily().Content(nil, catalog)
	if !sealed {
		t.Fatal("cold family")
	}
	builder := snapshot.NewFrozen(catalog, identity.StoreID(3))
	if err := snapshot.PutFrozenColumn(&builder, programschema.CallTargetFamily().Axis(catalog), content); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal cold publication: %v", err)
	}
	return Program{
		ModuleKey: mountLawIdentity(t, module),
		Program: programschema.Program{
			Frozen: frozen,
			ArtifactID: mountLawIdentity(t, "artifact"), ProgramID: mountLawIdentity(t, "program"),
			SchemaID: schema,
		},
	}
}

func sealMountLaw(t *testing.T, rows []Program) (snapshot.Snapshot, snapshot.Axis[identity.ContentID, Program]) {
	t.Helper()
	schema := mountLawIdentity(t, "link-schema")
	denominator, derived := DenominatorID(mountLawIdentity(t, "link"))
	if !derived {
		t.Fatal("directory denominator")
	}
	content, sealed := Content(rows, denominator)
	if !sealed {
		t.Fatal("mount directory content")
	}
	address := Axis(schema, 0)
	builder := snapshot.NewBuilder(schema, identity.StoreID(9), identity.Generation(1))
	if err := snapshot.PutColumn(&builder, address, content); err != nil {
		t.Fatalf("put mount column: %v", err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return published, address
}

// The directory answers for exactly the modules it was sealed over: a mounted
// module resolves to its row, and a module the Link does not mount is outside
// the published universe rather than merely missing from it.
func TestDirectoryAnswersForItsOwnModulesOnly(t *testing.T) {
	first, second := mountLawProgram(t, "module-a"), mountLawProgram(t, "module-b")
	published, address := sealMountLaw(t, []Program{first, second})

	for _, want := range []Program{first, second} {
		row, resolved := Mounted(&published, address, want.ModuleKey)
		if !resolved {
			t.Fatalf("module %x resolves to no row", want.ModuleKey[:4])
		}
		if row.ModuleKey != want.ModuleKey || row.ArtifactID != want.ArtifactID {
			t.Fatalf("module %x resolved to another row", want.ModuleKey[:4])
		}
	}
	if _, resolved := Mounted(&published, address, mountLawIdentity(t, "unmounted")); resolved {
		t.Fatal("the directory resolved an unmounted module")
	}
}

// One program mounted at two module keys is one frozen value in two rows. The
// directory shares it: that is what carrying the cold publication by value
// buys, and it is what compile-once reuse depends on.
func TestOneProgramMountedTwiceSharesOneColdPublication(t *testing.T) {
	first := mountLawProgram(t, "module-a")
	second := first
	second.ModuleKey = mountLawIdentity(t, "module-b")
	published, address := sealMountLaw(t, []Program{first, second})

	left, leftOK := Mounted(&published, address, first.ModuleKey)
	right, rightOK := Mounted(&published, address, second.ModuleKey)
	if !leftOK || !rightOK {
		t.Fatal("both mounts resolve")
	}
	if left.Frozen.Store() != right.Frozen.Store() || left.Frozen.Generation() != right.Frozen.Generation() {
		t.Fatal("two mounts of one program address two cold stores")
	}
	if left.ArtifactID != right.ArtifactID || left.ProgramID != right.ProgramID {
		t.Fatal("two mounts of one program name two artifacts")
	}
}

// A directory is one statement about one Link, so a module key offered twice
// and a row that does not authenticate are both refused rather than sealed.
func TestDirectoryRefusesADuplicateOrIncompleteMount(t *testing.T) {
	denominator, derived := DenominatorID(mountLawIdentity(t, "link"))
	if !derived {
		t.Fatal("denominator")
	}
	row := mountLawProgram(t, "module-a")

	if _, sealed := Content([]Program{row, row}, denominator); sealed {
		t.Fatal("one module key sealed two rows")
	}
	incomplete := row
	incomplete.ArtifactID = identity.ContentID{}
	if _, sealed := Content([]Program{incomplete}, denominator); sealed {
		t.Fatal("a row with no artifact identity sealed into the directory")
	}
	unpublished := row
	unpublished.Frozen = snapshot.Frozen{}
	if _, sealed := Content([]Program{unpublished}, denominator); sealed {
		t.Fatal("a row with no cold publication sealed into the directory")
	}
	if _, sealed := Content(nil, denominator); sealed {
		t.Fatal("a Link with no mount sealed a directory")
	}
	if _, sealed := Content([]Program{row}, identity.ContentID{}); sealed {
		t.Fatal("a directory sealed under no denominator")
	}
	if _, derived := DenominatorID(identity.ContentID{}); derived {
		t.Fatal("an unavailable Link derived a directory denominator")
	}
}

// Two Links never share a module directory, so one Link's mounts can never be
// proven against another Link's publication.
func TestDirectoryDenominatorIsPerLink(t *testing.T) {
	first, firstOK := DenominatorID(mountLawIdentity(t, "link"))
	second, secondOK := DenominatorID(mountLawIdentity(t, "other-link"))
	if !firstOK || !secondOK {
		t.Fatal("directory denominators")
	}
	if first == second {
		t.Fatal("two Links derived one module directory")
	}
}
