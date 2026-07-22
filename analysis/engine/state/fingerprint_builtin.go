package state

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

// fingerprintScratch owns typed temporary ordering buffers for one serial
// fingerprint session. Canonical fingerprinting still visits and emits every
// semantic element; only the backing arrays are reused between lane misses.
type fingerprintScratch struct {
	identities             []identity.Term
	pathKeys               []keyspace.Key
	dynamicKeys            []dynamicindex.Key
	pathProducts           []fingerprintPathProduct
	proofs                 []pathevidence.BranchProof
	implications           []pathevidence.PathPresenceImplication
	heapPrefixEnabled      bool
	heapObjectPrefix       []fingerprintHeapObjectPrefix
	heapObjectNext         []fingerprintHeapObjectPrefix
	heapObjectHits         uint64
	pathPrefixEnabled      bool
	pathRefinements        []fingerprintPrefixEntry[fingerprintPathProduct]
	pathRefinementNext     []fingerprintPrefixEntry[fingerprintPathProduct]
	pathMembers            []fingerprintPrefixEntry[fingerprintPathProduct]
	pathMemberNext         []fingerprintPrefixEntry[fingerprintPathProduct]
	pathProofPrefix        []fingerprintPrefixEntry[pathevidence.BranchProof]
	pathProofNext          []fingerprintPrefixEntry[pathevidence.BranchProof]
	pathImplicationRecords []fingerprintPathImplicationRecord
	pathImplications       []fingerprintPrefixEntry[fingerprintPathImplicationRecord]
	pathImplicationNext    []fingerprintPrefixEntry[fingerprintPathImplicationRecord]
	pathPrefixHits         uint64
}

type fingerprintPrefixEntry[T any] struct {
	value    T
	input    uint64
	output   uint64
	registry *axis.Registry
	keys     *keyspace.KeySpace
}

type fingerprintHeapObjectPrefix struct {
	id       identity.Term
	object   heapidentity.TableObject
	input    uint64
	output   uint64
	registry *axis.Registry
	keys     *keyspace.KeySpace
}

// fingerprintWithPreviousPrefix replays only the unchanged canonical prefix
// from the immediately preceding lane value. Input hash and semantic equality
// are both exact certificates; after the first changed element, differing FNV
// state naturally prevents reuse of the remaining suffix. Two vectors are
// swapped transactionally, so retained memory is O(current lane size).
func fingerprintWithPreviousPrefix[T any](
	w *fingerprintWriter,
	values []T,
	previous, next *[]fingerprintPrefixEntry[T],
	same func(T, T) bool,
	emit func(T),
	hits *uint64,
) bool {
	prior := *previous
	candidate := (*next)[:0]
	for index, value := range values {
		input := w.sum64()
		reused := false
		if index < len(prior) {
			entry := prior[index]
			if entry.input == input && entry.registry == w.reg && entry.keys == w.keys && same(entry.value, value) {
				if !w.checkpoint() {
					return false
				}
				w.h.RestoreSum64(entry.output)
				(*hits)++
				reused = true
			}
		}
		if !reused {
			emit(value)
		}
		if w.errVal != nil {
			return false
		}
		candidate = append(candidate, fingerprintPrefixEntry[T]{
			value: value, input: input, output: w.sum64(), registry: w.reg, keys: w.keys,
		})
	}
	clear(prior)
	*previous = candidate
	*next = prior[:0]
	return true
}

