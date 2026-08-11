package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/verify"
)

func verifyGates(intent cutplan.Intent, source, target semantic.Snapshot) ([]cutplan.GateEvidence, error) {
	if err := verifyDeclaredImportEvidence(intent, source.Structure, target.Structure); err != nil {
		return nil, fmt.Errorf("typed import gate: %w", err)
	}
	before, err := sourceMap(source)
	if err != nil {
		return nil, fmt.Errorf("source map before cut: %w", err)
	}
	after, err := sourceMap(target)
	if err != nil {
		return nil, fmt.Errorf("source map after cut: %w", err)
	}
	required, err := cutplan.GateRequirements(intent)
	if err != nil {
		return nil, err
	}
	semanticEvidence := map[cutplan.Gate]string{}
	for _, gate := range required {
		switch gate {
		case cutplan.GateDiagnostics:
			delta, verifyErr := semantic.VerifyDiagnosticDelta(source.Diagnostics, target.Diagnostics, nil, nil)
			if verifyErr != nil {
				return nil, fmt.Errorf("diagnostic gate: %w", verifyErr)
			}
			digest, digestErr := digestEvidence(delta)
			if digestErr != nil {
				return nil, digestErr
			}
			semanticEvidence[gate] = digest
		case cutplan.GateResidue:
			queries, queryErr := residueQueries(intent, source)
			if queryErr != nil {
				return nil, queryErr
			}
			residues, residueErr := target.Residues(queries)
			if residueErr != nil {
				return nil, fmt.Errorf("residue gate: %w", residueErr)
			}
			for _, residue := range residues {
				if len(residue.Sites) != 0 {
					return nil, fmt.Errorf("residue gate: %s remains at %s", residue.Object.Object, residue.Sites[0].Path)
				}
			}
			digest, digestErr := digestEvidence(residues)
			if digestErr != nil {
				return nil, digestErr
			}
			semanticEvidence[gate] = digest
		}
	}
	dispositions := make([]verify.GateDisposition, 0, len(required))
	for _, gate := range required {
		disposition := verify.GateDisposition{Gate: gate}
		if digest, external := semanticEvidence[gate]; external {
			disposition.External = &verify.ExternalEvidence{Kind: gate, Passed: true, Digest: digest}
		}
		dispositions = append(dispositions, disposition)
	}
	report, err := verify.Verify(verify.Request{
		Before:  verify.Snapshot{Sources: before},
		After:   verify.Snapshot{Sources: after},
		Imports: allImports(intent), RequestedGates: required, Dispositions: dispositions,
	})
	if err != nil {
		return nil, err
	}
	result := make([]cutplan.GateEvidence, 0, len(report.Evidence))
	for _, evidence := range report.Evidence {
		result = append(result, cutplan.GateEvidence{Gate: evidence.Gate, ResultSHA256: evidence.Digest})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Gate < result[j].Gate })
	return result, nil
}

