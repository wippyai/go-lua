package composite

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/engine/rows/scalarlower"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// queryCanonicalProgram is the small test-only mirror of the analyzer's
// assemble boundary. It deliberately uses the same sealed artifact rows,
// owner admissions, and bootstrap witness as production, then stops at the
// committed program so this law can read the borrowed query answers.
func queryCanonicalProgram(t testing.TB, record LinkInputs, bound *ProgramBinding) (*engine.CommittedProgram, []QuerySite) {
	t.Helper()
	if bound == nil || !bound.Available() || record.Source == nil || len(record.Artifacts) == 0 {
		t.Fatal("query canonical fixture has no sealed binding")
	}
	vocabulary, vocabularyOK := StructureVocabulary()
	if !vocabularyOK {
		t.Fatal("sealed structure vocabulary")
	}
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}

	templates := make(map[identity.ContentID]*rows.ArtifactScalarTemplate, len(record.Artifacts))
	roles := make(map[identity.ContentID][]engine.MountedProgramRole, len(record.Artifacts))
	mounts := make([]engine.MountedProgramArtifact, 0, len(record.Artifacts))
	for index, mount := range record.Artifacts {
		if !mount.Available() {
			t.Fatalf("mounted artifact %d is unavailable", index)
		}
		artifactID := mount.Snapshot.ArtifactID()
		template := templates[artifactID]
		if template == nil {
			var directory *scalarlower.RoleDirectory
			var lowered bool
			template, directory, lowered = scalarlower.Lower(mount.Snapshot, vocabulary)
			if !lowered || template == nil || directory == nil || !template.Available() {
				t.Fatalf("lower artifact %d into a scalar template", index)
			}
			boundRoles := make([]engine.MountedProgramRole, 0, directory.Count())
			for roleIndex := 0; roleIndex < directory.Count(); roleIndex++ {
				key, scalar, roleOK := directory.At(roleIndex)
				capability, capabilityOK := rules.CapabilityByKey(key)
				if !roleOK || !capabilityOK || !capability.Mounted() {
					t.Fatalf("artifact %d role %q has no mounted capability", index, key)
				}
				boundRoles = append(boundRoles, engine.MountedProgramRole{Scalar: scalar, Capability: capability})
			}
			templates[artifactID] = template
			roles[artifactID] = boundRoles
		}
		mounts = append(mounts, engine.MountedProgramArtifact{Template: template, Roles: roles[artifactID], Module: mount.ModuleKey})
	}

	sourceID := record.Source.ContentID()
	point, pointOK := identity.DeriveContentID("analysis/link-bootstrap-point/v1", sourceID[:])
	if !pointOK {
		t.Fatal("link bootstrap point")
	}
	catalogs, catalogsOK := rules.BootstrapCatalogs()
	if !catalogsOK {
		t.Fatal("link bootstrap catalogs")
	}
	witness, witnessOK := engine.NewProgramBootstrap(sourceID, point, catalogs...)
	if !witnessOK {
		t.Fatal("link bootstrap witness")
	}
	sites, sitesOK := SelectedQuerySites(record.Artifacts)
	if !sitesOK || len(sites) == 0 {
		t.Fatal("selected query sites")
	}
	linkAdmissions, linkOK := rules.LinkAdmissions()
	if !linkOK {
		t.Fatal("link admissions")
	}
	mounted, activations, _, mountedOK := rules.MountedAdmissions(record.Artifacts)
	if !mountedOK {
		t.Fatal("mounted admissions")
	}
	queries, queriesOK := bound.QueryAdmissions(sites)
	if !queriesOK {
		t.Fatal("query admissions")
	}
	program, refusal, committed := engine.ConstructProgram(engine.ProgramDeclaration{
		Binding:   bound.SchemaBinding(),
		Mounts:    mounts,
		Bootstrap: witness,
		Admission: engine.MountedProgramAdmission{Link: linkAdmissions, Mounted: mounted, Activation: activations, Queries: queries},
	})
	if !committed || program == nil {
		t.Fatalf("construct committed query program: stage=%v lowered=%t refusal=%v", refusal.Stage(), refusal.Lowered(), refusal.Commit())
	}
	return program, sites
}