func fingerprintValues(w *fingerprintWriter, st State) {
	w.bool("top", st.values.top)
	values := st.values.cloneValues()
	keys := make([]uint64, 0, len(values))
	byKey := make(map[uint64]product.Value, len(values))
	for key, value := range values {
		n := uint64(key)
		keys = append(keys, n)
		byKey[n] = value
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	w.int64("count", int64(len(keys)))
	for _, key := range keys {
		w.uint64("slot", key)
		w.product("value", byKey[key])
	}
}

func fingerprintPathEvidence(w *fingerprintWriter, st State) {
	refinements := collectPathProducts(w.scratch.pathProducts[:0], st.pathEvidence.ForEachPathRefinement)
	w.bool("refinements-bottom", st.pathEvidence.RefinementsBottom())
	w.bool("refinements-top", !st.pathEvidence.RefinementsBottom() && len(refinements) == 0)
	fingerprintPathProducts(w, "refinement", refinements, &w.scratch.pathRefinements, &w.scratch.pathRefinementNext)
	clear(refinements)
	w.scratch.pathProducts = refinements[:0]

	members := collectPathProducts(w.scratch.pathProducts[:0], st.pathEvidence.ForEachPathStaticMember)
	w.bool("members-bottom", st.pathEvidence.StaticMembersBottom())
	w.bool("members-top", !st.pathEvidence.StaticMembersBottom() && len(members) == 0)
	fingerprintPathProducts(w, "member", members, &w.scratch.pathMembers, &w.scratch.pathMemberNext)
	clear(members)
	w.scratch.pathProducts = members[:0]

	proofs := w.scratch.proofs[:0]
	st.pathEvidence.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
		proofs = append(proofs, proof)
		return true
	})
	for _, proof := range proofs {
		_ = keyspaceEncoding(w, proof.Path)
		if proof.Other.Kind != keyspace.KindInvalid {
			_ = keyspaceEncoding(w, proof.Other)
		}
	}
	if w.errVal != nil {
		return
	}
	sort.Slice(proofs, func(i, j int) bool { return fingerprintBranchProofLess(w, proofs[i], proofs[j]) })
	w.bool("proofs-bottom", st.pathEvidence.ProofsBottom())
	w.bool("proofs-top", !st.pathEvidence.ProofsBottom() && len(proofs) == 0)
	w.int64("proof-count", int64(len(proofs)))
	emitProof := func(proof pathevidence.BranchProof) {
		w.int64("proof-kind", int64(proof.Kind))
		w.pathKey("proof-path", proof.Path)
		w.string("proof-presence", presenceFingerprint(proof.Presence))
		if proof.Other.Kind == keyspace.KindInvalid {
			w.bool("proof-has-other", false)
		} else {
			w.bool("proof-has-other", true)
			w.pathKey("proof-other", proof.Other)
		}
	}
	if w.scratch.pathPrefixEnabled {
		fingerprintWithPreviousPrefix(w, proofs, &w.scratch.pathProofPrefix, &w.scratch.pathProofNext,
			func(left, right pathevidence.BranchProof) bool {
				return left.Kind == right.Kind && left.Path == right.Path && left.Other == right.Other && presence.Equal(left.Presence, right.Presence)
			}, emitProof, &w.scratch.pathPrefixHits)
	} else {
		for _, proof := range proofs {
			emitProof(proof)
		}
	}
	clear(proofs)
	w.scratch.proofs = proofs[:0]

	implications := w.scratch.implications[:0]
	st.pathEvidence.ForEachPathPresenceImplication(func(implication pathevidence.PathPresenceImplication) bool {
		implications = append(implications, implication)
		return true
	})
	w.bool("implications-bottom", st.pathEvidence.PathPresenceImplicationsBottom())
	w.bool("implications-top", !st.pathEvidence.PathPresenceImplicationsBottom() && len(implications) == 0)
	w.int64("implication-count", int64(len(implications)))
	implicationRecords := w.scratch.pathImplicationRecords[:0]
	for _, implication := range implications {
		implicationRecords = append(implicationRecords, fingerprintPathImplicationRecord{
			implication: implication,
			record:      pathPresenceImplicationRecord(w, implication),
		})
	}
	if w.errVal != nil {
		return
	}
	sort.Slice(implicationRecords, func(i, j int) bool { return implicationRecords[i].record < implicationRecords[j].record })
	emitImplication := func(record fingerprintPathImplicationRecord) { w.string("implication", record.record) }
	if w.scratch.pathPrefixEnabled {
		fingerprintWithPreviousPrefix(w, implicationRecords, &w.scratch.pathImplications, &w.scratch.pathImplicationNext,
			func(left, right fingerprintPathImplicationRecord) bool {
				return left.record == right.record && pathPresenceImplicationSame(w.reg, left.implication, right.implication)
			}, emitImplication, &w.scratch.pathPrefixHits)
	} else {
		for _, record := range implicationRecords {
			emitImplication(record)
		}
	}
	clear(implications)
	w.scratch.implications = implications[:0]
	clear(implicationRecords)
	w.scratch.pathImplicationRecords = implicationRecords[:0]
}

