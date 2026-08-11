package cutplan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidateIntent rejects ambiguity before any resolver or file writer sees a
// cut.  It never infers an omitted route, output, owner, or verification row.
func ValidateIntent(intent Intent) error {
	if intent.Schema != Version {
		return fmt.Errorf("cutplan schema must be %d", Version)
	}
	if !safeLabel(intent.Name) {
		return fmt.Errorf("intent name is required and must be a plain label")
	}
	if len(intent.Operations) == 0 {
		return fmt.Errorf("intent has no operations")
	}
	seen := map[string]bool{}
	for _, operation := range intent.Operations {
		if !safeLabel(operation.ID) {
			return fmt.Errorf("operation id %q is not a plain label", operation.ID)
		}
		if seen[operation.ID] {
			return fmt.Errorf("duplicate operation id %q", operation.ID)
		}
		seen[operation.ID] = true
		if err := validateOperation(operation); err != nil {
			return fmt.Errorf("operation %s: %w", operation.ID, err)
		}
	}
	if _, err := topologicalOrder(intent.Operations); err != nil {
		return err
	}
	if _, err := resolutionRequirements(intent); err != nil {
		return err
	}
	return validateCriticalPairs(intent.Operations)
}

func validateOperation(operation Operation) error {
	if err := validateAfter(operation); err != nil {
		return err
	}
	if !safeOwner(operation.Authority.From) || !safeOwner(operation.Authority.To) || operation.Authority.From == operation.Authority.To {
		return fmt.Errorf("must declare distinct from/to authority")
	}
	if err := validateEdits(operation.Edits); err != nil {
		return err
	}
	if err := validateFootprint(operation); err != nil {
		return err
	}
	if err := validateBindings(operation.Bindings, operation.Edits, operation.Footprint); err != nil {
		return err
	}
	if err := validateImports(operation.Imports, operation.Edits, operation.Footprint); err != nil {
		return err
	}
	return validateVerification(operation.Verify, operation.Footprint)
}

func validateAfter(operation Operation) error {
	seen := map[string]bool{}
	for _, dependency := range operation.After {
		if !safeLabel(dependency) {
			return fmt.Errorf("invalid after dependency %q", dependency)
		}
		if dependency == operation.ID {
			return fmt.Errorf("operation cannot depend on itself")
		}
		if seen[dependency] {
			return fmt.Errorf("duplicate after dependency %q", dependency)
		}
		seen[dependency] = true
	}
	return nil
}

func validateEdits(edits []Edit) error {
	if len(edits) == 0 {
		return fmt.Errorf("has no edits")
	}
	seen := map[string]bool{}
	for _, edit := range edits {
		key, err := validateEdit(edit)
		if err != nil {
			return err
		}
		if seen[key] {
			return fmt.Errorf("duplicate edit %s", key)
		}
		seen[key] = true
	}
	return nil
}

func validateEdit(edit Edit) (string, error) {
	payloads := 0
	if edit.Relocate != nil {
		payloads++
	}
	if edit.Retire != nil {
		payloads++
	}
	if edit.Generate != nil {
		payloads++
	}
	if payloads != 1 {
		return "", fmt.Errorf("edit %q must contain exactly one payload", edit.Kind)
	}
	switch edit.Kind {
	case EditRelocate:
		if edit.Relocate == nil || edit.Retire != nil || edit.Generate != nil {
			return "", fmt.Errorf("relocate edit has wrong payload")
		}
		if err := validateRelocate(*edit.Relocate); err != nil {
			return "", err
		}
		return relocateKey(*edit.Relocate), nil
	case EditRetire:
		if edit.Retire == nil || edit.Relocate != nil || edit.Generate != nil {
			return "", fmt.Errorf("retire edit has wrong payload")
		}
		if err := validateRetire(*edit.Retire); err != nil {
			return "", err
		}
		return "retire\x00" + edit.Retire.Source + "\x00" + symbolListKey(edit.Retire.Symbols), nil
	case EditGenerate:
		if edit.Generate == nil || edit.Relocate != nil || edit.Retire != nil {
			return "", fmt.Errorf("generate edit has wrong payload")
		}
		if err := validateGenerate(*edit.Generate); err != nil {
			return "", err
		}
		return "generate\x00" + string(edit.Generate.Provider) + "\x00" + strings.Join(sorted(edit.Generate.Inputs), "\x00") + "\x00" + edit.Generate.Destination, nil
	default:
		return "", fmt.Errorf("unknown edit kind %q", edit.Kind)
	}
}

