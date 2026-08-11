package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

const qualificationModule = "example.com/flashrefactor/qualification"

// TestSyntheticCrossPackageQualification is the durable positive acceptance
// cut. It uses one atomic operation with containment, four package-level
// objects, a moved test, and ordinary/internal/external consumers. The test
// speaks only through the public Bench lifecycle; its postconditions are
// independently re-resolved from the applied tree.
func TestSyntheticCrossPackageQualification(t *testing.T) {
	root, intent := qualificationFixture(t)
	before := qualificationSourceDigest(t, root)
	registry, err := generate.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	bench, err := New(Config{
		Root:      root,
		Semantic:  semantic.Config{Root: root, Flashrefactor: "flashrefactor-v3"},
		Registry:  registry,
		Toolchain: cutplan.Toolchain{HelperBuild: "flashrefactor-v3"},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := bench.Prepare(context.Background(), intent)
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	qualificationSourceUnchanged(t, before, qualificationSourceDigest(t, root), "first prepare")
	second, err := bench.Prepare(context.Background(), intent)
	if err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	qualificationSourceUnchanged(t, before, qualificationSourceDigest(t, root), "second prepare")
	firstBytes, err := json.Marshal(first.Lock)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := json.Marshal(second.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("prepare did not produce byte-identical canonical locks")
	}
	if _, err := bench.Replay(context.Background(), first.Lock); err != nil {
		t.Fatalf("replay: %v", err)
	}
	qualificationSourceUnchanged(t, before, qualificationSourceDigest(t, root), "replay")

	if err := bench.Apply(context.Background(), first.Lock); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := cutplan.VerifyOutputs(root, first.Lock); err != nil {
		t.Fatalf("applied outputs: %v", err)
	}
	target := qualificationTargetSnapshot(t, bench, first.Lock)
	if err := semantic.VerifyExpected(targetEvidence(first.Lock.Evidence.Resolution.Objects), target.Objects); err != nil {
		t.Fatalf("target typed evidence: %v", err)
	}
	qualificationRouteConservation(t, first.Lock)
	qualificationNoResidue(t, first.rendered.source, target, intent)
	if delta, err := semantic.VerifyDiagnosticDelta(first.rendered.source.Diagnostics, target.Diagnostics, nil, nil); err != nil {
		t.Fatalf("diagnostic delta: %v", err)
	} else if len(delta.Added) != 0 || len(delta.Removed) != 0 {
		t.Fatalf("diagnostics changed: %#v", delta)
	}
	gates, err := verifyGates(first.Lock.Intent, first.rendered.source, target)
	if err != nil {
		t.Fatalf("post-apply gates: %v", err)
	}
	if err := compareGates(first.Lock.Evidence.Gates, gates); err != nil {
		t.Fatalf("exact import-DAG gate evidence: %v", err)
	}
	qualificationRequiredGates(t, gates)
}

// TestContainmentResidueExcludesUnchangedParentButNotMovedFields proves that
// Link remains resolution context only: an uncut consumer may use Link and a
// stable field, but every reference to an extracted field remains in the
// write closure.
func TestContainmentResidueExcludesUnchangedParentButNotMovedFields(t *testing.T) {
	t.Run("unchanged-parent-reference-outside-cut", func(t *testing.T) {
		root, intent := qualificationFixture(t)
		qualificationWrite(t, root, "core/unrelated.go", `package core

func stableLinkValue(link Link) int { return link.Stable }
`)

		if _, err := qualificationBench(t, root).Prepare(context.Background(), intent); err != nil {
			t.Fatalf("prepare with unrelated Link reference: %v", err)
		}
	})

	t.Run("moved-field-reference-outside-cut", func(t *testing.T) {
		root, intent := qualificationFixture(t)
		qualificationWrite(t, root, "core/unrelated.go", `package core

func extractedLinkValue(link Link) int { return link.Count }
`)

		if _, err := qualificationBench(t, root).Prepare(context.Background(), intent); err == nil {
			t.Fatal("prepare accepted a moved field reference outside the write footprint")
		}
	})
}

func qualificationTargetSnapshot(t *testing.T, bench Bench, lock cutplan.Lock) semantic.Snapshot {
	t.Helper()
	session, err := semantic.NewSession(bench.config.Semantic)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	target, err := session.CollectVirtual(context.Background(), lock.Intent, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func qualificationRequiredGates(t *testing.T, gates []cutplan.GateEvidence) {
	t.Helper()
	expected := map[cutplan.Gate]bool{
		cutplan.GateDiagnostics: true,
		cutplan.GateImportDAG:   true,
		cutplan.GateResidue:     true,
	}
	if len(gates) != len(expected) {
		t.Fatalf("gate denominator=%#v", gates)
	}
	for _, gate := range gates {
		if !expected[gate.Gate] || gate.ResultSHA256 == "" {
			t.Fatalf("unexpected or incomplete gate evidence: %#v", gate)
		}
		delete(expected, gate.Gate)
	}
	if len(expected) != 0 {
		t.Fatalf("missing required gates: %#v", expected)
	}
}

func qualificationNoResidue(t *testing.T, source, target semantic.Snapshot, intent cutplan.Intent) {
	t.Helper()
	queries, err := residueQueries(intent, source)
	if err != nil {
		t.Fatal(err)
	}
	residues, err := target.Residues(queries)
	if err != nil {
		t.Fatal(err)
	}
	for _, residue := range residues {
		if len(residue.Sites) != 0 {
			t.Fatalf("old residue %s remains at %#v", residue.Object.Object, residue.Sites)
		}
	}
}

func qualificationRouteConservation(t *testing.T, lock cutplan.Lock) {
	t.Helper()
	required, err := cutplan.ReferenceRouteRequirements(lock.Intent)
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[string]cutplan.ObjectEvidence{}
	for _, object := range lock.Evidence.Resolution.Objects {
		evidence[object.Object.Object] = object
	}
	routes := map[string]cutplan.ReferenceRoute{}
	for _, route := range lock.Evidence.Routes {
		key := qualificationRouteKey(route.From, route.To)
		if _, exists := routes[key]; exists {
			t.Fatalf("duplicate route %s", key)
		}
		routes[key] = route
	}
	if len(routes) != len(required) {
		t.Fatalf("route denominator=%d, required=%d", len(routes), len(required))
	}
	for _, requiredRoute := range required {
		key := qualificationRouteKey(requiredRoute.From, requiredRoute.To)
		route, exists := routes[key]
		if !exists {
			t.Fatalf("missing route %s", key)
		}
		source, sourceExists := evidence[route.From.Object]
		target, targetExists := evidence[route.To.Object]
		if !sourceExists || source.Role != cutplan.ObjectSource || !targetExists || target.Role != cutplan.ObjectTarget {
			t.Fatalf("route %s has incomplete typed endpoints", key)
		}
		sources, targets := map[string]bool{}, map[string]bool{}
		for _, site := range route.Sites {
			if site.Source.Role != site.Target.Role {
				t.Fatalf("route %s changes semantic role", key)
			}
			sources[qualificationPositionKey(site.Source)] = true
			targets[qualificationPositionKey(site.Target)] = true
		}
		if !qualificationSamePositionSets(sources, qualificationEndpointSet(source)) || !qualificationSamePositionSets(targets, qualificationEndpointSet(target)) {
			t.Fatalf("route %s does not conserve complete typed endpoints", key)
		}
	}
}

func qualificationEndpointSet(object cutplan.ObjectEvidence) map[string]bool {
	result := map[string]bool{}
	result[qualificationPositionKey(object.Definition)] = true
	for _, reference := range object.References {
		result[qualificationPositionKey(reference)] = true
	}
	return result
}

func qualificationPositionKey(position cutplan.Position) string {
	return fmt.Sprintf("%s:%d:%s", position.Path, position.Offset, position.Role)
}

func qualificationSamePositionSets(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}

func qualificationRouteKey(from, to cutplan.SymbolRef) string {
	return from.Object + "\x00" + to.Object
}

func qualificationFixture(t *testing.T) (string, cutplan.Intent) {
	t.Helper()
	root := t.TempDir()
	qualificationWrite(t, root, "go.mod", "module "+qualificationModule+"\n\ngo 1.23.3\n")
	qualificationWrite(t, root, "core/link.go", `package core

var trace []string

func mark(name string) int {
	trace = append(trace, name)
	return len(trace)
}

type Link struct {
	Count  int
	Label  int
	Stable int
}

func NewLink() Link {
	return Link{
		Count:  mark("count"),
		Label:  mark("label"),
		Stable: mark("stable"),
	}
}
`)
	qualificationWrite(t, root, "core/exports.go", `package core

type Token struct {
	Name string
}

const DefaultName = "default"

var DefaultToken = Token{Name: DefaultName}

func UseToken(token Token) string {
	return token.Name
}
`)
	qualificationWrite(t, root, "core/link_test.go", `package core

import "testing"

func TestLinkOrder(t *testing.T) {
	trace = nil
	link := NewLink()
	if link.Count != 1 || link.Label != 2 || link.Stable != 3 {
		t.Fatalf("unexpected evaluation order: %#v / %#v", link, trace)
	}
}
`)
	qualificationWrite(t, root, "core/link_external_test.go", `package core_test

import (
	"testing"

	"`+qualificationModule+`/core"
)

func TestExternalToken(t *testing.T) {
	token := core.Token{Name: core.DefaultName}
	if got := core.UseToken(token); got != core.DefaultName {
		t.Fatalf("token = %q", got)
	}
	if core.DefaultToken.Name != core.DefaultName {
		t.Fatalf("default token = %#v", core.DefaultToken)
	}
}
`)
	qualificationWrite(t, root, "core/moved_test.go", `package core

import "testing"

func TestMoved(t *testing.T) {
	if got := len("moved"); got != 5 {
		t.Fatalf("moved length = %d", got)
	}
}

func TestCoreMarker(t *testing.T) {
	if len("core") != 4 {
		t.Fatal("core marker")
	}
}
`)
	qualificationWrite(t, root, "flow/moved_test.go", `package flow

import "testing"

func TestFlowMarker(t *testing.T) {
	if len("flow") != 4 {
		t.Fatal("flow marker")
	}
}
`)
	qualificationWrite(t, root, "consumer/use.go", `package consumer

import "`+qualificationModule+`/core"

func TokenName(token core.Token) string {
	return core.UseToken(token) + core.DefaultToken.Name + core.DefaultName
}
`)
	qualificationCopyRunner(t, root)
	return root, qualificationIntent()
}

func qualificationIntent() cutplan.Intent {
	core := qualificationModule + "/core"
	flow := qualificationModule + "/flow"
	ref := func(object string) cutplan.SymbolRef { return cutplan.SymbolRef{Object: object} }
	corePackage := func(name string) cutplan.SymbolRef { return ref(core + "#package:" + name) }
	flowPackage := func(name string) cutplan.SymbolRef { return ref(flow + "#package:" + name) }
	coreField := func(owner, name string) cutplan.SymbolRef { return ref(core + "#type:" + owner + "/field:" + name) }
	flowField := func(owner, name string) cutplan.SymbolRef { return ref(flow + "#type:" + owner + "/field:" + name) }
	countFrom, countTo := coreField("Link", "Count"), flowField("State", "Count")
	labelFrom, labelTo := coreField("Link", "Label"), flowField("State", "Label")
	state, link, stateType := coreField("Link", "state"), corePackage("Link"), flowPackage("State")
	packageSubjects := []cutplan.Relocation{
		{From: corePackage("Token"), To: flowPackage("Token")},
		{From: corePackage("DefaultName"), To: flowPackage("DefaultName")},
		{From: corePackage("DefaultToken"), To: flowPackage("DefaultToken")},
		{From: corePackage("UseToken"), To: flowPackage("UseToken")},
	}
	packageTargets := make([]cutplan.SymbolRef, 0, len(packageSubjects))
	packageSources := make([]cutplan.SymbolRef, 0, len(packageSubjects))
	for _, subject := range packageSubjects {
		packageSources = append(packageSources, subject.From)
		packageTargets = append(packageTargets, subject.To)
	}
	imports := []cutplan.Import{
		{Consumer: "core/link.go", To: &cutplan.ImportRef{Path: flow, Name: "flow"}, Symbols: []cutplan.SymbolRef{stateType}},
		{Consumer: "consumer/use.go", From: &cutplan.ImportRef{Path: core, Name: "core"}, To: &cutplan.ImportRef{Path: flow, Name: "flow"}, Symbols: packageTargets},
		{Consumer: "core/link_external_test.go", From: &cutplan.ImportRef{Path: core, Name: "core"}, To: &cutplan.ImportRef{Path: flow, Name: "flow"}, Symbols: packageTargets},
	}
	return cutplan.Intent{Schema: cutplan.Version, Name: "synthetic-cross-package-qualification", Operations: []cutplan.Operation{{
		ID: "core-to-flow", Authority: cutplan.Authority{From: "core", To: "flow"},
		Edits: []cutplan.Edit{
			{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{Source: "core/link.go", Destination: cutplan.Destination{Path: "flow/state.go", Package: "flow"}, Subjects: []cutplan.Relocation{{From: countFrom, To: countTo}, {From: labelFrom, To: labelTo}}, Containment: &cutplan.Containment{Parent: link, Child: stateType, Through: state}}},
			{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{Source: "core/exports.go", Destination: cutplan.Destination{Path: "flow/exports.go", Package: "flow"}, Subjects: packageSubjects}},
			{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{Source: "core/moved_test.go", Destination: cutplan.Destination{Path: "flow/moved_test.go", Package: "flow"}, Subjects: []cutplan.Relocation{{From: corePackage("TestMoved"), To: flowPackage("TestMoved")}}}},
		},
		Bindings: []cutplan.Binding{
			{Consumer: "core/link_test.go", From: countFrom, To: countTo, Form: cutplan.BindingField, Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverField, Object: state}}},
			{Consumer: "core/link_test.go", From: labelFrom, To: labelTo, Form: cutplan.BindingField, Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverField, Object: state}}},
			{Consumer: "consumer/use.go", From: packageSubjects[0].From, To: packageSubjects[0].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "consumer/use.go", From: packageSubjects[1].From, To: packageSubjects[1].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "consumer/use.go", From: packageSubjects[2].From, To: packageSubjects[2].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "consumer/use.go", From: packageSubjects[3].From, To: packageSubjects[3].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "core/link_external_test.go", From: packageSubjects[0].From, To: packageSubjects[0].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "core/link_external_test.go", From: packageSubjects[1].From, To: packageSubjects[1].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "core/link_external_test.go", From: packageSubjects[2].From, To: packageSubjects[2].To, Form: cutplan.BindingPackageSelector},
			{Consumer: "core/link_external_test.go", From: packageSubjects[3].From, To: packageSubjects[3].To, Form: cutplan.BindingPackageSelector},
		},
		Imports:   imports,
		Footprint: cutplan.Footprint{Read: []string{"consumer/use.go", "core/exports.go", "core/link.go", "core/link_external_test.go", "core/link_test.go", "core/moved_test.go", "flow/moved_test.go"}, Write: []string{"consumer/use.go", "core/exports.go", "core/link.go", "core/link_external_test.go", "core/link_test.go", "core/moved_test.go", "flow/exports.go", "flow/moved_test.go", "flow/state.go"}},
		Verify:    cutplan.Verification{Laws: []cutplan.Law{{ID: "external-token", Package: "./core", Test: "TestExternalToken"}, {ID: "link-order", Package: "./core", Test: "TestLinkOrder"}, {ID: "moved", Package: "./flow", Test: "TestMoved"}}, Gates: []cutplan.Gate{cutplan.GateDiagnostics, cutplan.GateImportDAG, cutplan.GateResidue}},
	}}}
}