type fingerprintPathProduct struct {
	key   keyspace.Key
	value product.Value
}

type fingerprintPathImplicationRecord struct {
	implication pathevidence.PathPresenceImplication
	record      string
}

func collectPathProducts(out []fingerprintPathProduct, visit func(func(keyspace.Key, product.Value) bool)) []fingerprintPathProduct {
	visit(func(key keyspace.Key, value product.Value) bool {
		out = append(out, fingerprintPathProduct{key: key, value: value})
		return true
	})
	return out
}

func fingerprintPathProducts(
	w *fingerprintWriter,
	label string,
	values []fingerprintPathProduct,
	previous, next *[]fingerprintPrefixEntry[fingerprintPathProduct],
) {
	for _, value := range values {
		_ = keyspaceEncoding(w, value.key)
	}
	if w.errVal != nil {
		return
	}
	sort.Slice(values, func(i, j int) bool {
		return keyspaceEncoding(w, values[i].key) < keyspaceEncoding(w, values[j].key)
	})
	w.int64(label+"-count", int64(len(values)))
	emit := func(value fingerprintPathProduct) {
		w.pathKey(label+"-key", value.key)
		w.product(label+"-value", value.value)
	}
	if !w.scratch.pathPrefixEnabled {
		for _, value := range values {
			emit(value)
		}
		return
	}
	domain := product.Domain(w.reg)
	fingerprintWithPreviousPrefix(w, values, previous, next, func(left, right fingerprintPathProduct) bool {
		if left.key != right.key {
			return false
		}
		return domain.Same != nil && domain.Same(left.value, right.value) || domain.Equal(left.value, right.value)
	}, emit, &w.scratch.pathPrefixHits)
}

func fingerprintBranchProofLess(w *fingerprintWriter, a, b pathevidence.BranchProof) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Path != b.Path {
		return keyspaceEncoding(w, a.Path) < keyspaceEncoding(w, b.Path)
	}
	aHasOther := a.Other.Kind != keyspace.KindInvalid
	bHasOther := b.Other.Kind != keyspace.KindInvalid
	if aHasOther != bHasOther {
		return !aHasOther
	}
	if aHasOther && a.Other != b.Other {
		return keyspaceEncoding(w, a.Other) < keyspaceEncoding(w, b.Other)
	}
	return presenceFingerprint(a.Presence) < presenceFingerprint(b.Presence)
}

func pathPresenceImplicationRecord(w *fingerprintWriter, implication pathevidence.PathPresenceImplication) string {
	var record strings.Builder
	appendRecordString(&record, keyspaceEncoding(w, implication.Trigger))
	appendRecordBool(&record, implication.HasTriggerPathEqual)
	if implication.HasTriggerPathEqual {
		appendRecordString(&record, keyspaceEncoding(w, implication.TriggerOther))
	}
	appendRecordBool(&record, implication.HasTriggerPresence)
	if implication.HasTriggerPresence || !implication.HasTriggerValue {
		appendRecordString(&record, presenceFingerprint(implication.TriggerPresence))
	}
	appendRecordBool(&record, implication.HasTriggerValue)
	if implication.HasTriggerValue {
		appendRecordProduct(&record, w, implication.TriggerValue)
	}
	appendRecordString(&record, keyspaceEncoding(w, implication.Target))
	appendRecordBool(&record, implication.HasTargetValue)
	if implication.HasTargetValue {
		appendRecordProduct(&record, w, implication.TargetValue)
	} else {
		appendRecordString(&record, presenceFingerprint(implication.TargetPresence))
	}
	return record.String()
}

