package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

func TestSourcePublicationsMatchTypedTargetProjections(t *testing.T) {
	spec := completeBootSpec("Lua 5.3", InitialMutable)
	spec.Operations = append(spec.Operations,
		callbackLifecycleOperation("source-callback", CallbackSyncOptionalOnce, CallbackRetainedRequiredMany),
		spawnTestOperation("source-spawn"),
		OperationSpec{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"source-resume"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesVariable, Var: 0},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}},
			},
			Resumes: []ResumeSpec{completeResume(ResumeSourceValueFormal, 0, 0)},
			Effects: RowSpec{Tail: RowClosed},
		},
	)
	contract := mustSeal(t, spec)

	receipt, ok := contract.SemanticSourceReceipt()
	if !ok {
		t.Fatal("sealed Contract has no semantic-source publication")
	}
	publications := receipt.Publications(semanticSourceSchema(t))
	if len(publications) != 37 {
		t.Fatalf("Target publications = %d, want 37", len(publications))
	}
	got := targetPublishedCounts(t, publications)
	want := targetProjectionCounts(t, contract)
	if len(got) != len(want) {
		t.Fatalf("Target publication vocabulary = %d, want %d", len(got), len(want))
	}
	for key, expected := range want {
		if actual, found := got[key]; !found || actual != expected {
			t.Fatalf("Target publication %08x/%d = %d/%v, want %d", key.origin, key.facet, actual, found, expected)
		}
	}
}

func TestSourcePublicationsAreReplayAndPermutationStable(t *testing.T) {
	leftFormal := typ.NewTypeParam("Element", typ.String)
	rightFormal := typ.NewTypeParam("Item", typ.String)
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		genericAlpha(leftFormal, 2),
		providerBeta(),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		providerBeta(),
		genericAlpha(rightFormal, 1),
	}})

	leftReceipt, leftOK := left.SemanticSourceReceipt()
	replayReceipt, replayOK := left.SemanticSourceReceipt()
	rightReceipt, rightOK := right.SemanticSourceReceipt()
	if !leftOK || !replayOK || !rightOK {
		t.Fatal("sealed publication unavailable")
	}
	schema := semanticSourceSchema(t)
	first := leftReceipt.Publications(schema)
	replay := replayReceipt.Publications(schema)
	second := rightReceipt.Publications(schema)
	if len(first) != len(replay) || len(first) != len(second) {
		t.Fatal("sealed publication count changed")
	}
	for index := range first {
		if first[index] != replay[index] {
			t.Fatalf("publication replay changed at %d", index)
		}
		if first[index] != second[index] {
			t.Fatalf("publication permutation changed at %d", index)
		}
	}
	if _, ok := (&Contract{}).SemanticSourceReceipt(); ok {
		t.Fatal("unsealed Contract published semantic-source rows")
	}
}

func TestSemanticSourceReceiptRejectsForeignOrStaleOwner(t *testing.T) {
	left := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	right := mustSeal(t, completeBootSpec("Lua 5.4", InitialMutable))
	if left.ContentID() == right.ContentID() {
		t.Fatal("owner-fence fixture identities unexpectedly match")
	}

	cached := left.semanticReceipt
	left.semanticReceipt = right.semanticReceipt
	if _, ok := left.SemanticSourceReceipt(); ok {
		t.Fatal("foreign cached Target receipt crossed the Contract owner fence")
	}

	left.semanticReceipt = cached
	stale := cached
	stale.owner = right.ContentID()
	left.semanticReceipt = stale
	if _, ok := left.SemanticSourceReceipt(); ok {
		t.Fatal("stale cached Target receipt crossed the Contract owner fence")
	}
}

func TestSourcePublicationsIncludeOpaqueDerivedRows(t *testing.T) {
	contract := mustSeal(t, Spec{
		Operations: []OperationSpec{protocolOperation("source-protocol", []typ.Type{typ.Any})},
		Protocols: []ProtocolSpec{{
			Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []StateSpec{{Name: "open"}, {Name: "closed", Final: true}},
			Transitions: []TransitionSpec{{
				Operation: 1,
				Input:     InputSource{Kind: InputSourceValueFormal},
				From:      1,
				Outcomes:  []TransitionOutcomeSpec{{Outcome: 0, To: 2}},
			}},
			Escapes: []EscapeSpec{{Operation: 1, Input: InputSource{Kind: InputSourceValueFormal}}},
		}},
	})
	receipt, ok := contract.SemanticSourceReceipt()
	if !ok {
		t.Fatal("sealed Contract has no semantic-source publication")
	}
	publications := receipt.Publications(semanticSourceSchema(t))
	counts := targetPublishedCounts(t, publications)
	if got := counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSuspension}]; got != 3 {
		t.Fatalf("opaque suspension rows = %d, want 3", got)
	}
	if got := counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolEscape}]; got != 2 {
		t.Fatalf("protocol escape rows = %d, want authored plus opaque 2", got)
	}
	if got := counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolTransitionOutcome}]; got != 1 {
		t.Fatalf("protocol transition outcome rows = %d, want 1", got)
	}
}

