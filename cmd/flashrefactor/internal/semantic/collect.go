package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// Collect gathers a complete structured workspace snapshot. diagnosticFiles
// limits the diagnostic delta denominator; an empty list means every source
// diagnostic in the loaded workspace.
func (session *Session) Collect(ctx context.Context, intent cutplan.Intent, diagnosticFiles []string) (Snapshot, error) {
	requests, scope, err := collectionInputs(intent, cutplan.ObjectSource)
	if err != nil {
		return Snapshot{}, err
	}
	return session.collect(ctx, requests, scope, diagnosticFiles, nil, cutplan.ObjectSource)
}

// Survey resolves a caller-selected source set without constructing an Intent
// or a virtual target. It is read-only evidence for proposal review only.
func (session *Session) Survey(ctx context.Context, symbols []cutplan.SymbolRef) (Snapshot, error) {
	return session.survey(ctx, symbols, "", false)
}

// SurveyBoundary extends a read-only source survey with one exact prospective
// destination path. When that destination already exists, its package joins
// the typed survey frontier so a caller can reject a wrong or ambiguous target
// before constructing any Intent. A missing destination remains a valid
// future output and is not fabricated as source evidence.
func (session *Session) SurveyBoundary(ctx context.Context, symbols []cutplan.SymbolRef, destination string) (Snapshot, error) {
	if destination == "" {
		return Snapshot{}, fmt.Errorf("survey boundary requires an exact destination")
	}
	return session.survey(ctx, symbols, destination, true)
}

func (session *Session) survey(ctx context.Context, symbols []cutplan.SymbolRef, destination string, expandImporters bool) (Snapshot, error) {
	if len(symbols) == 0 {
		return Snapshot{}, fmt.Errorf("survey requires selected source symbols")
	}
	requests := make([]SymbolRequest, 0, len(symbols))
	seen := map[string]bool{}
	for _, symbol := range symbols {
		if symbol.Object == "" || seen[symbol.Object] {
			return Snapshot{}, fmt.Errorf("survey symbols must be non-empty and unique")
		}
		seen[symbol.Object] = true
		// An architectural boundary survey must include every typed importer of
		// its selected exported surface; otherwise a compiler could manufacture
		// a smaller route closure than source actually has. Generic discovery
		// keeps its narrower historical frontier because it never emits an
		// Intent or a route denominator.
		requests = append(requests, SymbolRequest{Object: symbol, Role: cutplan.ObjectSource, Impact: expandImporters})
	}
	scope := loadScope{}
	if destination != "" {
		full, relative, err := session.overlayPath(destination)
		if err != nil {
			return Snapshot{}, fmt.Errorf("survey destination %s: %w", destination, err)
		}
		info, err := os.Stat(full)
		if err == nil {
			if !info.Mode().IsRegular() {
				return Snapshot{}, fmt.Errorf("survey destination is not a regular file: %s", relative)
			}
			scope.Files = []string{relative}
		} else if !os.IsNotExist(err) {
			return Snapshot{}, fmt.Errorf("survey destination %s: %w", relative, err)
		}
	}
	return session.collect(ctx, requests, scope, nil, nil, cutplan.ObjectSource)
}

// CollectVirtual evaluates a complete post-render tree in an isolated shadow.
// Every changed, new, or deleted file is materialized there, so package
// discovery cannot read the preimage or miss a newly-created package.
func (session *Session) CollectVirtual(ctx context.Context, intent cutplan.Intent, diagnosticFiles []string, files []VirtualFile) (Snapshot, error) {
	requests, scope, err := collectionInputs(intent, cutplan.ObjectTarget)
	if err != nil {
		return Snapshot{}, err
	}
	return session.collect(ctx, requests, scope, diagnosticFiles, files, cutplan.ObjectTarget)
}

