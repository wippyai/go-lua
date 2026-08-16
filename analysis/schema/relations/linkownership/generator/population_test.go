package generator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

const populationTestPackage = "example.test/program/link"

func populationFixture(fieldCount, methodCount int) (ScanResult, ManifestSet) {
	rootSurface := typeSurfaceID(populationTestPackage, "Link")
	root := DeclarationInfo{
		PackagePath: populationTestPackage, Kind: "type", Name: "Link", Type: "struct",
		Surface: "", SourceFile: "link.go", Line: 1, Column: 1,
	}
	root.FactID = declarationFactID(root)
	declarations := []DeclarationInfo{root}
	fields := make([]StructureField, 0, fieldCount)
	assignments := make([]SurfaceAssignmentRow, 0, fieldCount+methodCount+1)
	typedStorage := make([]StorageRow, 0, fieldCount)
	for index := 0; index < fieldCount; index++ {
		name := fmt.Sprintf("Field%02d", index)
		declaration := DeclarationInfo{
			PackagePath: populationTestPackage, Kind: "field", OwnerType: "Link",
			Surface: rootSurface, Path: name, Name: name, Type: "int",
			SourceFile: "link.go", Line: index + 2, Column: 1,
		}
		declaration.FactID = declarationFactID(declaration)
		declarations = append(declarations, declaration)
		field := StructureField{DeclarationID: declaration.FactID, SurfaceID: "surface-placeholder", Embedded: false}
		// The real surface ID is filled after the root SurfaceInfo is built.
		fields = append(fields, field)
	}
	methods := make([]MethodExposure, 0, methodCount)
	for index := 0; index < methodCount; index++ {
		name := fmt.Sprintf("Method%02d", index)
		declaration := DeclarationInfo{
			PackagePath: populationTestPackage, Kind: "method", OwnerType: "Link",
			Name: name, Type: "func()", Signature: "func()",
			SourceFile: "link.go", Line: fieldCount + index + 2, Column: 1,
		}
		declaration.FactID = declarationFactID(declaration)
		declarations = append(declarations, declaration)
		exposure := MethodExposure{
			PackagePath: populationTestPackage, RootType: "Link", Surface: rootSurface,
			Set: "value", Name: name, Signature: "func()", TargetDeclID: declaration.FactID,
			Disposition: "declared",
		}
		exposure.FactID = methodExposureFactID(exposure)
		methods = append(methods, exposure)
	}
	function := DeclarationInfo{
		PackagePath: populationTestPackage, Kind: "func", Name: "Pick", Type: "func()", Signature: "func()",
		SourceFile: "link.go", Line: fieldCount + methodCount + 2, Column: 1,
	}
	function.FactID = declarationFactID(function)
	declarations = append(declarations, function)

	surface := SurfaceInfo{
		PackagePath: populationTestPackage, RootType: "Link", Surface: rootSurface,
		Kind: "named-root", Type: "struct", SourceFile: "link.go", Line: 1, Column: 1,
		OriginDeclID: root.FactID,
	}
	surface.FactID = surfaceInfoFactID(surface)
	for index := range fields {
		fields[index].SurfaceID = surface.FactID
		assignments = append(assignments, SurfaceAssignmentRow{Kind: "field", FactID: fields[index].FactID, OwnerSurface: rootSurface, Name: declarations[index+1].Name})
	}
	for _, exposure := range methods {
		assignments = append(assignments, SurfaceAssignmentRow{Kind: "effective-method", FactID: exposure.FactID, OwnerSurface: rootSurface, Name: exposure.Name})
	}
	assignments = append(assignments, SurfaceAssignmentRow{Kind: "semantic-package-function", FactID: function.FactID, OwnerSurface: "package:" + populationTestPackage, Name: function.Name})
	for index := range fields {
		fields[index].FactID = structureFieldFactID(fields[index])
		// The assignment was assembled before FactID was available.
		assignments[index].FactID = fields[index].FactID
		typedStorage = append(typedStorage, StorageRow{FactID: fields[index].FactID, OwnerSurface: rootSurface, Disposition: StoragePublicSurface})
	}

	// Keep the positive fixture production-shaped: it exercises a typed
	// inbound caller, an ownership import, cross-owner index evidence, a
	// complete typed relation inputs, and structural facts carried by residue.
	consumerPackage := "example.test/program/consumer"
	useOne := UseSite{
		PackagePath: consumerPackage, SourceFile: "consumer.go", Line: 2, Column: 1,
		Symbol: "Link", Evidence: "selector", Type: "Link", TargetDeclID: root.FactID, Role: Reference,
	}
	useOne.FactID = useSiteFactID(useOne)
	useTwo := UseSite{
		PackagePath: consumerPackage, SourceFile: "consumer.go", Line: 3, Column: 1,
		Symbol: "LinkAlias", Evidence: "alias", Type: "Link", TargetDeclID: root.FactID, Role: TypeInstance,
	}
	useTwo.FactID = useSiteFactID(useTwo)
	uses := []UseSite{useOne, useTwo}

	array := StructureArray{DeclarationID: root.FactID, SurfaceID: surface.FactID, Path: "Array", Element: "int", Length: 1}
	array.FactID = structureArrayFactID(array)
	slice := StructureSlice{DeclarationID: root.FactID, SurfaceID: surface.FactID, Path: "Slice", Element: "string"}
	slice.FactID = structureSliceFactID(slice)
	mapRow := StructureMap{DeclarationID: root.FactID, SurfaceID: surface.FactID, Path: "Map", Key: "string", Value: "int"}
	mapRow.FactID = structureMapFactID(mapRow)
	named := StructureNamedReference{
		DeclarationID: declarations[1].FactID, SurfaceID: surface.FactID, Path: "Field00.Target",
		TargetDeclID: root.FactID, TargetPackagePath: populationTestPackage, TargetName: "Link", Origin: true,
	}
	named.FactID = structureNamedReferenceFactID(named)
	sourceFacts := []string{array.FactID}
	sort.Strings(sourceFacts)
	splitRecipients := []string{"owner-consumer", "owner-link"}
	splitPlan := splitPlanID(splitRecipients)

	declarationRows := make([]DeclarationRow, 0, len(declarations))
	for _, declaration := range declarations {
		declarationRows = append(declarationRows, DeclarationRow{
			FactID: declaration.FactID, PackagePath: declaration.PackagePath, Kind: declaration.Kind,
			Owner: "owner-link", Surface: rootSurface, Name: declaration.Name,
			Type: declaration.Type, Signature: declaration.Signature,
		})
	}
	useRows := []UseRow{
		{FactID: useOne.FactID, PackagePath: useOne.PackagePath, SourceFile: useOne.SourceFile, Line: useOne.Line, Column: useOne.Column, Symbol: useOne.Symbol, Evidence: useOne.Evidence, TargetDeclID: useOne.TargetDeclID, Type: useOne.Type, Role: useOne.Role},
		{FactID: useTwo.FactID, PackagePath: useTwo.PackagePath, SourceFile: useTwo.SourceFile, Line: useTwo.Line, Column: useTwo.Column, Symbol: useTwo.Symbol, Evidence: useTwo.Evidence, TargetDeclID: useTwo.TargetDeclID, Type: useTwo.Type, Role: useTwo.Role},
	}
	scan := ScanResult{
		Root: RootInventory{PackagePath: populationTestPackage, SourceDir: "."},
		Sources: SourceInventory{Packages: []PackageInfo{
			{Path: populationTestPackage, Name: "link", Directory: "."},
			{Path: consumerPackage, Name: "consumer", Directory: "."},
		}, ProductionSources: []ProductionSource{
			{PackagePath: populationTestPackage, Path: "link.go"},
			{PackagePath: consumerPackage, Path: "consumer.go"},
		}},
		Types: TypeInventory{
			Declarations: declarations,
			Exposures:    methods,
			Surfaces:     []SurfaceInfo{surface},
			Structure: StructureProjection{
				Fields: fields, Arrays: []StructureArray{array}, Slices: []StructureSlice{slice}, Maps: []StructureMap{mapRow},
				NamedReferences: []StructureNamedReference{named},
			},
		},
		Uses: uses,
		Dependencies: DependencyInventory{ImportEdges: []ImportEdge{{
			From: consumerPackage, To: populationTestPackage, SourceFile: "consumer.go", Line: 1, Column: 1,
		}}},
		ProductionOnly: true,
	}
	set := ManifestSet{
		Catalog: CatalogManifest{
			Owners: []OwnerRow{
				{ID: "owner-consumer", PackagePath: consumerPackage, Surface: "package:" + consumerPackage, Kind: "component"},
				{ID: "owner-link", PackagePath: populationTestPackage, Surface: rootSurface, Kind: "root"},
			},
			Declarations: declarationRows,
			Uses:         useRows,
			ImportEdges:  []OwnershipImportEdgeRow{{FromOwner: "owner-consumer", ToOwner: "owner-link", SourceFile: "consumer.go", Line: 1, Column: 1}},
		},
		Indexes: IndexManifest{
			IndexPlans: []IndexPlanRow{{
				ID: "index-plan-link", Owner: "owner-link", DeclarationFactID: function.FactID,
				SourceFactIDs: sourceFacts,
			}},
			ReferencePlans: []ReferencePlanRow{{
				ID: "reference-plan-hot", Issuer: "owner-link", Consumer: "owner-consumer", DeclarationFactID: function.FactID,
				SourceFactIDs: sourceFacts,
			}},
			IdentityPlans: []IdentityPlanRow{{
				ID: "identity-plan-link", Owner: "owner-link", DeclarationFactID: function.FactID,
				RelationKind: IdentityRelationDirect, DirectFactIDs: sourceFacts,
			}},
		},
		Surfaces: SurfaceManifest{Assignments: assignments, Storage: typedStorage},
		Residue: ResidueManifest{Rows: []ResidueRow{
			{Kind: "delete", CurrentFact: array.FactID, Destination: ResidueDeleteDestination},
			{Kind: "move", CurrentFact: named.FactID, Destination: "owner-link"},
			{Kind: "split", CurrentFact: mapRow.FactID, Destination: splitPlan},
			{Kind: "move", CurrentFact: slice.FactID, Destination: "owner-link"},
		}, SplitPlans: []SplitPlanRow{{ID: splitPlan, OwnerIDs: splitRecipients}}},
	}
	return scan, set
}

