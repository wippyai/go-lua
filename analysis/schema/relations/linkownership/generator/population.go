package generator

// This file is the cold population proof for the ownership ledger.  It is
// deliberately a pure join: it consumes an already completed production
// ScanResult and an already parsed ManifestSet, and retains no indexes in the
// result (the temporary indexes below die with the validation call).  In
// particular, it does not infer an owner from a package name, a surface from
// a declaration, or an index/query relation from source text.

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var ErrManifestPopulation = errors.New("link ownership: manifest population validation failed")

// ValidateManifestPopulation proves that manifests are an exact population
// of scan facts.  The four manifest files must have been parsed by
// ParseManifestFiles; the validator nevertheless repeats cross-file checks so
// callers cannot manufacture a permissive ManifestSet by hand.
func ValidateManifestPopulation(scan ScanResult, manifests ManifestSet) error {
	if !scan.ProductionOnly {
		return populationError("scan is not production-only")
	}
	if scan.Root.PackagePath == "" {
		return populationError("scan root package is empty")
	}

	packages, err := populationPackages(scan)
	if err != nil {
		return err
	}
	surfaces, err := populationSurfaces(scan, packages)
	if err != nil {
		return err
	}
	declarations, facts, err := populationDeclarations(scan, manifests, packages, surfaces)
	if err != nil {
		return err
	}
	if err := populationSurfaceOrigins(scan.Types.Surfaces, declarations, surfaces); err != nil {
		return err
	}
	if err := populationUses(scan, manifests, declarations, facts); err != nil {
		return err
	}
	if err := populationStructure(scan, declarations, surfaces, packages, facts); err != nil {
		return err
	}
	if err := populationExposures(scan, declarations, surfaces, facts); err != nil {
		return err
	}
	if err := populationImports(scan, manifests, packages, surfaces); err != nil {
		return err
	}
	if err := populationIndexes(scan, manifests, declarations, surfaces, facts); err != nil {
		return err
	}
	finalFacts, err := populationSurfaceAssignments(scan, manifests, declarations, surfaces, facts)
	if err != nil {
		return err
	}
	if err := populationStructuralStorage(scan, manifests, surfaces, declarations, facts, finalFacts); err != nil {
		return err
	}
	if err := populationResidue(scan, manifests, facts, finalFacts); err != nil {
		return err
	}
	if err := populationRejectUnusedOwners(manifests); err != nil {
		return err
	}
	return nil
}

func populationError(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrManifestPopulation, fmt.Sprintf(format, args...))
}

type populationFact struct {
	kind string
}

func addPopulationFact(facts map[string]populationFact, id, kind string) error {
	if id == "" {
		return populationError("%s fact has empty ID", kind)
	}
	if previous, exists := facts[id]; exists {
		return populationError("fact %q is duplicated across %s and %s", id, previous.kind, kind)
	}
	facts[id] = populationFact{kind: kind}
	return nil
}

func populationPackages(scan ScanResult) (map[string]PackageInfo, error) {
	// Sources.Packages is the Link-family owner universe.  Packages discovered
	// only through inbound ProductionSources, typed uses, or import evidence
	// remain evidence rows; they never become authored Link surface owners by
	// inference.
	result := make(map[string]PackageInfo, len(scan.Sources.Packages)+1)
	for _, pkg := range scan.Sources.Packages {
		if pkg.Path == "" || pkg.Name == "" || pkg.Directory == "" {
			return nil, populationError("incomplete scanned package %+v", pkg)
		}
		if _, exists := result[pkg.Path]; exists {
			return nil, populationError("duplicate scanned package %q", pkg.Path)
		}
		result[pkg.Path] = pkg
	}
	if _, exists := result[scan.Root.PackagePath]; !exists {
		return nil, populationError("root package %q is absent from scanned package inventory", scan.Root.PackagePath)
	}
	return result, nil
}

type populationSurface struct {
	info SurfaceInfo
}

func populationSurfaces(scan ScanResult, packages map[string]PackageInfo) (map[string]populationSurface, error) {
	result := make(map[string]populationSurface, len(scan.Types.Surfaces))
	for _, surface := range scan.Types.Surfaces {
		if surface.FactID == "" || surface.FactID != surfaceInfoFactID(surface) {
			return nil, populationError("noncanonical scanned surface %+v", surface)
		}
		if surface.PackagePath == "" || surface.RootType == "" || surface.Surface == "" || surface.Kind == "" || surface.Type == "" || surface.OriginDeclID == "" {
			return nil, populationError("incomplete scanned surface %+v", surface)
		}
		if _, exists := packages[surface.PackagePath]; !exists {
			return nil, populationError("surface %q has unknown package %q", surface.Surface, surface.PackagePath)
		}
		if _, exists := result[surface.Surface]; exists {
			return nil, populationError("duplicate scanned surface %q", surface.Surface)
		}
		result[surface.Surface] = populationSurface{info: surface}
	}
	return result, nil
}

func populationSurfaceFor(surface string, packagePath string, surfaces map[string]populationSurface) (SurfaceInfo, error) {
	if surface == "" {
		return SurfaceInfo{}, populationError("surface is empty")
	}
	if found, exists := surfaces[surface]; exists {
		if packagePath != "" && found.info.PackagePath != packagePath {
			return SurfaceInfo{}, populationError("surface %q belongs to %q, want %q", surface, found.info.PackagePath, packagePath)
		}
		return found.info, nil
	}
	// Package-level functions and owners have no TypeShape surface.  Their
	// only legal surface spelling is the explicit package plane.
	if strings.HasPrefix(surface, "package:") && strings.TrimPrefix(surface, "package:") == packagePath {
		return SurfaceInfo{PackagePath: packagePath, Surface: surface}, nil
	}
	return SurfaceInfo{}, populationError("unknown surface %q", surface)
}

func populationSurfaceOrigins(scanned []SurfaceInfo, declarations map[string]DeclarationInfo, surfaces map[string]populationSurface) error {
	for _, surface := range scanned {
		origin, exists := declarations[surface.OriginDeclID]
		if !exists {
			return populationError("surface %q references unknown origin declaration %q", surface.Surface, surface.OriginDeclID)
		}
		if origin.PackagePath != surface.PackagePath {
			return populationError("surface %q origin package mismatch", surface.Surface)
		}
		if surface.ParentSurface != "" {
			parent, exists := surfaces[surface.ParentSurface]
			if !exists || parent.info.PackagePath != surface.PackagePath {
				return populationError("surface %q references unknown/foreign parent surface %q", surface.Surface, surface.ParentSurface)
			}
		}
	}
	return nil
}

