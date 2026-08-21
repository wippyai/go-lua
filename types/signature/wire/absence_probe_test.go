package wire

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// This is an adversarial probe on the type wire, not a codec unit suite. The
// attack model is a payload that arrives incomplete: truncated in transit,
// written by an older or hostile producer, or corrupted in a cache. A codec's
// only defence is that it either states what a payload carries or refuses it.
//
// The specific unsoundness a silently absorbed absence produces is directional
// and severe. Dropping a member from a union narrows it, so a value the
// producer said may be a string or a number is read as only a number and every
// string-rejecting judgment downstream is proven on evidence that was never
// sent. Dropping the inner node of an optional leaves an optional over nothing,
// which is the type of a value that is definitely nil. Dropping a type
// parameter's constraint widens the parameter to unconstrained, admitting
// arguments the producer's declaration excluded.
//
// The probes below therefore attack every node position the encoder actually
// emits, not a hand-picked few: a fix that guards the three positions someone
// thought of is exactly what this sweep exists to find.

// absenceProbeType is one representative type and the name a failure reports it
// by. The corpus is chosen for node variety - every child-carrying wire kind the
// encoder can emit appears in at least one entry.
type absenceProbeType struct {
	name  string
	value typ.Type
}

func absenceProbeCorpus() []absenceProbeType {
	constrained := typ.NewTypeParam("T", typ.Number)
	return []absenceProbeType{
		{name: "optional", value: typeexpr.Optional(typ.String)},
		{name: "union", value: typeexpr.Union(typ.String, typ.Number, typ.Boolean)},
		{name: "optional-union", value: typeexpr.Optional(typeexpr.Union(typ.String, typ.Number))},
		{name: "array", value: typ.NewArray(typeexpr.Optional(typ.String))},
		{name: "map", value: typetable.NewMap(typ.String, typeexpr.Union(typ.String, typ.Number))},
		{name: "tuple", value: typ.NewTuple(typ.String, typ.Number, typ.Boolean)},
		{name: "function", value: typ.Func().Param("key", typ.String).OptParam("limit", typ.Number).
			Returns(typeexpr.Optional(typ.String)).Build()},
		{name: "record", value: typetable.NewRecord().
			Field("id", typ.String).
			Field("tags", typ.NewArray(typ.String)).
			Field("owner", typeexpr.Union(typ.String, typ.Number)).Build()},
		{name: "alias", value: typ.NewAlias("Name", typeexpr.Union(typ.String, typ.Number))},
		{name: "meta", value: typ.NewMeta(typetable.NewRecord().Field("id", typ.String).Build())},
		{name: "generic", value: typ.NewGeneric("Box", []*typ.TypeParam{constrained},
			typetable.NewRecord().Field("value", constrained).Build())},
		{name: "recursive", value: typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().Field("value", typ.String).Field("next", typeexpr.Optional(self)).Build()
		})},
	}
}

// absenceProbeStep is one hop of a path into the decoded JSON document: a
// member name in an object, or a position in an array.
type absenceProbeStep struct {
	key   string
	index int
	array bool
}

func (step absenceProbeStep) String() string {
	if step.array {
		return fmt.Sprintf("[%d]", step.index)
	}
	return "." + step.key
}

type absenceProbePath []absenceProbeStep

func (path absenceProbePath) String() string {
	rendered := make([]string, 0, len(path))
	for _, step := range path {
		rendered = append(rendered, step.String())
	}
	return "$" + strings.Join(rendered, "")
}

// absenceProbeNodeMembers are the object members that carry exactly one type
// node. Every one of them is a position where the producer stated a type.
var absenceProbeNodeMembers = map[string]bool{
	"element": true, "key": true, "value": true, "target": true, "of": true,
	"body": true, "generic": true, "metatable": true, "mapKey": true,
	"mapValue": true, "type": true, "constraint": true, "variadic": true,
}

// absenceProbeNodeLists are the object members that carry a list of type nodes.
var absenceProbeNodeLists = map[string]bool{
	"elements": true, "members": true, "returns": true, "typeArgs": true,
}

