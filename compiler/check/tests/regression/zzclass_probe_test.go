package regression

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	value "github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func zzReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func zzClassDescribe(t *testing.T, label string, ty typ.Type) {
	t.Helper()
	switch v := ty.(type) {
	case *typ.Recursive:
		fk, keyed := typ.FamilyKeyOf(v)
		t.Logf("[%s] Recursive name=%q keyed=%v key=%v body=%s", label, v.Name, keyed, fk, func() string {
			if v.Body == nil {
				return "<nil>"
			}
			return v.Body.String()
		}())
	case *typ.Ref:
		t.Logf("[%s] Ref module=%q name=%q", label, v.Module, v.Name)
	case *typ.Alias:
		t.Logf("[%s] Alias -> %s", label, v.UnaliasedTarget().String())
		zzClassDescribe(t, label+"/target", v.UnaliasedTarget())
	default:
		t.Logf("[%s] %T: %s", label, ty, ty.String())
	}
}

// zzClassStore is the store.lua module: a self-method class exported with both
// the type alias (M.Store) and the constructor (M.new(): Store).
const zzClassStore = `
type Store = {
    cache: {[string]: string},
    get: (self: Store, key: string) -> string?,
    put: (self: Store, key: string, value: string) -> Store,
}

local Store = {}
Store.__index = Store

local M = {}
M.Store = Store

function M.new(): Store
    local self: Store = {
        cache = {},
        get = Store.get,
        put = Store.put,
    }
    setmetatable(self, Store)
    return self
end

function Store:get(key: string): string?
    return self.cache[key]
end

function Store:put(key: string, value: string): Store
    self.cache[key] = value
    return self
end

return M
`

func zzClassCheck(t *testing.T, label, readerProbe string) {
	t.Helper()
	opts := []testutil.Option{testutil.WithStdlib()}
	storeMod := testutil.CheckAndExport(zzClassStore, "store", opts...)
	if storeMod.HasError() {
		t.Logf("[%s] store export errors: %v", label, testutil.ErrorMessages(storeMod.Errors))
	}
	readerOpts := append([]testutil.Option{}, opts...)
	readerOpts = append(readerOpts, testutil.WithModule("store", storeMod))
	r := testutil.Check(readerProbe, readerOpts...)
	if !r.HasError() {
		t.Logf("[%s] NO ERROR", label)
		return
	}
	for _, e := range r.Errors {
		t.Logf("[%s] err: %s @ %d:%d", label, e.Message, e.Position.Line, e.Position.Column)
	}
}

// zzClassExportPair returns the new()-return type and the Store alias target of a
// module's exported manifest so a probe can compare constructor identity vs alias.
func zzClassExportPair(t *testing.T, mod *testutil.ModuleResult) (newReturn typ.Type, alias typ.Type) {
	t.Helper()
	m := mod.Manifest
	alias = m.Types["Store"]
	if rec, ok := m.EnrichedExport().(*typ.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == "new" {
				if fn, ok := f.Type.(*typ.Function); ok && len(fn.Returns) == 1 {
					newReturn = fn.Returns[0]
				}
			}
		}
	}
	return newReturn, alias
}

// TestZZClass_ContaminationPair dumps the constructor-return vs alias of the
// self-method store both in isolation and after a contaminating first export,
// with the subtype verdicts, to localize where identity diverges.
func TestZZClass_ContaminationPair(t *testing.T) {
	dump := func(label string, mod *testutil.ModuleResult) {
		nr, al := zzClassExportPair(t, mod)
		t.Logf("[%s] new-return: %v (%T)", label, fmtT(nr), nr)
		t.Logf("[%s] alias:      %v (%T)", label, fmtT(al), al)
		if nr != nil && al != nil {
			t.Logf("[%s] new<:alias=%v  alias<:new=%v  consistent=%v  equals=%v",
				label,
				subtype.IsSubtype(nr, al), subtype.IsSubtype(al, nr),
				subtype.Consistent(nr, al), typ.TypeEquals(nr, al))
		}
	}

	clean := testutil.CheckAndExport(zzClassStore, "store",
		testutil.WithStdlib())
	dump("clean", clean)

	_ = testutil.CheckAndExport(zzRecordStore, "recstore",
		testutil.WithStdlib())
	after := testutil.CheckAndExport(zzClassStore, "store",
		testutil.WithStdlib())
	dump("after-contam", after)
}

func fmtT(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	if a, ok := t.(*typ.Alias); ok {
		t = a.UnaliasedTarget()
	}
	if rec, ok := t.(*typ.Recursive); ok && rec.Body != nil {
		return rec.Name + " => " + rec.Body.String()
	}
	return t.String()
}