func (session *Session) collect(ctx context.Context, requests []SymbolRequest, scope loadScope, diagnosticFiles []string, files []VirtualFile, role cutplan.ObjectRole) (Snapshot, error) {
	if session == nil || session.scratch == "" {
		return Snapshot{}, fmt.Errorf("semantic session is closed")
	}
	if err := validateRequests(requests, role); err != nil {
		return Snapshot{}, err
	}
	diagnostics, err := session.normalizeDiagnosticPaths(diagnosticFiles)
	if err != nil {
		return Snapshot{}, err
	}
	loadRoot, overlay, cleanup, err := session.virtualWorkspace(files)
	if err != nil {
		return Snapshot{}, err
	}
	defer cleanup()
	scope, err = materializeScope(loadRoot, role, scope)
	if err != nil {
		return Snapshot{}, err
	}
	result, err := session.config.Loader.Load(ctx, LoadRequest{
		Root: loadRoot, Scratch: session.scratch,
		Environment: cloneStrings(session.environment), BuildFlags: cloneStrings(session.buildFlags), Patterns: []string{"./..."},
		scope: scope, Requests: requests, Overlay: overlay, DiagnosticPaths: diagnostics,
	})
	if err != nil {
		return Snapshot{}, err
	}
	if err := validateAuthority(result.Toolchain); err != nil {
		return Snapshot{}, err
	}
	if len(result.WorkspaceFailures) != 0 {
		return Snapshot{}, fmt.Errorf("structured workspace load failed: %s", result.WorkspaceFailures[0])
	}
	if result.Workspace == nil {
		return Snapshot{}, fmt.Errorf("structured loader returned no typed workspace")
	}
	if err := ValidateEvidence(requests, result.Objects); err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		Toolchain: cutplan.Toolchain{HelperBuild: session.config.Flashrefactor, GoVersion: result.Toolchain.Go.Version, GoExecutableSHA256: result.Toolchain.Go.SHA256, Resolver: result.Toolchain.Loader, BuildEnvSHA256: result.Toolchain.BuildEnvSHA256, ModuleGraphSHA256: result.Toolchain.ModuleGraphSHA256},
		Authority: result.Toolchain, Workspace: result.Workspace, Objects: result.Objects, Diagnostics: result.Diagnostics,
		Structure: result.Workspace.Structure(),
	}
	return canonicalSnapshot(snapshot), nil
}

// Requests derives one role-specific collection denominator from the reviewed
// cutplan. This package never reproduces relocation ownership or roles.
func Requests(intent cutplan.Intent, role cutplan.ObjectRole) ([]SymbolRequest, error) {
	if role != cutplan.ObjectSource && role != cutplan.ObjectTarget {
		return nil, fmt.Errorf("invalid requested object role %q", role)
	}
	requirements, err := cutplan.ResolutionRequirements(intent)
	if err != nil {
		return nil, err
	}
	impacts, err := cutplan.ImpactObjects(intent)
	if err != nil {
		return nil, err
	}
	impact := make(map[string]bool, len(impacts))
	for _, object := range impacts {
		impact[object.Object] = true
	}
	result := make([]SymbolRequest, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement.Role == role {
			result = append(result, SymbolRequest{Object: requirement.Object, Role: requirement.Role, Impact: impact[requirement.Object.Object]})
		}
	}
	return result, nil
}

func collectionInputs(intent cutplan.Intent, role cutplan.ObjectRole) ([]SymbolRequest, loadScope, error) {
	requests, err := Requests(intent, role)
	if err != nil {
		return nil, loadScope{}, err
	}
	paths := cutplan.ReadPaths(intent)
	scope := loadScope{Files: goPaths(paths)}
	if role == cutplan.ObjectTarget {
		scope.Files = goPaths(cutplan.WritePaths(intent))
		scope.ExpandFileOwners = true
		scope.RemovedSurfaceOwners, err = removedExportedSourceOwners(intent)
		if err != nil {
			return nil, loadScope{}, err
		}
	}
	return requests, scope, nil
}

func goPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, ".go") {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

// materializeScope evaluates declared paths in the concrete pre/post state.
// Source inputs are required to exist. Target outputs that are absent after
// rendering or application are retired, not future package roots.
func materializeScope(root string, role cutplan.ObjectRole, scope loadScope) (loadScope, error) {
	result := loadScope{
		ExpandFileOwners:     scope.ExpandFileOwners,
		RemovedSurfaceOwners: append([]string(nil), scope.RemovedSurfaceOwners...),
	}
	for _, path := range scope.Files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if !pathInsideRoot(root, full) {
			return loadScope{}, fmt.Errorf("semantic scope path escapes workspace: %s", path)
		}
		info, err := os.Stat(full)
		if os.IsNotExist(err) && role == cutplan.ObjectTarget {
			continue
		}
		if err != nil {
			return loadScope{}, fmt.Errorf("semantic scope path %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return loadScope{}, fmt.Errorf("semantic scope path is not a regular file: %s", path)
		}
		result.Files = append(result.Files, path)
	}
	return result, nil
}

// Merge proves that source and target collections satisfy the one generated
// cutplan denominator, then returns their role-indexed union. It does not
// invent requirements or flatten their distinct structural worlds.
func Merge(source, target Snapshot, requirements []cutplan.ResolutionRequirement) (Merged, error) {
	if source.Workspace == nil || target.Workspace == nil {
		return Merged{}, fmt.Errorf("merge requires source and target workspaces")
	}
	if source.Toolchain != target.Toolchain || source.Authority != target.Authority {
		return Merged{}, fmt.Errorf("source and target semantic authorities differ")
	}
	canonical, err := canonicalRequirements(requirements)
	if err != nil {
		return Merged{}, err
	}
	sourceRequests := requestsForRequirements(canonical, cutplan.ObjectSource)
	targetRequests := requestsForRequirements(canonical, cutplan.ObjectTarget)
	if err := ValidateEvidence(sourceRequests, source.Objects); err != nil {
		return Merged{}, fmt.Errorf("source merge evidence: %w", err)
	}
	if err := ValidateEvidence(targetRequests, target.Objects); err != nil {
		return Merged{}, fmt.Errorf("target merge evidence: %w", err)
	}
	if err := validateRequirementLocations(canonical, source.Objects, target.Objects); err != nil {
		return Merged{}, err
	}
	objects := append(append([]cutplan.ObjectEvidence(nil), source.Objects...), target.Objects...)
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Object.Object != objects[j].Object.Object {
			return objects[i].Object.Object < objects[j].Object.Object
		}
		return objects[i].Role < objects[j].Role
	})
	return Merged{Source: canonicalSnapshot(source), Target: canonicalSnapshot(target), Requirements: canonical, Objects: objects}, nil
}

func validateRequirementLocations(requirements []cutplan.ResolutionRequirement, source, target []cutplan.ObjectEvidence) error {
	byObject := make(map[string]cutplan.ResolutionRequirement, len(requirements))
	for _, requirement := range requirements {
		byObject[requirement.Object.Object] = requirement
	}
	for _, evidence := range append(append([]cutplan.ObjectEvidence(nil), source...), target...) {
		requirement := byObject[evidence.Object.Object]
		if requirement.Path != "" && evidence.Definition.Path != requirement.Path {
			return fmt.Errorf("%s evidence definition does not match requirement for %s", evidence.Role, evidence.Object.Object)
		}
		if requirement.Package != "" && evidence.Package != requirement.Package {
			return fmt.Errorf("%s evidence package does not match requirement for %s", evidence.Role, evidence.Object.Object)
		}
	}
	return nil
}