func TestValidateManifestPopulationAcceptsExactTypedPopulation(t *testing.T) {
	scan, manifests := populationFixture(2, 2)
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestPopulationRejectsUnusedOwner(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	manifests.Catalog.Owners = append(manifests.Catalog.Owners, OwnerRow{
		ID: "owner-unused", PackagePath: populationTestPackage, Surface: "package:" + populationTestPackage, Kind: "component",
	})
	err := ValidateManifestPopulation(scan, manifests)
	if !errors.Is(err, ErrManifestPopulation) || !strings.Contains(err.Error(), "owner-unused") {
		t.Fatalf("error=%v, want unused-owner rejection", err)
	}
}

func TestValidateManifestPopulationJoinsResidueDestinations(t *testing.T) {
	tests := []struct {
		name string
		edit func(ScanResult, *ManifestSet)
	}{
		{name: "junk destination", edit: func(_ ScanResult, manifests *ManifestSet) {
			manifests.Residue.Rows[1].Destination = "junk"
		}},
		{name: "wrong owner", edit: func(_ ScanResult, manifests *ManifestSet) {
			manifests.Residue.Rows[1].Destination = "owner-consumer"
		}},
		{name: "exposure wrong owner", edit: func(scan ScanResult, manifests *ManifestSet) {
			exposure := scan.Types.Exposures[0]
			assignments := manifests.Surfaces.Assignments[:0]
			for _, assignment := range manifests.Surfaces.Assignments {
				if assignment.FactID != exposure.FactID {
					assignments = append(assignments, assignment)
				}
			}
			manifests.Surfaces.Assignments = assignments
			manifests.Residue.Rows = append(manifests.Residue.Rows, ResidueRow{Kind: "move", CurrentFact: exposure.FactID, Destination: "owner-consumer"})
		}},
		{name: "wrong delete constant", edit: func(_ ScanResult, manifests *ManifestSet) {
			manifests.Residue.Rows[0].Destination = "v1:not-private-representation"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(1, 1)
			test.edit(scan, &manifests)
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationAcceptsExposureMoveOwnerJoin(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	exposure := scan.Types.Exposures[0]
	assignments := manifests.Surfaces.Assignments[:0]
	for _, assignment := range manifests.Surfaces.Assignments {
		if assignment.FactID != exposure.FactID {
			assignments = append(assignments, assignment)
		}
	}
	manifests.Surfaces.Assignments = assignments
	manifests.Residue.Rows = append(manifests.Residue.Rows, ResidueRow{Kind: "move", CurrentFact: exposure.FactID, Destination: "owner-link"})
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestPopulationReferenceCallersAreConsumerScoped(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	query := scan.Types.Declarations[len(scan.Types.Declarations)-1]
	consumerTwoPackage := "example.test/program/consumer-two"
	first := UseSite{
		PackagePath: "example.test/program/consumer", SourceFile: "consumer.go", Line: 10, Column: 1,
		Symbol: "Pick", Evidence: "call", Type: query.Type, TargetDeclID: query.FactID, Role: CallCallee,
	}
	first.FactID = useSiteFactID(first)
	second := UseSite{
		PackagePath: consumerTwoPackage, SourceFile: "consumer-two.go", Line: 10, Column: 1,
		Symbol: "Pick", Evidence: "call", Type: query.Type, TargetDeclID: query.FactID, Role: CallCallee,
	}
	second.FactID = useSiteFactID(second)
	useRow := func(use UseSite) UseRow {
		return UseRow{FactID: use.FactID, PackagePath: use.PackagePath, SourceFile: use.SourceFile, Line: use.Line, Column: use.Column, Symbol: use.Symbol, Evidence: use.Evidence, TargetDeclID: use.TargetDeclID, Type: use.Type, Role: use.Role}
	}
	scan.Uses = []UseSite{first, second}
	scan.Sources.Packages = append(scan.Sources.Packages, PackageInfo{Path: consumerTwoPackage, Name: "consumer_two", Directory: "."})
	scan.Sources.ProductionSources = append(scan.Sources.ProductionSources, ProductionSource{PackagePath: consumerTwoPackage, Path: "consumer-two.go"})
	scan.Dependencies.ImportEdges = append(scan.Dependencies.ImportEdges, ImportEdge{From: consumerTwoPackage, To: populationTestPackage, SourceFile: "consumer-two.go", Line: 1, Column: 1})
	manifests.Catalog.Uses = []UseRow{useRow(first), useRow(second)}
	manifests.Catalog.Owners = append(manifests.Catalog.Owners, OwnerRow{ID: "owner-consumer-two", PackagePath: consumerTwoPackage, Surface: "package:" + consumerTwoPackage, Kind: "component"})
	manifests.Catalog.ImportEdges = append(manifests.Catalog.ImportEdges, OwnershipImportEdgeRow{FromOwner: "owner-consumer-two", ToOwner: "owner-link", SourceFile: "consumer-two.go", Line: 1, Column: 1})
	globalCallers := []string{first.FactID, second.FactID}
	sort.Strings(globalCallers)
	manifests.Indexes.IndexPlans[0].CallerUseFactIDs = globalCallers
	manifests.Indexes.ReferencePlans = []ReferencePlanRow{
		{ID: "reference-plan-one", Issuer: "owner-link", Consumer: "owner-consumer", DeclarationFactID: query.FactID, SourceFactIDs: manifests.Indexes.ReferencePlans[0].SourceFactIDs, CallerUseFactIDs: []string{first.FactID}},
		{ID: "reference-plan-two", Issuer: "owner-link", Consumer: "owner-consumer-two", DeclarationFactID: query.FactID, SourceFactIDs: manifests.Indexes.ReferencePlans[0].SourceFactIDs, CallerUseFactIDs: []string{second.FactID}},
	}
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatalf("complete consumer-scoped caller sets rejected: %v", err)
	}
	for _, test := range []struct {
		name    string
		callers []string
	}{
		{name: "cross-consumer", callers: globalCallers},
		{name: "subset", callers: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := manifests
			bad.Indexes.ReferencePlans = append([]ReferencePlanRow(nil), manifests.Indexes.ReferencePlans...)
			bad.Indexes.ReferencePlans[0].CallerUseFactIDs = test.callers
			err := ValidateManifestPopulation(scan, bad)
			if !errors.Is(err, ErrManifestPopulation) || !strings.Contains(err.Error(), "callers") {
				t.Fatalf("error=%v, want consumer-scoped caller rejection", err)
			}
		})
	}
}

func TestValidateManifestPopulationJoinsStorageDisposition(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ManifestSet)
	}{
		{name: "public requires assignment", edit: func(manifests *ManifestSet) {
			manifests.Surfaces.Assignments = manifests.Surfaces.Assignments[1:]
		}},
		{name: "unadmitted disposition is closed", edit: func(manifests *ManifestSet) {
			manifests.Surfaces.Storage[0].Disposition = StorageDisposition("v2:unadmitted")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(1, 1)
			test.edit(&manifests)
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationRejectsFinalIndexWithoutWitness(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	query := scan.Types.Declarations[len(scan.Types.Declarations)-1]
	manifests.Indexes.IndexPlans = nil
	manifests.Indexes.ReferencePlans = nil
	manifests.Indexes.IdentityPlans = nil
	manifests.Indexes.Indexes = []IndexRow{{
		ID: "index-final", Owner: "owner-link", QueryFactID: query.FactID,
		SourceFactIDs: []string{scan.Types.Structure.Arrays[0].FactID},
		PatternID:     "pattern:unadmitted",
	}}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) || !strings.Contains(err.Error(), "no same-scan witness") {
		t.Fatalf("error=%v, want same-scan witness rejection", err)
	}
}

func TestValidateManifestPopulationAllowsLexicallyLaterIdentityParent(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	declaration := scan.Types.Declarations[len(scan.Types.Declarations)-1]
	source := scan.Types.Structure.Arrays[0].FactID
	var parent, child IdentityRow
	found := false
	for index := 0; index < 4096 && !found; index++ {
		parent = IdentityRow{
			ID: "parent-placeholder", Owner: "owner-link", DeclarationFactID: declaration.FactID,
			PatternID: fmt.Sprintf("pattern:parent-%04d", index), RelationKind: IdentityRelationDirect,
			DirectFactIDs: []string{source},
		}
		parentID, err := identityDigest(parent, []IdentityRow{parent})
		if err != nil {
			t.Fatal(err)
		}
		parent.ID = parentID
		child = IdentityRow{
			ID: "child-placeholder", Owner: "owner-link", DeclarationFactID: declaration.FactID,
			PatternID: fmt.Sprintf("pattern:child-%04d", index), RelationKind: IdentityRelationComposite,
			DirectFactIDs: []string{source}, ParentIdentityIDs: []string{parent.ID},
		}
		childID, err := identityDigest(child, []IdentityRow{parent, child})
		if err != nil {
			t.Fatal(err)
		}
		child.ID = childID
		found = parent.ID > child.ID
	}
	if !found {
		t.Fatal("could not construct a parent digest that sorts after its child")
	}
	manifests.Indexes.Identities = []IdentityRow{child, parent}
	err := ValidateManifestPopulation(scan, manifests)
	if !errors.Is(err, ErrManifestPopulation) || !strings.Contains(err.Error(), "no same-scan witness") {
		t.Fatalf("error=%v, want no-witness rejection after accepting lexical parent order", err)
	}
}

func TestValidateManifestPopulationRejectsUnsurfacedStructuralStorage(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	array := scan.Types.Structure.Arrays[0]
	array.DeclarationID = scan.Types.Declarations[len(scan.Types.Declarations)-1].FactID
	array.SurfaceID = ""
	array.Path = ""
	array.FactID = structureArrayFactID(array)
	scan.Types.Structure.Arrays[0] = array
	manifests.Residue.Rows = manifests.Residue.Rows[1:]
	manifests.Surfaces.Storage = append(manifests.Surfaces.Storage, StorageRow{
		FactID: array.FactID, OwnerSurface: "package:" + populationTestPackage, Disposition: StoragePublicSurface,
	})
	manifests.Indexes.IndexPlans[0].SourceFactIDs = []string{array.FactID}
	manifests.Indexes.ReferencePlans[0].SourceFactIDs = []string{array.FactID}
	manifests.Indexes.IdentityPlans[0].DirectFactIDs = []string{array.FactID}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsSameIDMismatchedFields(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	manifests.Catalog.Declarations[1].Type = "string"
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsDuplicateUseEqualLength(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	if len(manifests.Catalog.Uses) != 2 {
		t.Fatalf("fixture uses=%d, want 2", len(manifests.Catalog.Uses))
	}
	// Equal row count must not make a duplicate substitution look like a
	// permutation: use two is omitted and use one appears twice.
	manifests.Catalog.Uses[1] = manifests.Catalog.Uses[0]
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsExposureSemanticDrift(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ScanResult)
	}{
		{name: "target kind", edit: func(scan *ScanResult) {
			exposure := scan.Types.Exposures[0]
			function := scan.Types.Declarations[len(scan.Types.Declarations)-1]
			exposure.TargetDeclID = function.FactID
			exposure.Name = function.Name
			exposure.Signature = function.Signature
			exposure.FactID = methodExposureFactID(exposure)
			scan.Types.Exposures[0] = exposure
		}},
		{name: "method set", edit: func(scan *ScanResult) {
			exposure := scan.Types.Exposures[0]
			exposure.Set = "unknown-set"
			exposure.FactID = methodExposureFactID(exposure)
			scan.Types.Exposures[0] = exposure
		}},
		{name: "disposition", edit: func(scan *ScanResult) {
			exposure := scan.Types.Exposures[0]
			exposure.Disposition = "unknown-disposition"
			exposure.FactID = methodExposureFactID(exposure)
			scan.Types.Exposures[0] = exposure
		}},
		{name: "root surface", edit: func(scan *ScanResult) {
			exposure := scan.Types.Exposures[0]
			exposure.RootType = "Other"
			exposure.FactID = methodExposureFactID(exposure)
			scan.Types.Exposures[0] = exposure
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(1, 1)
			test.edit(&scan)
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationRejectsExposureForeignTargetPackage(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	foreign := scan.Types.Declarations[2]
	foreign.PackagePath = "example.test/program/consumer"
	foreign.Surface = ""
	foreign.SourceFile = "consumer.go"
	foreign.Line = 20
	foreign.FactID = declarationFactID(foreign)
	scan.Types.Declarations = append(scan.Types.Declarations, foreign)
	manifests.Catalog.Declarations = append(manifests.Catalog.Declarations, DeclarationRow{
		FactID: foreign.FactID, PackagePath: foreign.PackagePath, Kind: foreign.Kind, Owner: "owner-consumer",
		Surface: "package:" + foreign.PackagePath, Name: foreign.Name, Type: foreign.Type, Signature: foreign.Signature,
	})
	exposure := scan.Types.Exposures[0]
	exposure.TargetDeclID = foreign.FactID
	exposure.FactID = methodExposureFactID(exposure)
	scan.Types.Exposures[0] = exposure
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsExposureRootAndDispositionDrift(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ScanResult, *ManifestSet, *MethodExposure)
	}{
		{name: "synthetic surface", edit: func(scan *ScanResult, manifests *ManifestSet, exposure *MethodExposure) {
			root := scan.Types.Surfaces[0]
			nested := SurfaceInfo{
				PackagePath: populationTestPackage, RootType: "Link", Surface: root.Surface + "#Nested",
				ParentSurface: root.Surface, Path: "Nested", Kind: "anonymous-state", Type: "struct",
				SourceFile: root.SourceFile, Line: root.Line + 1, Column: root.Column, OriginDeclID: scan.Types.Declarations[1].FactID,
			}
			nested.FactID = surfaceInfoFactID(nested)
			scan.Types.Surfaces = append(scan.Types.Surfaces, nested)
			exposure.Surface = nested.Surface
		}},
		{name: "declared receiver", edit: func(scan *ScanResult, manifests *ManifestSet, exposure *MethodExposure) {
			target := scan.Types.Declarations[2]
			target.OwnerType = "Other"
			target.SourceFile = "other.go"
			target.Line = 50
			target.FactID = declarationFactID(target)
			scan.Types.Declarations = append(scan.Types.Declarations, target)
			manifests.Catalog.Declarations = append(manifests.Catalog.Declarations, DeclarationRow{
				FactID: target.FactID, PackagePath: target.PackagePath, Kind: target.Kind, Owner: "owner-link",
				Surface: typeSurfaceID(populationTestPackage, "Link"), Name: target.Name, Type: target.Type, Signature: target.Signature,
			})
			exposure.TargetDeclID = target.FactID
		}},
		{name: "promoted receiver", edit: func(_ *ScanResult, _ *ManifestSet, exposure *MethodExposure) {
			exposure.Disposition = "promoted"
		}},
		{name: "aliased named root", edit: func(_ *ScanResult, _ *ManifestSet, exposure *MethodExposure) {
			exposure.Disposition = "aliased"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(1, 1)
			oldFactID := scan.Types.Exposures[0].FactID
			exposure := scan.Types.Exposures[0]
			test.edit(&scan, &manifests, &exposure)
			exposure.FactID = methodExposureFactID(exposure)
			scan.Types.Exposures[0] = exposure
			for index := range manifests.Surfaces.Assignments {
				assignment := &manifests.Surfaces.Assignments[index]
				if assignment.FactID != oldFactID {
					continue
				}
				assignment.FactID = exposure.FactID
				if test.name == "synthetic surface" {
					assignment.OwnerSurface = exposure.Surface
				}
			}
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationRejectsMissingAndExtraFacts(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	manifests.Catalog.Declarations = manifests.Catalog.Declarations[:len(manifests.Catalog.Declarations)-1]
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("missing error=%v, want ErrManifestPopulation", err)
	}

	scan, manifests = populationFixture(1, 1)
	manifests.Residue.Rows = []ResidueRow{{Kind: "delete", CurrentFact: "not-scanned", Destination: "old"}}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("extra error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsOwnerDAGCycle(t *testing.T) {
	owners := map[string]OwnerRow{
		"a": {ID: "a", PackagePath: "a", Surface: "package:a", Kind: "component"},
		"b": {ID: "b", PackagePath: "b", Surface: "package:b", Kind: "component"},
	}
	err := populationOwnerDAG(owners, map[string][]string{"a": {"b"}, "b": {"a"}})
	if !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsResidueOverlap(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	fieldFact := scan.Types.Structure.Fields[0].FactID
	manifests.Residue.Rows = []ResidueRow{{Kind: "delete", CurrentFact: fieldFact, Destination: "old"}}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsDirectEmptyStructurePathOwnerDrift(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	function := scan.Types.Declarations[len(scan.Types.Declarations)-1]
	row := scan.Types.Structure.Arrays[0]
	row.DeclarationID = function.FactID
	row.SurfaceID = ""
	row.Path = ""
	row.FactID = structureArrayFactID(row)
	scan.Types.Structure.Arrays[0] = row
	manifests.Residue.Rows[0].CurrentFact = row.FactID
	manifests.Indexes.IndexPlans[0].SourceFactIDs = []string{row.FactID}
	manifests.Indexes.ReferencePlans[0].SourceFactIDs = []string{row.FactID}
	manifests.Indexes.IdentityPlans[0].DirectFactIDs = []string{row.FactID}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsEmptySurfacedStructurePath(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	root := scan.Types.Surfaces[0]
	nested := SurfaceInfo{
		PackagePath: populationTestPackage, RootType: "Link", Surface: root.Surface + "#Nested",
		ParentSurface: root.Surface, Path: "Nested", Kind: "anonymous-state", Type: "struct",
		SourceFile: root.SourceFile, Line: root.Line + 1, Column: root.Column, OriginDeclID: scan.Types.Declarations[1].FactID,
	}
	nested.FactID = surfaceInfoFactID(nested)
	scan.Types.Surfaces = append(scan.Types.Surfaces, nested)
	row := scan.Types.Structure.Arrays[0]
	row.SurfaceID = nested.FactID
	row.Path = ""
	row.FactID = structureArrayFactID(row)
	scan.Types.Structure.Arrays[0] = row
	manifests.Residue.Rows[0].CurrentFact = row.FactID
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestProductionStructurePopulationPathCensus(t *testing.T) {
	scan, err := Scan(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	packages, err := populationPackages(scan)
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := populationSurfaces(scan, packages)
	if err != nil {
		t.Fatal(err)
	}
	emptySurfaced := 0
	emptyKinds := make(map[string]int)
	check := func(kind, path, surfaceID string) {
		t.Helper()
		if path != "" || surfaceID == "" {
			return
		}
		emptySurfaced++
		if surface, exists := findSurfaceByID(surfaces, surfaceID); exists {
			emptyKinds[surface.Kind]++
		}
		if err := populationStructurePath(kind, path, surfaceID, surfaces); err != nil {
			t.Fatalf("production %s: %v", kind, err)
		}
	}
	for _, row := range scan.Types.Structure.Arrays {
		check("array", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.Slices {
		check("slice", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.Maps {
		check("map", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.Channels {
		check("channel", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.NamedReferences {
		check("named-reference", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.MethodReferences {
		check("method-reference", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.OtherReferences {
		check("other-reference", row.Path, row.SurfaceID)
	}
	for _, row := range scan.Types.Structure.Cycles {
		check("cycle", row.Path, row.SurfaceID)
	}
	declarations := make(map[string]DeclarationInfo, len(scan.Types.Declarations))
	for _, declaration := range scan.Types.Declarations {
		declarations[declaration.FactID] = declaration
	}
	internalMethods := 0
	for _, row := range scan.Types.Structure.MethodReferences {
		if row.TargetDeclID == "" {
			continue
		}
		internalMethods++
		target, exists := declarations[row.TargetDeclID]
		if !exists || target.Kind != "interface-method" || target.Type != row.Type || declarationMethodReceiver(target) != row.Receiver || populationMethodObjectKey(target) != row.MethodKey {
			t.Fatalf("production internal method target is not an exact declaration join: row=%+v target=%+v", row, target)
		}
	}
	if emptySurfaced == 0 {
		t.Fatal("production scan had no surfaced empty-path facts to exercise root legality")
	}
	t.Logf("production surfaced empty-path census: %d by kind=%v; internal method references=%d", emptySurfaced, emptyKinds, internalMethods)
}

func TestValidateManifestPopulationRejectsNamedReferenceNonRootTarget(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	row := scan.Types.Structure.NamedReferences[0]
	target := scan.Types.Declarations[1]
	row.TargetDeclID = target.FactID
	row.TargetName = target.Name
	row.FactID = structureNamedReferenceFactID(row)
	scan.Types.Structure.NamedReferences[0] = row
	for index := range manifests.Residue.Rows {
		if manifests.Residue.Rows[index].CurrentFact != "" {
			// The named row is the second residue row in the positive fixture.
			if index == 1 {
				manifests.Residue.Rows[index].CurrentFact = row.FactID
			}
		}
	}
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationRejectsOpenOtherReferenceDisposition(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	row := StructureOtherReference{
		DeclarationID: scan.Types.Declarations[0].FactID, SurfaceID: scan.Types.Surfaces[0].FactID,
		Path: "Other", Disposition: OtherReferenceDisposition(255), Type: "int",
	}
	row.FactID = structureOtherReferenceFactID(row)
	scan.Types.Structure.OtherReferences = append(scan.Types.Structure.OtherReferences, row)
	manifests.Residue.Rows = append(manifests.Residue.Rows, ResidueRow{Kind: "move", CurrentFact: row.FactID, Destination: "other"})
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationAllowsMultipleOwnersPerPackage(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	manifests.Catalog.Owners = append(manifests.Catalog.Owners, OwnerRow{
		ID: "owner-consumer-alt", PackagePath: "example.test/program/consumer", Surface: "package:example.test/program/consumer", Kind: "component",
	})
	manifests.Catalog.ImportEdges = append(manifests.Catalog.ImportEdges, OwnershipImportEdgeRow{
		FromOwner: "owner-consumer-alt", ToOwner: "owner-link", SourceFile: "consumer.go", Line: 1, Column: 1,
	})
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
	manifests.Catalog.ImportEdges[0].ToOwner = "owner-consumer-alt"
	if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("wrong endpoint error=%v, want ErrManifestPopulation", err)
	}
}

func TestValidateManifestPopulationDoesNotInventInboundConnectorOwner(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	// A production source outside Sources.Packages is typed/import evidence,
	// not a Link-family owner surface.  Population must not require or invent
	// an inbound connector owner for it.
	scan.Sources.ProductionSources = append(scan.Sources.ProductionSources, ProductionSource{
		PackagePath: "example.test/inbound/connector", Path: "connector.go",
	})
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestPopulationRejectsOpenResidueKinds(t *testing.T) {
	for _, kind := range []string{"", "default", "parallel", "arbitrary"} {
		t.Run(kind, func(t *testing.T) {
			scan, manifests := populationFixture(1, 1)
			manifests.Residue.Rows[0].Kind = kind
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationRejectsSeventeenthFieldOrMethod(t *testing.T) {
	for _, test := range []struct {
		name            string
		fields, methods int
	}{
		{name: "field", fields: 17},
		{name: "method", methods: 17},
	} {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(test.fields, test.methods)
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationAcceptsOmittedSurfaceFactsAsResidue(t *testing.T) {
	tests := []struct {
		name            string
		fields, methods int
		fact            func(ScanResult) string
	}{
		{name: "seventeenth field", fields: 17, fact: func(scan ScanResult) string {
			return scan.Types.Structure.Fields[16].FactID
		}},
		{name: "seventeenth method", methods: 17, fact: func(scan ScanResult) string {
			return scan.Types.Exposures[16].FactID
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationFixture(test.fields, test.methods)
			omitted := test.fact(scan)
			allAssignments := manifests.Surfaces.Assignments
			assignments := make([]SurfaceAssignmentRow, 0, len(allAssignments)-1)
			for _, assignment := range allAssignments {
				if assignment.FactID != omitted {
					assignments = append(assignments, assignment)
				}
			}
			manifests.Surfaces.Assignments = assignments
			storage := make([]StorageRow, 0, len(manifests.Surfaces.Storage))
			for _, row := range manifests.Surfaces.Storage {
				if row.FactID != omitted {
					storage = append(storage, row)
				}
			}
			manifests.Surfaces.Storage = storage
			rowKind, destination := "move", "owner-link"
			if test.name == "seventeenth method" {
				rowKind, destination = "delete", ResidueDeleteDestination
			}
			manifests.Residue.Rows = append(manifests.Residue.Rows, ResidueRow{Kind: rowKind, CurrentFact: omitted, Destination: destination})
			if err := ValidateManifestPopulation(scan, manifests); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateManifestPopulationAllowsOmittedPackageFunctionAssignment(t *testing.T) {
	scan, manifests := populationFixture(1, 1)
	assignments := make([]SurfaceAssignmentRow, 0, len(manifests.Surfaces.Assignments)-1)
	for _, assignment := range manifests.Surfaces.Assignments {
		if assignment.Kind != "semantic-package-function" {
			assignments = append(assignments, assignment)
		}
	}
	manifests.Surfaces.Assignments = assignments
	// The declaration remains a catalog-final fact; omitting only its surface
	// publication assignment is legal in inventory mode.
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
}

func populationMethodReferenceFixture() (ScanResult, ManifestSet) {
	scan, manifests := populationFixture(1, 1)
	method := DeclarationInfo{
		PackagePath: populationTestPackage, Kind: "interface-method", OwnerType: "API",
		Name: "Do", Type: "func()", Signature: "func()", SourceFile: "api.go", Line: 50, Column: 1,
	}
	method.FactID = declarationFactID(method)
	scan.Types.Declarations = append(scan.Types.Declarations, method)
	manifests.Catalog.Declarations = append(manifests.Catalog.Declarations, DeclarationRow{
		FactID: method.FactID, PackagePath: method.PackagePath, Kind: method.Kind, Owner: "owner-link",
		Surface: typeSurfaceID(populationTestPackage, "Link"), Name: method.Name, Type: method.Type, Signature: method.Signature,
	})
	row := StructureMethodReference{
		DeclarationID: scan.Types.Declarations[0].FactID, SurfaceID: scan.Types.Surfaces[0].FactID, Path: "API.Do",
		TargetDeclID: method.FactID, TargetPackagePath: method.PackagePath, TargetName: method.Name,
		MethodKey: populationMethodObjectKey(method), Type: method.Type, Receiver: declarationMethodReceiver(method),
	}
	row.FactID = structureMethodReferenceFactID(row)
	scan.Types.Structure.MethodReferences = append(scan.Types.Structure.MethodReferences, row)
	manifests.Residue.Rows = append(manifests.Residue.Rows, ResidueRow{Kind: "move", CurrentFact: row.FactID, Destination: "owner-link"})
	return scan, manifests
}

func TestValidateManifestPopulationRejectsInternalMethodReferenceDrift(t *testing.T) {
	tests := []struct {
		name string
		edit func(*StructureMethodReference)
	}{
		{name: "type", edit: func(row *StructureMethodReference) { row.Type += ".changed" }},
		{name: "receiver", edit: func(row *StructureMethodReference) { row.Receiver += ".changed" }},
		{name: "method key", edit: func(row *StructureMethodReference) { row.MethodKey += ".changed" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan, manifests := populationMethodReferenceFixture()
			row := scan.Types.Structure.MethodReferences[0]
			oldFactID := row.FactID
			test.edit(&row)
			row.FactID = structureMethodReferenceFactID(row)
			scan.Types.Structure.MethodReferences[0] = row
			for index := range manifests.Residue.Rows {
				if manifests.Residue.Rows[index].CurrentFact == oldFactID {
					manifests.Residue.Rows[index].CurrentFact = row.FactID
				}
			}
			if err := ValidateManifestPopulation(scan, manifests); !errors.Is(err, ErrManifestPopulation) {
				t.Fatalf("error=%v, want ErrManifestPopulation", err)
			}
		})
	}
}

func TestValidateManifestPopulationAcceptsInternalMethodReference(t *testing.T) {
	scan, manifests := populationMethodReferenceFixture()
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		t.Fatal(err)
	}
}