// TestZZClass_SelfImportTwice runs the self-method importer path TWICE with no
// other module in between, to test whether a process-global advances state that
// flips a clean run to an error on a repeat.
func TestZZClass_SelfImportTwice(t *testing.T) {
	run := func(label string) {
		base := []testutil.Option{
			testutil.WithStdlib(),
		}
		storeMod := testutil.CheckAndExport(zzClassStore, "store", base...)
		entryOpts := append(append([]testutil.Option{}, base...), testutil.WithModule("store", storeMod))
		r := testutil.Check(`
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`, entryOpts...)
		if !r.HasError() {
			t.Logf("[%s] NO ERROR", label)
			return
		}
		for _, e := range r.Errors {
			t.Logf("[%s] err: %s @ %d:%d", label, e.Message, e.Position.Line, e.Position.Column)
		}
	}
	run("first")
	run("second")
	run("third")
}

// TestZZClass_MapFieldIndexInMethod isolates the standalone map-index bug: a
// class method indexes a map-typed field of self (self.sessions[id]). This must
// type-check cleanly; the oracle reports "cannot index type {[string]: ...}".
func TestZZClass_MapFieldIndexInMethod(t *testing.T) {
	src := `
type Snapshot = { id: string }
type Store = {
    sessions: {[string]: Snapshot},
    open: (self: Store, id: string) -> Snapshot?,
}
local Store = {}
Store.__index = Store
local M = {}
function M.new(): Store
    local self: Store = { sessions = {}, open = Store.open }
    setmetatable(self, Store)
    return self
end
function Store:open(id: string): Snapshot?
    local existing = self.sessions[id]
    return existing
end
return M
`
	r := testutil.Check(src, testutil.WithStdlib())
	if !r.HasError() {
		t.Logf("[map-index] NO ERROR")
		return
	}
	for _, e := range r.Errors {
		t.Logf("[map-index] err: %s @ %d:%d", e.Message, e.Position.Line, e.Position.Column)
	}
}

// TestZZClass_ImportedMapValue reproduces map-of-time-record-store standalone:
// a class whose map field value is an IMPORTED record alias (protocol.Snapshot),
// indexed inside a method. The export compilation of store.lua must be clean.
func TestZZClass_ImportedMapValue(t *testing.T) {
	protocolSrc := `
type Snapshot = {
    id: string,
    last_value: string?,
    flags: {[string]: boolean},
}
local M = {}
M.Snapshot = Snapshot
return M
`
	storeSrc := `
local protocol = require("protocol")

type Store = {
    sessions: {[string]: protocol.Snapshot},
    open: (self: Store, id: string) -> protocol.Snapshot,
    get: (self: Store, id: string) -> protocol.Snapshot?,
}
local Store = {}
Store.__index = Store
local M = {}
M.Store = Store
function M.new(): Store
    local self: Store = { sessions = {}, open = Store.open, get = Store.get }
    setmetatable(self, Store)
    return self
end
function Store:open(id: string): protocol.Snapshot
    local existing = self.sessions[id]
    if existing then
        return existing
    end
    local created: protocol.Snapshot = {
        id = id,
        last_value = nil,
        flags = {},
    }
    self.sessions[id] = created
    return created
end
function Store:get(id: string): protocol.Snapshot?
    return self.sessions[id]
end
return M
`
	base := []testutil.Option{testutil.WithStdlib()}
	proto := testutil.CheckAndExport(protocolSrc, "protocol", base...)
	t.Logf("[imported-map] protocol errors: %v", testutil.ErrorMessages(proto.Errors))
	storeOpts := append(append([]testutil.Option{}, base...), testutil.WithModule("protocol", proto))
	store := testutil.CheckAndExport(storeSrc, "store", storeOpts...)
	if len(store.Errors) == 0 {
		t.Logf("[imported-map] store: NO ERROR")
		return
	}
	for _, e := range store.Errors {
		t.Logf("[imported-map] store err: %s @ %d:%d", e.Message, e.Position.Line, e.Position.Column)
	}
}

// zzCleanOtherStore defines a DIFFERENT, well-typed class also named Store, with
// no errors, to test whether merely interning a same-named family in a prior
// compilation contaminates a later one.
const zzCleanOtherStore = `
type Store = {
    items: {[number]: string},
    add: (self: Store, v: string) -> (),
}
local Store = {}
Store.__index = Store
local M = {}
M.Store = Store
function M.new(): Store
    local self: Store = { items = {}, add = Store.add }
    setmetatable(self, Store)
    return self
end
function Store:add(v: string)
    self.items[#self.items + 1] = v
end
return M
`