func TestTypedSemanticSourceViewsAreDetachedAndRetainEmptyFamilies(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	views, ok := contract.SemanticSourceViews()
	if !ok {
		t.Fatal("sealed Contract has no typed semantic-source views")
	}
	if views.Contract().Count() != 1 {
		t.Fatalf("Contract view count = %d, want one", views.Contract().Count())
	}
	empty := views.Gsub()
	if empty.Count() != 0 {
		t.Fatalf("empty Gsub view count = %d, want zero", empty.Count())
	}
	emptyCursor := empty.Cursor()
	if _, ok := emptyCursor.Next(); ok {
		t.Fatal("empty Gsub cursor yielded a row")
	}
	digests := views.Operation().Digests()
	if len(digests) != views.Operation().Count() || len(digests) == 0 || !digests[0].Available() {
		t.Fatalf("operation view digests = %d/%d", len(digests), views.Operation().Count())
	}
	digests[0] = identity.ContentID{}
	if replayed, ok := views.Operation().DigestAt(0); !ok || !replayed.Available() {
		t.Fatal("mutating detached digest slice changed the view")
	}
	cursor := views.Operation().Cursor()
	for index := 0; index < views.Operation().Count(); index++ {
		if _, ok := cursor.Next(); !ok {
			t.Fatalf("operation cursor ended at row %d", index)
		}
	}
	if _, ok := cursor.Next(); ok {
		t.Fatal("operation cursor yielded beyond its detached count")
	}
}

type targetSourceKey struct {
	origin semanticsource.Origin
	facet  semanticsource.Facet
}

func semanticSourceSchema(t *testing.T) semanticsource.ProgramSchema {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatalf("relation schema: %v", err)
	}
	return schema
}

func targetPublishedCounts(t *testing.T, publications []semanticsource.Publication) map[targetSourceKey]int {
	t.Helper()
	counts := make(map[targetSourceKey]int, len(publications))
	for _, publication := range publications {
		token := publication.Definition().Token()
		key := targetSourceKey{origin: token.Origin(), facet: token.Facet()}
		if _, duplicate := counts[key]; duplicate {
			t.Fatalf("duplicate Target publication %08x/%d", key.origin, key.facet)
		}
		counts[key] = publication.Count()
	}
	return counts
}

