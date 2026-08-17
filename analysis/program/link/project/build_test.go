package project

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestMountsCanonicalAndAmbiguousContentFailsClosed(t *testing.T) {
	left := projectProgram(t, `local value = {left = 1}`)
	right := projectProgram(t, `local value = {right = 2}`)
	first := projectDraft(t, []Module{{Name: "right", Program: right}, {Name: "left", Program: left}})
	second := projectDraft(t, []Module{{Name: "left", Program: left}, {Name: "right", Program: right}})
	for index := 0; index < first.Mounts().Count(); index++ {
		firstShard, firstOK := first.Mounts().At(index)
		secondShard, secondOK := second.Mounts().At(index)
		firstOrdinal, firstOrdinalOK := first.Mounts().Index(firstShard)
		secondOrdinal, secondOrdinalOK := second.Mounts().Index(secondShard)
		if !firstOK || !secondOK || !firstOrdinalOK || !secondOrdinalOK || firstOrdinal != secondOrdinal {
			t.Fatalf("canonical shard order differs at %d", index)
		}
		firstName, _ := first.Mounts().Name(firstShard)
		secondName, _ := second.Mounts().Name(secondShard)
		if firstName != secondName {
			t.Fatalf("canonical mount name differs: %q != %q", firstName, secondName)
		}
	}
	ambiguous := projectDraft(t, []Module{{Name: "first", Program: left}, {Name: "second", Program: left}})
	for index := 0; index < ambiguous.Mounts().Count(); index++ {
		shard, ok := ambiguous.Mounts().At(index)
		owner, ownerOK := ambiguous.Mounts().Program(shard)
		if !ok || !ownerOK || owner != left {
			t.Fatalf("duplicate Program mount %d was not independently addressable", index)
		}
	}
}