// TestZZClass_ContaminationClean runs a CLEAN, differently-bodied Store first,
// then the self-method Store importer, to isolate whether contamination needs an
// errored first module or just a same-named family.
func TestZZClass_ContaminationClean(t *testing.T) {
	first := testutil.CheckAndExport(zzCleanOtherStore, "other",
		testutil.WithStdlib())
	t.Logf("[clean-first] other errors: %v", testutil.ErrorMessages(first.Errors))
	zzClassCheck(t, "clean-first self-method", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`)
}

// TestZZClass_RealFamilyCollapse takes the actual checked new()-return types of
// two distinct clean Store modules and pushes them through the value-domain
// canonicalization + product interner to observe whether they collapse to one
// representative (the cross-module contamination vector).
func TestZZClass_RealFamilyCollapse(t *testing.T) {
	base := []testutil.Option{testutil.WithStdlib()}
	// Check self FIRST so its family is interned before any other Store family.
	self := testutil.CheckAndExport(zzClassStore, "store", base...)
	other := testutil.CheckAndExport(zzCleanOtherStore, "other", base...)

	nrOther, _ := zzClassExportPair(t, other)
	nrSelf, _ := zzClassExportPair(t, self)
	unwrap := func(x typ.Type) typ.Type {
		if a, ok := x.(*typ.Alias); ok {
			return a.UnaliasedTarget()
		}
		return x
	}
	tOther := unwrap(nrOther)
	tSelf := unwrap(nrSelf)
	t.Logf("other new-return body: %v", fmtT(nrOther))
	t.Logf("self  new-return body: %v", fmtT(nrSelf))
	if ro, ok := tOther.(*typ.Recursive); ok {
		fk, keyed := typ.FamilyKeyOf(ro)
		t.Logf("other keyed=%v key=%v", keyed, fk)
	}
	if rs, ok := tSelf.(*typ.Recursive); ok {
		fk, keyed := typ.FamilyKeyOf(rs)
		t.Logf("self  keyed=%v key=%v", keyed, fk)
	}

	t.Logf("SameConvergedFact(other,self)=%v", value.SameConvergedFact(tOther, tSelf))
	t.Logf("TypeEquals(other,self)=%v", typ.TypeEquals(tOther, tSelf))
	if ro, ok := tOther.(*typ.Recursive); ok {
		if rs, ok := tSelf.(*typ.Recursive); ok {
			t.Logf("bodies TypeEquals=%v", typ.TypeEquals(ro.Body, rs.Body))
			t.Logf("other.Body=%T self.Body=%T", ro.Body, rs.Body)
		}
	}
	repOther := value.CanonicalRecursiveFamily(tOther)
	repSelf := value.CanonicalRecursiveFamily(tSelf)
	t.Logf("family rep other: %p  self: %p  same=%v", repOther, repSelf, repOther == repSelf)
	t.Logf("phash other=%d self=%d", typ.ProductFamilyHash(tOther), typ.ProductFamilyHash(tSelf))

	avOther := product.FromType(tOther)
	avSelf := product.FromType(tSelf)
	t.Logf("product proj other: %s", avOther.ProjectValue().String())
	t.Logf("product proj self:  %s", avSelf.ProjectValue().String())
	t.Logf("product Equal(other,self)=%v", avOther.Equal(avSelf))
}

func TestZZClass_DumpExport(t *testing.T) {
	label := "flow"
	storeMod := testutil.CheckAndExport(zzClassStore, "store", testutil.WithStdlib())
	m := storeMod.Manifest
	t.Logf("[%s] Export type: %s (%T)", label, m.Export.String(), m.Export)
	aliasStore := m.Types["Store"]
	t.Logf("[%s] Type \"Store\": %s (%T)", label, aliasStore.String(), aliasStore)
	zzClassDescribe(t, label+"/alias-Store", aliasStore)
	// pull the return type of new() out of the export record
	if rec, ok := m.Export.(*typ.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == "new" {
				if fn, ok := f.Type.(*typ.Function); ok && len(fn.Returns) == 1 {
					zzClassDescribe(t, label+"/new-return", fn.Returns[0])
					t.Logf("[%s] alias==newReturn ptr? %v consistent? %v",
						label, aliasStore == fn.Returns[0], subtype.Consistent(fn.Returns[0], aliasStore))
				}
			}
		}
	}
	// Enriched export: what an importer's require() actually receives.
	enriched := m.EnrichedExport()
	t.Logf("[%s] Enriched export: %s (%T)", label, enriched.String(), enriched)
	if rec, ok := enriched.(*typ.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == "new" {
				if fn, ok := f.Type.(*typ.Function); ok && len(fn.Returns) == 1 {
					zzClassDescribe(t, label+"/enriched-new-return", fn.Returns[0])
					t.Logf("[%s] enriched: alias consistent newReturn? %v",
						label, subtype.Consistent(fn.Returns[0], aliasStore))
				}
			}
		}
	}
}

// Contamination check: run a map-of-record store (with Snapshot) THEN the
// self-method store in the same process, to see if global state leaks Snapshot.
const zzRecordStore = `
type Snapshot = {
    id: string,
    last_value: string?,
    flags: {[string]: boolean},
}
type Store = {
    sessions: {[string]: Snapshot},
    put: (self: Store, id: string, snapshot: Snapshot) -> (),
    get: (self: Store, id: string) -> Snapshot?,
}
local Store = {}
Store.__index = Store
local M = {}
M.Store = Store
function M.new(): Store
    local self: Store = { sessions = {}, put = Store.put, get = Store.get }
    setmetatable(self, Store)
    return self
end
function M.put(self: Store, id: string, snapshot: Snapshot)
    self.sessions[id] = snapshot
end
function M.get(self: Store, id: string): Snapshot?
    return self.sessions[id]
end
return M
`

func TestZZClass_Contamination(t *testing.T) {
	// First check a record store with Snapshot.
	rec := testutil.CheckAndExport(zzRecordStore, "store",
		testutil.WithStdlib())
	t.Logf("record store errors: %v", testutil.ErrorMessages(rec.Errors))
	// Then the self-method store with the same module name "store".
	zzClassCheck(t, "contamination self-method", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`)
}