func canonicalRequirements(values []cutplan.ResolutionRequirement) ([]cutplan.ResolutionRequirement, error) {
	result := append([]cutplan.ResolutionRequirement(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Object.Object < result[j].Object.Object })
	seen := map[string]bool{}
	for _, value := range result {
		if value.Object.Object == "" || (value.Role != cutplan.ObjectSource && value.Role != cutplan.ObjectTarget) {
			return nil, fmt.Errorf("invalid resolution requirement")
		}
		if seen[value.Object.Object] {
			return nil, fmt.Errorf("duplicate resolution requirement: %s", value.Object.Object)
		}
		seen[value.Object.Object] = true
	}
	return result, nil
}

func requestsForRequirements(requirements []cutplan.ResolutionRequirement, role cutplan.ObjectRole) []SymbolRequest {
	result := make([]SymbolRequest, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement.Role == role {
			result = append(result, SymbolRequest{Object: requirement.Object, Role: role})
		}
	}
	return result
}

func (session *Session) normalizeDiagnosticPaths(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		relative, err := session.relativePath(path)
		if err != nil {
			return nil, fmt.Errorf("diagnostic path %q: %w", path, err)
		}
		if seen[relative] {
			return nil, fmt.Errorf("duplicate diagnostic path: %s", relative)
		}
		seen[relative] = true
		result = append(result, relative)
	}
	sort.Strings(result)
	return result, nil
}

func validateAuthority(authority ToolchainEvidence) error {
	if authority.Loader != packagesAuthority {
		return fmt.Errorf("unrecognized semantic loader authority: %q", authority.Loader)
	}
	if authority.Go.Path == "" || len(authority.Go.SHA256) != 64 || authority.Go.Version == "" {
		return fmt.Errorf("incomplete Go executable identity")
	}
	if len(authority.BuildEnvSHA256) != 64 || len(authority.ModuleGraphSHA256) != 64 {
		return fmt.Errorf("incomplete semantic build context identity")
	}
	return nil
}

func validateRequests(requests []SymbolRequest, expectedRole cutplan.ObjectRole) error {
	if err := validateRequestShape(requests); err != nil {
		return err
	}
	for _, request := range requests {
		if request.Role != expectedRole {
			return fmt.Errorf("object %s has role %q; %s collection requires role %q", request.Object.Object, request.Role, expectedRole, expectedRole)
		}
	}
	return nil
}

func validateRequestShape(requests []SymbolRequest) error {
	seen := make(map[string]cutplan.ObjectRole, len(requests))
	for _, request := range requests {
		if request.Object.Object == "" {
			return fmt.Errorf("object request has empty object")
		}
		if request.Role != cutplan.ObjectSource && request.Role != cutplan.ObjectTarget {
			return fmt.Errorf("object %s has invalid role %q", request.Object.Object, request.Role)
		}
		if previous, duplicate := seen[request.Object.Object]; duplicate && previous != request.Role {
			return fmt.Errorf("object request mixes roles for %s", request.Object.Object)
		} else if duplicate {
			return fmt.Errorf("duplicate object request: %s", request.Object.Object)
		}
		seen[request.Object.Object] = request.Role
	}
	return nil
}