func TestQueryPublicationsSealFamilyCodecAndCanonicalizeBorrowedAnswers(t *testing.T) {
	record := mountedRecord(t, "query-canonical-cell", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	committed, sites := queryCanonicalProgram(t, record, bound)

	publications, published := bound.QueryPublications(committed, sites)
	if !published || len(publications) != len(sites) {
		t.Fatalf("query publications = %d/%t, sites = %d", len(publications), published, len(sites))
	}
	valuePublications := make([]QueryPublication, 0, 1)
	effectPublications := make([]QueryPublication, 0, 1)
	for index, publication := range publications {
		contract := publication.Contract()
		if !publication.Key.Available() || !contract.Available() || !contract.FamilyID().Available() || !contract.Codec().Available() {
			t.Fatalf("publication %d does not carry complete address and codec identity", index)
		}
		position, positioned := queryPositionForFamily(publication.Site.Family)
		if !positioned || position < 0 || position >= len(registry.queries) || registry.queries[position] == nil {
			t.Fatalf("publication %d names no sealed query registration", index)
		}
		registration := registry.queries[position]
		if contract.FamilyID() != identity.ContentID(registration.ID()) || contract.Codec() != registration.Freezer() {
			t.Fatalf("publication %d does not carry its registration family and codec identity", index)
		}
		if publication.Site.Authority != publication.Site.Family || publication.Site.Family == "" {
			t.Fatalf("publication %d lost sealed family authority", index)
		}
		switch publication.Site.Family {
		case QueryFamilyValueSummary:
			valuePublications = append(valuePublications, publication)
		case QueryFamilyEffectExact:
			effectPublications = append(effectPublications, publication)
		default:
			t.Fatalf("publication %d names unknown family %q", index, publication.Site.Family)
		}
	}
	if len(valuePublications) == 0 || len(effectPublications) == 0 {
		t.Fatal("fixture did not publish both Value and Effect query families")
	}

	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal query fixture: %v", sealFailure)
	}
	state, status := sealed.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("solve query fixture: status=%v state=%v", status, state)
	}
	publishedSnapshot, snapshotOK := sealed.PublishedSnapshot(state)
	if !snapshotOK {
		t.Fatal("solver published no snapshot")
	}
	view := publishedSnapshot.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, publishedSnapshot.QueryFamily())
	if !queryPlanOK {
		t.Fatal("open published query column")
	}

	type queryHit struct {
		publication QueryPublication
		answer      engine.Answer
	}
	valueHits := make([]queryHit, 0, len(valuePublications))
	effectHits := make([]queryHit, 0, len(effectPublications))
	for _, publication := range publications {
		answer, status := snapshot.Query(&view, queryPlan, publication.Key)
		switch status {
		case snapshot.ReadProvenAbsent:
			continue
		case snapshot.ReadHit:
			if !answer.Available() {
				t.Fatalf("read %q query answer: status=%v available=%t", publication.Site.Family, status, answer.Available())
			}
		default:
			t.Fatalf("read %q query answer: status=%v available=%t", publication.Site.Family, status, answer.Available())
		}
		hit := queryHit{publication: publication, answer: answer}
		switch publication.Site.Family {
		case QueryFamilyValueSummary:
			valueHits = append(valueHits, hit)
		case QueryFamilyEffectExact:
			effectHits = append(effectHits, hit)
		default:
			t.Fatalf("query answer names unknown family %q", publication.Site.Family)
		}
	}
	if len(valueHits) == 0 || len(effectHits) == 0 {
		t.Fatal("fixture yielded no borrowed answers for both query families")
	}

	for index, hit := range valueHits {
		cell, encoded := hit.publication.CanonicalCell(hit.answer)
		if !encoded || !cell.Available() || cell.ContractID() != hit.publication.Contract().ContentID() {
			t.Fatal("Value publication did not seal its borrowed answer under its complete identity")
		}
		if index == 0 {
			if _, wrong := hit.publication.CanonicalCell(effectHits[0].answer); wrong {
				t.Fatal("Value publication accepted an Effect answer")
			}
		}
		assertCanonicalCellHasNoCallback(t, cell)
	}
	for index, hit := range effectHits {
		cell, encoded := hit.publication.CanonicalCell(hit.answer)
		if !encoded || !cell.Available() || cell.ContractID() != hit.publication.Contract().ContentID() {
			t.Fatal("Effect publication did not seal its borrowed answer under its complete identity")
		}
		if index == 0 {
			if _, wrong := hit.publication.CanonicalCell(valueHits[0].answer); wrong {
				t.Fatal("Effect publication accepted a Value answer")
			}
		}
		assertCanonicalCellHasNoCallback(t, cell)
	}
}

func assertCanonicalCellHasNoCallback(t testing.TB, cell engine.CanonicalResultCell) {
	t.Helper()
	typeOfCell := reflect.TypeOf(cell)
	for index := 0; index < typeOfCell.NumField(); index++ {
		if typeOfCell.Field(index).Type.Kind() == reflect.Func {
			t.Fatalf("canonical result cell retained callback field %q", typeOfCell.Field(index).Name)
		}
	}
}