func validateRelocate(value Relocate) error {
	if !safePath(value.Source) || !safePath(value.Destination.Path) || !safePackageClause(value.Destination.Package) || value.Source == value.Destination.Path {
		return fmt.Errorf("relocate needs distinct exact source and destination")
	}
	if err := validateRelocations(value.Subjects); err != nil {
		return err
	}
	if value.Containment != nil {
		if !safeSymbolRef(value.Containment.Parent) || !safeSymbolRef(value.Containment.Child) || !safeSymbolRef(value.Containment.Through) ||
			value.Containment.Parent == value.Containment.Child || value.Containment.Parent == value.Containment.Through || value.Containment.Child == value.Containment.Through ||
			!fieldMember(value.Containment.Through) {
			return fmt.Errorf("invalid containment")
		}
	}
	return nil
}

func validateRelocations(values []Relocation) error {
	if len(values) == 0 {
		return fmt.Errorf("relocate subject list is empty")
	}
	from, to := map[string]bool{}, map[string]bool{}
	for _, value := range values {
		if !safeSymbolRef(value.From) || !safeSymbolRef(value.To) || value.From == value.To {
			return fmt.Errorf("invalid relocation subject")
		}
		if from[value.From.Object] || to[value.To.Object] {
			return fmt.Errorf("relocation subjects must map each source and target exactly once")
		}
		from[value.From.Object], to[value.To.Object] = true, true
	}
	return nil
}

func validateRetire(value Retire) error {
	if !safePath(value.Source) {
		return fmt.Errorf("retire needs exact source")
	}
	return validateSymbols("retire symbol", value.Symbols)
}

func validateGenerate(value Generate) error {
	if !safeProvider(value.Provider) || !safePath(value.Destination) {
		return fmt.Errorf("generate needs a provider key and exact destination")
	}
	return exactPaths("generator input", value.Inputs)
}

func safeProvider(provider Provider) bool {
	return safeLabel(string(provider))
}

func validateFootprint(operation Operation) error {
	if len(operation.Footprint.Read) == 0 || len(operation.Footprint.Write) == 0 {
		return fmt.Errorf("footprint requires exact non-empty read and write paths")
	}
	if err := exactPaths("read footprint", operation.Footprint.Read); err != nil {
		return err
	}
	if err := exactPaths("write footprint", operation.Footprint.Write); err != nil {
		return err
	}
	read := stringSet(operation.Footprint.Read)
	write := stringSet(operation.Footprint.Write)
	claimedRead := map[string]bool{}
	claimedWrite := map[string]bool{}
	for _, edit := range operation.Edits {
		switch edit.Kind {
		case EditRelocate:
			if !read[edit.Relocate.Source] || !write[edit.Relocate.Source] || !write[edit.Relocate.Destination.Path] {
				return fmt.Errorf("relocate source must be read/write and destination must be written")
			}
			claimedRead[edit.Relocate.Source] = true
			claimedWrite[edit.Relocate.Source] = true
			claimedWrite[edit.Relocate.Destination.Path] = true
			// Destination may be an existing component file; when so, it must
			// be fingerprinted and is still claimed by this relocation.
			claimedRead[edit.Relocate.Destination.Path] = true
		case EditRetire:
			if !read[edit.Retire.Source] || !write[edit.Retire.Source] {
				return fmt.Errorf("retire source must be read/write")
			}
			claimedRead[edit.Retire.Source] = true
			claimedWrite[edit.Retire.Source] = true
		case EditGenerate:
			if !write[edit.Generate.Destination] {
				return fmt.Errorf("generate destination must be written")
			}
			for _, input := range edit.Generate.Inputs {
				if !read[input] {
					return fmt.Errorf("generator input must be read: %s", input)
				}
				claimedRead[input] = true
			}
			claimedWrite[edit.Generate.Destination] = true
		}
	}
	for _, binding := range operation.Bindings {
		claimedRead[binding.Consumer] = true
		claimedWrite[binding.Consumer] = true
	}
	for _, route := range operation.Imports {
		claimedRead[route.Consumer] = true
		claimedWrite[route.Consumer] = true
	}
	for _, path := range operation.Footprint.Read {
		if !claimedRead[path] {
			return fmt.Errorf("read footprint path is not claimed by an edit, binding, or import: %s", path)
		}
	}
	for _, path := range operation.Footprint.Write {
		if !claimedWrite[path] {
			return fmt.Errorf("write footprint path is not claimed by an edit, binding, or import: %s", path)
		}
	}
	return nil
}

