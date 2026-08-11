package generator

// This file is the sealed Run path.  It deliberately has one interpretation
// boundary: the scanner supplies facts once, the four authored files parse
// once, population validates once, and only that proven snapshot is rendered.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var ErrWriteForbidden = errors.New("link ownership: final mode cannot write generated output")

// generatorSourceFiles is intentionally closed and ordered.  Any change to a
// source file that can affect scan, validation, digesting, or rendering makes
// the checked-in artifact stale; tests are not generator inputs.
var generatorSourceFiles = [...]string{
	"program/internal/schema/linkownership/cmd/main.go",
	"program/internal/schema/linkownership/doc.go",
	"program/internal/schema/linkownership/generator/declarations.go",
	"program/internal/schema/linkownership/generator/gate.go",
	"program/internal/schema/linkownership/generator/population.go",
	"program/internal/schema/linkownership/generator/projections.go",
	"program/internal/schema/linkownership/generator/run.go",
	"program/internal/schema/linkownership/generator/scanner.go",
	"program/internal/schema/linkownership/generator/structure_projection.go",
	"program/internal/schema/linkownership/generator/typed_schema.go",
	"program/internal/schema/linkownership/generator/typewalk.go",
}

// Run seals the cold ownership ledger. A manifest omission, parse failure,
// population mismatch, changed source, or stale generated artifact is an
// error; there is no fallback owner table or partial generated authority.
func Run(root string, mode Mode, write bool) (Report, error) {
	if mode != ModeInventory && mode != ModeFinal {
		return Report{Mode: mode}, ErrInvalidMode
	}
	if write && mode != ModeInventory {
		return Report{Mode: mode}, ErrWriteForbidden
	}
	canonical, err := canonicalRoot(root)
	if err != nil {
		return Report{Mode: mode}, err
	}
	scan, err := scanFamilyAtCanonicalRoot(canonical, linkImportPath)
	if err != nil {
		return Report{Mode: mode}, err
	}
	return runWithScan(canonical, mode, write, scan)
}

// runWithScan is the post-scan sealing boundary. Keeping it separate makes
// the no-bypass law testable without giving tests (or callers) an alternate
// exported gate; Run remains the sole production entry point.
func runWithScan(canonical string, mode Mode, write bool, scan ScanResult) (Report, error) {
	if mode != ModeInventory && mode != ModeFinal {
		return Report{Mode: mode}, ErrInvalidMode
	}
	if write && mode != ModeInventory {
		return Report{Mode: mode}, ErrWriteForbidden
	}
	report := Report{Mode: mode, Scan: scan}
	files, missing, err := readManifestFiles(canonical)
	if err != nil {
		return report, err
	}
	if len(missing) != 0 {
		report.FinalBlockers = append(report.FinalBlockers, "manifest missing: "+fmt.Sprint(missing))
		return report, fmt.Errorf("%w: %v", ErrManifestMissing, missing)
	}
	report.ManifestPresent = true
	manifests, err := ParseManifestFiles(files)
	if err != nil {
		return report, err
	}
	if err := validateManifestPortableText(canonical, scan.Build.Context, manifests); err != nil {
		return report, err
	}
	if err := ValidateManifestPopulation(scan, manifests); err != nil {
		return report, err
	}
	for _, row := range manifests.Residue.Rows {
		report.FinalBlockers = append(report.FinalBlockers, "residue: "+row.Kind+" "+row.CurrentFact+" -> "+row.Destination)
	}
	for _, plan := range manifests.Residue.SplitPlans {
		report.FinalBlockers = append(report.FinalBlockers, "non-evidentiary residue split-plan: "+plan.ID)
	}
	planCount := len(manifests.Indexes.IndexPlans) + len(manifests.Indexes.ReferencePlans) + len(manifests.Indexes.IdentityPlans)
	for _, row := range manifests.Indexes.IndexPlans {
		report.FinalBlockers = append(report.FinalBlockers, "non-evidentiary inventory plan: index-plan "+row.ID)
	}
	for _, row := range manifests.Indexes.ReferencePlans {
		report.FinalBlockers = append(report.FinalBlockers, "non-evidentiary inventory plan: reference-plan "+row.ID)
	}
	for _, row := range manifests.Indexes.IdentityPlans {
		report.FinalBlockers = append(report.FinalBlockers, "non-evidentiary inventory plan: identity-plan "+row.ID)
	}
	if mode == ModeFinal && (len(manifests.Residue.Rows) != 0 || len(manifests.Residue.SplitPlans) != 0 || planCount != 0) {
		return report, ErrFinalBlocked
	}
	sources, err := generatorSourceSnapshot(canonical)
	if err != nil {
		return report, err
	}
	digest, err := generatedInputDigestFromSnapshot(scan, manifests, sources)
	if err != nil {
		return report, err
	}
	expected, err := renderGenerated(manifests, digest)
	if err != nil {
		return report, err
	}
	actual, readErr := readRegularFileAt(canonical, filepath.ToSlash(filepath.Join(schemaDir, generatedFile)), true)
	if readErr == nil && bytes.Equal(actual, expected) {
		if err := verifySealedInputs(canonical, scan, manifests, sources); err != nil {
			return report, err
		}
		report.GeneratedFresh = true
		return report, nil
	}
	if write {
		if readErr != nil && !os.IsNotExist(readErr) {
			return report, readErr
		}
		if err := verifySealedInputs(canonical, scan, manifests, sources); err != nil {
			return report, err
		}
		if err := writeGeneratedAtomically(canonical, filepath.ToSlash(filepath.Join(schemaDir, generatedFile)), expected); err != nil {
			return report, err
		}
		actual, err := readRegularFileAt(canonical, filepath.ToSlash(filepath.Join(schemaDir, generatedFile)), false)
		if err != nil {
			return report, err
		}
		if !bytes.Equal(actual, expected) {
			return report, fmt.Errorf("%w: generated readback differs after atomic write", ErrManifestStale)
		}
		report.GeneratedFresh = true
		return report, nil
	}
	report.FinalBlockers = append(report.FinalBlockers, "generated ownership artifact is stale")
	if readErr != nil && !os.IsNotExist(readErr) {
		return report, readErr
	}
	return report, ErrManifestStale
}