// verifyDeclaredImportEvidence proves each reviewed ImportRef against the
// corresponding typed source world. Source syntax proves only spelling; this
// boundary additionally proves the imported package clause through go/types.
// A consumer visible in multiple package variants is admissible only when all
// variants report precisely the same import-spec set.
func verifyDeclaredImportEvidence(intent cutplan.Intent, source, target semantic.StructuralSnapshot) error {
	for _, route := range allImports(intent) {
		if route.From != nil {
			if err := requireTypedImport(source, route.Consumer, *route.From, "pre-cut"); err != nil {
				return err
			}
		}
		if route.To != nil {
			if err := requireTypedImport(target, route.Consumer, *route.To, "post-cut"); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireTypedImport(snapshot semantic.StructuralSnapshot, consumer string, want cutplan.ImportRef, world string) error {
	imports, err := importsForConsumer(snapshot, consumer)
	if err != nil {
		return fmt.Errorf("%s import consumer %s: %w", world, consumer, err)
	}
	if _, found := imports[typedImportKey(want)]; !found {
		return fmt.Errorf("%s import consumer %s lacks exact {path:%q name:%q alias:%q}", world, consumer, want.Path, want.Name, want.Alias)
	}
	return nil
}

func importsForConsumer(snapshot semantic.StructuralSnapshot, consumer string) (map[string]cutplan.ImportRef, error) {
	var baseline map[string]cutplan.ImportRef
	seen := false
	for _, file := range snapshot.Files {
		if file.Path != consumer {
			continue
		}
		imports, err := typedImportSet(file.Imports)
		if err != nil {
			return nil, fmt.Errorf("package variant %s: %w", file.PackageID, err)
		}
		if !seen {
			baseline, seen = imports, true
			continue
		}
		if !sameTypedImportSet(baseline, imports) {
			return nil, fmt.Errorf("package variants disagree on exact imports")
		}
	}
	if !seen {
		return nil, fmt.Errorf("is absent from typed structural snapshot")
	}
	return baseline, nil
}

func typedImportSet(values []cutplan.ImportRef) (map[string]cutplan.ImportRef, error) {
	result := make(map[string]cutplan.ImportRef, len(values))
	for _, value := range values {
		if value.Path == "" || value.Name == "" || value.Name == "_" || value.Alias == "." || value.Alias == "_" {
			return nil, fmt.Errorf("invalid typed import evidence")
		}
		key := typedImportKey(value)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate exact import evidence")
		}
		result[key] = value
	}
	return result, nil
}

func typedImportKey(value cutplan.ImportRef) string {
	return value.Path + "\x00" + value.Name + "\x00" + value.Alias
}

func sameTypedImportSet(left, right map[string]cutplan.ImportRef) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, found := right[key]; !found {
			return false
		}
	}
	return true
}

// residueQueries derives the complete post-cut denominator from the moved and
// retired source surface. Containment Parent is pre-cut resolution context for
// the renderer, not a relocated subject, so its unrelated references are not
// residue. Authors still cannot omit a relocated source, retiree, or use site:
// a source site outside the reviewed write closure is an unrepresentable cut.
func residueQueries(intent cutplan.Intent, source semantic.Snapshot) ([]semantic.ResidueQuery, error) {
	impacts, err := cutplan.ImpactObjects(intent)
	if err != nil {
		return nil, err
	}
	moved := make(map[string]bool, len(impacts))
	for _, object := range impacts {
		moved[object.Object] = true
	}
	writes := map[string]bool{}
	for _, path := range cutplan.WritePaths(intent) {
		writes[path] = true
	}
	queries := make([]semantic.ResidueQuery, 0)
	for _, object := range source.Objects {
		if object.Role != cutplan.ObjectSource || !moved[object.Object.Object] {
			continue
		}
		paths := map[string]bool{object.Definition.Path: true}
		for _, site := range object.References {
			paths[site.Path] = true
		}
		result := make([]string, 0, len(paths))
		for path := range paths {
			if !writes[path] {
				return nil, fmt.Errorf("residue source site for %s is outside write footprint: %s", object.Object.Object, path)
			}
			result = append(result, path)
		}
		sort.Strings(result)
		queries = append(queries, semantic.ResidueQuery{Object: object.Object, Paths: result})
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("residue gate requested without derived source denominator")
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].Object.Object < queries[j].Object.Object })
	return queries, nil
}

func sourceMap(snapshot semantic.Snapshot) (verify.SourceMap, error) {
	if snapshot.Workspace == nil {
		return nil, fmt.Errorf("snapshot has no semantic workspace")
	}
	paths := map[string]bool{}
	for _, file := range snapshot.Workspace.Files() {
		paths[file.Path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	result := verify.SourceMap{}
	for _, path := range ordered {
		file, err := snapshot.Workspace.File(path)
		if err != nil {
			return nil, fmt.Errorf("canonical source %s: %w", path, err)
		}
		result[path] = verify.SourceFile{Path: path, Package: file.PackagePath, Source: append([]byte(nil), file.Source...)}
	}
	return result, nil
}

func allImports(intent cutplan.Intent) []cutplan.Import {
	result := make([]cutplan.Import, 0)
	for _, operation := range intent.Operations {
		result = append(result, operation.Imports...)
	}
	return result
}

func digestEvidence(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("canonical gate evidence: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