func validateBindings(values []Binding, edits []Edit, footprint Footprint) error {
	routes := declaredRelocationPairs(edits)
	seen := map[string]bool{}
	for _, value := range values {
		if !safePath(value.Consumer) || !safeSymbolRef(value.From) || !safeSymbolRef(value.To) || value.From == value.To {
			return fmt.Errorf("invalid binding")
		}
		if !routes[routeKey(value.From, value.To)] {
			return fmt.Errorf("binding is not an exact declared relocation subject: %s", routeKey(value.From, value.To))
		}
		if !knownBindingForm(value.Form) {
			return fmt.Errorf("invalid binding form %q", value.Form)
		}
		if (value.Form == BindingDirect || value.Form == BindingPackageSelector) && len(value.Receiver) != 0 {
			return fmt.Errorf("%s binding cannot have receiver steps", value.Form)
		}
		if (value.Form == BindingField || value.Form == BindingMethodCall) && len(value.Receiver) == 0 {
			return fmt.Errorf("%s binding requires receiver steps", value.Form)
		}
		for _, step := range value.Receiver {
			if (step.Kind != ReceiverField && step.Kind != ReceiverDirectView) || !safeSymbolRef(step.Object) {
				return fmt.Errorf("invalid receiver path step")
			}
		}
		if !contains(footprint.Read, value.Consumer) || !contains(footprint.Write, value.Consumer) {
			return fmt.Errorf("binding consumer must be read and written")
		}
		key := bindingKey(value)
		if seen[key] {
			return fmt.Errorf("duplicate binding")
		}
		seen[key] = true
	}
	return nil
}

func knownBindingForm(form BindingForm) bool {
	switch form {
	case BindingDirect, BindingField, BindingMethodCall, BindingPackageSelector:
		return true
	default:
		return false
	}
}

func validateImports(values []Import, edits []Edit, footprint Footprint) error {
	targets := declaredImportTargets(edits)
	seen := map[string]bool{}
	for _, value := range values {
		if !safePath(value.Consumer) || (value.From == nil && value.To == nil) {
			return fmt.Errorf("import needs a consumer and at least one endpoint")
		}
		if value.From != nil && !validImportRef(*value.From) {
			return fmt.Errorf("invalid source import")
		}
		if value.To != nil && !validImportRef(*value.To) {
			return fmt.Errorf("invalid destination import")
		}
		if value.From != nil && value.To != nil && *value.From == *value.To {
			return fmt.Errorf("import endpoints are identical")
		}
		if err := validateSymbols("import symbol", value.Symbols); err != nil {
			return err
		}
		for _, symbol := range value.Symbols {
			if !targets[symbol.Object] {
				return fmt.Errorf("import symbol is outside target cut surface: %s", symbol.Object)
			}
		}
		if !contains(footprint.Read, value.Consumer) || !contains(footprint.Write, value.Consumer) {
			return fmt.Errorf("import consumer must be read and written")
		}
		key := importKey(value)
		if seen[key] {
			return fmt.Errorf("duplicate import")
		}
		seen[key] = true
	}
	return nil
}

func validImportRef(value ImportRef) bool {
	return safeImportPath(value.Path) && safePackageClause(value.Name) && safeImportAlias(value.Alias)
}

func validateVerification(value Verification, footprint Footprint) error {
	if len(value.Laws) == 0 || len(value.Gates) == 0 {
		return fmt.Errorf("verify requires exact named laws and gates")
	}
	if err := validateLaws(value.Laws); err != nil {
		return err
	}
	seenGate := map[Gate]bool{}
	for _, gate := range value.Gates {
		if !knownGate(gate) {
			return fmt.Errorf("unknown verification gate %q", gate)
		}
		if seenGate[gate] {
			return fmt.Errorf("duplicate verification gate %q", gate)
		}
		seenGate[gate] = true
	}
	return nil
}

func knownGate(gate Gate) bool {
	switch gate {
	case GateDiagnostics, GateImportDAG, GateResidue:
		return true
	default:
		return false
	}
}

func validateLaws(values []Law) error {
	seen := map[string]bool{}
	for _, value := range values {
		if !safeLabel(value.ID) || !ConcretePackage(value.Package) || !token.IsIdentifier(value.Test) || !strings.HasPrefix(value.Test, "Test") {
			return fmt.Errorf("law must use an id, package, and exact top-level Test name")
		}
		key := lawKey(value)
		if seen[key] {
			return fmt.Errorf("duplicate law %q", value.ID)
		}
		seen[key] = true
	}
	return nil
}

