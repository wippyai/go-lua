package relcut

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// modulePath is the import prefix the map's surface packages are stated in.
const modulePath = "github.com/wippyai/go-lua/"

// ValidateConsumers checks the consumer map against itself and against a
// repository.
//
// Against itself: one entry per path, every read carries a declared class,
// every target carries a declared kind, an owner-column target names both its
// relation and its column, a gap entry names a declared gap and no column, a
// column entry names no gap, and every declared gap is reached by at least one
// consumer that cites it.
//
// Against the repository: every consumer file is still in the tree and still
// names every symbol the entry attributes to it, and every surface package
// still declares that symbol. A consumer that stopped reading a symbol, a
// symbol that left its package, and a file that was deleted are all the same
// finding - the map has gone stale and the cut cannot be planned from it.
func ValidateConsumers(consumers ConsumerMap, repositoryRoot string) []Finding {
	var findings []Finding
	refuse := func(entry, detail string) {
		findings = append(findings, Finding{Severity: SeverityRefused, Entry: entry, Detail: detail})
	}

	classes := map[ReadClass]struct{}{
		ReadClassQuery: {}, ReadClassResult: {}, ReadClassDeclaration: {},
		ReadClassAdmission: {}, ReadClassRowABI: {},
	}
	kinds := map[TargetKind]struct{}{
		TargetOwnerColumn: {}, TargetCertificate: {}, TargetDeclaredSurface: {},
		TargetRuntimeComposition: {}, TargetRowABI: {}, TargetGap: {},
	}

	gaps := map[string]struct{}{}
	for _, gap := range consumers.Gaps {
		if gap.ID == "" {
			refuse("<unnamed gap>", "gap has no identity")
			continue
		}
		if _, held := gaps[gap.ID]; held {
			refuse(gap.ID, "gap identity is used twice")
		}
		if strings.TrimSpace(gap.Distinction) == "" {
			refuse(gap.ID, "gap names no unpublished distinction")
		}
		gaps[gap.ID] = struct{}{}
	}

	surfaces := map[string]struct{}{}
	for _, surface := range consumers.Surfaces {
		if surface.Package == "" {
			refuse("<unnamed surface>", "surface names no package")
			continue
		}
		surfaces[surface.Package] = struct{}{}
	}

	cited := map[string]struct{}{}
	paths := map[string]struct{}{}
	for _, consumer := range consumers.Consumers {
		if consumer.Path == "" {
			refuse("<unnamed>", "consumer has no path")
			continue
		}
		if _, held := paths[consumer.Path]; held {
			refuse(consumer.Path, "consumer path is listed twice")
		}
		paths[consumer.Path] = struct{}{}

		if len(consumer.Reads) == 0 {
			refuse(consumer.Path, "consumer names no read")
		}
		for _, read := range consumer.Reads {
			if _, held := classes[read.Class]; !held {
				refuse(consumer.Path, fmt.Sprintf("read %s.%s carries undeclared class %q", read.Package, read.Symbol, read.Class))
			}
			if _, held := surfaces[read.Package]; !held {
				refuse(consumer.Path, fmt.Sprintf("read %s.%s names a package the map does not declare as a surface", read.Package, read.Symbol))
			}
			if read.Uses <= 0 {
				refuse(consumer.Path, fmt.Sprintf("read %s.%s is attributed no use", read.Package, read.Symbol))
			}
		}

		if _, held := kinds[consumer.Target.Kind]; !held {
			refuse(consumer.Path, fmt.Sprintf("target kind %q is not declared", consumer.Target.Kind))
		}
		switch consumer.Target.Kind {
		case TargetGap:
			if consumer.Gap == "" {
				refuse(consumer.Path, "target is a gap but the entry names none")
				break
			}
			if _, held := gaps[consumer.Gap]; !held {
				refuse(consumer.Path, fmt.Sprintf("names unknown gap %q", consumer.Gap))
			}
			cited[consumer.Gap] = struct{}{}
			if consumer.Target.Column != "" {
				refuse(consumer.Path, "a gap entry names a column; a gap is the absence of one")
			}
		case TargetOwnerColumn:
			if consumer.Gap != "" {
				refuse(consumer.Path, "a served entry names a gap")
			}
			if consumer.Target.Relation == "" || consumer.Target.Column == "" {
				refuse(consumer.Path, "owner-column target names no relation or no column")
			}
		default:
			if consumer.Gap != "" {
				refuse(consumer.Path, "a served entry names a gap")
			}
			if consumer.Target.Package == "" {
				refuse(consumer.Path, fmt.Sprintf("target %q names no package", consumer.Target.Kind))
			}
		}
		if consumer.ManifestEntry == "" && consumer.Target.Note == "" {
			refuse(consumer.Path, "consumer is outside every manifest entry and carries no note saying why")
		}
	}
	for id := range gaps {
		if _, held := cited[id]; !held {
			refuse(id, "gap is declared but no consumer cites it")
		}
	}

	if repositoryRoot == "" {
		return findings
	}

	declared := map[string]map[string]struct{}{}
	for _, consumer := range consumers.Consumers {
		source, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(consumer.Path)))
		if err != nil {
			refuse(consumer.Path, fmt.Sprintf("consumer file is no longer readable; the map is stale: %v", err))
			continue
		}
		text := string(source)
		for _, read := range consumer.Reads {
			if !strings.Contains(text, "."+read.Symbol) {
				refuse(consumer.Path, fmt.Sprintf("no longer names %s.%s; the map is stale", read.Package, read.Symbol))
			}
			exported, err := packageSymbols(declared, repositoryRoot, read.Package)
			if err != nil {
				refuse(consumer.Path, fmt.Sprintf("surface package %s: %v", read.Package, err))
				continue
			}
			if _, held := exported[read.Symbol]; !held {
				refuse(consumer.Path, fmt.Sprintf("%s no longer declares %s; the map is stale", read.Package, read.Symbol))
			}
		}
	}
	return findings
}

// packageSymbols returns the exported top-level identities one package
// declares, memoised across the whole validation.
func packageSymbols(cache map[string]map[string]struct{}, repositoryRoot, importPath string) (map[string]struct{}, error) {
	if held, ok := cache[importPath]; ok {
		return held, nil
	}
	relative := strings.TrimPrefix(importPath, modulePath)
	if relative == importPath {
		return nil, fmt.Errorf("import path is outside %s", modulePath)
	}
	directory := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, directory, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}
	symbols := map[string]struct{}{}
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				switch typed := declaration.(type) {
				case *ast.FuncDecl:
					if typed.Recv == nil && typed.Name.IsExported() {
						symbols[typed.Name.Name] = struct{}{}
					}
				case *ast.GenDecl:
					for _, spec := range typed.Specs {
						switch named := spec.(type) {
						case *ast.TypeSpec:
							if named.Name.IsExported() {
								symbols[named.Name.Name] = struct{}{}
							}
						case *ast.ValueSpec:
							for _, name := range named.Names {
								if name.IsExported() {
									symbols[name.Name] = struct{}{}
								}
							}
						}
					}
				}
			}
		}
	}
	cache[importPath] = symbols
	return symbols, nil
}