// absenceProbeStructLists are members carrying named wire rows rather than bare
// type nodes. The rows themselves are walked into, because the type node each
// one holds is a stated type; the row is not a type node itself.
var absenceProbeStructLists = map[string]bool{
	"fields": true, "staticMembers": true, "params": true, "typeParams": true, "methods": true,
}

// absenceProbeSites enumerates every path in one decoded payload at which the
// producer emitted a type node. Only emitted positions are collected: a member
// the encoder left out is a legitimate absence and is not an attack site.
func absenceProbeSites(document any, path absenceProbePath, sites *[]absenceProbePath) {
	object, objectOK := document.(map[string]any)
	if !objectOK {
		return
	}
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		member := object[name]
		switch {
		case absenceProbeNodeMembers[name]:
			if _, ok := member.(map[string]any); !ok {
				continue
			}
			next := append(append(absenceProbePath{}, path...), absenceProbeStep{key: name})
			*sites = append(*sites, next)
			absenceProbeSites(member, next, sites)
		case absenceProbeNodeLists[name], absenceProbeStructLists[name]:
			list, listOK := member.([]any)
			if !listOK {
				continue
			}
			for index, entry := range list {
				if _, ok := entry.(map[string]any); !ok {
					continue
				}
				next := append(append(absenceProbePath{}, path...),
					absenceProbeStep{key: name}, absenceProbeStep{index: index, array: true})
				if absenceProbeNodeLists[name] {
					*sites = append(*sites, next)
				}
				absenceProbeSites(entry, next, sites)
			}
		}
	}
}

// absenceProbeMutation is how one stated node is taken away. The two forms
// are separate defects with separate fixes and are never reported together.
//
// absenceProbeBlank leaves the node's position in place and empties it: an
// object member is deleted, a list position becomes an explicit null. The
// payload still says how many nodes it carries, so the decoder can see that
// one of them says nothing, and refusing is a local decision.
//
// absenceProbeDrop removes the position itself from a list. Nothing in the
// payload records how many members the producer sent, so the shortened list is
// indistinguishable from a shorter type that was encoded honestly. Refusing
// this requires the payload to witness its own arity; that is a codec
// obligation, not an impossibility.
type absenceProbeMutation uint8

const (
	absenceProbeBlank absenceProbeMutation = iota
	absenceProbeDrop
)

func (mutation absenceProbeMutation) String() string {
	if mutation == absenceProbeDrop {
		return "dropped entry"
	}
	return "blanked node"
}

// absenceProbeRemove takes the node at one path away from a decoded document
// in the form the mutation names.
func absenceProbeRemove(document any, path absenceProbePath, mutation absenceProbeMutation) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	if len(path) == 1 {
		object, objectOK := document.(map[string]any)
		if !objectOK || path[0].array {
			return nil, false
		}
		delete(object, path[0].key)
		return document, true
	}
	step := path[0]
	if step.array {
		list, listOK := document.([]any)
		if !listOK || step.index < 0 || step.index >= len(list) {
			return nil, false
		}
		if len(path) == 2 && path[1].array {
			return nil, false
		}
		updated, ok := absenceProbeRemove(list[step.index], path[1:], mutation)
		if !ok {
			return nil, false
		}
		list[step.index] = updated
		return list, true
	}
	object, objectOK := document.(map[string]any)
	if !objectOK {
		return nil, false
	}
	member, present := object[step.key]
	if !present {
		return nil, false
	}
	if len(path) == 2 && path[1].array {
		list, listOK := member.([]any)
		if !listOK || path[1].index < 0 || path[1].index >= len(list) {
			return nil, false
		}
		if mutation == absenceProbeDrop {
			object[step.key] = append(append([]any{}, list[:path[1].index]...), list[path[1].index+1:]...)
		} else {
			replaced := append([]any{}, list...)
			replaced[path[1].index] = nil
			object[step.key] = replaced
		}
		return document, true
	}
	updated, ok := absenceProbeRemove(member, path[1:], mutation)
	if !ok {
		return nil, false
	}
	object[step.key] = updated
	return document, true
}