func pathPresenceImplicationSame(reg *axis.Registry, left, right pathevidence.PathPresenceImplication) bool {
	if left.Trigger != right.Trigger || left.Target != right.Target ||
		left.HasTriggerPathEqual != right.HasTriggerPathEqual ||
		left.HasTriggerPresence != right.HasTriggerPresence ||
		left.HasTriggerValue != right.HasTriggerValue ||
		left.HasTargetValue != right.HasTargetValue {
		return false
	}
	if left.HasTriggerPathEqual && left.TriggerOther != right.TriggerOther {
		return false
	}
	if (left.HasTriggerPresence || !left.HasTriggerValue) && !presence.Equal(left.TriggerPresence, right.TriggerPresence) {
		return false
	}
	valueDomain := product.Domain(reg)
	sameProduct := func(a, b product.Value) bool {
		return valueDomain.Same != nil && valueDomain.Same(a, b) || valueDomain.Equal(a, b)
	}
	if left.HasTriggerValue && !sameProduct(left.TriggerValue, right.TriggerValue) {
		return false
	}
	if left.HasTargetValue {
		return sameProduct(left.TargetValue, right.TargetValue)
	}
	return presence.Equal(left.TargetPresence, right.TargetPresence)
}

func fingerprintDynamicIndex(w *fingerprintWriter, st State) {
	w.bool("top", st.dynamicIndex.top)
	values := st.dynamicIndex.values
	keys := sortedDynamicIndexKeys(w, values)
	w.int64("count", int64(len(keys)))
	for _, key := range keys {
		fingerprintDynamicIndexKey(w, key)
		fingerprintDynamicIndexFact(w, values[key])
	}
}

func fingerprintHeapTableIdentity(w *fingerprintWriter, st State) {
	w.bool("top", st.heapTableIdentity.top)
	objects := st.heapTableIdentity.values
	ids := w.scratch.identities[:0]
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
	w.int64("count", int64(len(ids)))
	if !w.scratch.heapPrefixEnabled {
		for _, id := range ids {
			w.identityTerm("identity", id)
			fingerprintHeapTableObject(w, objects[id])
		}
		clear(ids)
		w.scratch.identities = ids[:0]
		return
	}
	previous := w.scratch.heapObjectPrefix
	next := w.scratch.heapObjectNext[:0]
	objectDomain := heapidentity.ObjectDomain(w.reg)
	for index, id := range ids {
		w.identityTerm("identity", id)
		object := objects[id]
		input := w.sum64()
		reused := false
		if index < len(previous) {
			prior := previous[index]
			if prior.id == id && prior.input == input && prior.registry == w.reg && prior.keys == w.keys {
				same := objectDomain.Same != nil && objectDomain.Same(prior.object, object)
				if !same {
					same = objectDomain.Equal(prior.object, object)
				}
				if same {
					if !w.checkpoint() {
						return
					}
					w.h.RestoreSum64(prior.output)
					w.scratch.heapObjectHits++
					reused = true
				}
			}
		}
		if !reused {
			fingerprintHeapTableObject(w, object)
		}
		if w.errVal != nil {
			return
		}
		next = append(next, fingerprintHeapObjectPrefix{
			id: id, object: object, input: input, output: w.sum64(), registry: w.reg, keys: w.keys,
		})
	}
	clear(ids)
	w.scratch.identities = ids[:0]
	clear(previous)
	w.scratch.heapObjectPrefix = next
	w.scratch.heapObjectNext = previous[:0]
}

