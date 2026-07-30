package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2LayerImportManifest(t *testing.T) {
	packages := productionPackages(t,
		modulePath+"/analysis/...",
		modulePath+"/compiler/...",
		modulePath+"/cmd/...",
	)

	for _, boundary := range v2LayerImportBoundaries {
		boundary := boundary
		t.Run(boundary.name, func(t *testing.T) {
			validateV2Boundary(t, boundary)
			if boundary.delegatedTo != "" {
				t.Logf("enforced by existing architecture gate %s", boundary.delegatedTo)
				return
			}
			for _, pkg := range packages {
				if !matchesAnyV2Prefix(pkg.ImportPath, boundary.subjects) || matchesAnyV2Prefix(pkg.ImportPath, boundary.exceptSubjects) {
					continue
				}
				for _, imported := range pkg.Imports {
					if !v2RepositoryImport(imported) {
						continue
					}
					if len(boundary.allowedRepository) != 0 && !matchesAnyV2Prefix(imported, boundary.allowedRepository) {
						t.Fatalf("%s imports repository package %q outside its neutral allowlist", pkg.ImportPath, imported)
					}
					if matchesAnyV2Prefix(imported, boundary.forbidden) {
						t.Fatalf("%s imports forbidden dependency %q", pkg.ImportPath, imported)
					}
				}
			}
		})
	}
}

func TestRetiredParallelEnginePackagesStayDeleted(t *testing.T) {
	packages := productionPackages(t,
		modulePath+"/analysis/...",
	)
	for _, retired := range retiredParallelPackagePrefixes {
		for _, pkg := range packages {
			if v2PrefixMatches(pkg.ImportPath, retired) {
				t.Errorf("retired parallel engine package %s regrew as %s", retired, pkg.ImportPath)
			}
		}
	}

	root := repoRoot(t)
	for _, retired := range retiredParallelSourcePaths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(retired))); !os.IsNotExist(err) {
			t.Errorf("retired parallel engine source %s exists (stat error: %v)", retired, err)
		}
	}
}

func validateV2Boundary(t *testing.T, boundary v2ImportBoundary) {
	t.Helper()
	if boundary.name == "" || len(boundary.subjects) == 0 {
		t.Fatal("v2 import boundary requires a name and at least one subject")
	}
	if boundary.delegatedTo != "" && (len(boundary.forbidden) != 0 || len(boundary.allowedRepository) != 0 || len(boundary.exceptSubjects) != 0) {
		t.Fatalf("delegated boundary %q must not duplicate enforcement data", boundary.delegatedTo)
	}
	if boundary.delegatedTo != "" && v2DelegatedImportGates[boundary.delegatedTo] == nil {
		t.Fatalf("delegated boundary owner %q is not linked into the manifest", boundary.delegatedTo)
	}
	if boundary.delegatedTo == "" && len(boundary.forbidden) == 0 && len(boundary.allowedRepository) == 0 {
		t.Fatal("non-delegated v2 boundary has no import policy")
	}
	for _, group := range [][]v2PackagePrefix{boundary.subjects, boundary.forbidden, boundary.allowedRepository, boundary.exceptSubjects} {
		for _, prefix := range group {
			if !validV2Prefix(prefix) {
				t.Fatalf("invalid exact package prefix %q", prefix)
			}
		}
	}
}

func validV2Prefix(prefix v2PackagePrefix) bool {
	value := string(prefix)
	return value != "" && !strings.HasSuffix(value, "/") && v2RepositoryImport(value)
}

func v2RepositoryImport(importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func v2PrefixMatches(importPath string, prefix v2PackagePrefix) bool {
	value := string(prefix)
	return importPath == value || strings.HasPrefix(importPath, value+"/")
}

func matchesAnyV2Prefix(importPath string, prefixes []v2PackagePrefix) bool {
	for _, prefix := range prefixes {
		if v2PrefixMatches(importPath, prefix) {
			return true
		}
	}
	return false
}

func TestV2PackagePrefixMatchingIsSegmentExact(t *testing.T) {
	prefix := v2PackagePrefix(modulePath + "/analysis/check/projection")
	for _, test := range []struct {
		path string
		want bool
	}{
		{modulePath + "/analysis/check/projection", true},
		{modulePath + "/analysis/check/projection/internal", true},
		{modulePath + "/analysis/check/projectionx", false},
		{modulePath + "/analysis/check/project", false},
	} {
		if got := v2PrefixMatches(test.path, prefix); got != test.want {
			t.Errorf("v2PrefixMatches(%q, %q) = %v, want %v", test.path, prefix, got, test.want)
		}
	}
}