// ConcretePackage admits exactly one Go package selector: a clean repository
// relative path or an explicit import path. Patterns would let same-named
// tests in a different package satisfy a law denominator.
func ConcretePackage(value string) bool {
	if strings.Contains(value, "...") || strings.ContainsAny(value, "*?[]{} ,;|&`$\\") {
		return false
	}
	if strings.HasPrefix(value, "./") {
		return safePackage(value) && value != "./"
	}
	return safeImportPath(value)
}

func declaredRelocationPairs(edits []Edit) map[string]bool {
	result := map[string]bool{}
	for _, edit := range edits {
		if edit.Kind != EditRelocate {
			continue
		}
		for _, subject := range edit.Relocate.Subjects {
			result[routeKey(subject.From, subject.To)] = true
		}
	}
	return result
}

// Imports only make target cut-surface declarations visible. Containment's
// child is the sole structural target that is not a relocation subject.
func declaredImportTargets(edits []Edit) map[string]bool {
	result := map[string]bool{}
	for _, edit := range edits {
		if edit.Kind != EditRelocate {
			continue
		}
		for _, subject := range edit.Relocate.Subjects {
			result[subject.To.Object] = true
		}
		if edit.Relocate.Containment != nil {
			result[edit.Relocate.Containment.Child.Object] = true
		}
	}
	return result
}

// Fingerprint reads only exact footprint paths.  It never walks directories.
func Fingerprint(root string, paths, absent []string) (InputFingerprint, error) {
	if err := exactPaths("fingerprint path", paths); err != nil {
		return InputFingerprint{}, err
	}
	if err := exactPaths("absent path", absent); err != nil {
		return InputFingerprint{}, err
	}
	result := InputFingerprint{Absent: sorted(absent)}
	for _, path := range sorted(paths) {
		full, err := existingFile(root, path)
		if err != nil {
			return InputFingerprint{}, err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return InputFingerprint{}, fmt.Errorf("read %s: %w", path, err)
		}
		digest := sha256.Sum256(data)
		result.Files = append(result.Files, HashPath{Path: path, SHA256: hex.EncodeToString(digest[:])})
	}
	for _, path := range result.Absent {
		if err := absentDestination(root, path); err != nil {
			return InputFingerprint{}, err
		}
	}
	return result, ValidateInputs(result)
}

func existingFile(root, path string) (string, error) {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", fmt.Errorf("resolve input %s: %w", path, err)
	}
	if !inside(rootReal, real) {
		return "", fmt.Errorf("input escapes workspace through symlink: %s", path)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("stat input %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("input is not a regular file: %s", path)
	}
	return real, nil
}

func absentDestination(root, path string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if _, err := os.Lstat(full); err == nil {
		return fmt.Errorf("absent path exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat absent %s: %w", path, err)
	}
	for parent := filepath.Dir(full); ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(parent); err == nil {
			realParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return fmt.Errorf("resolve absent parent %s: %w", path, err)
			}
			if !inside(rootReal, realParent) {
				return fmt.Errorf("absent path escapes workspace through symlink: %s", path)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat absent parent %s: %w", path, err)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("absent path has no workspace parent: %s", path)
		}
	}
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ValidateInputs validates a generated byte/absence fingerprint without disk
// access.  Whether it is complete for an Intent is checked by BuildLock.
func ValidateInputs(input InputFingerprint) error {
	seen := map[string]bool{}
	for _, file := range input.Files {
		if !safePath(file.Path) || !sha256Pattern.MatchString(file.SHA256) {
			return fmt.Errorf("invalid file fingerprint")
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate input path %s", file.Path)
		}
		seen[file.Path] = true
	}
	for _, path := range input.Absent {
		if !safePath(path) {
			return fmt.Errorf("invalid absent path")
		}
		if seen[path] {
			return fmt.Errorf("path %s appears as both present and absent", path)
		}
		seen[path] = true
	}
	return nil
}

func validateSymbols(label string, values []SymbolRef) error {
	if len(values) == 0 {
		return fmt.Errorf("%s list is empty", label)
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !safeSymbolRef(value) {
			return fmt.Errorf("invalid %s %q", label, value.Object)
		}
		if seen[value.Object] {
			return fmt.Errorf("duplicate %s %q", label, value.Object)
		}
		seen[value.Object] = true
	}
	return nil
}