func fingerprintHeapTableObject(w *fingerprintWriter, object heapidentity.TableObject) {
	w.bool("object-bottom", object.IsBottom())
	if object.IsBottom() {
		return
	}
	w.product("object-root", object.Root())
	staticKeys := w.scratch.pathKeys[:0]
	object.VisitStaticMembers(func(key keyspace.Key, _ product.Value) bool {
		staticKeys = append(staticKeys, key)
		_ = keyspaceEncoding(w, key)
		return w.errVal == nil
	})
	sort.Slice(staticKeys, func(i, j int) bool {
		return keyspaceEncoding(w, staticKeys[i]) < keyspaceEncoding(w, staticKeys[j])
	})
	w.int64("static-count", int64(len(staticKeys)))
	for _, key := range staticKeys {
		value, present := object.StaticMember(key)
		if !present {
			w.errVal = ErrFingerprintCoverage
			return
		}
		w.pathKey("static-key", key)
		w.product("static-value", value)
	}
	clear(staticKeys)
	w.scratch.pathKeys = staticKeys[:0]
	w.bool("dynamic-top", object.DynamicIndexFactsTop())
	dynamicKeys := w.scratch.dynamicKeys[:0]
	object.VisitDynamicIndexFacts(func(key dynamicindex.Key, _ dynamicindex.Fact) bool {
		dynamicKeys = append(dynamicKeys, key)
		_ = keyspaceEncoding(w, key.Table)
		return w.errVal == nil
	})
	sort.Slice(dynamicKeys, func(i, j int) bool {
		left, right := keyspaceEncoding(w, dynamicKeys[i].Table), keyspaceEncoding(w, dynamicKeys[j].Table)
		if left != right {
			return left < right
		}
		return dynamicKeys[i].Site < dynamicKeys[j].Site
	})
	w.int64("dynamic-count", int64(len(dynamicKeys)))
	for _, key := range dynamicKeys {
		fact, present := object.DynamicIndexFact(key)
		if !present {
			w.errVal = ErrFingerprintCoverage
			return
		}
		fingerprintDynamicIndexKey(w, key)
		fingerprintDynamicIndexFact(w, fact)
	}
	clear(dynamicKeys)
	w.scratch.dynamicKeys = dynamicKeys[:0]
	w.bool("stable-shape", object.StableShape())
	w.bool("prefix-stable-shape", object.PrefixStableShape())
}

func fingerprintFrozenTables(w *fingerprintWriter, st State) {
	bottom, top, tables := st.frozenTables.snapshot(identityTermLess)
	w.bool("bottom", bottom)
	w.bool("top", top)
	w.int64("count", int64(len(tables)))
	for _, id := range tables {
		w.identityTerm("identity", id)
	}
}

func fingerprintEffectDeltas(w *fingerprintWriter, st State) {
	w.bool("top", st.effectDeltas.top)
	values := st.effectDeltas.values
	keys := make([]effectdelta.Key, 0, len(values))
	for key := range values {
		keys = append(keys, key)
		_ = keyspaceEncoding(w, key.Target)
	}
	if w.errVal != nil {
		return
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keyspaceEncoding(w, keys[i].Target), keyspaceEncoding(w, keys[j].Target)
		if left != right {
			return left < right
		}
		if keys[i].Site != keys[j].Site {
			return keys[i].Site < keys[j].Site
		}
		return keys[i].Kind < keys[j].Kind
	})
	w.int64("count", int64(len(keys)))
	for _, key := range keys {
		w.pathKey("target", key.Target)
		w.string("site", string(key.Site))
		w.int64("kind", int64(key.Kind))
		value := values[key]
		w.product("before", value.Before)
		w.product("after", value.After)
		w.int64("change", int64(value.Change))
	}
}

func fingerprintEscapeEvents(w *fingerprintWriter, st State) {
	snapshot := st.escapeEvents.Snapshot()
	w.bool("bottom", snapshot.Bottom)
	w.bool("top", snapshot.Top)
	w.int64("count", int64(len(snapshot.Facts)))
	for _, fact := range snapshot.Facts {
		w.string("target", fact.Target.String())
		w.int64("kind", int64(fact.Kind))
		w.bool("recursive", fact.Recursive)
	}
}

func fingerprintChannelSelect(w *fingerprintWriter, st State) {
	snapshot := st.channelSelect.Snapshot()
	w.bool("bottom", snapshot.Bottom)
	w.bool("top", snapshot.Top)
	records := make([]string, 0, len(snapshot.Facts))
	for _, fact := range snapshot.Facts {
		records = append(records, channelSelectFactRecord(w, fact))
	}
	sort.Strings(records)
	w.int64("count", int64(len(records)))
	for _, record := range records {
		w.string("fact", record)
	}
}

