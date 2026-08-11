package index

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestRawSetAdmitsEverySealedPayloadKind keeps the write geometry complete:
// fixed, open-tail, and closed-call nil-fill rows all have to reach one
// Heap-owned descriptor. A missing descriptor is a declaration failure, not
// an omitted mutation branch.
func TestRawSetAdmitsEverySealedPayloadKind(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
		want   rawPayloadKind
	}{
		{name: "fixed", source: `local id = {}; local tags = {}; tags["source"] = id; return tags`, want: rawPayloadFixed},
		{name: "tail", source: `local function many(...) return ... end; local record = {}; record.first, record.second, record.third = many(); return record`, want: rawPayloadTail},
		{name: "nil-fill", source: `local record = {}; record.first, record.second = 1; return record`, want: rawPayloadNil},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			topology, _, _, _, packs := rawSetPayloadTopology(t, fixture.source)
			payloads, _, writes, ok := buildRawSetPayloads(topology, packs)
			if !ok || len(writes) == 0 {
				t.Fatal("complete RawSet payload admission")
			}
			found := false
			counts := make(map[rawPayloadKind]int)
			for _, write := range writes {
				descriptor, descriptorOK := payloadAt(payloads, write.tag)
				if !descriptorOK {
					t.Fatal("write descriptor tag")
				}
				if descriptor.kind == fixture.want {
					found = true
				}
				counts[descriptor.kind]++
			}
			if !found {
				t.Fatalf("write payload kind %d not admitted (counts=%v)", fixture.want, counts)
			}
		})
	}
}

func TestRawSetInstancesAllPayloadKinds(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
	}{
		{name: "fixed", source: `local id = {}; local tags = {}; tags["source"] = id; return tags`},
		{name: "tail", source: `local function many(...) return ... end; local record = {}; record.first, record.second, record.third = many(); return record`},
		{name: "nil-fill", source: `local record = {}; record.first, record.second = 1; return record`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			topology, valueSchema, heapSchema, calls, packSchema := rawSetPayloadTopology(t, fixture.source)
			composition := engine.NewComposition()
			values, valuesOK := valueowner.Declare(composition, rawSetKey(21), rawSetKey(22), valueSchema)
			heap, heapOK := heapowner.Declare(composition, rawSetKey(23), heapSchema)
			packs, packsOK := packowner.Declare(composition, rawSetKey(24), packSchema)
			rule, ruleOK := DeclareRawSet(composition, rawSetKey(25), rawSetKey(26), rawSetKey(27), topology, values, heap, packs)
			_, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
				Semantic: rawSetKey(28), Project: func(engine.Observation) bool { return true },
				Result: engine.FrozenResult[bool]{Semantic: rawSetKey(29), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
					if value {
						return 1
					}
					return 0
				}},
			}, func(query *engine.Query[bool]) bool {
				_, ok := engine.QueryReadFrom(query, values.ExactRead())
				return ok
			})
			if !valuesOK || !heapOK || !packsOK || !ruleOK || rule == nil || !queryOK || !composition.Seal() {
				t.Fatalf("RawSet complete payload declaration values=%t heap=%t packs=%t rule=%t ruleNil=%t query=%t", valuesOK, heapOK, packsOK, ruleOK, rule == nil, queryOK)
			}
			_ = calls
			writes := 0
			for index := 0; index < heapSchema.IndexAccessCount(); index++ {
				candidate, candidateOK := heapSchema.IndexAccessAt(index)
				access, accessOK := topology.Access(candidate)
				if !candidateOK || !accessOK {
					t.Fatal("write access")
				}
				if !access.Write() {
					continue
				}
				writes++
				if _, instanceOK := rule.Instance(access); !instanceOK {
					t.Fatalf("RawSet rejected %s payload instance", fixture.name)
				}
			}
			if writes == 0 {
				t.Fatal("no write rows")
			}
		})
	}
}

// TestRawSetDynamicNoRouteRequiresTheKeySelectionShape keeps the explicit
// empty-route disposition honest: a live receiver must carry exactly one
// authenticated dynamic-key selection, while a missing receiver carries none.
// A Heap route may never be accepted with an absent dynamic-key cell.
func TestRawSetDynamicNoRouteRequiresTheKeySelectionShape(t *testing.T) {
	topology, _, heapSchema, _, packs := rawSetPayloadTopology(t, `local id = {}; local tags = {}; local key = "source"; tags[key] = id; return tags`)
	payloads, _, writes, ok := buildRawSetPayloads(topology, packs)
	if !ok {
		t.Fatal("dynamic RawSet payload admission")
	}
	var access Access
	var descriptor rawPayload
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, candidateOK := heapSchema.IndexAccessAt(index)
		candidateAccess, accessOK := topology.Access(candidate)
		if !candidateOK || !accessOK || !candidateAccess.Write() {
			continue
		}
		if _, dynamic := candidateAccess.DynamicKey(); !dynamic {
			continue
		}
		candidateDescriptor, descriptorOK := writes[candidate]
		if !descriptorOK {
			t.Fatal("dynamic RawSet write descriptor")
		}
		candidatePayload, payloadOK := payloadAt(payloads, candidateDescriptor.tag)
		if !payloadOK {
			t.Fatal("dynamic RawSet payload descriptor")
		}
		access, descriptor = candidateAccess, candidatePayload
		break
	}
	if !access.Write() || descriptor.kind == rawPayloadInvalid {
		t.Fatal("dynamic RawSet write access")
	}
	validKey := rawSelected[valuedomain.Value]{valid: true, found: true, present: true}
	absentKey := validKey
	absentKey.present = false
	if rawSetSelectionShape(access, descriptor, rawSetView{keyCount: 0, heapCount: 1}) {
		t.Fatal("dynamic Heap route admitted without key selection")
	}
	if rawSetSelectionShape(access, descriptor, rawSetView{keyCount: 1, heapCount: 1, key: absentKey, sourceCount: len(descriptor.sources)}) {
		t.Fatal("dynamic Heap route admitted with absent key cell")
	}
	if !rawSetSelectionShape(access, descriptor, rawSetView{keyCount: 1, heapCount: 0, key: validKey}) {
		t.Fatal("dynamic no-route shape rejected exact key selection")
	}
	if rawSetSelectionShape(access, descriptor, rawSetView{keyCount: 1, heapCount: 0}) {
		t.Fatal("dynamic no-route shape admitted invalid key selection")
	}
}

func rawSetPayloadTopology(t testing.TB, source string) (*Topology, *valuedomain.Schema, heapdomain.Schema, *calldomain.Algebra, *pack.Schema) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "raw_set_payload_law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := staticdomain.Seal(linked, types)
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valueOK := valuedomain.Seal(linked, heapSchema)
	calls, callsOK := calldomain.New(linked)
	packs, packsOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heapSchema, valueSchema, calls)
	if !typesOK || staticErr != nil || !heapOK || !valueOK || !callsOK || !packsOK || !topologyOK {
		t.Fatal("RawSet payload schemas")
	}
	return topology, valueSchema, heapSchema, calls, packs
}