// targetProjectionCounts independently replays typed Target ownership. This
// test-only traversal intentionally uses public projections; production
// publication reads the sealed tables and does not pay this cost.
func targetProjectionCounts(t *testing.T, contract *Contract) map[targetSourceKey]int {
	t.Helper()
	counts := make(map[targetSourceKey]int, 37)
	for _, key := range targetPublicationKeys() {
		counts[key] = 0
	}
	counts[targetSourceKey{origin: semanticsource.OriginTargetContract}] = 1
	counts[targetSourceKey{origin: semanticsource.OriginTargetBoot}] = contract.InitialRootCount()
	counts[targetSourceKey{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootEntry}] = contract.InitialEntryCount()
	counts[targetSourceKey{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootMetatableAttachment}] = contract.InitialMetatableAttachmentCount()
	counts[targetSourceKey{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootBinding}] = contract.InitialBindingCount()

	for index := 0; index < contract.OperationCount(); index++ {
		op, ok := contract.OperationAt(index)
		if !ok {
			t.Fatalf("OperationAt(%d)", index)
		}
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation}]++
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetABI}]++
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOperationEffect}] += contract.EffectCount(op)
		for effect := 0; effect < contract.EffectCount(op); effect++ {
			if _, found := contract.PublicationEffectDescriptor(op, effect); found {
				counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetPublicationEffect}]++
			}
		}
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSubedge}] += contract.SubedgeCount(op)
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetBinding}] += contract.BindingCount(op)
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResume}] += contract.ResumeCount(op)
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSpawn}] += contract.SpawnCount(op)
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSuspension}] += contract.SuspensionCount(op)
		for transfer := 0; transfer < contract.TransferCount(op); transfer++ {
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetTransfer}]++
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetTransferOutcome}] += contract.TransferOutcomeCount(op, transfer)
		}
		for subedge := 0; subedge < contract.SubedgeCount(op); subedge++ {
			id, found := contract.SubedgeAt(op, subedge)
			if !found {
				t.Fatalf("SubedgeAt(%d, %d)", op, subedge)
			}
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSubedgeArgumentOrigin}] += contract.ArgumentOriginCount(id)
		}
		for callbackIndex := 0; callbackIndex < contract.CallbackCount(op); callbackIndex++ {
			callback, found := contract.CallbackAt(op, callbackIndex)
			if !found {
				t.Fatalf("CallbackAt(%d, %d)", op, callbackIndex)
			}
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallback}]++
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackEffect}] += contract.CallbackEffectCount(callback)
			for effect := 0; effect < contract.CallbackEffectCount(callback); effect++ {
				if _, found := contract.CallbackPublicationEffectDescriptor(callback, effect); found {
					counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetPublicationEffect}]++
				}
			}
			if _, _, _, _, found := contract.CallbackRelease(callback); found {
				counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackRelease}]++
			}
		}
		for resumeIndex := 0; resumeIndex < contract.ResumeCount(op); resumeIndex++ {
			resume, found := contract.ResumeIDAt(op, resumeIndex)
			if !found {
				t.Fatalf("ResumeIDAt(%d, %d)", op, resumeIndex)
			}
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResumeOutcome}] += contract.ResumeOutcomeCount(resume)
		}
		for spawnIndex := 0; spawnIndex < contract.SpawnCount(op); spawnIndex++ {
			spawn, found := contract.SpawnIDAt(op, spawnIndex)
			if !found {
				t.Fatalf("SpawnIDAt(%d, %d)", op, spawnIndex)
			}
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSpawnSibling}] += contract.SpawnSiblingCount(spawn)
		}
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOutcome}]++
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackResult}] += contract.CallbackResultCount(op, outcome)
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResultAlias}] += contract.ResultAliasCount(op, outcome)
			counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetFreshResult}] += contract.FreshResultCount(op, outcome)
			for produced := 0; produced < contract.ProducedCount(op, outcome); produced++ {
				counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetProduced}]++
				counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetProducedCapture}] += contract.ProducedCaptureCount(op, outcome, produced)
			}
		}
		if _, _, _, _, _, found := contract.GsubTableReplacement(op); found {
			counts[targetSourceKey{origin: semanticsource.OriginTargetGsub}]++
		}
	}
	if _, found := contract.Opaque(); found {
		counts[targetSourceKey{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOpaque}] = 1
	}
	for index := 0; index < contract.ProtocolCount(); index++ {
		protocol, ok := contract.ProtocolAt(index)
		if !ok {
			t.Fatalf("ProtocolAt(%d)", index)
		}
		counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol}]++
		counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolState}] += contract.StateCount(protocol)
		counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolAcquisition}] += contract.ProtocolAcquisitionCount(protocol)
		counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolEscape}] += contract.EscapeCount(protocol)
		counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolCallbackHolder}] += contract.ProtocolCallbackHolderCount(protocol)
		for transition := 0; transition < contract.TransitionCount(protocol); transition++ {
			counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolTransition}]++
			counts[targetSourceKey{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolTransitionOutcome}] += contract.TransitionOutcomeCount(protocol, transition)
		}
	}
	return counts
}

func targetPublicationKeys() [37]targetSourceKey {
	return [37]targetSourceKey{
		{origin: semanticsource.OriginTargetContract},
		{origin: semanticsource.OriginTargetOperation},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetABI},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSubedge},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallback},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetBinding},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResume},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSpawn},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOpaque},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOperationEffect},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackEffect},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackRelease},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetOutcome},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetTransfer},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetTransferOutcome},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSuspension},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResumeOutcome},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSpawnSibling},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetSubedgeArgumentOrigin},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetCallbackResult},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetResultAlias},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetProduced},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetProducedCapture},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetFreshResult},
		{origin: semanticsource.OriginTargetOperation, facet: semanticsource.FacetTargetPublicationEffect},
		{origin: semanticsource.OriginTargetProtocol},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolState},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolAcquisition},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolTransition},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolTransitionOutcome},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolEscape},
		{origin: semanticsource.OriginTargetProtocol, facet: semanticsource.FacetTargetProtocolCallbackHolder},
		{origin: semanticsource.OriginTargetBoot},
		{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootEntry},
		{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootMetatableAttachment},
		{origin: semanticsource.OriginTargetBoot, facet: semanticsource.FacetTargetBootBinding},
		{origin: semanticsource.OriginTargetGsub},
	}
}