func channelSelectFactRecord(w *fingerprintWriter, fact channelselectfact.Fact) string {
	var record strings.Builder
	appendRecordString(&record, string(fact.Select))
	appendRecordInt64(&record, int64(fact.Kind))
	appendRecordString(&record, fact.Result.String())
	appendRecordString(&record, fact.Case.String())
	appendRecordInt64(&record, int64(fact.Index))
	appendRecordBool(&record, fact.HasDefault)
	appendRecordBool(&record, fact.HasPayload)
	if fact.HasPayload {
		appendRecordProduct(&record, w, fact.Payload)
	}
	return record.String()
}

func fingerprintStoreRelations(w *fingerprintWriter, st State) {
	bottom, top, relations := st.storeRelations.snapshot(storeRelationLess)
	w.bool("bottom", bottom)
	w.bool("top", top)
	w.int64("count", int64(len(relations)))
	for _, relation := range relations {
		w.string("source", relation.Source.String())
		w.string("into", relation.Into.String())
	}
}

func fingerprintKeyMemberships(w *fingerprintWriter, st State) {
	lane := st.keyMemberships
	w.bool("bottom", lane.bottom)
	w.bool("dynamic-top", lane.dynamicTop)
	fingerprintKeyMembershipSet(w, "path", lane.path)
	fingerprintKeyMembershipSet(w, "dynamic", lane.dynamic)
	fingerprintKeyMembershipSet(w, "dynamic-all", lane.dynamicAll)

	for origin := range lane.valueOrigins {
		_ = keyspaceEncoding(w, origin.Container)
	}
	if w.errVal != nil {
		return
	}
	valueOrigins := sortedSetValues(lane.valueOrigins, func(a, b DynamicIndexValueOrigin) bool {
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		left, right := keyspaceEncoding(w, a.Container), keyspaceEncoding(w, b.Container)
		if left != right {
			return left < right
		}
		return a.Site < b.Site
	})
	if w.errVal != nil {
		return
	}
	w.int64("value-origin-count", int64(len(valueOrigins)))
	for _, origin := range valueOrigins {
		w.string("value", origin.Value.String())
		w.pathKey("container", origin.Container)
		w.string("site", string(origin.Site))
	}

	for origin := range lane.readOrigins {
		_ = keyspaceEncoding(w, origin.Container)
	}
	if w.errVal != nil {
		return
	}
	readOrigins := sortedSetValues(lane.readOrigins, func(a, b DynamicIndexReadOrigin) bool {
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		left, right := keyspaceEncoding(w, a.Container), keyspaceEncoding(w, b.Container)
		if left != right {
			return left < right
		}
		return a.Key < b.Key
	})
	if w.errVal != nil {
		return
	}
	w.int64("read-origin-count", int64(len(readOrigins)))
	for _, origin := range readOrigins {
		w.string("value", origin.Value.String())
		w.pathKey("container", origin.Container)
		w.string("key", origin.Key.String())
	}

	for restore := range lane.pendingRestores {
		_ = keyspaceEncoding(w, restore.Container)
	}
	if w.errVal != nil {
		return
	}
	restores := sortedSetValues(lane.pendingRestores, func(a, b PendingDynamicAllValueRestore) bool {
		left, right := keyspaceEncoding(w, a.Container), keyspaceEncoding(w, b.Container)
		if left != right {
			return left < right
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		return a.Key < b.Key
	})
	if w.errVal != nil {
		return
	}
	w.int64("restore-count", int64(len(restores)))
	for _, restore := range restores {
		w.pathKey("container", restore.Container)
		w.string("table", restore.Table.String())
		w.string("key", restore.Key.String())
	}
}

func fingerprintKeyMembershipSet(w *fingerprintWriter, label string, values map[KeyMembership]struct{}) {
	for membership := range values {
		if membership.Container.Kind != keyspace.KindInvalid {
			_ = keyspaceEncoding(w, membership.Container)
		}
	}
	if w.errVal != nil {
		return
	}
	items := sortedSetValues(values, func(a, b KeyMembership) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		// Path memberships deliberately have no dynamic container. Their zero
		// key is part of the canonical spelling, not a KeySpace-owned path.
		if a.Container.Kind != keyspace.KindInvalid {
			left, right := keyspaceEncoding(w, a.Container), keyspaceEncoding(w, b.Container)
			if left != right {
				return left < right
			}
		}
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		return a.Table < b.Table
	})
	w.int64(label+"-count", int64(len(items)))
	for _, membership := range items {
		w.int64(label+"-kind", int64(membership.Kind))
		w.string(label+"-key", membership.Key.String())
		if membership.Container.Kind != keyspace.KindInvalid {
			w.pathKey(label+"-container", membership.Container)
		} else {
			w.string(label+"-container", "")
		}
		w.string(label+"-site", string(membership.Site))
		w.string(label+"-table", membership.Table.String())
	}
}