func qualificationCopyRunner(t *testing.T, root string) {
	t.Helper()
	repository := qualificationRepositoryRoot(t)
	source := filepath.Join(repository, "scripts", "bounded_test.sh")
	bytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "scripts", "bounded_test.sh")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, bytes, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, info.Mode()); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != string(bytes) {
		t.Fatalf("fixture bounded runner did not retain exact bytes: %v", err)
	}
	actualInfo, err := os.Stat(target)
	if err != nil || actualInfo.Mode() != info.Mode() {
		t.Fatalf("fixture bounded runner did not retain mode: %v", err)
	}
}

func qualificationRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate qualification test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
}

func qualificationWrite(t *testing.T, root, path, source string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func qualificationSourceDigest(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()
	result := map[string][sha256.Size]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".flashrefactor" || len(relative) > len(".flashrefactor/") && relative[:len(".flashrefactor/")] == ".flashrefactor/" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = sha256.Sum256(bytes)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func qualificationSourceUnchanged(t *testing.T, before, after map[string][sha256.Size]byte, stage string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s changed source denominator", stage)
	}
	var changed []string
	for path, digest := range before {
		if after[path] != digest {
			changed = append(changed, path)
		}
	}
	if len(changed) != 0 {
		sort.Strings(changed)
		t.Fatalf("%s changed source: %v", stage, changed)
	}
}
