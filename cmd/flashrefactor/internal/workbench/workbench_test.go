package workbench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/render"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
)

func TestRouteConstructionRejectsMissingWitness(t *testing.T) {
	from := cutplan.SymbolRef{Object: "example.com/a#package:Old"}
	to := cutplan.SymbolRef{Object: "example.com/b#package:New"}
	intent := relocationIntent(from, to)
	_, err := deriveRoutes(intent, nil, semantic.Snapshot{}, semantic.Snapshot{})
	if err == nil || !strings.Contains(err.Error(), "workspaces") {
		t.Fatalf("route construction accepted missing semantic witnesses: %v", err)
	}
}

func TestTypedImportEvidenceRequiresExactClauseAndRawAlias(t *testing.T) {
	route := cutplan.Import{Consumer: "p/use.go", From: &cutplan.ImportRef{Path: "example.com/old", Name: "oldpkg", Alias: ""}, To: &cutplan.ImportRef{Path: "example.com/new", Name: "newpkg", Alias: "named"}}
	intent := cutplan.Intent{Operations: []cutplan.Operation{{Imports: []cutplan.Import{route}}}}
	source := semantic.StructuralSnapshot{Files: []semantic.StructuralFile{{Path: "p/use.go", PackageID: "p", Imports: []cutplan.ImportRef{{Path: "example.com/old", Name: "oldpkg", Alias: ""}}}}}
	target := semantic.StructuralSnapshot{Files: []semantic.StructuralFile{{Path: "p/use.go", PackageID: "p", Imports: []cutplan.ImportRef{{Path: "example.com/new", Name: "newpkg", Alias: "named"}}}}}
	if err := verifyDeclaredImportEvidence(intent, source, target); err != nil {
		t.Fatalf("exact typed import evidence rejected: %v", err)
	}
	target.Files[0].Imports[0].Name = "wrong"
	if err := verifyDeclaredImportEvidence(intent, source, target); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("wrong imported package clause accepted: %v", err)
	}
	target.Files[0].Imports[0] = cutplan.ImportRef{Path: "example.com/new", Name: "newpkg", Alias: ""}
	if err := verifyDeclaredImportEvidence(intent, source, target); err == nil || !strings.Contains(err.Error(), "alias") {
		t.Fatalf("wrong raw alias spelling accepted: %v", err)
	}
}

func TestTypedImportEvidenceRejectsMissingOrDivergentConsumer(t *testing.T) {
	intent := cutplan.Intent{Operations: []cutplan.Operation{{Imports: []cutplan.Import{{Consumer: "p/use.go", To: &cutplan.ImportRef{Path: "example.com/new", Name: "new", Alias: ""}}}}}}
	if err := verifyDeclaredImportEvidence(intent, semantic.StructuralSnapshot{}, semantic.StructuralSnapshot{}); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("missing consumer accepted: %v", err)
	}
	target := semantic.StructuralSnapshot{Files: []semantic.StructuralFile{
		{Path: "p/use.go", PackageID: "p [p.test]", Imports: []cutplan.ImportRef{{Path: "example.com/new", Name: "new", Alias: ""}}},
		{Path: "p/use.go", PackageID: "p.test", Imports: []cutplan.ImportRef{{Path: "example.com/new", Name: "new", Alias: "other"}}},
	}}
	if err := verifyDeclaredImportEvidence(intent, semantic.StructuralSnapshot{}, target); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("divergent package variants accepted: %v", err)
	}
}

func TestTypedImportEvidenceRejectsDotAndBlank(t *testing.T) {
	intent := cutplan.Intent{Operations: []cutplan.Operation{{Imports: []cutplan.Import{{Consumer: "p/use.go", To: &cutplan.ImportRef{Path: "example.com/new", Name: "new", Alias: ""}}}}}}
	for _, alias := range []string{".", "_"} {
		target := semantic.StructuralSnapshot{Files: []semantic.StructuralFile{{Path: "p/use.go", PackageID: "p", Imports: []cutplan.ImportRef{{Path: "example.com/new", Name: "new", Alias: alias}}}}}
		if err := verifyDeclaredImportEvidence(intent, semantic.StructuralSnapshot{}, target); err == nil || !strings.Contains(err.Error(), "invalid typed import") {
			t.Fatalf("%q import evidence accepted: %v", alias, err)
		}
	}
}