func fingerprintTypestates(w *fingerprintWriter, st State) {
	store := st.typestates
	top := typestate.Domain.Equal(store, typestate.Domain.Top())
	w.bool("top", top)
	resources := store.Resources()
	w.int64("resource-count", int64(len(resources)))
	for _, resource := range resources {
		w.string("resource-id", resource.ID.String())
		w.string("protocol", string(resource.Protocol))
		slot, ok := store.Lookup(resource)
		w.bool("has-slot", ok)
		if !ok {
			continue
		}
		w.string("current", string(slot.Current))
		w.string("final", string(slot.Obligation.Final))
		w.string("finals", string(slot.Obligation.Finals))
		w.int64("locality", int64(slot.Locality))
	}
	invalids := store.InvalidTransitions()
	w.int64("invalid-count", int64(len(invalids)))
	for _, invalid := range invalids {
		w.string("invalid-id", invalid.Resource.ID.String())
		w.string("invalid-protocol", string(invalid.Resource.Protocol))
		w.string("invalid-expected", string(invalid.Expected))
		w.string("invalid-found", string(invalid.Found))
		w.uint64("invalid-site", uint64(invalid.Site))
	}
}

func fingerprintPlacement(w *fingerprintWriter, st State) {
	w.bool("top", st.placement.top)
	placements := st.placement.values
	ids := w.scratch.identities[:0]
	for id := range placements {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
	w.int64("count", int64(len(ids)))
	for _, id := range ids {
		w.identityTerm("identity", id)
		w.int64("placement", int64(placements[id]))
	}
	clear(ids)
	w.scratch.identities = ids[:0]
}

func fingerprintLenFloors(w *fingerprintWriter, st State) {
	lane := st.lenFloors.lane
	w.bool("bottom", lane.Bottom())
	fingerprintKeyInt64Map(w, "floor", func() map[keyspace.Key]int64 {
		out := make(map[keyspace.Key]int64, len(lane.Values()))
		for key, value := range lane.Values() {
			out[key] = value.Lo
		}
		return out
	}())
}

func fingerprintNumFloors(w *fingerprintWriter, st State) {
	w.bool("bottom", st.numFloors.lane.Bottom())
	fingerprintKeyInt64Map(w, "floor", st.numFloors.lane.Values())
}

func fingerprintNumCeils(w *fingerprintWriter, st State) {
	w.bool("bottom", st.numCeils.lane.Bottom())
	fingerprintKeyInt64Map(w, "ceil", st.numCeils.lane.Values())
}

func fingerprintKeyInt64Map(w *fingerprintWriter, label string, values map[keyspace.Key]int64) {
	keys := sortedKeyspaceKeys(w, values)
	w.int64(label+"-count", int64(len(keys)))
	for _, key := range keys {
		w.pathKey(label+"-key", key)
		w.int64(label+"-value", values[key])
	}
}

func fingerprintDiffRelations(w *fingerprintWriter, st State) {
	bottom, top, constraints := st.diffRelations.snapshot(relConstraintLess)
	w.bool("bottom", bottom)
	w.bool("top", top)
	w.int64("count", int64(len(constraints)))
	for _, constraint := range constraints {
		w.int64("coa", constraint.CoA)
		fingerprintRelOperand(w, "a", constraint.A)
		w.int64("cob", constraint.CoB)
		fingerprintRelOperand(w, "b", constraint.B)
		fingerprintRelOperand(w, "c", constraint.C)
		w.int64("k", constraint.K)
	}
}

func fingerprintRelOperand(w *fingerprintWriter, label string, operand RelOperand) {
	w.string(label+"-key", operand.Key.String())
	w.int64(label+"-kind", int64(operand.Kind))
}

func fingerprintUserLattices(w *fingerprintWriter, st State) {
	lane := st.userLattices
	w.bool("top", lane.top)
	type record struct {
		axisID userlattice.AxisID
		path   string
		elem   userlattice.ElementID
	}
	records := make([]record, 0, len(lane.values))
	runtime := userlattice.RuntimeFor(w.reg)
	for key, value := range lane.values {
		axis, ok := runtime.AxisBySlot(key.axis)
		if !ok {
			w.errVal = fmt.Errorf("%w: user-lattice axis slot %d", ErrFingerprintCoverage, key.axis)
			return
		}
		path := keyspaceEncoding(w, key.path)
		if w.errVal != nil {
			return
		}
		elem := axis.ElementName(value)
		if elem == "" {
			w.errVal = fmt.Errorf("%w: user-lattice axis %q element %d", ErrFingerprintCoverage, axis.ID(), value)
			return
		}
		records = append(records, record{axisID: axis.ID(), path: path, elem: elem})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].axisID != records[j].axisID {
			return records[i].axisID < records[j].axisID
		}
		if records[i].path != records[j].path {
			return records[i].path < records[j].path
		}
		return records[i].elem < records[j].elem
	})
	w.int64("entry-count", int64(len(records)))
	for _, record := range records {
		w.string("axis", string(record.axisID))
		w.string("path", record.path)
		w.string("element", string(record.elem))
	}
}

