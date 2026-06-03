package lua

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func zzTypeStr(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	id := uint64(0)
	keyed := false
	var fk typ.FamilyKey
	if rec, ok := t.(*typ.Recursive); ok {
		id = rec.ID
		if k, ok2 := typ.FamilyKeyOf(rec); ok2 {
			keyed = true
			fk = k
		}
	}
	return typ.FormatShort(t) + " [recID=" + zzU(id) + " keyed=" + zzB(keyed) + " key=" + fk.String() + "]"
}

func zzU(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func zzB(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func zztypeEquals(a, b typ.Type) bool { return typ.TypeEquals(a, b) }

func zzExtractNewReturn(export typ.Type) typ.Type {
	rec := unwrap.Record(export)
	if rec == nil {
		return nil
	}
	f := rec.GetField("new")
	if f == nil {
		return nil
	}
	fn := unwrap.Function(f.Type)
	if fn == nil || len(fn.Returns) == 0 {
		return nil
	}
	return fn.Returns[0]
}

// zzProbeFixture runs one fixture's full check phase (deps then entry) and
// surfaces the panic stack trace when a panic occurs, so the cross-module class
// identity bug is debuggable with a real trace.
func zzProbeFixture(t *testing.T, name string) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	var target namedSuite
	found := false
	for _, s := range suites {
		if s.Name == name {
			target = s
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fixture %q not found", name)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Logf("PANIC in %s: %v\n%s", name, r, debug.Stack())
		}
	}()
	diags, entry := zzProbeDiags(target)
	t.Logf("entry=%s diagcount=%d", entry, len(diags))
	for _, d := range diags {
		t.Logf("DIAG %s:%d:%d [%s] %s", d.file, d.line, d.column, d.code, d.message)
	}
}

func zzProbeDiags(s namedSuite) (diags []diagAlias, entryFile string) {
	files := resolveFiles(s)
	stdlib := resolveStdlib(s)
	var baseOpts []testutil.Option
	if stdlib {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		if m := resolvePackageManifest(pkg); m != nil {
			baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
		}
	}
	sources := make(map[string]string)
	for _, f := range files {
		sources[f] = readFixtureFile(s.Dir, f)
	}
	type namedModule struct {
		name string
		mod  *testutil.ModuleResult
	}
	var moduleOrder []namedModule
	for _, f := range files[:len(files)-1] {
		modOpts := append([]testutil.Option{}, baseOpts...)
		for _, nm := range moduleOrder {
			modOpts = append(modOpts, testutil.WithModule(nm.name, nm.mod))
		}
		name := strings.TrimSuffix(f, ".lua")
		mod := testutil.CheckAndExport(sources[f], name, modOpts...)
		moduleOrder = append(moduleOrder, namedModule{name, mod})
		for _, d := range mod.Errors {
			diags = append(diags, diagAlias{d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message})
		}
	}
	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, nm := range moduleOrder {
		entryOpts = append(entryOpts, testutil.WithModule(nm.name, nm.mod))
	}
	entryFile = files[len(files)-1]
	result := testutil.Check(sources[entryFile], entryOpts...)
	for _, d := range result.Diagnostics {
		diags = append(diags, diagAlias{d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message})
	}
	return diags, entryFile
}

type diagAlias struct {
	file    string
	line    int
	column  int
	code    string
	message string
}

// TestZZExportStoreManifest exports store.lua and reports whether the exported
// M.new return type and Types["Store"] are the same recursive family.
func TestZZExportStoreManifest(t *testing.T) {
	src := readFixtureFile("testdata/fixtures/modules/imported-self-method-store", "store.lua")
	mod := testutil.CheckAndExport(src, "store",
		testutil.WithStdlib())
	m := mod.Manifest
	if m == nil {
		t.Fatalf("nil manifest")
	}
	storeT := m.Types["Store"]
	t.Logf("Types[Store] = %s", zzTypeStr(storeT))
	t.Logf("Export       = %s", zzTypeStr(m.Export))
	newRet := zzExtractNewReturn(m.Export)
	t.Logf("new() return = %s", zzTypeStr(newRet))
	if newRet != nil && storeT != nil {
		t.Logf("TypeEquals(new-return, Types[Store]) = %v", zztypeEquals(newRet, storeT))
	}
	for _, e := range mod.Errors {
		t.Logf("EXPORT-ERR %s:%d [%s] %s", e.Position.File, e.Position.Line, e.Code.Name(), e.Message)
	}

	enriched := m.EnrichedExport()
	t.Logf("EnrichedExport = %s", zzTypeStr(enriched))
	enrNew := zzExtractNewReturn(enriched)
	t.Logf("enriched new() return = %s", zzTypeStr(enrNew))
	if enrNew != nil && storeT != nil {
		t.Logf("TypeEquals(enriched-new-return, Types[Store]) = %v", zztypeEquals(enrNew, storeT))
	}
	// Unwrap aliases to inspect the inner recursive family identity.
	t.Logf("inner Types[Store] = %s", zzTypeStr(zzUnalias(storeT)))
	t.Logf("inner enriched-new = %s", zzTypeStr(zzUnalias(enrNew)))
}

// TestZZCompareStoreFamilies exports both Store-defining fixtures' producer
// modules and reports the resolved Store family bodies + collision status.
func TestZZCompareStoreFamilies(t *testing.T) {
	exp := func(dir, file, name string) typ.Type {
		src := readFixtureFile(dir, file)
		// Resolve protocol dep for the map-of-record store.
		var opts []testutil.Option
		opts = append(opts, testutil.WithStdlib())
		if name == "store2" {
			psrc := readFixtureFile("testdata/fixtures/modules/imported-map-of-record-store", "protocol.lua")
			pmod := testutil.CheckAndExport(psrc, "protocol", testutil.WithStdlib())
			opts = append(opts, testutil.WithModule("protocol", pmod))
		}
		mod := testutil.CheckAndExport(src, name, opts...)
		return mod.Manifest.Types["Store"]
	}
	s1 := exp("testdata/fixtures/modules/imported-self-method-store", "store.lua", "store1")
	s2 := exp("testdata/fixtures/modules/imported-map-of-record-store", "store.lua", "store2")
	t.Logf("store1 = %s", zzTypeStr(zzUnalias(s1)))
	t.Logf("store2 = %s", zzTypeStr(zzUnalias(s2)))
	if r1, ok := zzUnalias(s1).(*typ.Recursive); ok {
		t.Logf("store1 body = %s", typ.FormatShort(r1.Body))
	}
	if r2, ok := zzUnalias(s2).(*typ.Recursive); ok {
		t.Logf("store2 body = %s", typ.FormatShort(r2.Body))
	}
	t.Logf("TypeEquals(s1,s2)=%v", zztypeEquals(s1, s2))
	t.Logf("phash s1=%d s2=%d", typ.ProductFamilyHash(zzUnalias(s1)), typ.ProductFamilyHash(zzUnalias(s2)))
	rep2 := value.CanonicalRecursiveFamily(zzUnalias(s2))
	rep1 := value.CanonicalRecursiveFamily(zzUnalias(s1))
	t.Logf("rep1==rep2 COLLISION=%v", rep1 == rep2)
	t.Logf("rep1 = %s", typ.FormatShort(rep1))
	t.Logf("rep2 = %s", typ.FormatShort(rep2))
	t.Logf("SameConvergedFact(s2,s1)=%v", value.SameConvergedFact(zzUnalias(s2), zzUnalias(s1)))
}

func zzUnalias(t typ.Type) typ.Type {
	for {
		al, ok := t.(*typ.Alias)
		if !ok || al.Target == nil {
			return t
		}
		t = al.Target
	}
}

// TestZZImporterStore reproduces the importer mismatch deterministically by
// interning the Snapshot family from imported-map-of-record-store into the
// process-global value interner first, then running imported-self-method-store.
func TestZZImporterStore(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	pollute := func(name string) {
		for _, s := range suites {
			if s.Name != name {
				continue
			}
			func() {
				defer func() { _ = recover() }()
				_, _ = zzProbeDiags(s)
			}()
		}
	}
	pollute("modules/imported-map-of-record-store")
	pollute("modules/imported-map-of-time-record-store")
	for _, s := range suites {
		if s.Name != "modules/imported-self-method-store" {
			continue
		}
		diags, entry := zzProbeDiags(s)
		t.Logf("[importer] entry=%s diagcount=%d", entry, len(diags))
		for _, d := range diags {
			t.Logf("DIAG %s:%d:%d [%s] %s", d.file, d.line, d.column, d.code, d.message)
		}
	}
}

func TestZZClassProbePanic(t *testing.T) {
	zzProbeFixture(t, "realworld/plugin-runtime-pipeline")
}

func TestZZClassProbeStore(t *testing.T) {
	zzProbeFixture(t, "modules/imported-self-method-store")
}

// TestZZClassProbeStoreAfterOthers reproduces the oracle's failure by running
// several module fixtures first, so the process-global recursive-ID counter has
// advanced before the store fixture, exposing whether the mismatch is
// counter-order sensitive.
func TestZZClassProbeStoreAfterOthers(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, s := range suites {
		if !strings.HasPrefix(s.Name, "modules/") {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			_, _ = zzProbeDiags(s)
		}()
		if s.Name == "modules/imported-self-method-store" {
			diags, entry := zzProbeDiags(s)
			t.Logf("[after-others] entry=%s diagcount=%d", entry, len(diags))
			for _, d := range diags {
				t.Logf("DIAG %s:%d:%d [%s] %s", d.file, d.line, d.column, d.code, d.message)
			}
		}
	}
}