// The workspace is trusted, but normal concurrent edits are expected. We do
// not claim to defeat an active swap-back attacker: immediately before an
// accept/write we re-read the complete cold input snapshot and fail on drift.
// Static component/leaf checks plus the repeated snapshots make ordinary
// concurrent changes fail closed without an OS-specific secure-filesystem
// subsystem.
func verifySealedInputs(root string, scan ScanResult, manifests ManifestSet, sources []generatorSourceDigest) error {
	files, missing, err := readManifestFiles(root)
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return fmt.Errorf("%w: manifest changed or missing: %v", ErrManifestStale, missing)
	}
	want := manifests.CanonicalFiles()
	if !bytes.Equal(files.Catalog, want.Catalog) || !bytes.Equal(files.Indexes, want.Indexes) || !bytes.Equal(files.Surfaces, want.Surfaces) || !bytes.Equal(files.Residue, want.Residue) {
		return fmt.Errorf("%w: manifest changed during sealing", ErrManifestStale)
	}
	currentSources, err := generatorSourceSnapshot(root)
	if err != nil {
		return err
	}
	if len(currentSources) != len(sources) {
		return fmt.Errorf("%w: generator source closure changed during sealing", ErrManifestStale)
	}
	for index := range sources {
		if currentSources[index] != sources[index] {
			return fmt.Errorf("%w: generator source changed during sealing", ErrManifestStale)
		}
	}
	currentDigest, err := productionManifestDigest(root, scan.Sources.ProductionSources)
	if err != nil {
		return err
	}
	if currentDigest != scan.Build.SourceDigest {
		return fmt.Errorf("%w: scanned production source changed during sealing", ErrManifestStale)
	}
	return nil
}