func absenceProbeDocument(t *testing.T, value typ.Type) ([]byte, any) {
	t.Helper()
	encoded, err := EncodeType(value)
	if err != nil {
		t.Fatalf("encode %s: %v", value, err)
	}
	payload, err := json.Marshal(encoded)
	if err != nil {
		t.Fatalf("marshal wire for %s: %v", value, err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("unmarshal wire for %s: %v", value, err)
	}
	return payload, document
}

func absenceProbeDecode(payload []byte) (typ.Type, error) {
	var wire TypeWire
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, err
	}
	return DecodeType(&wire)
}

// TestWirePayloadRoundTripIsLossless is the sweep's control. Every probe below
// reads "the decoder accepted a mutated payload" as evidence of a lost
// distinction, which is only evidence if the unmutated payload decodes back to
// the type that was encoded.
func TestWirePayloadRoundTripIsLossless(t *testing.T) {
	for _, probe := range absenceProbeCorpus() {
		t.Run(probe.name, func(t *testing.T) {
			payload, _ := absenceProbeDocument(t, probe.value)
			decoded, err := absenceProbeDecode(payload)
			if err != nil {
				t.Fatalf("round trip of %s refused its own payload: %v", probe.value, err)
			}
			if !typ.TypeEquals(decoded, probe.value) {
				t.Fatalf("round trip of %s decoded to %s", probe.value, decoded)
			}
		})
	}
}

// absenceProbeSweep applies one mutation at every stated node position of one
// type's payload and returns the positions the decoder answered for anyway.
func absenceProbeSweep(t *testing.T, probe absenceProbeType, mutation absenceProbeMutation) []string {
	t.Helper()
	_, document := absenceProbeDocument(t, probe.value)
	var sites []absenceProbePath
	absenceProbeSites(document, nil, &sites)
	if len(sites) == 0 {
		t.Fatalf("%s emitted no child type node, so this probe attacks nothing", probe.value)
	}
	admitted := make([]string, 0)
	for _, site := range sites {
		if mutation == absenceProbeDrop && (len(site) == 0 || !site[len(site)-1].array) {
			continue
		}
		_, mutable := absenceProbeDocument(t, probe.value)
		pruned, removed := absenceProbeRemove(mutable, site, mutation)
		if !removed {
			t.Fatalf("could not apply %s at %s", mutation, site)
		}
		payload, err := json.Marshal(pruned)
		if err != nil {
			t.Fatalf("marshal mutated payload at %s: %v", site, err)
		}
		decoded, decodeErr := absenceProbeDecode(payload)
		if decodeErr != nil {
			continue
		}
		admitted = append(admitted, fmt.Sprintf("%s -> %s (producer stated %s)", site, decoded, probe.value))
	}
	return admitted
}

// TestWirePayloadRefusesEveryBlankedStatedNode is the direct sweep. At every
// position where the producer emitted a type node, the node is emptied while
// its position stays: an object member is deleted, a list slot becomes null.
//
// The payload still states that a node belongs there, so the decoder is being
// asked for a type by a document that carries none, and refusing is the only
// answer it can give truthfully. Answering is how a union sheds the member it
// could not read and narrows, how an optional over nothing becomes a value
// that is definitely nil, and how a constrained type parameter loses its
// constraint and admits every argument the producer excluded.
func TestWirePayloadRefusesEveryBlankedStatedNode(t *testing.T) {
	for _, probe := range absenceProbeCorpus() {
		t.Run(probe.name, func(t *testing.T) {
			if admitted := absenceProbeSweep(t, probe, absenceProbeBlank); len(admitted) != 0 {
				t.Fatalf("the decoder answered for %d payload(s) whose stated node carried nothing:\n%s",
					len(admitted), strings.Join(admitted, "\n"))
			}
		})
	}
}