func TestColdContentReplaysAcrossInputPermutationAndObservesTarget(t *testing.T) {
	left := projectProgram(t, `local value = {left = 1}`)
	right := projectProgram(t, `local value = {right = 2}`)
	contract := projectTarget(t, "GlobalEnvRoot")
	first, err := Build(Input{Modules: []Module{{Name: "right", Program: right}, {Name: "left", Program: left}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(Input{Modules: []Module{{Name: "left", Program: left}, {Name: "right", Program: right}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	want := first.Cold().ContentID()
	if !want.Available() || want != second.Cold().ContentID() {
		t.Fatal("Project content changed under canonical input permutation")
	}
	if first.Cold().TargetID() != contract.ContentID() {
		t.Fatal("Project did not retain its exact Target constituent")
	}
	component, err := first.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if got := component.Cold().ContentID(); got != want {
		t.Fatal("Project content changed during finalization")
	}
	alternate, err := Build(Input{Modules: []Module{{Name: "left", Program: left}, {Name: "right", Program: right}}, Target: projectTarget(t, "AlternateGlobalEnvRoot")})
	if err != nil {
		t.Fatal(err)
	}
	if alternate.Cold().ContentID() == want {
		t.Fatal("Project content omitted Target identity")
	}
	renamed, err := Build(Input{Modules: []Module{{Name: "renamed", Program: left}, {Name: "right", Program: right}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Cold().ContentID() == want {
		t.Fatal("Project content omitted mount name")
	}
	replaced := projectProgram(t, `local value = {replacement = 3}`)
	changed, err := Build(Input{Modules: []Module{{Name: "left", Program: replaced}, {Name: "right", Program: right}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Cold().ContentID() == want {
		t.Fatal("Project content omitted Program identity")
	}
}

func TestMountRelationIDIsStableDetachedAndOwnerFenced(t *testing.T) {
	p := projectProgram(t, `local value = {needle = 1}; local function f() end; f(); return value.needle`)
	modules := []Module{{Name: "main", Program: p}}
	contract := projectTarget(t, "GlobalEnvRoot")
	seal := func(targetContract *target.Contract, mounted []Module) *Component {
		t.Helper()
		draft, err := Build(Input{Modules: mounted, Target: targetContract})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}

	first := seal(contract, modules)
	second := seal(contract, modules)
	firstID, firstOK := first.MountRelationID()
	secondID, secondOK := second.MountRelationID()
	if !firstOK || !secondOK || !firstID.Available() || firstID != secondID {
		t.Fatal("equivalent Project reseal changed Mount relation identity")
	}
	firstShard, firstShardOK := first.Mounts().At(0)
	secondShard, secondShardOK := second.Mounts().At(0)
	if !firstShardOK || !secondShardOK {
		t.Fatal("equivalent Project reseal lost mounted Shard")
	}
	if _, accepted := first.Mounts().Index(secondShard); accepted || firstShard == secondShard {
		t.Fatal("stable Mount relation identity collapsed hot Shard owner fences")
	}

	targetChanged := seal(projectTarget(t, "AlternateGlobalEnvRoot"), modules)
	targetChangedID, targetChangedOK := targetChanged.MountRelationID()
	if !targetChangedOK || targetChangedID != firstID {
		t.Fatal("Target-only change widened Mount relation identity")
	}
	if targetChanged.Cold().ContentID() == first.Cold().ContentID() {
		t.Fatal("Target-only fixture did not change enclosing Project identity")
	}

	renamed := seal(contract, []Module{{Name: "renamed", Program: p}})
	renamedID, renamedOK := renamed.MountRelationID()
	if !renamedOK || renamedID == firstID {
		t.Fatal("mount-name delta did not change Mount relation identity")
	}

	var nilComponent *Component
	if _, ok := nilComponent.MountRelationID(); ok {
		t.Fatal("nil Component exposed a Mount relation identity")
	}
	if _, ok := (&Component{}).MountRelationID(); ok {
		t.Fatal("authority-free Component exposed a Mount relation identity")
	}
}

func TestModuleKeyIsOneDependencyLocalMountRow(t *testing.T) {
	left := projectProgram(t, `local value = {left = 1}`)
	right := projectProgram(t, `local value = {right = 2}`)
	contract := projectTarget(t, "GlobalEnvRoot")
	seal := func(targetContract *target.Contract, modules []Module) *Component {
		t.Helper()
		draft, err := Build(Input{Modules: modules, Target: targetContract})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	keyForName := func(component *Component, name string) (identity.ContentID, int) {
		t.Helper()
		for index := 0; index < component.Mounts().Count(); index++ {
			shard, shardOK := component.Mounts().At(index)
			gotName, nameOK := component.Mounts().Name(shard)
			if shardOK && nameOK && gotName == name {
				key, keyOK := component.ModuleKey(shard)
				if !keyOK || !key.Available() {
					t.Fatalf("ModuleKey(%q) unavailable", name)
				}
				return key, index
			}
		}
		t.Fatalf("mount %q unavailable", name)
		return identity.ContentID{}, -1
	}

	baseModules := []Module{{Name: "left", Program: left}, {Name: "right", Program: right}}
	base := seal(contract, baseModules)
	permuted := seal(contract, []Module{{Name: "right", Program: right}, {Name: "left", Program: left}})
	targetChanged := seal(projectTarget(t, "AlternateGlobalEnvRoot"), baseModules)
	// A second mount of left sorts before the existing left row and therefore
	// changes its dense Shard ordinal. That physical movement must not rename
	// either pre-existing authored mount.
	expanded := seal(contract, []Module{{Name: "aaa", Program: left}, {Name: "left", Program: left}, {Name: "right", Program: right}})

	leftKey, leftIndex := keyForName(base, "left")
	rightKey, _ := keyForName(base, "right")
	for label, component := range map[string]*Component{"permuted": permuted, "target": targetChanged, "expanded": expanded} {
		gotLeft, gotLeftIndex := keyForName(component, "left")
		gotRight, _ := keyForName(component, "right")
		if gotLeft != leftKey || gotRight != rightKey {
			t.Fatalf("%s change renamed an unaffected mount row", label)
		}
		if label == "expanded" && gotLeftIndex == leftIndex {
			t.Fatal("expanded fixture did not move the left dense ordinal")
		}
	}

	duplicateKey, _ := keyForName(expanded, "aaa")
	if duplicateKey == leftKey {
		t.Fatal("two authored names mounting the same Program collapsed")
	}
	renamed := seal(contract, []Module{{Name: "renamed", Program: left}, {Name: "right", Program: right}})
	renamedKey, _ := keyForName(renamed, "renamed")
	if renamedKey == leftKey {
		t.Fatal("mount rename did not change its local identity")
	}
	replacement := projectProgram(t, `local value = {replacement = 3}`)
	replaced := seal(contract, []Module{{Name: "left", Program: replacement}, {Name: "right", Program: right}})
	replacedKey, _ := keyForName(replaced, "left")
	if replacedKey == leftKey {
		t.Fatal("mounted Program change did not change its local identity")
	}

	baseShard, _ := base.Mounts().At(leftIndex)
	permutedShard, _ := permuted.Mounts().At(leftIndex)
	if _, ok := base.ModuleKey(permutedShard); ok || baseShard == permutedShard {
		t.Fatal("stable scalar ModuleKey collapsed exact hot Shard ownership")
	}
}

func TestProjectContentFailsClosedForUnavailableOrNoncanonicalConstituents(t *testing.T) {
	p := projectProgram(t, `return 1`)
	id := p.ContentID()
	if got := contentID(identity.ContentID{}, []mountRow{{name: "main", program: p, id: id}}); got.Available() {
		t.Fatal("unavailable Target identity admitted")
	}
	contract := projectTarget(t, "GlobalEnvRoot")
	if got := contentID(contract.ContentID(), []mountRow{{name: "z", program: p, id: id}, {name: "a", program: p, id: id}}); got.Available() {
		t.Fatal("noncanonical mount order admitted")
	}
	if got := contentID(contract.ContentID(), []mountRow{{name: "main", program: p, id: identity.ContentID{1}}}); got.Available() {
		t.Fatal("mismatched Program identity admitted")
	}
}

func TestBaseApplicationsHaveOneExactTypedSource(t *testing.T) {
	p := projectProgram(t, `
		local left, right = {}, {}
		local unary, length = -1, #left
		local arithmetic, bitwise = 1 + 2, 1 & 2
		local concat, equal = "a" .. "b", left == right
		local less, lessEqual = left < right, left <= right
		local get = left.value
		left.value = get
		for key, value in pairs(left) do get = value end
		local dependency = require("dependency")
	`)
	apps := projectDraft(t, []Module{{Name: "main", Program: p}}).Applications()
	operators := apps.Operators()
	classifiers := []func(Application) (Shard, keyspace.Term, bool){
		apps.Call,
		operators.UnaryNumeric,
		operators.Length,
		operators.Arithmetic,
		operators.Bitwise,
		operators.Concat,
		operators.Equality,
		operators.OrderPrimary,
		operators.OrderFallback,
		operators.IndexGet,
		operators.IndexSet,
		apps.Generic,
	}
	bases := apps.Bases()
	for index := 0; index < bases.Count(); index++ {
		application, ok := bases.At(index)
		if !ok {
			t.Fatalf("base %d unavailable", index)
		}
		row, rowOK := apps.application(application)
		matches := 0
		for _, classify := range classifiers {
			shard, term, classified := classify(application)
			if !classified {
				continue
			}
			matches++
			if !rowOK || shard.ordinal != row.shard || term != row.term {
				t.Fatalf("base %d typed source does not round-trip occurrence", index)
			}
		}
		if matches != 1 {
			t.Fatalf("base %d has %d typed source classifications", index, matches)
		}
		if !apps.IsBase(application) {
			t.Fatalf("base %d was not admitted by Applications.IsBase", index)
		}
		if _, _, _, imported := apps.Import(application); imported {
			t.Fatalf("base %d also classified as Import", index)
		}
	}
	for index := 0; index < apps.Imports().Count(); index++ {
		application, ok := apps.Imports().At(index)
		if !ok {
			t.Fatalf("import %d unavailable", index)
		}
		for _, classify := range classifiers {
			if _, _, classified := classify(application); classified {
				t.Fatalf("import %d also classified as Base", index)
			}
		}
		if apps.IsBase(application) {
			t.Fatalf("import %d was classified as Base by Applications.IsBase", index)
		}
	}
}

func TestApplicationsIsBaseFencesOwnerRangeAndAllocatesNothing(t *testing.T) {
	p := projectProgram(t, `local function f() end; f(); return require("dependency")`)
	first := projectDraft(t, []Module{{Name: "main", Program: p}})
	second := projectDraft(t, []Module{{Name: "main", Program: p}})
	applications := first.Applications()
	foreignApplications := second.Applications()

	base, ok := applications.Bases().At(0)
	if !ok || !applications.IsBase(base) {
		t.Fatal("canonical base was not recognized")
	}
	foreign, ok := foreignApplications.At(0)
	if !ok {
		t.Fatal("equivalent reseal did not produce same-ordinal application")
	}
	if applications.IsBase(foreign) {
		t.Fatal("same-ordinal application from equivalent reseal crossed Project owner fence")
	}
	if foreignApplications.IsBase(base) {
		t.Fatal("application from first Project crossed equivalent reseal owner fence")
	}

	if applications.IsBase(Application{}) {
		t.Fatal("zero application handle was admitted")
	}
	outOfRange := Application{authority: applications.authority, ordinal: uint32(applications.Count() + 1)}
	if applications.IsBase(outOfRange) {
		t.Fatal("out-of-range application handle was admitted")
	}

	if allocations := testing.AllocsPerRun(1000, func() { _ = applications.IsBase(base) }); allocations != 0 {
		t.Fatalf("Applications.IsBase allocated %v times per call", allocations)
	}
}

func TestKeysUseDenseProgramMappingAndFinalizeFencesIdentity(t *testing.T) {
	p := projectProgram(t, `local value = {needle = 1}; return value.needle`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	shard, ok := draft.Mounts().At(0)
	if !ok {
		t.Fatal("missing mount")
	}
	key := projectExactStringKey(t, p, "needle")
	linked, mapped := draft.Keys().ForProgram(shard, p, key)
	if !mapped {
		t.Fatal("Program key not mapped")
	}
	value, valid := draft.Keys().Exact(linked)
	if !valid || value.Kind != keyspace.LiteralString || value.String != "needle" {
		t.Fatalf("wrong quotient payload: %#v", value)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if component.Keys().Count() == 0 {
		t.Fatal("final component lost key authority")
	}
	if _, ok := component.Keys().ForProgram(Shard{authority: component.authority, ordinal: 2}, p, key); ok {
		t.Fatal("foreign shard accepted")
	}
}

func TestDraftCopiesShareFinalizationFence(t *testing.T) {
	p := projectProgram(t, `local value = {needle = 1}; local function f() end; f(); return value.needle`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	mounts := draft.Mounts()
	keys := draft.Keys()
	applications := draft.Applications()
	calls := applications.Calls()
	imports := applications.Imports()
	bases := applications.Bases()
	cold := draft.Cold()
	shard, shardOK := mounts.At(0)
	programKey := projectExactStringKey(t, p, "needle")
	linkedKey, keyOK := keys.ForProgram(shard, p, programKey)
	application, applicationOK := applications.At(0)
	if !shardOK || !keyOK || !applicationOK {
		t.Fatal("failed to issue pre-finalization handles")
	}
	copy := *draft
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if copy.Mounts().Count() != 0 || copy.Keys().Count() != 0 || copy.Applications().Count() != 0 {
		t.Fatal("copied Draft retained construction authority after finalization")
	}
	if mounts.Count() != 0 || keys.Count() != 0 || applications.Count() != 0 || cold.ContentID().Available() || cold.TargetID().Available() || calls.Count() != 0 || imports.Count() != 0 || bases.Count() != 0 {
		t.Fatal("saved Draft views remained live after finalization")
	}
	if _, ok := mounts.Index(shard); ok {
		t.Fatal("saved Draft Mounts accepted a handle after finalization")
	}
	if _, ok := keys.Index(linkedKey); ok {
		t.Fatal("saved Draft Keys accepted a handle after finalization")
	}
	if _, ok := applications.Index(application); ok {
		t.Fatal("saved Draft Applications accepted a handle after finalization")
	}
	if second, err := copy.Finalize(); err == nil || second != nil {
		t.Fatal("copied Draft finalized a second component")
	}
	if second, err := draft.Finalize(); err == nil || second != nil {
		t.Fatal("consumed Draft finalized a second component")
	}
	// The opaque handles retain only the immutable authority, so the one
	// finalized Component may continue consuming handles issued before the
	// Draft fence closed.
	if component.Mounts().Count() != 1 || component.Keys().Count() == 0 || component.Applications().Count() == 0 {
		t.Fatal("finalized Component views did not remain live")
	}
	if got, ok := component.Mounts().Index(shard); !ok || got != 0 {
		t.Fatal("pre-finalization Shard did not retain its exact authority")
	}
	if got, ok := component.Keys().Index(linkedKey); !ok || got < 0 {
		t.Fatal("pre-finalization Key did not retain its exact authority")
	}
	if got, ok := component.Applications().Index(application); !ok || got < 0 {
		t.Fatal("pre-finalization Application did not retain its exact authority")
	}
}

func TestProjectHandlesFenceEquivalentResealsAndForeignRelations(t *testing.T) {
	text := `local value = {needle = 1}; local function f() end; f(); return value.needle`
	p := projectProgram(t, text)
	foreignProgram := projectProgram(t, text)
	contract := projectTarget(t, "GlobalEnvRoot")
	foreignTarget := projectTarget(t, "GlobalEnvRoot")
	firstDraft, err := Build(Input{Modules: []Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	secondDraft, err := Build(Input{Modules: []Module{{Name: "main", Program: p}}, Target: foreignTarget})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.Cold().ContentID() != second.Cold().ContentID() {
		t.Fatal("equivalent Project reseals changed ContentID")
	}

	firstShard, ok := first.Mounts().At(0)
	if !ok {
		t.Fatal("first Project mount unavailable")
	}
	secondShard, ok := second.Mounts().At(0)
	if !ok {
		t.Fatal("second Project mount unavailable")
	}
	if index, ok := first.Mounts().Index(firstShard); !ok || index != 0 {
		t.Fatal("Mounts At/Index did not round-trip")
	}
	if roundTrip, ok := first.Mounts().At(0); !ok || roundTrip != firstShard {
		t.Fatal("Mounts Index/At did not round-trip")
	}
	if _, ok := first.Mounts().Index(secondShard); ok {
		t.Fatal("equivalent reseal Shard crossed the Project authority fence")
	}

	programKey := projectExactStringKey(t, p, "needle")
	firstKey, ok := first.Keys().ForProgram(firstShard, p, programKey)
	if !ok {
		t.Fatal("first Project Program key unavailable")
	}
	secondKey, ok := second.Keys().ForProgram(secondShard, p, programKey)
	if !ok {
		t.Fatal("second Project Program key unavailable")
	}
	firstKeyIndex, firstKeyIndexOK := first.Keys().Index(firstKey)
	if !firstKeyIndexOK || firstKeyIndex < 0 {
		t.Fatal("Keys At/Index did not round-trip")
	}
	if roundTrip, ok := first.Keys().At(firstKeyIndex); !ok || roundTrip != firstKey {
		t.Fatal("Keys Index/At did not round-trip")
	}
	if _, ok := first.Keys().Index(secondKey); ok {
		t.Fatal("equivalent reseal Key crossed the Project authority fence")
	}
	if _, ok := first.Keys().ForProgram(firstShard, foreignProgram, programKey); ok {
		t.Fatal("foreign equivalent Program borrowed a mounted Program-key mapping")
	}

	var targetKey target.ExactKey
	for index := 0; index < contract.ExactKeyCount(); index++ {
		candidate, candidateOK := contract.ExactKeyAt(index)
		value, valueOK := contract.ExactKeyValue(candidate)
		if candidateOK && valueOK && value.Kind == keyspace.LiteralString && value.String == "_G" {
			targetKey = candidate
			break
		}
	}
	if targetKey == 0 {
		t.Fatal("target exact key fixture unavailable")
	}
	if _, ok := first.Keys().ForTarget(contract, targetKey); !ok {
		t.Fatal("local exact Target was rejected")
	}
	if _, ok := first.Keys().ForTarget(foreignTarget, targetKey); ok {
		t.Fatal("foreign exact Target borrowed a Project mapping")
	}

	firstApplications := first.Applications()
	secondApplications := second.Applications()
	firstApplication, ok := firstApplications.At(0)
	if !ok {
		t.Fatal("first Project Application unavailable")
	}
	secondApplication, ok := secondApplications.At(0)
	if !ok {
		t.Fatal("second Project Application unavailable")
	}
	firstApplicationIndex, firstApplicationIndexOK := firstApplications.Index(firstApplication)
	if !firstApplicationIndexOK || firstApplicationIndex < 0 {
		t.Fatal("Applications At/Index did not round-trip")
	}
	if roundTrip, ok := firstApplications.At(firstApplicationIndex); !ok || roundTrip != firstApplication {
		t.Fatal("Applications Index/At did not round-trip")
	}
	if _, ok := firstApplications.Index(secondApplication); ok {
		t.Fatal("equivalent reseal Application crossed the Project authority fence")
	}
}

func TestImportRejectsForeignRootCall(t *testing.T) {
	p := projectProgram(t, `require("dependency")`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	applications := draft.Applications()
	imported, ok := applications.Imports().At(0)
	if !ok {
		t.Fatal("missing Import application")
	}
	row, ok := applications.application(imported)
	if !ok || row.root == 0 {
		t.Fatal("Import row has no root Call")
	}
	if _, _, _, ok := applications.Import(imported); !ok {
		t.Fatal("valid Import relation rejected")
	}
	rootIndex := row.root - 1
	original := draft.state.authority.applications[rootIndex]
	draft.state.authority.applications[rootIndex].shard++
	if _, _, _, ok := applications.Import(imported); ok {
		t.Fatal("Import accepted a root Call from another shard")
	}
	draft.state.authority.applications[rootIndex] = original
	draft.state.authority.applications[rootIndex].kind = applicationMeta
	if _, _, _, ok := applications.Import(imported); ok {
		t.Fatal("Import accepted a root non-Call application")
	}
}

func TestApplicationsExposeTypedSubsequencesAndFinalizedIDs(t *testing.T) {
	p := projectProgram(t, `local function f() end; f(); local dependency = require("dependency")`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	apps := draft.Applications()
	if apps.Count() == 0 || apps.Calls().Count() == 0 || apps.Bases().Count() < apps.Calls().Count() || apps.Imports().Count() != 1 {
		t.Fatalf("unexpected application denominator: total=%d bases=%d calls=%d imports=%d", apps.Count(), apps.Bases().Count(), apps.Calls().Count(), apps.Imports().Count())
	}
	imported, ok := apps.Imports().At(0)
	if !ok {
		t.Fatal("missing import application")
	}
	shard, term, call, valid := apps.Import(imported)
	if !valid || shard.ordinal == 0 || term == 0 {
		t.Fatal("import relation missing")
	}
	if _, _, valid := apps.Call(call); !valid {
		t.Fatal("import target is not its call application")
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	index, ok := component.Applications().Index(imported)
	if !ok {
		t.Fatal("application canonical index missing")
	}
	found, ok := component.Applications().At(index)
	if !ok {
		t.Fatal("application canonical position unavailable")
	}
	if order, ok := component.Applications().Compare(imported, found); !ok || order != 0 {
		t.Fatal("application identity did not round-trip")
	}
}

func projectProgram(t testing.TB, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "project-law", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func projectDraft(t testing.TB, modules []Module) *Draft {
	t.Helper()
	contract := projectTarget(t, "GlobalEnvRoot")
	draft, err := Build(Input{Modules: modules, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func projectTarget(t testing.TB, root string) *target.Contract {
	t.Helper()
	contract, err := target.Seal(&target.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []target.InitialRootSpec{{Identity: root, Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: root}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: root, Key: targetStringKey("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: root}, Mutability: target.InitialMutable},
			{Root: root, Key: targetStringKey("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: root, Key: targetStringKey("_G")}, {Name: "__link_absent", Root: root, Key: targetStringKey("__link_absent")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func targetStringKey(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func projectExactStringKey(t testing.TB, p *program.Program, want string) keyspace.Key {
	t.Helper()
	keys := p.Source().Keys()
	for index := 0; index < keys.ExactCount(); index++ {
		key, value, ok := keys.ExactAt(index)
		if ok && value.Kind == keyspace.LiteralString && value.String == want {
			return key
		}
	}
	t.Fatalf("missing Program exact key %q", want)
	return 0
}