func populationDeclarations(scan ScanResult, manifests ManifestSet, packages map[string]PackageInfo, surfaces map[string]populationSurface) (map[string]DeclarationInfo, map[string]populationFact, error) {
	result := make(map[string]DeclarationInfo, len(scan.Types.Declarations))
	facts := make(map[string]populationFact, len(scan.Types.Declarations)+len(scan.Uses))
	for _, declaration := range scan.Types.Declarations {
		if declaration.FactID == "" || declaration.FactID != declarationFactID(declaration) {
			return nil, nil, populationError("noncanonical scanned declaration %+v", declaration)
		}
		if declaration.PackagePath == "" || declaration.Kind == "" || declaration.Name == "" || declaration.Type == "" || declaration.SourceFile == "" || declaration.Line <= 0 || declaration.Column <= 0 {
			return nil, nil, populationError("incomplete scanned declaration %+v", declaration)
		}
		if _, exists := packages[declaration.PackagePath]; !exists {
			return nil, nil, populationError("declaration %q has unknown package %q", declaration.FactID, declaration.PackagePath)
		}
		if _, exists := result[declaration.FactID]; exists {
			return nil, nil, populationError("duplicate scanned declaration %q", declaration.FactID)
		}
		result[declaration.FactID] = declaration
		if err := addPopulationFact(facts, declaration.FactID, "declaration"); err != nil {
			return nil, nil, err
		}
	}
	seenRows := make(map[string]struct{}, len(manifests.Catalog.Declarations))
	for _, row := range manifests.Catalog.Declarations {
		if row.FactID == "" {
			return nil, nil, populationError("catalog declaration has empty fact ID")
		}
		if _, duplicate := seenRows[row.FactID]; duplicate {
			return nil, nil, populationError("catalog declaration fact %q appears more than once", row.FactID)
		}
		seenRows[row.FactID] = struct{}{}
		declaration, exists := result[row.FactID]
		if !exists {
			return nil, nil, populationError("catalog declaration fact %q is not scanned", row.FactID)
		}
		if row.PackagePath != declaration.PackagePath || row.Kind != declaration.Kind || row.Name != declaration.Name || row.Type != declaration.Type || row.Signature != declaration.Signature {
			return nil, nil, populationError("catalog declaration %q does not exactly join scanned fields", row.FactID)
		}
		owner, ok := populationOwner(manifests.Catalog.Owners, row.Owner)
		if !ok {
			return nil, nil, populationError("catalog declaration %q references unknown owner %q", row.FactID, row.Owner)
		}
		if owner.PackagePath != row.PackagePath || row.Surface != owner.Surface {
			return nil, nil, populationError("catalog declaration %q owner/surface mismatch", row.FactID)
		}
		if declaration.Surface != "" && declaration.Surface != row.Surface {
			return nil, nil, populationError("catalog declaration %q has mismatched structural surface", row.FactID)
		}
		if _, err := populationSurfaceFor(row.Surface, row.PackagePath, surfaces); err != nil {
			return nil, nil, fmt.Errorf("%w: declaration %q: %v", ErrManifestPopulation, row.FactID, err)
		}
	}
	if len(seenRows) != len(result) {
		return nil, nil, populationError("catalog declarations are incomplete: got=%d want=%d", len(seenRows), len(result))
	}
	return result, facts, nil
}

func populationOwner(owners []OwnerRow, id string) (OwnerRow, bool) {
	for _, owner := range owners {
		if owner.ID == id {
			return owner, true
		}
	}
	return OwnerRow{}, false
}

func populationUses(scan ScanResult, manifests ManifestSet, declarations map[string]DeclarationInfo, facts map[string]populationFact) error {
	productionFiles := make(map[string]struct{}, len(scan.Sources.ProductionSources))
	for _, source := range scan.Sources.ProductionSources {
		if source.Path == "" || source.PackagePath == "" {
			return populationError("incomplete production source %+v", source)
		}
		if _, exists := productionFiles[source.Path]; exists {
			return populationError("duplicate production source %q", source.Path)
		}
		productionFiles[source.Path] = struct{}{}
	}
	seen := make(map[string]struct{}, len(scan.Uses))
	for _, use := range scan.Uses {
		if use.FactID == "" || use.FactID != useSiteFactID(use) {
			return populationError("noncanonical scanned use %+v", use)
		}
		if use.PackagePath == "" || use.SourceFile == "" || use.Line <= 0 || use.Column <= 0 || use.Symbol == "" || use.Evidence == "" || use.Type == "" || use.TargetDeclID == "" {
			return populationError("incomplete scanned use %+v", use)
		}
		if _, exists := productionFiles[use.SourceFile]; !exists {
			return populationError("use %q source file %q is outside ProductionSources", use.FactID, use.SourceFile)
		}
		if !use.Role.valid() {
			return populationError("scanned use %q has unknown role %q", use.FactID, use.Role)
		}
		if _, duplicate := seen[use.FactID]; duplicate {
			return populationError("duplicate scanned use %q", use.FactID)
		}
		seen[use.FactID] = struct{}{}
		if _, exists := declarations[use.TargetDeclID]; !exists {
			return populationError("use %q targets unknown declaration %q", use.FactID, use.TargetDeclID)
		}
		if err := addPopulationFact(facts, use.FactID, "use"); err != nil {
			return err
		}
	}
	if len(manifests.Catalog.Uses) != len(scan.Uses) {
		return populationError("catalog uses are incomplete: got=%d want=%d", len(manifests.Catalog.Uses), len(scan.Uses))
	}
	manifestFacts := make(map[string]struct{}, len(manifests.Catalog.Uses))
	for _, row := range manifests.Catalog.Uses {
		if row.FactID == "" {
			return populationError("catalog use has empty fact ID")
		}
		if _, duplicate := manifestFacts[row.FactID]; duplicate {
			return populationError("catalog use fact %q appears more than once", row.FactID)
		}
		manifestFacts[row.FactID] = struct{}{}
		use, exists := findUse(scan.Uses, row.FactID)
		if !exists {
			return populationError("catalog use fact %q is not scanned", row.FactID)
		}
		if row.PackagePath != use.PackagePath || row.SourceFile != use.SourceFile || row.Line != use.Line || row.Column != use.Column || row.Symbol != use.Symbol || row.Evidence != use.Evidence || row.Type != use.Type || row.TargetDeclID != use.TargetDeclID || row.Role != use.Role {
			return populationError("catalog use %q does not exactly join scanned fields", row.FactID)
		}
		if _, exists := declarations[row.TargetDeclID]; !exists {
			return populationError("catalog use %q targets unknown declaration %q", row.FactID, row.TargetDeclID)
		}
	}
	if len(manifestFacts) != len(seen) {
		return populationError("catalog uses are not an exact scanner permutation: got=%d want=%d", len(manifestFacts), len(seen))
	}
	for id := range seen {
		if _, exists := manifestFacts[id]; !exists {
			return populationError("scanned use %q is missing from catalog", id)
		}
	}
	return nil
}

func findUse(uses []UseSite, id string) (UseSite, bool) {
	for _, use := range uses {
		if use.FactID == id {
			return use, true
		}
	}
	return UseSite{}, false
}

