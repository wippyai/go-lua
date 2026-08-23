package composite

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/engine/rows/scalarlower"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// queryCanonicalProgram is the small test-only mirror of the analyzer's
// assemble boundary. It deliberately uses the same sealed artifact rows,
// owner admissions, bootstrap witness, Link context directory, and
// module-composition transition declarations as production, then stops at the
// committed program so this law can read the borrowed query answers.
func queryCanonicalProgram(t testing.TB, record LinkInputs, bound *ProgramBinding) (*engine.CommittedProgram, SelectedQueryTable) {
	t.Helper()
	if bound == nil || !bound.Available() || record.Source == nil || len(record.Artifacts) == 0 {
		t.Fatal("query canonical fixture has no sealed binding")
	}
	compilation := bound.Compilation()
	state := compilation.catalog
	if state == nil {
		t.Fatal("query canonical fixture has no compilation catalog")
	}
	vocabulary, vocabularyOK := StructureVocabulary(compilation)
	if !vocabularyOK {
		t.Fatal("sealed structure vocabulary")
	}
	issuance, issuanceOK := ArtifactIssuanceDirectory(compilation)
	if !issuanceOK {
		t.Fatal("sealed issuance vocabulary")
	}
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	templates := make(map[identity.ContentID]*rows.ArtifactScalarTemplate, len(record.Artifacts))
	roles := make(map[identity.ContentID][]engine.MountedProgramRole, len(record.Artifacts))
	factors := make(map[identity.ContentID][]engine.MountedProgramFactor, len(record.Artifacts))
	mounts := make([]engine.MountedProgramArtifact, 0, len(record.Artifacts))
	for index, mount := range record.Artifacts {
		if !mount.Available() {
			t.Fatalf("mounted artifact %d is unavailable", index)
		}
		artifactID := mount.Snapshot.ArtifactID()
		template := templates[artifactID]
		if template == nil {
			var directory *scalarlower.MountDirectory
			var lowered bool
			template, directory, lowered = scalarlower.Lower(mount.Snapshot, vocabulary, issuance)
			if !lowered || template == nil || directory == nil || !template.Available() {
				t.Fatalf("lower artifact %d into a scalar template", index)
			}
			boundRoles := make([]engine.MountedProgramRole, 0, directory.RoleCount())
			for roleIndex := 0; roleIndex < directory.RoleCount(); roleIndex++ {
				key, scalar, roleOK := directory.RoleAt(roleIndex)
				capability, capabilityOK := rules.MountedCapabilityForArtifactRole(key)
				if !roleOK || !capabilityOK || !capability.Mounted() {
					t.Fatalf("artifact %d role %q has no mounted capability", index, key)
				}
				boundRoles = append(boundRoles, engine.MountedProgramRole{Scalar: scalar, Capability: capability})
			}
			boundFactors := make([]engine.MountedProgramFactor, 0, directory.FactorCount())
			for factorIndex := 0; factorIndex < directory.FactorCount(); factorIndex++ {
				key, scalar, factorOK := directory.FactorAt(factorIndex)
				capability, capabilityOK := bound.FactorCapability(key)
				if !factorOK || !capabilityOK {
					t.Fatalf("artifact %d factor %q has no sealed capability", index, key)
				}
				boundFactors = append(boundFactors, engine.MountedProgramFactor{Scalar: scalar, Capability: capability})
			}
			templates[artifactID] = template
			roles[artifactID] = boundRoles
			factors[artifactID] = boundFactors
		}
		mounts = append(mounts, engine.MountedProgramArtifact{Template: template, Roles: roles[artifactID], Factors: factors[artifactID], Module: mount.ModuleKey})
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
	contexts := record.Source.ContextDirectory()
	if !contexts.Available() || contexts.LinkID() != sourceID {
		t.Fatal("query canonical context directory")
	}
	table, tableOK := SelectedQuerySites(compilation, record.Artifacts, contexts)
	if !tableOK || table.Count() == 0 {
		t.Fatal("selected query sites")
	}
	linkAdmissions, linkOK := rules.LinkAdmissions()
	if !linkOK {
		t.Fatal("link admissions")
	}
	mountedPoint, mountedPointOK := rules.MountedPointAdmissions()
	if !mountedPointOK {
		t.Fatal("mounted-point admissions")
	}
	mounted, activations, mountedFailure := rules.MountedAdmissions(record.Artifacts, contexts)
	if mountedFailure.Available() {
		t.Fatalf("mounted admissions refused: %s", mountedFailure)
	}
	queries, queriesOK := bound.QueryAdmissions(table)
	if !queriesOK {
		t.Fatal("query admissions")
	}
	// Reuse the exact pre-bind composition owner retained by ProgramBinding,
	// then perform the same GenerationID join as production
	// pointTransitionAdmissions. The mirror must not rebuild composition or
	// mint a second transition/context authority.
	composition, compositionOK := bound.ModuleComposition()
	transitions, generations := composition.Transitions(), composition.Generations()
	if !compositionOK || len(transitions) != len(generations) {
		t.Fatal("query canonical composition admissions")
	}
	generationByID := make(map[identity.ContentID]modulecomposition.InitGeneration, len(generations))
	for index, generation := range generations {
		if !generation.Available() || !generation.ID().Available() {
			t.Fatalf("query canonical generation admission %d", index)
		}
		if _, duplicate := generationByID[generation.ID()]; duplicate {
			t.Fatalf("query canonical duplicate generation admission %d", index)
		}
		generationByID[generation.ID()] = generation
	}
	pointTransitions := make([]engine.ProgramPointTransitionAdmission, 0, len(transitions))
	seenGenerations := make(map[identity.ContentID]struct{}, len(transitions))
	for index, transition := range transitions {
		generation, generationOK := generationByID[transition.GenerationID()]
		if !transition.Available() || !generationOK || transition.GenerationID() != generation.ID() {
			t.Fatalf("query canonical composition admission %d", index)
		}
		if _, duplicate := seenGenerations[generation.ID()]; duplicate {
			t.Fatalf("query canonical duplicate transition admission %d", index)
		}
		seenGenerations[generation.ID()] = struct{}{}
		pointTransitions = append(pointTransitions, engine.ProgramPointTransitionAdmission{Transition: transition, Generation: generation})
	}
	if len(seenGenerations) != len(generationByID) {
		t.Fatal("query canonical composition generations are not all joined")
	}
	program, refusal, committed := engine.ConstructProgram(engine.ProgramDeclaration{
		Binding:          bound.SchemaBinding(),
		Mounts:           mounts,
		Bootstrap:        witness,
		Contexts:         contexts,
		Admission:        engine.MountedProgramAdmission{Link: linkAdmissions, Mounted: mounted, MountedPoint: mountedPoint, Activation: activations, Queries: queries},
		PointTransitions: pointTransitions,
	})
	if !committed || program == nil {
		row, rowOK := refusal.ConstructionRow()
		t.Fatalf("construct committed query program: stage=%v lowered=%t lowering=%v seal=%v construction=%v row=%d/%t schedule=%d", refusal.Stage(), refusal.Lowered(), refusal.LoweringFailure(), refusal.Seal(), refusal.Commit(), row, rowOK, refusal.ScheduleRow())
	}
	return program, table
}

func TestQueryPublicationsSealFamilyCodecAndCanonicalizeBorrowedAnswers(t *testing.T) {
	record := mountedRecord(t, "query-canonical-cell", "local root = {}; return root")
	bound := materializerBinding(t, record)
	catalogState := bound.Compilation().catalog
	committed, table := queryCanonicalProgram(t, record, bound)

	publications, published := bound.QueryPublications(committed, table)
	if !published || len(publications) != table.Count() {
		t.Fatalf("query publications = %d/%t, sites = %d", len(publications), published, table.Count())
	}
	valuePublications := make([]QueryPublication, 0, 1)
	effectPublications := make([]QueryPublication, 0, 1)
	placementPublications := make([]QueryPublication, 0, 1)
	for index, publication := range publications {
		contract := publication.Contract()
		if !publication.Key.Available() || !contract.Available() || !contract.FamilyID().Available() || !contract.Codec().Available() {
			t.Fatalf("publication %d does not carry complete address and codec identity", index)
		}
		position, positioned := queryPositionForFamily(catalogState, publication.Site.Family)
		if !positioned || position < 0 || position >= len(catalogState.queries) || catalogState.queries[position] == nil {
			t.Fatalf("publication %d names no sealed query registration", index)
		}
		registration := catalogState.queries[position]
		if contract.FamilyID() != identity.ContentID(registration.EntryID()) || contract.Codec() != registration.Freezer() {
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
		case QueryFamilyPlacementSummary:
			placementPublications = append(placementPublications, publication)
		default:
			t.Fatalf("publication %d names unknown family %q", index, publication.Site.Family)
		}
	}
	if len(valuePublications) == 0 || len(effectPublications) == 0 || len(placementPublications) == 0 {
		t.Fatal("fixture did not publish Value, Effect, and Placement query families")
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
	placementHits := make([]queryHit, 0, len(placementPublications))
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
		case QueryFamilyPlacementSummary:
			placementHits = append(placementHits, hit)
		default:
			t.Fatalf("query answer names unknown family %q", publication.Site.Family)
		}
	}
	if len(valueHits) == 0 || len(effectHits) == 0 || len(placementHits) == 0 {
		t.Fatal("fixture yielded no borrowed answers for Value, Effect, and Placement query families")
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
	for index, hit := range placementHits {
		cell, encoded := hit.publication.CanonicalCell(hit.answer)
		if !encoded || !cell.Available() || cell.ContractID() != hit.publication.Contract().ContentID() {
			t.Fatal("Placement publication did not seal its borrowed answer under its complete identity")
		}
		if index == 0 {
			if _, wrong := hit.publication.CanonicalCell(valueHits[0].answer); wrong {
				t.Fatal("Placement publication accepted a Value answer")
			}
			if _, wrong := hit.publication.CanonicalCell(effectHits[0].answer); wrong {
				t.Fatal("Placement publication accepted an Effect answer")
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