func sortedDynamicIndexKeys[V any](w *fingerprintWriter, values map[dynamicindex.Key]V) []dynamicindex.Key {
	keys := make([]dynamicindex.Key, 0, len(values))
	for key := range values {
		keys = append(keys, key)
		_ = keyspaceEncoding(w, key.Table)
	}
	if w.errVal != nil {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keyspaceEncoding(w, keys[i].Table), keyspaceEncoding(w, keys[j].Table)
		if left != right {
			return left < right
		}
		return keys[i].Site < keys[j].Site
	})
	return keys
}

func sortedKeyspaceKeys[V any](w *fingerprintWriter, values map[keyspace.Key]V) []keyspace.Key {
	keys := make([]keyspace.Key, 0, len(values))
	for key := range values {
		keys = append(keys, key)
		_ = keyspaceEncoding(w, key)
	}
	if w.errVal != nil {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool {
		return keyspaceEncoding(w, keys[i]) < keyspaceEncoding(w, keys[j])
	})
	return keys
}

func fingerprintDynamicIndexKey(w *fingerprintWriter, key dynamicindex.Key) {
	w.pathKey("table", key.Table)
	w.string("site", string(key.Site))
}

func fingerprintDynamicIndexFact(w *fingerprintWriter, fact dynamicindex.Fact) {
	w.string("key-presence", presenceFingerprint(fact.KeyPresence))
	w.product("key-value", fact.KeyValue)
	w.product("value", fact.Value)
	w.int64("admission", int64(fact.Admission))
}

func presenceFingerprint(value presence.Value) string {
	switch {
	case presence.Equal(value, presence.Bottom()):
		return "bottom"
	case presence.Equal(value, presence.Absent()):
		return "absent"
	case presence.Equal(value, presence.Present()):
		return "present"
	default:
		return "top"
	}
}

func appendRecordString(record *strings.Builder, value string) {
	record.WriteString(strconv.Itoa(len(value)))
	record.WriteByte(':')
	record.WriteString(value)
	record.WriteByte(';')
}

func appendRecordBool(record *strings.Builder, value bool) {
	if value {
		record.WriteString("1;")
	} else {
		record.WriteString("0;")
	}
}

func appendRecordInt64(record *strings.Builder, value int64) {
	record.WriteString(strconv.FormatInt(value, 10))
	record.WriteByte(';')
}

func appendRecordProduct(record *strings.Builder, w *fingerprintWriter, value product.Value) {
	record.WriteString(strconv.FormatUint(product.Hash(w.reg, value), 10))
	record.WriteByte(':')
	record.WriteString(strconv.FormatInt(int64(product.ShapeOf(value)), 10))
	record.WriteByte(';')
}