func safeSymbolRef(value SymbolRef) bool {
	parts := strings.Split(value.Object, "#")
	if len(parts) != 2 || !safeImportPath(parts[0]) {
		return false
	}
	object := parts[1]
	if strings.HasPrefix(object, "package:") {
		return token.IsIdentifier(strings.TrimPrefix(object, "package:"))
	}
	segments := strings.Split(object, "/")
	if len(segments) != 2 || !strings.HasPrefix(segments[0], "type:") {
		return false
	}
	if !token.IsIdentifier(strings.TrimPrefix(segments[0], "type:")) {
		return false
	}
	member := segments[1]
	if strings.HasPrefix(member, "field:") {
		return token.IsIdentifier(strings.TrimPrefix(member, "field:"))
	}
	if strings.HasPrefix(member, "method:") {
		return token.IsIdentifier(strings.TrimPrefix(member, "method:"))
	}
	return false
}

func fieldMember(value SymbolRef) bool {
	return strings.Contains(value.Object, "#type:") && strings.Contains(value.Object, "/field:")
}

func exactPaths(label string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if !safePath(value) {
			return fmt.Errorf("invalid %s %q", label, value)
		}
		if seen[value] {
			return fmt.Errorf("duplicate %s %q", label, value)
		}
		seen[value] = true
	}
	return nil
}

func safePath(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") || hasForbidden(value) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	return clean == value && clean != "." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}

func safePackage(value string) bool {
	if value == "" || hasForbidden(value) || strings.ContainsAny(value, " \\;|&`$") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return clean == value || (strings.HasPrefix(value, "./") && clean == strings.TrimPrefix(value, "./"))
}

func safeImportPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || hasForbidden(value) || containsControl(value) || strings.ContainsAny(value, " ;|&`$") {
		return false
	}
	clean := pathpkg.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func safeImportAlias(value string) bool {
	return value == "" || (value != "." && value != "_" && token.IsIdentifier(value))
}
func safePackageClause(value string) bool { return value != "_" && token.IsIdentifier(value) }
func safeOwner(value string) bool         { return safeLabel(value) }
func safeLabel(value string) bool {
	return value != "" && !hasForbidden(value) && !strings.ContainsAny(value, " /\\;|&`$\n\r\t")
}
func safeToolVersion(value string) bool {
	return value != "" && !hasForbidden(value) && !containsControl(value) && !strings.ContainsAny(value, " \\;|&`$")
}
func hasForbidden(value string) bool { return strings.ContainsAny(value, "*?[]{}\x00") }
func containsControl(value string) bool {
	return strings.ContainsAny(value, "\n\r\t")
}

func validatePosition(value Position) error {
	if !safePath(value.Path) || value.Offset < 0 || value.Line < 1 || value.Column < 1 {
		return fmt.Errorf("invalid source position")
	}
	if (len(value.PackageIDs) == 0) != (value.Role == "") {
		return fmt.Errorf("semantic site must bind both package variant and role")
	}
	if len(value.PackageIDs) != 0 && !knownSiteRole(value.Role) {
		return fmt.Errorf("invalid semantic site identity")
	}
	for index, packageID := range value.PackageIDs {
		if containsControl(packageID) || strings.Contains(packageID, "\x00") || packageID == "" {
			return fmt.Errorf("invalid semantic package variant")
		}
		if index != 0 && value.PackageIDs[index-1] >= packageID {
			return fmt.Errorf("semantic package variants must be sorted and unique")
		}
	}
	return nil
}

func validateSemanticSite(value Position) error {
	if err := validatePosition(value); err != nil {
		return err
	}
	if len(value.PackageIDs) == 0 || !knownSiteRole(value.Role) {
		return fmt.Errorf("semantic site is missing package variant or role")
	}
	return nil
}

func knownSiteRole(value SiteRole) bool {
	switch value {
	case SiteDeclaration, SiteUse, SiteSelector, SiteImport:
		return true
	default:
		return false
	}
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func sameStrings(left, right []string) bool {
	return strings.Join(sorted(left), "\x00") == strings.Join(sorted(right), "\x00")
}

func symbolListKey(values []SymbolRef) string {
	objects := make([]string, 0, len(values))
	for _, value := range values {
		objects = append(objects, value.Object)
	}
	return strings.Join(sorted(objects), "\x00")
}