// TestWirePayloadRefusesADroppedListEntry is the harder half, and it is a
// separate defect with a separate fix. Here the lost entry leaves no trace:
// the list is simply shorter, and nothing in the payload records how long the
// producer made it.
//
// So the decoder is not failing to notice something visible - it has nothing
// to notice. That is the finding. A boundary whose whole job is to carry a
// type faithfully cannot detect its own truncation, and the types it hands
// back are strictly narrower unions, shorter tuples, and functions that lost
// a result. The obligation is on the payload: it must witness its own arity,
// through emitted member counts or a content digest over the encoded tree, so
// that a shortened list stops being a well-formed document.
func TestWirePayloadRefusesADroppedListEntry(t *testing.T) {
	for _, probe := range absenceProbeCorpus() {
		t.Run(probe.name, func(t *testing.T) {
			if admitted := absenceProbeSweep(t, probe, absenceProbeDrop); len(admitted) != 0 {
				t.Fatalf("the decoder answered for %d truncated payload(s), because nothing in the payload states how many nodes were sent:\n%s",
					len(admitted), strings.Join(admitted, "\n"))
			}
		})
	}
}

// TestWireUnionPayloadRefusesADegenerateMemberList states the invariant that
// makes a lost union member detectable at all. A union of one is not a union;
// the encoder never emits one, because typeexpr.Union collapses a single
// member to that member. A payload carrying one is therefore a union that lost
// members in transit, and the type it would decode to is strictly narrower
// than the one that was sent.
//
// The same holds for an intersection, in the other direction: a lost member
// widens it, and a value is admitted where the producer required both parts.
func TestWireUnionPayloadRefusesADegenerateMemberList(t *testing.T) {
	for _, probe := range []struct {
		name    string
		payload string
	}{
		{name: "single-member union", payload: `{"kind":"union","members":[{"kind":"string"}]}`},
		{name: "empty union", payload: `{"kind":"union","members":[]}`},
		{name: "member-less union", payload: `{"kind":"union"}`},
		{name: "single-member intersection", payload: `{"kind":"intersection","members":[{"kind":"string"}]}`},
		{name: "empty intersection", payload: `{"kind":"intersection","members":[]}`},
		{name: "member-less intersection", payload: `{"kind":"intersection"}`},
	} {
		t.Run(probe.name, func(t *testing.T) {
			decoded, err := absenceProbeDecode([]byte(probe.payload))
			if err == nil {
				t.Fatalf("a %s decoded to %s; the payload lost members and the decoder answered with what was left",
					probe.name, decoded)
			}
		})
	}
}

// TestWireTypeParameterConstraintIsNotOptionalOnceStated separates the two
// absences a type parameter can carry. A parameter with no constraint node was
// declared unconstrained and reads back unconstrained; that is a legitimate
// absence and the decoder is right to accept it.
//
// A parameter whose constraint was stated and then lost reads back the same
// way, and that is the hole: the two are indistinguishable in the payload, so
// the widest possible parameter is what a corrupted constrained parameter
// decodes to. The probe states the obligation as the distinction itself - a
// payload that loses a stated constraint must not be readable as the
// unconstrained declaration.
func TestWireTypeParameterConstraintIsNotOptionalOnceStated(t *testing.T) {
	constrained := typ.NewGeneric("Box", []*typ.TypeParam{typ.NewTypeParam("T", typ.Number)},
		typetable.NewRecord().Field("value", typ.Number).Build())
	unconstrained := typ.NewGeneric("Box", []*typ.TypeParam{typ.NewTypeParam("T", nil)},
		typetable.NewRecord().Field("value", typ.Number).Build())

	_, document := absenceProbeDocument(t, constrained)
	pruned, removed := absenceProbeRemove(document, absenceProbePath{
		{key: "typeParams"}, {index: 0, array: true}, {key: "constraint"},
	}, absenceProbeBlank)
	if !removed {
		t.Fatal("the constrained payload carries no typeParams[0].constraint node to remove")
	}
	payload, err := json.Marshal(pruned)
	if err != nil {
		t.Fatalf("marshal pruned payload: %v", err)
	}
	decoded, decodeErr := absenceProbeDecode(payload)
	if decodeErr != nil {
		return
	}
	if typ.TypeEquals(decoded, unconstrained) {
		t.Fatalf("a payload that lost the stated constraint T: number decoded to the unconstrained declaration %s, so every argument the constraint excluded is now admitted",
			decoded)
	}
	if typ.TypeEquals(decoded, constrained) {
		return
	}
	t.Fatalf("a payload that lost the stated constraint decoded to %s, which is neither the stated nor the unconstrained declaration", decoded)
}