// ValidateEvidence proves the loader returned exactly the requested object
// denominator, with one declaration and a duplicate-free use-site set.
func ValidateEvidence(requests []SymbolRequest, objects []cutplan.ObjectEvidence) error {
	if err := validateRequestShape(requests); err != nil {
		return err
	}
	wanted := make(map[string]cutplan.ObjectRole, len(requests))
	roles := make(map[string]cutplan.ObjectRole, len(requests))
	for _, request := range requests {
		wanted[request.Object.Object] = request.Role
		roles[request.Object.Object] = request.Role
	}
	seen := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		_, wantedObject := wanted[object.Object.Object]
		if !wantedObject {
			return fmt.Errorf("stale object evidence: %s", object.Object.Object)
		}
		if _, duplicate := seen[object.Object.Object]; duplicate {
			return fmt.Errorf("duplicate object evidence: %s", object.Object.Object)
		}
		seen[object.Object.Object] = struct{}{}
		if object.Role != roles[object.Object.Object] {
			return fmt.Errorf("wrong evidence role for %s: got %q want %q", object.Object.Object, object.Role, roles[object.Object.Object])
		}
		if object.Package == "" || object.Definition.Path == "" || len(object.Definition.PackageIDs) == 0 || object.Definition.Offset < 0 || object.Definition.Line < 1 || object.Definition.Column < 1 || object.Definition.Role != cutplan.SiteDeclaration {
			return fmt.Errorf("invalid definition for %s", object.Object.Object)
		}
		for _, reference := range object.References {
			if len(reference.PackageIDs) == 0 || reference.Path == "" || reference.Offset < 0 || reference.Line < 1 || reference.Column < 1 {
				return fmt.Errorf("invalid semantic site for %s", object.Object.Object)
			}
			switch reference.Role {
			case cutplan.SiteUse, cutplan.SiteSelector, cutplan.SiteImport:
			default:
				return fmt.Errorf("invalid semantic site role for %s", object.Object.Object)
			}
		}
		if _, err := uniquePositions(object.References); err != nil {
			return fmt.Errorf("object %s: %w", object.Object.Object, err)
		}
	}
	for _, request := range requests {
		if _, found := seen[request.Object.Object]; !found {
			return fmt.Errorf("missing object evidence: %s", request.Object.Object)
		}
	}
	return nil
}

func canonicalSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Objects = append([]cutplan.ObjectEvidence(nil), snapshot.Objects...)
	for index := range snapshot.Objects {
		snapshot.Objects[index].Definition = clonePosition(snapshot.Objects[index].Definition)
		for site := range snapshot.Objects[index].References {
			snapshot.Objects[index].References[site] = clonePosition(snapshot.Objects[index].References[site])
		}
		snapshot.Objects[index].References = sortedPositions(snapshot.Objects[index].References)
	}
	sort.Slice(snapshot.Objects, func(i, j int) bool { return snapshot.Objects[i].Object.Object < snapshot.Objects[j].Object.Object })
	snapshot.Diagnostics = canonicalDiagnostics(snapshot.Diagnostics)
	snapshot.Structure = canonicalStructure(snapshot.Structure)
	return snapshot
}

func clonePosition(value cutplan.Position) cutplan.Position {
	value.PackageIDs = append([]string(nil), value.PackageIDs...)
	return value
}

func canonicalDiagnostics(values []Diagnostic) []Diagnostic {
	result := append([]Diagnostic(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Position.Path != right.Position.Path {
			return left.Position.Path < right.Position.Path
		}
		if left.Position.Line != right.Position.Line {
			return left.Position.Line < right.Position.Line
		}
		if left.Position.Column != right.Position.Column {
			return left.Position.Column < right.Position.Column
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Message < right.Message
	})
	return result
}

func canonicalStructure(value StructuralSnapshot) StructuralSnapshot {
	value.Packages = append([]StructuralPackage(nil), value.Packages...)
	for index := range value.Packages {
		value.Packages[index].Imports = append([]string(nil), value.Packages[index].Imports...)
		sort.Strings(value.Packages[index].Imports)
	}
	sort.Slice(value.Packages, func(i, j int) bool { return value.Packages[i].ID < value.Packages[j].ID })
	value.Files = append([]StructuralFile(nil), value.Files...)
	for index := range value.Files {
		value.Files[index].Imports = append([]cutplan.ImportRef(nil), value.Files[index].Imports...)
		sort.Slice(value.Files[index].Imports, func(i, j int) bool {
			left, right := value.Files[index].Imports[i], value.Files[index].Imports[j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			return left.Alias < right.Alias
		})
	}
	sort.Slice(value.Files, func(i, j int) bool {
		if value.Files[i].Path != value.Files[j].Path {
			return value.Files[i].Path < value.Files[j].Path
		}
		return value.Files[i].PackageID < value.Files[j].PackageID
	})
	return value
}