func TestSourceMapUsesCanonicalBaseFileAcrossTestVariants(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/variants\n\ngo 1.23.3\n")
	writeFixture(t, root, "old.go", "package p\n\nfunc Old() {}\n")
	writeFixture(t, root, "keep.go", `package p

import "fmt"

func Keep() string { return fmt.Sprint("keep") }
`)
	from := cutplan.SymbolRef{Object: "example.com/variants#package:Old"}
	to := cutplan.SymbolRef{Object: "example.com/variants#package:New"}
	intent := relocationIntent(from, to)
	intent.Operations[0].Verify.Gates = []cutplan.Gate{cutplan.GateImportDAG}
	session, err := semantic.NewSession(semantic.Config{Root: root, Flashrefactor: "test", CacheParent: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	source, err := session.Collect(context.Background(), intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	targetFiles := []semantic.VirtualFile{
		{Path: "old.go", Content: []byte("package p\n")},
		{Path: "new.go", Content: []byte("package p\n\nfunc New() {}\n")},
		{Path: "added_test.go", Content: []byte("package p\n\nfunc testVariantMarker() {}\n")},
	}
	target, err := session.CollectVirtual(context.Background(), intent, nil, targetFiles)
	if err != nil {
		t.Fatal(err)
	}
	if sourceVariants, targetVariants := sourceMapVariants(source, "keep.go"), sourceMapVariants(target, "keep.go"); sourceVariants != 1 || targetVariants <= sourceVariants {
		t.Fatalf("test-variant multiplicity source/target = %d/%d, want one source base variant and more target variants", sourceVariants, targetVariants)
	}
	if _, err := verifyGates(intent, source, target); err != nil {
		t.Fatalf("unchanged import across test-variant multiplicity: %v", err)
	}

	targetFiles = append(targetFiles, semantic.VirtualFile{Path: "keep.go", Content: []byte(`package p

import "strings"

func Keep() string { return strings.ToUpper("keep") }
`)})
	changed, err := session.CollectVirtual(context.Background(), intent, nil, targetFiles)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyGates(intent, source, changed); err == nil || !strings.Contains(err.Error(), "exact import-spec delta") {
		t.Fatalf("undeclared real import change accepted: %v", err)
	}
}

func sourceMapVariants(snapshot semantic.Snapshot, path string) int {
	count := 0
	for _, file := range snapshot.Workspace.Files() {
		if file.Path == path {
			count++
		}
	}
	return count
}

func TestRenderDiffIsDeterministicAndRepresentsDeletion(t *testing.T) {
	first, err := renderDiff([]render.DiffInput{{Path: "b.go", Before: []byte("package b\n"), After: []byte("package b\n// x\n")}, {Path: "a.go", Before: []byte("package a\n"), Delete: true}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderDiff([]render.DiffInput{{Path: "a.go", Before: []byte("package a\n"), Delete: true}, {Path: "b.go", Before: []byte("package b\n"), After: []byte("package b\n// x\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || !strings.Contains(string(first), "+++ /dev/null") {
		t.Fatalf("unstable or incomplete diff: %q", first)
	}
}

func TestNewRejectsMismatchedHelperBuild(t *testing.T) {
	_, err := New(Config{Root: t.TempDir(), Semantic: semantic.Config{Flashrefactor: "semantic"}, Toolchain: cutplan.Toolchain{HelperBuild: "workbench"}})
	if err == nil || !strings.Contains(err.Error(), "helper build") {
		t.Fatalf("mismatched helper identities accepted: %v", err)
	}
}

func TestCanonicalGoVersion(t *testing.T) {
	version, err := canonicalGoVersion("go version go1.25.4 linux/amd64")
	if err != nil || version != "go1.25.4" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	if _, err := canonicalGoVersion("unknown"); err == nil {
		t.Fatal("invalid Go version identity accepted")
	}
}

func TestRouteWitnessResolvesTypedSitesWithoutPositionPairing(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/route\ngo 1.24\n")
	oldSource := "package p\n\nfunc Old() {}\n"
	writeFixture(t, root, "old.go", oldSource)
	from := cutplan.SymbolRef{Object: "example.com/route#package:Old"}
	to := cutplan.SymbolRef{Object: "example.com/route#package:New"}
	intent := relocationIntent(from, to)
	session, err := semantic.NewSession(semantic.Config{Root: root, Flashrefactor: "test", CacheParent: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	source, err := session.Collect(context.Background(), intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := session.CollectVirtual(context.Background(), intent, nil, []semantic.VirtualFile{
		{Path: "old.go", Content: []byte("package p\n")},
		{Path: "new.go", Content: []byte("package p\n\nfunc New() {}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	routes, err := deriveRoutes(intent, []render.RouteWitness{{From: from, To: to, Sites: []render.RouteWitnessSite{{
		Source: render.PhysicalWitnessSite{Path: "old.go", Offset: strings.Index(oldSource, "Old"), Role: cutplan.SiteDeclaration},
		Target: render.StructuralAnchor{Path: "new.go", Identifier: 2, Role: cutplan.SiteDeclaration, Name: "New"},
	}}}}, source, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || len(routes[0].Sites) != 1 || routes[0].Sites[0].Source.Role != cutplan.SiteDeclaration || routes[0].Sites[0].Target.Role != cutplan.SiteDeclaration {
		t.Fatalf("unexpected typed route: %#v", routes)
	}
}

func writeFixture(t *testing.T, root, path, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryCrashHelper(t *testing.T) {
	mode, root := os.Getenv("FLASHRECOVERY_MODE"), os.Getenv("FLASHRECOVERY_ROOT")
	if mode == "" {
		return
	}
	path := os.Getenv("FLASHRECOVERY_PATH")
	plan := transaction.Plan{Declared: []string{path}, Changes: []transaction.Change{{Path: path, Delete: true}}}
	if mode == "prepared" {
		_, _ = transaction.RunWithGuard(root, plan, func(transaction.Preimage) error { os.Exit(71); return nil }, nil)
	}
	if mode == "applied" {
		_, _ = transaction.Run(root, plan, func() error { os.Exit(72); return nil })
	}
	os.Exit(73)
}

func TestRecoveryAdaptersPreparedAppliedAndVerified(t *testing.T) {
	for _, state := range []string{"prepared", "applied", "verified"} {
		t.Run(state, func(t *testing.T) {
			root, bench, lock := recoveryFixture(t)
			crashRecovery(t, root, map[string]string{"prepared": "prepared", "applied": "applied", "verified": "applied"}[state])
			if state == "verified" {
				path := filepath.Join(root, ".flashrefactor", "transaction", "state.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), `"state":"applied"`, `"state":"verified"`, 1))
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			recovery, err := bench.InspectRecovery()
			if err != nil {
				t.Fatal(err)
			}
			if string(recovery.State) != state {
				t.Fatalf("state=%s", recovery.State)
			}
			if state == "prepared" {
				if err := bench.CompleteRecovery(context.Background(), lock); err == nil {
					t.Fatal("prepared recovery completed")
				}
				if err := bench.RollbackRecovery(); err != nil {
					t.Fatal(err)
				}
				if _, err := bench.InspectRecovery(); !errors.Is(err, transaction.ErrNoRecovery) {
					t.Fatalf("recovery remained: %v", err)
				}
				return
			}
			if err := bench.CompleteRecovery(context.Background(), lock); err != nil {
				t.Fatal(err)
			}
			arguments, err := os.ReadFile(filepath.Join(root, "gate.args"))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(arguments), "go\ntest\n./p\n-run\n^TestKeep$\n-count=1\n"; got != want {
				t.Fatalf("bounded gate arguments=%q, want %q", got, want)
			}
			if _, err := os.Stat(filepath.Join(root, "p", "old.go")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output not retained: %v", err)
			}
			if _, err := bench.InspectRecovery(); !errors.Is(err, transaction.ErrNoRecovery) {
				t.Fatalf("recovery remained: %v", err)
			}
		})
	}
}

func recoveryFixture(t *testing.T) (string, Bench, cutplan.Lock) {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/recovery\ngo 1.23\n")
	if err := os.Mkdir(filepath.Join(root, "p"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "p/keep.go", "package p\n")
	writeFixture(t, root, "scripts/bounded_test.sh", "#!/bin/sh\nprintf '%s\\n' \"$@\" > gate.args\n")
	if err := os.Chmod(filepath.Join(root, "scripts", "bounded_test.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := "package p\n\nfunc Old() {}\n"
	writeFixture(t, root, "p/old.go", old)
	from := cutplan.SymbolRef{Object: "example.com/recovery/p#package:Old"}
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "retire", Operations: []cutplan.Operation{{
		ID: "retire", Authority: cutplan.Authority{From: "old", To: "new"},
		Edits:     []cutplan.Edit{{Kind: cutplan.EditRetire, Retire: &cutplan.Retire{Source: "p/old.go", Symbols: []cutplan.SymbolRef{from}}}},
		Footprint: cutplan.Footprint{Read: []string{"p/old.go"}, Write: []string{"p/old.go"}},
		Verify:    cutplan.Verification{Laws: []cutplan.Law{{ID: "keep", Package: "./p", Test: "TestKeep"}}, Gates: []cutplan.Gate{cutplan.GateResidue}},
	}}}
	bench, err := New(Config{Root: root, Semantic: semantic.Config{Root: root, Flashrefactor: "test", CacheParent: t.TempDir()}, Toolchain: cutplan.Toolchain{HelperBuild: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := semantic.NewSession(bench.config.Semantic)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	source, err := session.Collect(context.Background(), intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := session.CollectVirtual(context.Background(), intent, nil, []semantic.VirtualFile{{Path: "p/old.go", Delete: true}})
	if err != nil {
		t.Fatal(err)
	}
	gates, err := verifyGates(intent, source, target)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := cutplan.Fingerprint(root, cutplan.ReadPaths(intent), nil)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := renderDiff([]render.DiffInput{{Path: "p/old.go", Before: []byte(old), Delete: true}})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := executionEvidence([]semantic.VirtualFile{{Path: "p/old.go", Delete: true}}, diff)
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := bench.toolchainAt(source)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := cutplan.BuildLock(intent, toolchain, cutplan.LockEvidence{Inputs: inputs, Resolution: cutplan.ResolutionEvidence{Objects: source.Objects}, Gates: gates, Execution: execution})
	if err != nil {
		t.Fatalf("build recovery lock: %v\ntoolchain: %#v", err, toolchain)
	}
	return root, bench, lock
}

func crashRecovery(t *testing.T, root, mode string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRecoveryCrashHelper$")
	command.Env = append(os.Environ(), "FLASHRECOVERY_MODE="+mode, "FLASHRECOVERY_ROOT="+root, "FLASHRECOVERY_PATH=p/old.go")
	err := command.Run()
	if err == nil {
		t.Fatal("crash helper survived")
	}
}

func relocationIntent(from, to cutplan.SymbolRef) cutplan.Intent {
	return cutplan.Intent{Schema: cutplan.Version, Name: "route", Operations: []cutplan.Operation{{
		ID: "move", Authority: cutplan.Authority{From: "old", To: "new"},
		Edits:     []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{Source: "old.go", Destination: cutplan.Destination{Path: "new.go", Package: "new"}, Subjects: []cutplan.Relocation{{From: from, To: to}}}}},
		Footprint: cutplan.Footprint{Read: []string{"old.go"}, Write: []string{"old.go", "new.go"}},
		Verify: cutplan.Verification{
			Laws:  []cutplan.Law{{ID: "move", Package: "./pkg", Test: "TestMove"}},
			Gates: []cutplan.Gate{cutplan.GateResidue},
		},
	}}}
}