// Mirror the exact oracle path for self-method-store: export store.lua then
// check main.lua with stdlib, reading the real fixture files.
func TestZZClass_OraclePath_SelfMethod(t *testing.T) {
	dir := "../../../../testdata/fixtures/modules/imported-self-method-store"
	storeSrc := zzReadFile(t, dir+"/store.lua")
	mainSrc := zzReadFile(t, dir+"/main.lua")
	base := []testutil.Option{
		testutil.WithStdlib(),
	}
	storeMod := testutil.CheckAndExport(storeSrc, "store", base...)
	t.Logf("store export errors: %v", testutil.ErrorMessages(storeMod.Errors))
	entryOpts := append(append([]testutil.Option{}, base...), testutil.WithModule("store", storeMod))
	r := testutil.Check(mainSrc, entryOpts...)
	for _, e := range r.Errors {
		t.Logf("main err: %s @ %d:%d", e.Message, e.Position.Line, e.Position.Column)
	}
	if !r.HasError() {
		t.Logf("main: NO ERROR")
	}
}

// Dump the manifest produced by the SECOND export after a first contaminating
// export, to see whether the self-method Store body picks up Snapshot.
func TestZZClass_Contamination_DumpManifest(t *testing.T) {
	_ = testutil.CheckAndExport(zzRecordStore, "store",
		testutil.WithStdlib())
	second := testutil.CheckAndExport(zzClassStore, "store",
		testutil.WithStdlib())
	m := second.Manifest
	t.Logf("second export type: %s", m.Export.String())
	if st := m.Types["Store"]; st != nil {
		t.Logf("second Store alias: %s", st.String())
		if a, ok := st.(*typ.Alias); ok {
			t.Logf("second Store target: %s", a.UnaliasedTarget().String())
		}
	}
}

// Same as above but first check uses a DIFFERENT module name to rule out
// name-collision in a global manifest registry.
func TestZZClass_Contamination_DiffName(t *testing.T) {
	rec := testutil.CheckAndExport(zzRecordStore, "recstore",
		testutil.WithStdlib())
	t.Logf("record store errors: %v", testutil.ErrorMessages(rec.Errors))
	zzClassCheck(t, "diffname self-method", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`)
}

// First check WITHOUT stdlib.
func TestZZClass_Contamination_NoStdlib(t *testing.T) {
	r := testutil.Check(zzRecordStore)
	t.Logf("record store nostdlib errors: %v", testutil.ErrorMessages(r.Errors))
	zzClassCheck(t, "nostdlib self-method", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`)
}

// First check the record store but DO NOT export; rule out export-side state.
func TestZZClass_Contamination_NoExport(t *testing.T) {
	r := testutil.Check(zzRecordStore, testutil.WithStdlib())
	t.Logf("record store inline errors: %v", testutil.ErrorMessages(r.Errors))
	zzClassCheck(t, "noexport self-method", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
store:put("name", "lua")
`)
}

// Isolate: assign new() to an UNannotated local, then call methods.
func TestZZClass_NoAnnot(t *testing.T) {
	zzClassCheck(t, "no-annot", `
local store_mod = require("store")
local store = store_mod.new()
store:put("name", "lua")
`)
}

// Isolate: just the annotated local (no method call) to test the assign alone.
func TestZZClass_AnnotOnly(t *testing.T) {
	zzClassCheck(t, "annot-only", `
local store_mod = require("store")
local store: store_mod.Store = store_mod.new()
`)
}

func TestZZClass_SelfMethodStore(t *testing.T) {
	zzClassCheck(t, "self-method", `
local store_mod = require("store")

local store: store_mod.Store = store_mod.new()
store:put("name", "lua")

local maybe_name = store:get("name")
if maybe_name then
    local value: string = maybe_name
end
`)
}