func validateManifestPortableText(root string, context BuildContext, manifests ManifestSet) error {
	prefixes := []string{root, context.home, context.goCache, context.goModCache, context.goPath, context.goRoot, context.goExecutable}
	check := func(label, value string) error {
		for _, prefix := range prefixes {
			if prefix != "" && strings.Contains(value, filepath.ToSlash(prefix)) {
				return fmt.Errorf("link ownership: %s contains private absolute path", label)
			}
		}
		for _, token := range strings.Fields(value) {
			token = strings.Trim(token, "()[]{},;\"'")
			if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "\\") || hasWindowsVolumePath(token) {
				return fmt.Errorf("link ownership: %s contains non-portable absolute path", label)
			}
		}
		if containsEmbeddedAbsolutePath(value) {
			return fmt.Errorf("link ownership: %s contains non-portable absolute path", label)
		}
		return nil
	}
	checkValues := func(label string, values ...string) error {
		for _, value := range values {
			if err := check(label, value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, row := range manifests.Catalog.Owners {
		if err := checkValues("owner", row.ID, row.PackagePath, row.Surface, row.Kind); err != nil {
			return err
		}
	}
	for _, row := range manifests.Catalog.Declarations {
		if err := checkValues("declaration", row.FactID, row.PackagePath, row.Kind, row.Owner, row.Surface, row.Name, row.Type, row.Signature); err != nil {
			return err
		}
	}
	for _, row := range manifests.Catalog.Uses {
		if err := checkValues("use", row.FactID, row.PackagePath, row.SourceFile, row.Symbol, row.Evidence, string(row.Role), row.TargetDeclID, row.Type); err != nil {
			return err
		}
	}
	for _, row := range manifests.Catalog.ImportEdges {
		if err := checkValues("ownership import edge", row.FromOwner, row.ToOwner, row.SourceFile); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.Indexes {
		values := []string{row.ID, row.Owner, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest}
		values = append(values, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("index", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.HotReferences {
		values := []string{row.ID, row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest}
		values = append(values, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("hot reference", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ColdReferences {
		values := []string{row.ID, row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest}
		values = append(values, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("cold reference", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ContextualReferences {
		values := []string{row.ID, row.Issuer, row.Consumer, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest}
		values = append(values, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("contextual reference", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.Identities {
		values := []string{row.ID, row.Owner, row.DeclarationFactID, string(row.RelationKind), row.PatternID}
		values = append(values, row.DirectFactIDs...)
		values = append(values, row.ParentIdentityIDs...)
		if err := checkValues("identity", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.IndexPlans {
		values := append([]string{"index-plan", row.ID, row.Owner, row.DeclarationFactID}, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("inventory plan", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.ReferencePlans {
		values := append([]string{"reference-plan", row.ID, row.Issuer, row.Consumer, row.DeclarationFactID}, row.SourceFactIDs...)
		values = append(values, row.CallerUseFactIDs...)
		if err := checkValues("inventory plan", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Indexes.IdentityPlans {
		values := append([]string{"identity-plan", row.ID, row.Owner, row.DeclarationFactID, string(row.RelationKind)}, row.DirectFactIDs...)
		values = append(values, row.ParentIdentityIDs...)
		if err := checkValues("inventory plan", values...); err != nil {
			return err
		}
	}
	for _, row := range manifests.Surfaces.Assignments {
		if err := checkValues("surface assignment", row.Kind, row.FactID, row.OwnerSurface, row.Name); err != nil {
			return err
		}
	}
	for _, row := range manifests.Surfaces.Storage {
		if err := checkValues("storage", row.FactID, row.OwnerSurface, string(row.Disposition)); err != nil {
			return err
		}
	}
	for _, row := range manifests.Residue.Rows {
		if err := checkValues("residue", row.Kind, row.CurrentFact, row.Destination); err != nil {
			return err
		}
	}
	for _, plan := range manifests.Residue.SplitPlans {
		values := append([]string{"split-plan", plan.ID}, plan.OwnerIDs...)
		if err := checkValues("split plan", values...); err != nil {
			return err
		}
	}
	return nil
}

func containsEmbeddedAbsolutePath(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) && (index == 0 || manifestPathDelimiter(value[index-1])) {
			return true
		}
		if value[index] == '/' && index+1 < len(value) && value[index+1] != '/' && value[index+1] != ' ' && (index == 0 || manifestPathDelimiter(value[index-1])) {
			return true
		}
		if index+2 < len(value) && ((value[index] >= 'A' && value[index] <= 'Z') || (value[index] >= 'a' && value[index] <= 'z')) && value[index+1] == ':' && (value[index+2] == '/' || value[index+2] == '\\') && (index == 0 || manifestPathDelimiter(value[index-1])) {
			return true
		}
	}
	return false
}

func manifestPathDelimiter(value byte) bool {
	return value == ' ' || value == ':' || value == '=' || value == '(' || value == '[' || value == '{' || value == ',' || value == ';' || value == '"' || value == '\''
}

func readManifestFiles(root string) (ManifestFiles, []string, error) {
	var files ManifestFiles
	entries := []struct {
		name string
		put  func([]byte)
	}{
		{catalogFile, func(value []byte) { files.Catalog = value }},
		{indexesFile, func(value []byte) { files.Indexes = value }},
		{surfacesFile, func(value []byte) { files.Surfaces = value }},
		{residueFile, func(value []byte) { files.Residue = value }},
	}
	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		value, err := readRegularFileAt(root, filepath.ToSlash(filepath.Join(schemaDir, entry.name)), true)
		if os.IsNotExist(err) {
			missing = append(missing, entry.name)
			continue
		}
		if err != nil {
			return ManifestFiles{}, nil, err
		}
		entry.put(value)
	}
	return files, missing, nil
}

// readRegularFileAt accepts one repository-relative regular file only.  It
// checks every path component with Lstat so neither a manifest nor a
// generator source can be redirected through a symlink authority.
func readRegularFileAt(root, relative string, allowMissing bool) ([]byte, error) {
	path, err := checkedRegularPath(root, relative, allowMissing)
	if err != nil || path == "" {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("link ownership: %s is not a regular file", relative)
	}
	return io.ReadAll(file)
}

// checkedRegularPath performs a static component/leaf no-symlink check. The
// workspace is trusted; Run snapshots all inputs again immediately before it
// accepts or writes, so ordinary concurrent edits fail closed.
func checkedRegularPath(root, relative string, allowMissing bool) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("link ownership: invalid relative file path %q", relative)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("link ownership: invalid relative file path %q", relative)
	}
	current := root
	parts := strings.Split(clean, string(filepath.Separator))
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("link ownership: invalid relative file path %q", relative)
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissing && index == len(parts)-1 {
				return "", err
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("link ownership: %s contains forbidden symlink %s", relative, current)
		}
		if index != len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("link ownership: %s has non-directory component %s", relative, current)
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return "", fmt.Errorf("link ownership: %s is not a regular file", relative)
		}
	}
	return current, nil
}

func writeGeneratedAtomically(root, relative string, expected []byte) error {
	path, err := checkedRegularPath(root, relative, true)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if path == "" {
		path = filepath.Join(root, filepath.FromSlash(relative))
	}
	dir := filepath.Dir(path)
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 || !dirInfo.IsDir() {
		return fmt.Errorf("link ownership: generated output directory is not a real directory")
	}
	temp, err := os.CreateTemp(dir, ".generated.go-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.Write(expected); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	keep = true
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// digestFrame is a length-prefixed, unambiguous commitment encoder.  It
// keeps every input plane explicit, avoids reflection/default serialization,
// and never writes host-private loader paths into the artifact identity.
type generatorSourceDigest struct{ path, digest string }

func generatedInputDigest(root string, scan ScanResult, manifests ManifestSet) (string, error) {
	sources, err := generatorSourceSnapshot(root)
	if err != nil {
		return "", err
	}
	return generatedInputDigestFromSnapshot(scan, manifests, sources)
}

func generatedInputDigestFromSnapshot(scan ScanResult, manifests ManifestSet, sources []generatorSourceDigest) (string, error) {
	hash := sha256.New()
	write := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	write("link-ownership-generated-input-v2")
	writeScanDigestPlane(write, scan)
	files := manifests.CanonicalFiles()
	write("catalog.schema")
	write(string(files.Catalog))
	write("indexes.schema")
	write(string(files.Indexes))
	write("surfaces.schema")
	write(string(files.Surfaces))
	write("residue.schema")
	write(string(files.Residue))
	write("generator-source-files-v2")
	for _, source := range sources {
		write(source.path)
		write(source.digest)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func generatorSourceSnapshot(root string) ([]generatorSourceDigest, error) {
	if err := validateGeneratorSourceClosure(root); err != nil {
		return nil, err
	}
	result := make([]generatorSourceDigest, 0, len(generatorSourceFiles))
	for _, relative := range generatorSourceFiles {
		data, err := readRegularFileAt(root, relative, false)
		if err != nil {
			return nil, fmt.Errorf("link ownership: hash generator source %s: %w", relative, err)
		}
		sum := sha256.Sum256(data)
		result = append(result, generatorSourceDigest{relative, hex.EncodeToString(sum[:])})
	}
	return result, nil
}

func validateGeneratorSourceClosure(root string) error {
	expected := make([]string, len(generatorSourceFiles))
	copy(expected, generatorSourceFiles[:])
	if !sort.StringsAreSorted(expected) {
		return fmt.Errorf("link ownership: generator source list is not sorted")
	}
	seen := make(map[string]struct{}, len(expected))
	for _, relative := range expected {
		if _, exists := seen[relative]; exists {
			return fmt.Errorf("link ownership: generator source list duplicates %s", relative)
		}
		seen[relative] = struct{}{}
		if _, err := checkedRegularPath(root, relative, false); err != nil {
			return fmt.Errorf("link ownership: invalid listed generator source %s: %w", relative, err)
		}
	}
	actual := make([]string, 0, len(expected))
	rootDir := filepath.Join(root, schemaDir)
	err := filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("link ownership: generator source closure contains forbidden symlink %s", relative)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("link ownership: generator source closure has non-regular input %s", relative)
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || relative == filepath.ToSlash(filepath.Join(schemaDir, generatedFile)) {
			return nil
		}
		parent := filepath.ToSlash(filepath.Dir(relative))
		if parent == schemaDir || strings.HasPrefix(parent, schemaDir+"/generator") || strings.HasPrefix(parent, schemaDir+"/cmd") {
			actual = append(actual, relative)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(actual)
	if len(actual) != len(expected) {
		return fmt.Errorf("link ownership: generator source closure differs: got %v want %v", actual, expected)
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return fmt.Errorf("link ownership: generator source closure differs: got %v want %v", actual, expected)
		}
	}
	return nil
}

func writeScanDigestPlane(write func(string), scan ScanResult) {
	write("scan-result-v1")
	write(scan.Root.PackagePath)
	write(scan.Root.SourceDir)
	writeBool(write, scan.ProductionOnly)
	write("source-digest")
	write(hex.EncodeToString(scan.Build.SourceDigest[:]))
	write("build-fingerprint")
	write(scan.Build.Fingerprint)
	writeBuildContext(write, scan.Build.Context)
	write("packages")
	writeInt(write, len(scan.Sources.Packages))
	for _, row := range scan.Sources.Packages {
		write(row.Path)
		write(row.Name)
		write(row.Directory)
	}
	write("production-sources")
	writeInt(write, len(scan.Sources.ProductionSources))
	for _, row := range scan.Sources.ProductionSources {
		write(row.PackagePath)
		write(row.Path)
	}
	write("declarations")
	writeInt(write, len(scan.Types.Declarations))
	for _, row := range scan.Types.Declarations {
		writeDeclarationDigestRow(write, row)
	}
	write("exposures")
	writeInt(write, len(scan.Types.Exposures))
	for _, row := range scan.Types.Exposures {
		write(row.FactID)
		write(row.PackagePath)
		write(row.RootType)
		write(row.Surface)
		write(row.Set)
		write(row.Name)
		write(row.Signature)
		write(row.TargetDeclID)
		write(row.Disposition)
	}
	write("surfaces")
	writeInt(write, len(scan.Types.Surfaces))
	for _, row := range scan.Types.Surfaces {
		write(row.FactID)
		write(row.PackagePath)
		write(row.RootType)
		write(row.Surface)
		write(row.ParentSurface)
		write(row.Path)
		write(row.Kind)
		write(row.Type)
		write(row.SourceFile)
		writeInt(write, row.Line)
		writeInt(write, row.Column)
		write(row.OriginDeclID)
	}
	writeStructureDigestPlane(write, scan.Types.Structure)
	write("import-edges")
	writeInt(write, len(scan.Dependencies.ImportEdges))
	for _, row := range scan.Dependencies.ImportEdges {
		write(row.From)
		write(row.To)
		write(row.SourceFile)
		writeInt(write, row.Line)
		writeInt(write, row.Column)
	}
	write("modules")
	writeInt(write, len(scan.Dependencies.Modules))
	for _, row := range scan.Dependencies.Modules {
		write(row.Path)
		write(row.Version)
		write(row.Sum)
		write(row.GoMod)
		write(row.ResolvedPath)
		write(row.ResolvedVersion)
		write(row.ResolvedSum)
		write(row.ResolvedGoMod)
		write(row.ResolvedContentDigest)
	}
	write("uses")
	writeInt(write, len(scan.Uses))
	for _, row := range scan.Uses {
		write(row.PackagePath)
		write(row.SourceFile)
		writeInt(write, row.Line)
		writeInt(write, row.Column)
		write(row.Symbol)
		write(row.Evidence)
		write(row.Type)
		write(row.TargetDeclID)
		write(string(row.Role))
		writeInt(write, len(row.AliasChain))
		for _, alias := range row.AliasChain {
			write(alias)
		}
		write(row.FactID)
	}
}

func writeBuildContext(write func(string), row BuildContext) {
	write("build-context-v1")
	for _, value := range []string{row.GOWORK, row.GOENV, row.GOTOOLCHAIN, row.GOOS, row.GOARCH, row.CGOEnabled, row.GOFLAGS, row.BuildTags, row.GOEXPERIMENT, row.GODEBUG, row.GOAMD64, row.GOARM64, row.GOARM, row.GO386, row.GOMIPS, row.GOMIPS64, row.GOPPC64, row.GORISCV64, row.GOWASM, row.GO111MODULE, row.GOPROXY, row.GOSUMDB, row.Toolchain, row.GoExecutableDigest, row.CCompilerDigest, row.CXXCompilerDigest, row.ARCompilerDigest} {
		write(value)
	}
}

func writeDeclarationDigestRow(write func(string), row DeclarationInfo) {
	for _, value := range []string{row.FactID, row.PackagePath, row.Kind, row.OwnerType, row.SyntheticPath, row.Surface, row.Path, row.Name, row.Type, row.Signature, row.AliasRHS, row.AliasTargetDeclID, row.SourceFile} {
		write(value)
	}
	writeInt(write, row.Line)
	writeInt(write, row.Column)
	writeBool(write, row.Exported)
}

func writeStructureDigestPlane(write func(string), value StructureProjection) {
	write("structure-v1")
	write("fields")
	writeInt(write, len(value.Fields))
	for _, row := range value.Fields {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		writeBool(write, row.Embedded)
	}
	write("arrays")
	writeInt(write, len(value.Arrays))
	for _, row := range value.Arrays {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.Element)
		write(strconv.FormatInt(row.Length, 10))
	}
	write("slices")
	writeInt(write, len(value.Slices))
	for _, row := range value.Slices {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.Element)
	}
	write("maps")
	writeInt(write, len(value.Maps))
	for _, row := range value.Maps {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.Key)
		write(row.Value)
	}
	write("channels")
	writeInt(write, len(value.Channels))
	for _, row := range value.Channels {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.Element)
		write(row.Direction)
	}
	write("named-references")
	writeInt(write, len(value.NamedReferences))
	for _, row := range value.NamedReferences {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.TargetDeclID)
		write(row.TargetPackagePath)
		write(row.TargetName)
		writeBool(write, row.Origin)
	}
	write("method-references")
	writeInt(write, len(value.MethodReferences))
	for _, row := range value.MethodReferences {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.TargetDeclID)
		write(row.TargetPackagePath)
		write(row.TargetName)
		write(row.MethodKey)
		write(row.Type)
		write(row.Receiver)
	}
	write("other-references")
	writeInt(write, len(value.OtherReferences))
	for _, row := range value.OtherReferences {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(strconv.Itoa(int(row.Disposition)))
		write(row.Type)
	}
	write("cycles")
	writeInt(write, len(value.Cycles))
	for _, row := range value.Cycles {
		write(row.FactID)
		write(row.DeclarationID)
		write(row.SurfaceID)
		write(row.Path)
		write(row.Type)
	}
}

func writeInt(write func(string), value int)   { write(strconv.Itoa(value)) }
func writeBool(write func(string), value bool) { write(strconv.FormatBool(value)) }

// renderGenerated emits only typed records and fixed arrays. It is a cold
// audit artifact: no generated map, registration hook, or runtime dispatch
// surface is introduced into the Link analyzer.
func renderGenerated(manifests ManifestSet, digest string) ([]byte, error) {
	if len(digest) != sha256.Size*2 {
		return nil, fmt.Errorf("link ownership: invalid generated input digest")
	}
	var out bytes.Buffer
	out.WriteString("// Code generated by linkownership; DO NOT EDIT.\n\npackage linkownership\n\n")
	out.WriteString("const generatedInputDigest = " + strconv.Quote(digest) + "\n\n")
	out.WriteString("type generatedOwner struct { id, packagePath, surface, kind string }\n")
	out.WriteString("type generatedDeclaration struct { factID, packagePath, kind, owner, surface, name, typ, signature string }\n")
	out.WriteString("type generatedUse struct { factID, packagePath, sourceFile, symbol, evidence, targetDeclID, typ, role string; line, column int }\n")
	out.WriteString("type generatedImportEdge struct { fromOwner, toOwner, sourceFile string; line, column int }\n")
	out.WriteString("type generatedIndex struct { id, owner, queryFactID, patternID, benchmarkReceiptDigest string; callersStart, callersEnd, sourceStart, sourceEnd int }\n")
	out.WriteString("type generatedReference struct { id, issuer, consumer, queryFactID, patternID, benchmarkReceiptDigest string; callersStart, callersEnd, sourceStart, sourceEnd int }\n")
	out.WriteString("type generatedIdentity struct { id, owner, declarationFactID, relationKind, patternID string; directStart, directEnd, parentStart, parentEnd int; computedDigest string }\n")
	out.WriteString("type generatedSurface struct { kind, factID, ownerSurface, name string }\n")
	out.WriteString("type generatedStorage struct { factID, ownerSurface, disposition string }\n")
	out.WriteString("type generatedResidue struct { kind, currentFact, destination string }\n\n")
	writeOwners(&out, manifests.Catalog.Owners)
	writeDeclarations(&out, manifests.Catalog.Declarations)
	writeUses(&out, manifests.Catalog.Uses)
	writeImportEdges(&out, manifests.Catalog.ImportEdges)
	writeIndexes(&out, manifests.Indexes.Indexes)
	writeHotReferences(&out, "generatedHotReferences", manifests.Indexes.HotReferences)
	writeColdReferences(&out, "generatedColdReferences", manifests.Indexes.ColdReferences)
	writeContextualReferences(&out, "generatedContextualReferences", manifests.Indexes.ContextualReferences)
	if err := writeIdentities(&out, manifests.Indexes.Identities); err != nil {
		return nil, err
	}
	writeSurfaces(&out, manifests.Surfaces.Assignments)
	writeStorages(&out, manifests.Surfaces.Storage)
	writeResidues(&out, manifests.Residue.Rows)
	return out.Bytes(), nil
}

func quoted(values ...string) string {
	result := ""
	for i, value := range values {
		if i != 0 {
			result += ", "
		}
		result += strconv.Quote(value)
	}
	return result
}
func writeOwners(out *bytes.Buffer, rows []OwnerRow) {
	fmt.Fprintf(out, "var generatedOwners = [...]generatedOwner{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s},\n", quoted(row.ID, row.PackagePath, row.Surface, row.Kind))
	}
	out.WriteString("}\n\n")
}
func writeDeclarations(out *bytes.Buffer, rows []DeclarationRow) {
	out.WriteString("var generatedDeclarations = [...]generatedDeclaration{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s},\n", quoted(row.FactID, row.PackagePath, row.Kind, row.Owner, row.Surface, row.Name, row.Type, row.Signature))
	}
	out.WriteString("}\n\n")
}
func writeUses(out *bytes.Buffer, rows []UseRow) {
	out.WriteString("var generatedUses = [...]generatedUse{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s, %d, %d},\n", quoted(row.FactID, row.PackagePath, row.SourceFile, row.Symbol, row.Evidence, row.TargetDeclID, row.Type, string(row.Role)), row.Line, row.Column)
	}
	out.WriteString("}\n\n")
}
func writeImportEdges(out *bytes.Buffer, rows []OwnershipImportEdgeRow) {
	out.WriteString("var generatedImportEdges = [...]generatedImportEdge{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s, %d, %d},\n", quoted(row.FromOwner, row.ToOwner, row.SourceFile), row.Line, row.Column)
	}
	out.WriteString("}\n\n")
}
func writeIndexes(out *bytes.Buffer, rows []IndexRow) {
	callers := make([]string, 0)
	sources := make([]string, 0)
	out.WriteString("var generatedIndexes = [...]generatedIndex{\n")
	for _, row := range rows {
		callerStart := len(callers)
		callers = append(callers, row.CallerUseFactIDs...)
		sourceStart := len(sources)
		sources = append(sources, row.SourceFactIDs...)
		fmt.Fprintf(out, "\t{%s, %d, %d, %d, %d},\n", quoted(row.ID, row.Owner, row.QueryFactID, row.PatternID, row.BenchmarkReceiptDigest), callerStart, len(callers), sourceStart, len(sources))
	}
	out.WriteString("}\n\n")
	writeStrings(out, "generatedIndexCallerUseFacts", callers)
	writeStrings(out, "generatedIndexSources", sources)
}

type generatedReferenceInput struct {
	id, issuer, consumer, queryFactID, patternID, benchmarkReceiptDigest string
	callers, sourceFactIDs                                               []string
}

func writeHotReferences(out *bytes.Buffer, name string, rows []HotReferenceRow) {
	inputs := make([]generatedReferenceInput, len(rows))
	for i, row := range rows {
		inputs[i] = generatedReferenceInput{id: row.ID, issuer: row.Issuer, consumer: row.Consumer, queryFactID: row.QueryFactID, patternID: row.PatternID, benchmarkReceiptDigest: row.BenchmarkReceiptDigest, callers: row.CallerUseFactIDs, sourceFactIDs: row.SourceFactIDs}
	}
	writeReferences(out, name, inputs)
}
func writeColdReferences(out *bytes.Buffer, name string, rows []ColdReferenceRow) {
	inputs := make([]generatedReferenceInput, len(rows))
	for i, row := range rows {
		inputs[i] = generatedReferenceInput{id: row.ID, issuer: row.Issuer, consumer: row.Consumer, queryFactID: row.QueryFactID, patternID: row.PatternID, benchmarkReceiptDigest: row.BenchmarkReceiptDigest, callers: row.CallerUseFactIDs, sourceFactIDs: row.SourceFactIDs}
	}
	writeReferences(out, name, inputs)
}
func writeContextualReferences(out *bytes.Buffer, name string, rows []ContextualReferenceRow) {
	inputs := make([]generatedReferenceInput, len(rows))
	for i, row := range rows {
		inputs[i] = generatedReferenceInput{id: row.ID, issuer: row.Issuer, consumer: row.Consumer, queryFactID: row.QueryFactID, patternID: row.PatternID, benchmarkReceiptDigest: row.BenchmarkReceiptDigest, callers: row.CallerUseFactIDs, sourceFactIDs: row.SourceFactIDs}
	}
	writeReferences(out, name, inputs)
}
func writeReferences(out *bytes.Buffer, name string, rows []generatedReferenceInput) {
	callers := make([]string, 0)
	sources := make([]string, 0)
	fmt.Fprintf(out, "var %s = [...]generatedReference{\n", name)
	for _, row := range rows {
		start := len(callers)
		callers = append(callers, row.callers...)
		sourceStart := len(sources)
		sources = append(sources, row.sourceFactIDs...)
		fmt.Fprintf(out, "\t{%s, %d, %d, %d, %d},\n", quoted(row.id, row.issuer, row.consumer, row.queryFactID, row.patternID, row.benchmarkReceiptDigest), start, len(callers), sourceStart, len(sources))
	}
	out.WriteString("}\n\n")
	writeStrings(out, name+"CallerUseFacts", callers)
	writeStrings(out, name+"Sources", sources)
}
func writeIdentities(out *bytes.Buffer, rows []IdentityRow) error {
	directFacts := make([]string, 0)
	parents := make([]string, 0)
	out.WriteString("var generatedIdentities = [...]generatedIdentity{\n")
	for _, row := range rows {
		start := len(directFacts)
		directFacts = append(directFacts, row.DirectFactIDs...)
		parentStart := len(parents)
		parents = append(parents, row.ParentIdentityIDs...)
		computed, err := identityDigest(row, rows)
		if err != nil {
			return fmt.Errorf("identity %q digest: %w", row.ID, err)
		}
		fmt.Fprintf(out, "\t{%s, %d, %d, %d, %d, %s},\n", quoted(row.ID, row.Owner, row.DeclarationFactID, string(row.RelationKind), row.PatternID), start, len(directFacts), parentStart, len(parents), strconv.Quote(computed))
	}
	out.WriteString("}\n\n")
	writeStrings(out, "generatedIdentityDirectFacts", directFacts)
	writeStrings(out, "generatedIdentityParents", parents)
	return nil
}
func writeStrings(out *bytes.Buffer, name string, values []string) {
	fmt.Fprintf(out, "var %s = [...]string{\n", name)
	for _, value := range values {
		fmt.Fprintf(out, "\t%s,\n", strconv.Quote(value))
	}
	out.WriteString("}\n\n")
}
func writeSurfaces(out *bytes.Buffer, rows []SurfaceAssignmentRow) {
	out.WriteString("var generatedSurfaces = [...]generatedSurface{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s},\n", quoted(row.Kind, row.FactID, row.OwnerSurface, row.Name))
	}
	out.WriteString("}\n\n")
}
func writeStorages(out *bytes.Buffer, rows []StorageRow) {
	out.WriteString("var generatedStorages = [...]generatedStorage{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "\t{%s},\n", quoted(row.FactID, row.OwnerSurface, string(row.Disposition)))
	}
	out.WriteString("}\n\n")
}
func writeResidues(out *bytes.Buffer, rows []ResidueRow) {
	out.WriteString("var generatedResidues = [...]generatedResidue{\n")
	for _, row := range rows {
		if row.Kind == "split" {
			// Split rows resolve through migration-only split plans. Do not emit
			// a dangling plan ID into the live generated artifact.
			continue
		}
		fmt.Fprintf(out, "\t{%s},\n", quoted(row.Kind, row.CurrentFact, row.Destination))
	}
	out.WriteString("}\n")
}