func populationImports(scan ScanResult, manifests ManifestSet, packages map[string]PackageInfo, surfaces map[string]populationSurface) error {
	// Sources.Packages is the authored family owner universe.  Inbound
	// ProductionSources, typed uses, and scanner import edges provide source
	// coordinates/evidence only; they do not create connector owners or infer a
	// unique owner for a package.  The owner endpoint selected for an internal
	// edge is an explicit catalog decision, checked against the exact scanner
	// edge and the owner DAG below.
	owners := make(map[string]OwnerRow, len(manifests.Catalog.Owners))
	ownersByPackage := make(map[string][]OwnerRow)
	for _, owner := range manifests.Catalog.Owners {
		if !validPopulationAtom(owner.ID) || !validPopulationAtom(owner.PackagePath) || !validPopulationAtom(owner.Surface) || !validPopulationAtom(owner.Kind) {
			return populationError("incomplete owner row %+v", owner)
		}
		if _, duplicate := owners[owner.ID]; duplicate {
			return populationError("duplicate owner ID %q", owner.ID)
		}
		if _, exists := packages[owner.PackagePath]; !exists {
			return populationError("owner %q has unknown package %q", owner.ID, owner.PackagePath)
		}
		if owner.Kind != "root" && owner.Kind != "component" && owner.Kind != "domain" && owner.Kind != "artifact" && owner.Kind != "cold" {
			return populationError("owner %q has unknown kind %q", owner.ID, owner.Kind)
		}
		if _, err := populationSurfaceFor(owner.Surface, owner.PackagePath, surfaces); err != nil {
			return fmt.Errorf("%w: owner %q: %v", ErrManifestPopulation, owner.ID, err)
		}
		owners[owner.ID] = owner
		ownersByPackage[owner.PackagePath] = append(ownersByPackage[owner.PackagePath], owner)
	}
	if len(owners) == 0 {
		return populationError("catalog has no owners")
	}
	edges := make(map[string]struct{}, len(manifests.Catalog.ImportEdges))
	adj := make(map[string][]string, len(owners))
	for _, row := range manifests.Catalog.ImportEdges {
		from, fromOK := owners[row.FromOwner]
		to, toOK := owners[row.ToOwner]
		if !fromOK || !toOK {
			return populationError("ownership import edge references unknown owner: %+v", row)
		}
		if row.SourceFile == "" || row.Line <= 0 || row.Column <= 0 || row.FromOwner == row.ToOwner {
			return populationError("malformed/self ownership import edge %+v", row)
		}
		key := populationImportOwnerKey(row)
		if _, duplicate := edges[key]; duplicate {
			return populationError("duplicate ownership import edge %q", key)
		}
		edges[key] = struct{}{}
		adj[from.ID] = append(adj[from.ID], to.ID)
		// The source package identity is checked against the scanner edge
		// below.  Keeping this check here prevents a future edge join from
		// silently treating owner IDs as package paths.
		if from.PackagePath == "" || to.PackagePath == "" {
			return populationError("ownership edge has empty package identity %+v", row)
		}
	}
	if err := populationOwnerDAG(owners, adj); err != nil {
		return err
	}
	scannerEdges := make(map[string]struct{}, len(scan.Dependencies.ImportEdges))
	for _, edge := range scan.Dependencies.ImportEdges {
		if edge.From == "" || edge.To == "" || edge.SourceFile == "" || edge.Line <= 0 || edge.Column <= 0 {
			return populationError("malformed scanned import edge %+v", edge)
		}
		key := populationImportScannerKey(edge)
		if _, duplicate := scannerEdges[key]; duplicate {
			return populationError("duplicate scanned import edge %q", key)
		}
		scannerEdges[key] = struct{}{}
		froms, fromOK := ownersByPackage[edge.From]
		tos, toOK := ownersByPackage[edge.To]
		if !fromOK || !toOK {
			// External connectors have no authored owner.  They are scanner
			// evidence but not ownership-DAG edges, and are intentionally not
			// attributed by this validator.
			continue
		}
		// Package identity is scanner evidence; it does not imply a unique
		// owner.  When a package has multiple authored owners, the catalog's
		// explicit owner-edge row is the authority.  Require at least one
		// explicit edge for this internal package edge, but never select a
		// first owner or demand a Cartesian product of owners.
		matched := false
		for _, from := range froms {
			for _, to := range tos {
				want := populationImportOwnerKey(OwnershipImportEdgeRow{FromOwner: from.ID, ToOwner: to.ID, SourceFile: edge.SourceFile, Line: edge.Line, Column: edge.Column})
				if _, exists := edges[want]; exists {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return populationError("scanner import edge %q is missing from catalog", key)
		}
	}
	for _, row := range manifests.Catalog.ImportEdges {
		from := owners[row.FromOwner]
		to := owners[row.ToOwner]
		key := populationImportScannerKey(ImportEdge{From: from.PackagePath, To: to.PackagePath, SourceFile: row.SourceFile, Line: row.Line, Column: row.Column})
		if _, exists := scannerEdges[key]; !exists {
			return populationError("catalog ownership import edge is not scanned: %q", key)
		}
	}
	return nil
}

func populationImportOwnerKey(row OwnershipImportEdgeRow) string {
	return strings.Join([]string{row.FromOwner, row.ToOwner, row.SourceFile, strconv.Itoa(row.Line), strconv.Itoa(row.Column)}, "\x00")
}

func populationImportScannerKey(edge ImportEdge) string {
	return strings.Join([]string{edge.From, edge.To, edge.SourceFile, strconv.Itoa(edge.Line), strconv.Itoa(edge.Column)}, "\x00")
}

func populationOwnerDAG(owners map[string]OwnerRow, adjacency map[string][]string) error {
	for id := range adjacency {
		sort.Strings(adjacency[id])
	}
	color := make(map[string]uint8, len(owners))
	var visit func(string) error
	visit = func(id string) error {
		switch color[id] {
		case 1:
			return populationError("ownership owner DAG contains a cycle at %q", id)
		case 2:
			return nil
		}
		color[id] = 1
		for _, next := range adjacency[id] {
			if _, exists := owners[next]; !exists {
				return populationError("ownership DAG references unknown owner %q", next)
			}
			if err := visit(next); err != nil {
				return err
			}
		}
		color[id] = 2
		return nil
	}
	ids := make([]string, 0, len(owners))
	for id := range owners {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func populationStructure(scan ScanResult, declarations map[string]DeclarationInfo, surfaces map[string]populationSurface, packages map[string]PackageInfo, facts map[string]populationFact) error {
	checkRow := func(kind, id, declarationID, surfaceID string, allowEmptySurface bool) (DeclarationInfo, error) {
		if id == "" {
			return DeclarationInfo{}, populationError("%s row has empty fact ID", kind)
		}
		declaration, exists := declarations[declarationID]
		if !exists {
			return DeclarationInfo{}, populationError("%s %q references unknown declaration %q", kind, id, declarationID)
		}
		if surfaceID == "" {
			if !allowEmptySurface || (declaration.Kind != "func" && declaration.Kind != "method" && declaration.Kind != "var" && declaration.Kind != "const") {
				return DeclarationInfo{}, populationError("%s %q has missing surface", kind, id)
			}
		} else {
			surface, exists := findSurfaceByID(surfaces, surfaceID)
			if !exists {
				return DeclarationInfo{}, populationError("%s %q references unknown surface ID %q", kind, id, surfaceID)
			}
			if surface.PackagePath != declaration.PackagePath {
				return DeclarationInfo{}, populationError("%s %q surface belongs to %q, want %q", kind, id, surface.PackagePath, declaration.PackagePath)
			}
			if declaration.Surface != "" && declaration.Surface != surface.Surface {
				return DeclarationInfo{}, populationError("%s %q surface does not match declaration", kind, id)
			}
		}
		return declaration, nil
	}
	for _, row := range scan.Types.Structure.Fields {
		if row.FactID != structureFieldFactID(row) {
			return populationError("noncanonical structure field %+v", row)
		}
		declaration, err := checkRow("structure field", row.FactID, row.DeclarationID, row.SurfaceID, false)
		if err != nil {
			return err
		}
		if declaration.Kind != "field" {
			return populationError("structure field %q targets non-field declaration", row.FactID)
		}
		if err := addPopulationFact(facts, row.FactID, "structure-field"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Arrays {
		if row.FactID != structureArrayFactID(row) || row.Element == "" || row.Length < 0 {
			return populationError("malformed structure array %+v", row)
		}
		if _, err := checkRow("structure array", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure array", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-array"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Slices {
		if row.FactID != structureSliceFactID(row) || row.Element == "" {
			return populationError("malformed structure slice %+v", row)
		}
		if _, err := checkRow("structure slice", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure slice", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-slice"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Maps {
		if row.FactID != structureMapFactID(row) || row.Key == "" || row.Value == "" {
			return populationError("malformed structure map %+v", row)
		}
		if _, err := checkRow("structure map", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure map", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-map"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Channels {
		if row.FactID != structureChannelFactID(row) || row.Element == "" || row.Direction == "" {
			return populationError("malformed structure channel %+v", row)
		}
		if _, err := checkRow("structure channel", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure channel", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-channel"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.NamedReferences {
		if row.FactID != structureNamedReferenceFactID(row) || row.TargetPackagePath == "" || row.TargetName == "" {
			return populationError("malformed structure named reference %+v", row)
		}
		declaration, err := checkRow("structure named reference", row.FactID, row.DeclarationID, row.SurfaceID, true)
		if err != nil {
			return err
		}
		if err := populationStructurePath("structure named reference", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if row.TargetDeclID != "" {
			target, exists := declarations[row.TargetDeclID]
			if !exists || target.PackagePath != row.TargetPackagePath || target.Name != row.TargetName || (target.Kind != "type" && target.Kind != "alias") || target.OwnerType != "" || target.SyntheticPath != "" {
				return populationError("structure named reference %q target declaration mismatch", row.FactID)
			}
		} else if _, internal := packages[row.TargetPackagePath]; internal {
			return populationError("internal structure named reference %q has no target declaration", row.FactID)
		}
		_ = declaration
		if err := addPopulationFact(facts, row.FactID, "structure-named-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.MethodReferences {
		if row.FactID != structureMethodReferenceFactID(row) || row.TargetPackagePath == "" || row.TargetName == "" || row.MethodKey == "" || row.Type == "" || row.Receiver == "" {
			return populationError("malformed structure method reference %+v", row)
		}
		if _, err := checkRow("structure method reference", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure method reference", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if row.TargetDeclID != "" {
			target, exists := declarations[row.TargetDeclID]
			if !exists || target.PackagePath != row.TargetPackagePath || target.Name != row.TargetName || target.Kind != "interface-method" || target.Type != row.Type || declarationMethodReceiver(target) != row.Receiver || populationMethodObjectKey(target) != row.MethodKey {
				return populationError("structure method reference %q target declaration mismatch", row.FactID)
			}
		} else if _, internal := packages[row.TargetPackagePath]; internal {
			return populationError("internal structure method reference %q has no target declaration", row.FactID)
		}
		if err := addPopulationFact(facts, row.FactID, "structure-method-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.OtherReferences {
		if row.FactID != structureOtherReferenceFactID(row) || row.Type == "" || !populationOtherDispositionValid(row.Disposition) {
			return populationError("malformed structure other reference %+v", row)
		}
		if _, err := checkRow("structure other reference", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure other reference", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-other-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Cycles {
		if row.FactID != structureCycleFactID(row) || row.Type == "" {
			return populationError("malformed structure cycle %+v", row)
		}
		if _, err := checkRow("structure cycle", row.FactID, row.DeclarationID, row.SurfaceID, true); err != nil {
			return err
		}
		if err := populationStructurePath("structure cycle", row.Path, row.SurfaceID, surfaces); err != nil {
			return err
		}
		if err := addPopulationFact(facts, row.FactID, "structure-cycle"); err != nil {
			return err
		}
	}
	return nil
}

// populationStructurePath follows the typed scanner contract instead of
// imposing a blanket nonempty-path rule.  Declaration-signature projections
// have no structural surface and may have an empty path.  A surfaced empty
// path is also legitimate for the exact named/alias root SurfaceInfo; nested
// anonymous/container roots carry their concrete path and must retain it.
func populationStructurePath(kind, path, surfaceID string, surfaces map[string]populationSurface) error {
	if path != "" || surfaceID == "" {
		return nil
	}
	surface, exists := findSurfaceByID(surfaces, surfaceID)
	if !exists {
		return populationError("%s references unknown surfaced empty-path ID %q", kind, surfaceID)
	}
	if (surface.Kind == "named-root" || surface.Kind == "alias-root") && surface.ParentSurface == "" && surface.Path == "" {
		return nil
	}
	return populationError("%s has an empty path on non-root surfaced row %q (kind=%q)", kind, surfaceID, surface.Kind)
}

// populationMethodObjectKey is the manifest-side recomputation of the stable
// methodObjectKey used by structure projection for internal interface methods.
// DeclarationInfo carries exactly the fields needed by that identity; source
// is the empty component for named interface declarations.
func populationMethodObjectKey(declaration DeclarationInfo) string {
	receiver := declarationMethodReceiver(declaration)
	if declaration.PackagePath == "" || declaration.Name == "" || declaration.Type == "" || receiver == "" {
		return ""
	}
	return strings.Join([]string{declaration.PackagePath, declaration.Name, declaration.Type, receiver, ""}, "\x00")
}

func populationOtherDispositionValid(disposition OtherReferenceDisposition) bool {
	switch disposition {
	case OtherPointer, OtherAnonymousStruct, OtherMember, OtherSignature, OtherTuple,
		OtherTypeParameter, OtherUnionTerm, OtherUnionTildeTerm, OtherInterfaceMethod:
		return true
	default:
		return false
	}
}

func populationExposures(scan ScanResult, declarations map[string]DeclarationInfo, surfaces map[string]populationSurface, facts map[string]populationFact) error {
	seen := make(map[string]struct{}, len(scan.Types.Exposures))
	for _, exposure := range scan.Types.Exposures {
		if exposure.FactID == "" || exposure.FactID != methodExposureFactID(exposure) || exposure.PackagePath == "" || exposure.RootType == "" || exposure.Surface == "" || exposure.Set == "" || exposure.Name == "" || exposure.Signature == "" || exposure.TargetDeclID == "" || exposure.Disposition == "" {
			return populationError("malformed method exposure %+v", exposure)
		}
		if _, duplicate := seen[exposure.FactID]; duplicate {
			return populationError("duplicate method exposure %q", exposure.FactID)
		}
		seen[exposure.FactID] = struct{}{}
		surface, err := populationSurfaceFor(exposure.Surface, exposure.PackagePath, surfaces)
		if err != nil {
			return err
		}
		if exposure.Surface != typeSurfaceID(exposure.PackagePath, exposure.RootType) ||
			surface.PackagePath != exposure.PackagePath || surface.RootType != exposure.RootType ||
			(surface.Kind != "named-root" && surface.Kind != "alias-root") ||
			surface.ParentSurface != "" || surface.Path != "" {
			return populationError("method exposure %q does not target the exact named/alias root surface", exposure.FactID)
		}
		if exposure.Set != "value" && exposure.Set != "pointer" {
			return populationError("method exposure %q has unknown method-set %q", exposure.FactID, exposure.Set)
		}
		if exposure.Disposition != "declared" && exposure.Disposition != "promoted" && exposure.Disposition != "embedded" && exposure.Disposition != "aliased" {
			return populationError("method exposure %q has unknown disposition %q", exposure.FactID, exposure.Disposition)
		}
		declaration, exists := declarations[exposure.TargetDeclID]
		if !exists || declaration.PackagePath != exposure.PackagePath || (declaration.Kind != "method" && declaration.Kind != "interface-method") || declaration.OwnerType == "" || declaration.Name != exposure.Name || declaration.Signature != exposure.Signature {
			return populationError("method exposure %q target declaration mismatch", exposure.FactID)
		}
		switch exposure.Disposition {
		case "declared":
			if declaration.OwnerType != exposure.RootType {
				return populationError("method exposure %q declared target receiver mismatch", exposure.FactID)
			}
		case "promoted", "embedded":
			if declaration.OwnerType == exposure.RootType {
				return populationError("method exposure %q %s target must have a promoted receiver", exposure.FactID, exposure.Disposition)
			}
		case "aliased":
			if surface.Kind != "alias-root" {
				return populationError("method exposure %q aliased target is not an alias root", exposure.FactID)
			}
		}
		if err := addPopulationFact(facts, exposure.FactID, "method-exposure"); err != nil {
			return err
		}
	}
	return nil
}

func populationIndexes(scan ScanResult, manifests ManifestSet, declarations map[string]DeclarationInfo, surfaces map[string]populationSurface, facts map[string]populationFact) error {
	owners := make(map[string]OwnerRow, len(manifests.Catalog.Owners))
	for _, owner := range manifests.Catalog.Owners {
		if !validPopulationAtom(owner.ID) || !validPopulationAtom(owner.PackagePath) || !validPopulationAtom(owner.Surface) {
			return populationError("incomplete owner row %+v", owner)
		}
		if _, exists := owners[owner.ID]; exists {
			return populationError("duplicate owner ID %q", owner.ID)
		}
		owners[owner.ID] = owner
	}
	catalogDeclarations := make(map[string]DeclarationRow, len(manifests.Catalog.Declarations))
	for _, row := range manifests.Catalog.Declarations {
		catalogDeclarations[row.FactID] = row
	}
	uses := make(map[string]UseSite, len(scan.Uses))
	productionFiles := make(map[string]struct{}, len(scan.Sources.ProductionSources))
	for _, source := range scan.Sources.ProductionSources {
		productionFiles[source.Path] = struct{}{}
	}
	for _, use := range scan.Uses {
		if !use.Role.valid() {
			return populationError("use %q has unknown closed role %q", use.FactID, use.Role)
		}
		if _, exists := uses[use.FactID]; exists {
			return populationError("duplicate scanned use %q", use.FactID)
		}
		if _, exists := productionFiles[use.SourceFile]; !exists {
			return populationError("use %q source file %q is outside ProductionSources", use.FactID, use.SourceFile)
		}
		uses[use.FactID] = use
	}

	// Every structural FactID has one scanner-owned surface/package identity.
	// This is the source-side join used by both final and inventory-only rows.
	structuralOwners := make(map[string]string)
	addStructuralOwner := func(factID, declarationID, surfaceID, kind string) error {
		if factID == "" {
			return populationError("%s structural source has empty FactID", kind)
		}
		declaration, exists := declarations[declarationID]
		if !exists {
			return populationError("%s structural source %q references unknown declaration %q", kind, factID, declarationID)
		}
		ownerSurface := "package:" + declaration.PackagePath
		if surfaceID != "" {
			surface, found := findSurfaceByID(surfaces, surfaceID)
			if !found {
				return populationError("%s structural source %q references unknown surface %q", kind, factID, surfaceID)
			}
			ownerSurface = surface.Surface
		}
		if previous, duplicate := structuralOwners[factID]; duplicate {
			return populationError("structural source %q has multiple owner surfaces %q and %q", factID, previous, ownerSurface)
		}
		structuralOwners[factID] = ownerSurface
		return nil
	}
	for _, row := range scan.Types.Structure.Fields {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "field"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Arrays {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "array"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Slices {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "slice"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Maps {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "map"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Channels {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "channel"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.NamedReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "named-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.MethodReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "method-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.OtherReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "other-reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Cycles {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, row.SurfaceID, "cycle"); err != nil {
			return err
		}
	}

	claim := make(map[string]string)
	claimID := func(id, kind string) error {
		if !validPopulationAtom(id) {
			return populationError("%s has invalid ID %q", kind, id)
		}
		if previous, exists := claim[id]; exists {
			return populationError("FactID %q is classified as both %s and %s", id, previous, kind)
		}
		claim[id] = kind
		return nil
	}
	checkFactList := func(values []string, kind string) error {
		if !factIDsCanonical(values) {
			return populationError("%s FactID list is not canonical", kind)
		}
		for _, fact := range values {
			if _, exists := facts[fact]; !exists {
				return populationError("%s references unknown scanner FactID %q", kind, fact)
			}
		}
		return nil
	}
	checkRelationFacts := func(values []string, expectedSurface, kind string) error {
		if len(values) == 0 {
			return populationError("%s has no source relation facts", kind)
		}
		if err := checkFactList(values, kind); err != nil {
			return err
		}
		for _, fact := range values {
			factKind := facts[fact].kind
			if !strings.HasPrefix(factKind, "structure-") {
				return populationError("%s FactID %q is not a structural relation fact", kind, fact)
			}
			if actual, exists := structuralOwners[fact]; !exists || actual != expectedSurface {
				return populationError("%s FactID %q belongs to %q, want %q", kind, fact, actual, expectedSurface)
			}
		}
		return nil
	}
	callersFor := func(query, consumerPackage string) []string {
		result := make([]string, 0)
		for _, use := range scan.Uses {
			if use.Role == CallCallee && use.TargetDeclID == query && (consumerPackage == "" || use.PackagePath == consumerPackage) {
				result = append(result, use.FactID)
			}
		}
		sort.Strings(result)
		return result
	}
	checkCallerUseFacts := func(query string, values []string, consumerPackage, kind string) error {
		if err := checkFactList(values, kind); err != nil {
			return err
		}
		for _, fact := range values {
			use, exists := uses[fact]
			if !exists || use.Role != CallCallee || use.TargetDeclID != query {
				return populationError("%s caller %q is not the exact CallCallee use for query %q", kind, fact, query)
			}
			if consumerPackage != "" && use.PackagePath != consumerPackage {
				return populationError("%s caller %q package %q does not match consumer package %q", kind, fact, use.PackagePath, consumerPackage)
			}
			if _, exists := productionFiles[use.SourceFile]; !exists {
				return populationError("%s caller %q source %q is outside ProductionSources", kind, fact, use.SourceFile)
			}
		}
		expected := callersFor(query, consumerPackage)
		if len(values) != len(expected) {
			return populationError("%s callers are incomplete: got=%d want=%d", kind, len(values), len(expected))
		}
		for index := range values {
			if values[index] != expected[index] {
				return populationError("%s callers are not the exact sorted CallCallee set", kind)
			}
		}
		return nil
	}
	callable := func(declaration DeclarationInfo) bool {
		return declaration.Kind == "func" || declaration.Kind == "method" || declaration.Kind == "interface-method"
	}
	checkQuery := func(query, ownerID, kind string) (OwnerRow, error) {
		declaration, exists := declarations[query]
		if !exists || !callable(declaration) {
			return OwnerRow{}, populationError("%s query %q is not a callable declaration FactID", kind, query)
		}
		catalog, exists := catalogDeclarations[query]
		if !exists || catalog.Owner != ownerID {
			return OwnerRow{}, populationError("%s query %q catalog owner %q does not match %q", kind, query, catalog.Owner, ownerID)
		}
		owner, exists := owners[ownerID]
		if !exists {
			return OwnerRow{}, populationError("%s references unknown owner %q", kind, ownerID)
		}
		return owner, nil
	}
	checkBenchmark := func(value, kind string) error {
		if !canonicalBenchmarkReceiptDigest(value) {
			return populationError("%s has invalid empirical benchmark receipt digest", kind)
		}
		return nil
	}
	checkPattern := func(value, kind string) error {
		if !validPopulationAtom(value) {
			return populationError("%s has invalid pattern ID", kind)
		}
		return nil
	}
	checkIndex := func(id, ownerID, query, pattern, benchmark string, sourceFacts, callerFacts []string, kind string) error {
		if err := claimID(id, kind); err != nil {
			return err
		}
		owner, err := checkQuery(query, ownerID, kind+" "+id)
		if err != nil {
			return err
		}
		if err := checkPattern(pattern, kind+" "+id); err != nil {
			return err
		}
		if err := checkBenchmark(benchmark, kind+" "+id); err != nil {
			return err
		}
		if err := checkRelationFacts(sourceFacts, owner.Surface, kind+" "+id+" sources"); err != nil {
			return err
		}
		if err := checkCallerUseFacts(query, callerFacts, "", kind+" "+id+" callers"); err != nil {
			return err
		}
		return populationError("%s %q has no same-scan witness", kind, id)
	}
	for _, row := range manifests.Indexes.Indexes {
		if err := checkIndex(row.ID, row.Owner, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest, row.SourceFactIDs, row.CallerUseFactIDs, "index"); err != nil {
			return err
		}
	}
	checkReference := func(id, kind, issuer, consumer, query, pattern, benchmark string, sourceFacts, callerFacts []string) error {
		if err := claimID(id, kind); err != nil {
			return err
		}
		issuerOwner, err := checkQuery(query, issuer, kind+" "+id)
		if err != nil {
			return err
		}
		consumerOwner, exists := owners[consumer]
		if !exists {
			return populationError("%s %q references unknown consumer", kind, id)
		}
		if issuer == consumer {
			return populationError("%s %q has identical issuer and consumer", kind, id)
		}
		if err := checkPattern(pattern, kind+" "+id); err != nil {
			return err
		}
		if err := checkBenchmark(benchmark, kind+" "+id); err != nil {
			return err
		}
		if err := checkRelationFacts(sourceFacts, issuerOwner.Surface, kind+" "+id+" sources"); err != nil {
			return err
		}
		if err := checkCallerUseFacts(query, callerFacts, consumerOwner.PackagePath, kind+" "+id+" callers"); err != nil {
			return err
		}
		_ = issuerOwner
		return populationError("%s %q has no same-scan witness", kind, id)
	}
	for _, row := range manifests.Indexes.HotReferences {
		if err := checkReference(row.ID, "hot-ref", row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest, row.SourceFactIDs, row.CallerUseFactIDs); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ColdReferences {
		if err := checkReference(row.ID, "cold-ref", row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest, row.SourceFactIDs, row.CallerUseFactIDs); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ContextualReferences {
		if err := checkReference(row.ID, "contextual-ref", row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest, row.SourceFactIDs, row.CallerUseFactIDs); err != nil {
			return err
		}
	}

	identityByID := make(map[string]IdentityRow, len(manifests.Indexes.Identities))
	for _, row := range manifests.Indexes.Identities {
		if _, duplicate := identityByID[row.ID]; duplicate {
			return populationError("identity %q is classified more than once", row.ID)
		}
		identityByID[row.ID] = row
	}
	ownerAdjacency := make(map[string][]string)
	for _, edge := range manifests.Catalog.ImportEdges {
		ownerAdjacency[edge.FromOwner] = append(ownerAdjacency[edge.FromOwner], edge.ToOwner)
	}
	ownerReachable := func(from, target string) bool {
		if from == target {
			return true
		}
		seen := map[string]bool{from: true}
		queue := []string{from}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, next := range ownerAdjacency[current] {
				if next == target {
					return true
				}
				if !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
		return false
	}
	firstIdentityID := ""
	for _, row := range manifests.Indexes.Identities {
		if firstIdentityID == "" {
			firstIdentityID = row.ID
		}
		if err := claimID(row.ID, "identity"); err != nil {
			return err
		}
		owner, exists := owners[row.Owner]
		if !exists {
			return populationError("identity %q references unknown owner", row.ID)
		}
		declaration, exists := declarations[row.DeclarationFactID]
		if !exists || !callable(declaration) {
			return populationError("identity %q declaration is not callable", row.ID)
		}
		catalog, exists := catalogDeclarations[row.DeclarationFactID]
		if !exists || catalog.Owner != row.Owner {
			return populationError("identity %q declaration catalog owner does not match owner", row.ID)
		}
		if !row.RelationKind.valid() || !validPopulationAtom(row.PatternID) {
			return populationError("identity %q has incomplete typed relation", row.ID)
		}
		if row.RelationKind == IdentityRelationDirect && len(row.ParentIdentityIDs) != 0 {
			return populationError("identity %q direct relation has parents", row.ID)
		}
		if row.RelationKind == IdentityRelationComposite && len(row.ParentIdentityIDs) == 0 {
			return populationError("identity %q composite relation has no parents", row.ID)
		}
		if err := checkRelationFacts(row.DirectFactIDs, owner.Surface, "identity "+row.ID+" direct facts"); err != nil {
			return err
		}
		if !factIDsCanonical(row.ParentIdentityIDs) {
			return populationError("identity %q parent IDs are not canonical", row.ID)
		}
		for _, parentID := range row.ParentIdentityIDs {
			parent, exists := identityByID[parentID]
			if !exists {
				return populationError("identity %q references unknown parent %q", row.ID, parentID)
			}
			if parent.Owner != row.Owner {
				if !ownerReachable(row.Owner, parent.Owner) {
					return populationError("identity %q parent owner %q is not reachable from owner %q in owner DAG", row.ID, parent.Owner, row.Owner)
				}
			}
		}
		computed, err := identityDigest(row, manifests.Indexes.Identities)
		if err != nil {
			return populationError("identity %q digest graph: %v", row.ID, err)
		}
		if row.ID != computed {
			return populationError("identity %q does not equal computed digest %q", row.ID, computed)
		}
	}
	if firstIdentityID != "" {
		return populationError("identity %q has no same-scan witness", firstIdentityID)
	}
	validateIndexPlan := func(row IndexPlanRow) error {
		if err := claimID(row.ID, "index-plan"); err != nil {
			return err
		}
		owner, err := checkQuery(row.DeclarationFactID, row.Owner, "index-plan "+row.ID)
		if err != nil {
			return err
		}
		if err := checkRelationFacts(row.SourceFactIDs, owner.Surface, "index-plan "+row.ID+" sources"); err != nil {
			return err
		}
		return checkCallerUseFacts(row.DeclarationFactID, row.CallerUseFactIDs, "", "index-plan "+row.ID+" callers")
	}
	for _, row := range manifests.Indexes.IndexPlans {
		if err := validateIndexPlan(row); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ReferencePlans {
		if err := claimID(row.ID, "reference-plan"); err != nil {
			return err
		}
		issuer, err := checkQuery(row.DeclarationFactID, row.Issuer, "reference-plan "+row.ID)
		if err != nil {
			return err
		}
		consumer, exists := owners[row.Consumer]
		if !exists {
			return populationError("reference-plan %q references unknown consumer", row.ID)
		}
		if row.Issuer == row.Consumer {
			return populationError("reference-plan %q has identical issuer and consumer", row.ID)
		}
		if err := checkRelationFacts(row.SourceFactIDs, issuer.Surface, "reference-plan "+row.ID+" sources"); err != nil {
			return err
		}
		if err := checkCallerUseFacts(row.DeclarationFactID, row.CallerUseFactIDs, consumer.PackagePath, "reference-plan "+row.ID+" callers"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.IdentityPlans {
		if err := claimID(row.ID, "identity-plan"); err != nil {
			return err
		}
		owner, exists := owners[row.Owner]
		if !exists {
			return populationError("identity-plan %q references unknown owner", row.ID)
		}
		declaration, exists := declarations[row.DeclarationFactID]
		if !exists || !callable(declaration) {
			return populationError("identity-plan %q declaration is not callable", row.ID)
		}
		catalog, exists := catalogDeclarations[row.DeclarationFactID]
		if !exists || catalog.Owner != row.Owner {
			return populationError("identity-plan %q declaration catalog owner does not match owner", row.ID)
		}
		if !row.RelationKind.valid() {
			return populationError("identity-plan %q has unknown relation kind", row.ID)
		}
		if row.RelationKind == IdentityRelationDirect && len(row.ParentIdentityIDs) != 0 {
			return populationError("identity-plan %q direct relation has parents", row.ID)
		}
		if row.RelationKind == IdentityRelationComposite && len(row.ParentIdentityIDs) == 0 {
			return populationError("identity-plan %q composite relation has no parents", row.ID)
		}
		if err := checkRelationFacts(row.DirectFactIDs, owner.Surface, "identity-plan "+row.ID+" direct facts"); err != nil {
			return err
		}
		if !factIDsCanonical(row.ParentIdentityIDs) {
			return populationError("identity-plan %q parent IDs are not canonical", row.ID)
		}
	}
	return nil
}

func validPopulationAtom(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\t\n\r") {
		return false
	}
	return !forbiddenClassification(value)
}

func populationSurfaceAssignments(scan ScanResult, manifests ManifestSet, declarations map[string]DeclarationInfo, surfaces map[string]populationSurface, facts map[string]populationFact) (map[string]struct{}, error) {
	final := make(map[string]struct{})
	// Catalog declarations and uses are final by definition of catalog.schema.
	// Surface rows then add the second, intentional publication plane for
	// fields, effective methods, and package functions.
	for _, row := range manifests.Catalog.Declarations {
		if _, exists := facts[row.FactID]; !exists {
			return nil, populationError("catalog declaration %q references unknown scanner fact", row.FactID)
		}
		final[row.FactID] = struct{}{}
	}
	for _, row := range manifests.Catalog.Uses {
		if _, exists := facts[row.FactID]; !exists {
			return nil, populationError("catalog use %q references unknown scanner fact", row.FactID)
		}
		final[row.FactID] = struct{}{}
	}
	fieldNames := make(map[string]string, len(scan.Types.Structure.Fields))
	for _, row := range scan.Types.Structure.Fields {
		declaration := declarations[row.DeclarationID]
		fieldNames[row.FactID] = declaration.Name
	}
	exposureNames := make(map[string]string, len(scan.Types.Exposures))
	exposureSurfaces := make(map[string]string, len(scan.Types.Exposures))
	for _, exposure := range scan.Types.Exposures {
		exposureNames[exposure.FactID] = exposure.Name
		exposureSurfaces[exposure.FactID] = exposure.Surface
	}
	assignments := make(map[string]string, len(manifests.Surfaces.Assignments))
	fieldCount := make(map[string]int)
	methodCount := make(map[string]int)
	for _, row := range manifests.Surfaces.Assignments {
		if row.FactID == "" || row.OwnerSurface == "" || row.Name == "" {
			return nil, populationError("incomplete surface assignment %+v", row)
		}
		if previous, exists := assignments[row.FactID]; exists {
			return nil, populationError("surface fact %q is assigned more than once (%s,%s)", row.FactID, previous, row.Kind)
		}
		assignments[row.FactID] = row.Kind
		switch row.Kind {
		case "field":
			want, exists := fieldNames[row.FactID]
			if !exists || row.Name != want {
				return nil, populationError("field surface assignment %q does not join a scanned field", row.FactID)
			}
			field, fieldExists := structureFieldByID(scan.Types.Structure.Fields, row.FactID)
			if !fieldExists || field.SurfaceID == "" {
				return nil, populationError("field surface assignment %q has no scanned surface", row.FactID)
			}
			surface, exists := findSurfaceByID(surfaces, field.SurfaceID)
			if !exists || surface.Surface != row.OwnerSurface {
				return nil, populationError("field surface assignment %q owner surface mismatch", row.FactID)
			}
			fieldCount[row.OwnerSurface]++
		case "effective-method":
			want, exists := exposureNames[row.FactID]
			if !exists || row.Name != want || exposureSurfaces[row.FactID] != row.OwnerSurface {
				return nil, populationError("effective-method surface assignment %q does not join a scanned exposure", row.FactID)
			}
			if _, exists := surfaces[row.OwnerSurface]; !exists {
				return nil, populationError("effective-method assignment %q references unknown surface %q", row.FactID, row.OwnerSurface)
			}
			methodCount[row.OwnerSurface]++
		case "semantic-package-function":
			declaration, exists := declarations[row.FactID]
			if !exists || declaration.Kind != "func" || declaration.OwnerType != "" || declaration.Name != row.Name {
				return nil, populationError("semantic package function assignment %q does not join a scanned function", row.FactID)
			}
			want := "package:" + declaration.PackagePath
			if row.OwnerSurface != want {
				return nil, populationError("semantic package function assignment %q has surface %q, want %q", row.FactID, row.OwnerSurface, want)
			}
			methodCount[row.OwnerSurface]++
		default:
			return nil, populationError("unknown surface assignment kind %q", row.Kind)
		}
		if _, exists := facts[row.FactID]; !exists {
			return nil, populationError("surface assignment %q references unknown scanner fact", row.FactID)
		}
		final[row.FactID] = struct{}{}
	}
	for surface, count := range fieldCount {
		if count > 16 {
			return nil, populationError("surface %q publishes %d fields; maximum is 16", surface, count)
		}
	}
	for surface, count := range methodCount {
		if count > 16 {
			return nil, populationError("surface %q publishes %d effective methods/functions; maximum is 16", surface, count)
		}
	}
	// A surface assignment is a final publication fact, but inventory mode
	// permits an overlarge surface to omit members. Every
	// omitted scanner fact must then be classified by residue; the final
	// partition proof below enforces that exact XOR.
	return final, nil
}

func structureFieldByID(rows []StructureField, id string) (StructureField, bool) {
	for _, row := range rows {
		if row.FactID == id {
			return row, true
		}
	}
	return StructureField{}, false
}

func findSurfaceByID(surfaces map[string]populationSurface, id string) (SurfaceInfo, bool) {
	for _, surface := range surfaces {
		if surface.info.FactID == id {
			return surface.info, true
		}
	}
	return SurfaceInfo{}, false
}

// populationStructuralStorage closes the structural representation plane. Every
// structural fact has exactly one typed storage row or residue row.
func populationStructuralStorage(scan ScanResult, manifests ManifestSet, surfaces map[string]populationSurface, declarations map[string]DeclarationInfo, facts map[string]populationFact, final map[string]struct{}) error {
	rows := manifests.Surfaces.Storage
	expected := make(map[string]string)
	structural := make(map[string]struct{})
	add := func(id, surfaceID, declarationID, kind string) error {
		if id == "" {
			return populationError("%s storage source has empty fact ID", kind)
		}
		if _, exists := facts[id]; !exists {
			return populationError("%s storage fact %q is not scanned", kind, id)
		}
		declaration, exists := declarations[declarationID]
		if !exists || declaration.PackagePath == "" {
			return populationError("%s storage fact %q has unknown declaration", kind, id)
		}
		if previous, exists := expected[id]; exists {
			return populationError("structural fact %q has multiple storage sources (%s,%s)", id, previous, surfaceID)
		}
		expected[id] = surfaceID
		structural[id] = struct{}{}
		return nil
	}
	for _, row := range scan.Types.Structure.Fields {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "field"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Arrays {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "array"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Slices {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "slice"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Maps {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "map"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Channels {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "channel"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.NamedReferences {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "named reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.MethodReferences {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "method reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.OtherReferences {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "other reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Cycles {
		if err := add(row.FactID, row.SurfaceID, row.DeclarationID, "cycle"); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(rows))
	surfaceAssigned := make(map[string]struct{})
	for _, assignment := range manifests.Surfaces.Assignments {
		if _, isStructural := structural[assignment.FactID]; isStructural {
			surfaceAssigned[assignment.FactID] = struct{}{}
		}
	}
	for _, row := range rows {
		if row.FactID == "" || row.OwnerSurface == "" || !row.Disposition.valid() {
			return populationError("malformed typed storage row %+v", row)
		}
		if _, duplicate := seen[row.FactID]; duplicate {
			return populationError("storage fact %q is classified more than once", row.FactID)
		}
		seen[row.FactID] = struct{}{}
		surfaceID, exists := expected[row.FactID]
		if !exists {
			return populationError("storage fact %q is not a structural scanner fact", row.FactID)
		}
		if surfaceID == "" {
			return populationError("storage fact %q has no named surface; non-public structural facts remain residue", row.FactID)
		}
		surface, exists := findSurfaceByID(surfaces, surfaceID)
		if !exists || surface.Surface != row.OwnerSurface {
			return populationError("storage fact %q owner surface mismatch", row.FactID)
		}
		_, published := surfaceAssigned[row.FactID]
		if row.Disposition != StoragePublicSurface {
			return populationError("storage fact %q has a non-public disposition without a storage witness", row.FactID)
		}
		if !published {
			return populationError("storage fact %q has public-surface disposition without a surface assignment", row.FactID)
		}
		final[row.FactID] = struct{}{}
	}
	// A surface assignment is final only when it has the exact public-surface
	// disposition; it is not a substitute for a typed storage row.
	for id := range structural {
		if _, stored := seen[id]; !stored {
			if _, published := final[id]; published {
				return populationError("structural fact %q is final without typed storage disposition", id)
			}
		}
	}
	return nil
}

func populationResidue(scan ScanResult, manifests ManifestSet, facts map[string]populationFact, final map[string]struct{}) error {
	structuralOwners := make(map[string]string)
	catalogDeclarations := make(map[string]DeclarationRow, len(manifests.Catalog.Declarations))
	for _, declaration := range manifests.Catalog.Declarations {
		catalogDeclarations[declaration.FactID] = declaration
	}
	addStructuralOwner := func(factID, declarationID, kind string) error {
		declaration, exists := catalogDeclarations[declarationID]
		if !exists {
			return populationError("%s residue fact %q references unknown catalog declaration %q", kind, factID, declarationID)
		}
		if _, exists := populationOwner(manifests.Catalog.Owners, declaration.Owner); !exists {
			return populationError("%s residue fact %q declaration owner %q is not a Catalog OwnerRow ID", kind, factID, declaration.Owner)
		}
		if previous, duplicate := structuralOwners[factID]; duplicate {
			return populationError("structural residue fact %q has multiple declaration owners %q and %q", factID, previous, declaration.Owner)
		}
		structuralOwners[factID] = declaration.Owner
		return nil
	}
	for _, row := range scan.Types.Structure.Fields {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "field"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Arrays {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "array"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Slices {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "slice"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Maps {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "map"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Channels {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "channel"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.NamedReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "named reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.MethodReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "method reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.OtherReferences {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "other reference"); err != nil {
			return err
		}
	}
	for _, row := range scan.Types.Structure.Cycles {
		if err := addStructuralOwner(row.FactID, row.DeclarationID, "cycle"); err != nil {
			return err
		}
	}
	moveOwners := make(map[string]string, len(structuralOwners)+len(scan.Types.Exposures))
	for factID, ownerID := range structuralOwners {
		moveOwners[factID] = ownerID
	}
	for _, exposure := range scan.Types.Exposures {
		declaration, exists := catalogDeclarations[exposure.TargetDeclID]
		if !exists {
			return populationError("method exposure residue fact %q references unknown target declaration %q", exposure.FactID, exposure.TargetDeclID)
		}
		if _, exists := populationOwner(manifests.Catalog.Owners, declaration.Owner); !exists {
			return populationError("method exposure residue fact %q target owner %q is not a Catalog OwnerRow ID", exposure.FactID, declaration.Owner)
		}
		if previous, duplicate := moveOwners[exposure.FactID]; duplicate && previous != declaration.Owner {
			return populationError("residue fact %q has multiple move owners %q and %q", exposure.FactID, previous, declaration.Owner)
		}
		moveOwners[exposure.FactID] = declaration.Owner
	}
	splitPlans := make(map[string]SplitPlanRow, len(manifests.Residue.SplitPlans))
	splitPlanUseCount := make(map[string]int, len(manifests.Residue.SplitPlans))
	recipientSets := make(map[string]string, len(manifests.Residue.SplitPlans))
	for _, plan := range manifests.Residue.SplitPlans {
		if !validPopulationAtom(plan.ID) || !factIDsCanonical(plan.OwnerIDs) || len(plan.OwnerIDs) < 2 {
			return populationError("malformed split plan %+v", plan)
		}
		if want := splitPlanID(plan.OwnerIDs); plan.ID != want {
			return populationError("split plan %q is not the canonical digest of its sorted recipients", plan.ID)
		}
		if _, duplicate := splitPlans[plan.ID]; duplicate {
			return populationError("split plan %q is duplicated", plan.ID)
		}
		recipientKey := strings.Join(plan.OwnerIDs, "\x00")
		if previous, duplicate := recipientSets[recipientKey]; duplicate && previous != plan.ID {
			return populationError("split recipient set has multiple plan IDs %q and %q", previous, plan.ID)
		}
		recipientSets[recipientKey] = plan.ID
		for _, ownerID := range plan.OwnerIDs {
			if _, exists := populationOwner(manifests.Catalog.Owners, ownerID); !exists {
				return populationError("split plan %q recipient %q is not an exact Catalog OwnerRow ID", plan.ID, ownerID)
			}
		}
		splitPlans[plan.ID] = plan
	}
	residue := make(map[string]struct{}, len(manifests.Residue.Rows))
	for _, row := range manifests.Residue.Rows {
		if row.CurrentFact == "" || row.Destination == "" || !validPopulationAtom(row.CurrentFact) || !validPopulationAtom(row.Destination) {
			return populationError("incomplete residue row %+v", row)
		}
		switch row.Kind {
		case "move", "split", "delete":
		default:
			return populationError("residue fact %q has closed-kind violation %q", row.CurrentFact, row.Kind)
		}
		if _, exists := facts[row.CurrentFact]; !exists {
			return populationError("residue fact %q is not scanned", row.CurrentFact)
		}
		fact := facts[row.CurrentFact]
		switch row.Kind {
		case "delete":
			if row.Destination != ResidueDeleteDestination {
				return populationError("delete residue fact %q has noncanonical destination %q", row.CurrentFact, row.Destination)
			}
			if fact.kind != "method-exposure" && !strings.HasPrefix(fact.kind, "structure-") {
				return populationError("delete residue fact %q has unsupported fact kind %q", row.CurrentFact, fact.kind)
			}
		case "move":
			if !strings.HasPrefix(fact.kind, "structure-") && fact.kind != "method-exposure" {
				return populationError("%s residue fact %q has unsupported fact kind %q", row.Kind, row.CurrentFact, fact.kind)
			}
			expectedOwner, exists := moveOwners[row.CurrentFact]
			if !exists {
				return populationError("%s residue fact %q has no declaration owner join", row.Kind, row.CurrentFact)
			}
			if row.Destination != expectedOwner {
				return populationError("%s residue fact %q destination %q does not equal structural declaration owner %q", row.Kind, row.CurrentFact, row.Destination, expectedOwner)
			}
		case "split":
			if !strings.HasPrefix(fact.kind, "structure-") {
				return populationError("split residue fact %q has unsupported fact kind %q", row.CurrentFact, fact.kind)
			}
			if _, exists := splitPlans[row.Destination]; !exists {
				return populationError("split residue fact %q references unknown split plan %q", row.CurrentFact, row.Destination)
			}
			splitPlanUseCount[row.Destination]++
		}
		if _, overlap := final[row.CurrentFact]; overlap {
			return populationError("fact %q is both final and residue", row.CurrentFact)
		}
		if _, duplicate := residue[row.CurrentFact]; duplicate {
			return populationError("residue fact %q is classified more than once", row.CurrentFact)
		}
		residue[row.CurrentFact] = struct{}{}
	}
	for planID := range splitPlans {
		if splitPlanUseCount[planID] == 0 {
			return populationError("split plan %q is unused", planID)
		}
	}
	for id := range facts {
		_, published := final[id]
		_, residual := residue[id]
		if published == residual {
			if published {
				return populationError("fact %q is both final and residue", id)
			}
			return populationError("fact %q is missing final/residue classification", id)
		}
	}
	return nil
}

func populationRejectUnusedOwners(manifests ManifestSet) error {
	owners := make(map[string]struct{}, len(manifests.Catalog.Owners))
	for _, owner := range manifests.Catalog.Owners {
		owners[owner.ID] = struct{}{}
	}
	demanded := make(map[string]string, len(owners))
	mark := func(ownerID, source string) error {
		if _, exists := owners[ownerID]; !exists {
			return populationError("%s references unknown owner %q", source, ownerID)
		}
		demanded[ownerID] = source
		return nil
	}
	for _, row := range manifests.Catalog.Declarations {
		if err := mark(row.Owner, "catalog declaration"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Catalog.ImportEdges {
		if err := mark(row.FromOwner, "ownership import edge"); err != nil {
			return err
		}
		if err := mark(row.ToOwner, "ownership import edge"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.Indexes {
		if err := mark(row.Owner, "index"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.HotReferences {
		if err := mark(row.Issuer, "hot reference"); err != nil {
			return err
		}
		if err := mark(row.Consumer, "hot reference"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ColdReferences {
		if err := mark(row.Issuer, "cold reference"); err != nil {
			return err
		}
		if err := mark(row.Consumer, "cold reference"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ContextualReferences {
		if err := mark(row.Issuer, "contextual reference"); err != nil {
			return err
		}
		if err := mark(row.Consumer, "contextual reference"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.Identities {
		if err := mark(row.Owner, "identity"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.IndexPlans {
		if err := mark(row.Owner, "index plan"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ReferencePlans {
		if err := mark(row.Issuer, "reference plan"); err != nil {
			return err
		}
		if err := mark(row.Consumer, "reference plan"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.IdentityPlans {
		if err := mark(row.Owner, "identity plan"); err != nil {
			return err
		}
	}
	for _, row := range manifests.Residue.Rows {
		if row.Kind == "move" {
			if err := mark(row.Destination, "residue move"); err != nil {
				return err
			}
		}
	}
	for _, plan := range manifests.Residue.SplitPlans {
		for _, ownerID := range plan.OwnerIDs {
			if err := mark(ownerID, "split plan"); err != nil {
				return err
			}
		}
	}
	for ownerID := range owners {
		if _, exists := demanded[ownerID]; !exists {
			return populationError("catalog owner %q is unused by declarations, ownership edges, final rows, or residue destinations", ownerID)
		}
	}
	return nil
}
