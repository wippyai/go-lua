package transformer

import (
	"encoding/binary"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const formalSlotCanonicalSize = len(lexicalidentity.StableLexicalBodyID{}) + 8 + 1

type slotSpaceBody struct {
	id     lexicalidentity.StableLexicalBodyID
	shape  Shape
	middle uint64
}

// SlotSpace is one immutable forest-local authority for formal scalar slots.
// Dense relation indexes are private acceleration coordinates; durable and
// cross-space identity always resolves through the full lexical body ID.
type SlotSpace struct {
	bodies []slotSpaceBody
	byBody map[lexicalidentity.StableLexicalBodyID]int
}

// FormalSlot is one vocabulary-qualified scalar coordinate in a sealed
// forest. The authority pointer makes accidental cross-forest equality
// impossible; Import performs the explicit full-width structural resolution.
type FormalSlot struct {
	space    *SlotSpace
	relation int
	root     formal.Root
}

func freezeSlotSpace(program *RelationProgram) (*SlotSpace, error) {
	if program == nil || len(program.bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal slot space has no frozen forest")
	}
	bodies := make([]slotSpaceBody, len(program.bodies))
	for index := range program.bodies {
		body := program.bodies[index]
		if body.body == (lexicalidentity.StableLexicalBodyID{}) {
			return nil, fmt.Errorf("transformer: formal slot space contains an invalid lexical body")
		}
		if body.relation.arena == nil || !body.relation.arena.middle.sealed {
			return nil, fmt.Errorf("transformer: formal slot space body has no sealed Middle schema")
		}
		bodies[index] = slotSpaceBody{id: body.body, shape: body.relation.Shape(), middle: body.relation.arena.middle.count()}
	}
	return newSlotSpace(bodies)
}

func newSlotSpace(bodies []slotSpaceBody) (*SlotSpace, error) {
	if len(bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal slot space has no bodies")
	}
	space := &SlotSpace{
		bodies: append([]slotSpaceBody(nil), bodies...),
		byBody: make(map[lexicalidentity.StableLexicalBodyID]int, len(bodies)),
	}
	for index, body := range space.bodies {
		if body.id == (lexicalidentity.StableLexicalBodyID{}) {
			return nil, fmt.Errorf("transformer: formal slot space contains an empty body schema")
		}
		if _, duplicate := space.byBody[body.id]; duplicate {
			return nil, fmt.Errorf("transformer: formal slot space repeats a lexical body")
		}
		space.byBody[body.id] = index
	}
	return space, nil
}

func formalRootCount(body slotSpaceBody, vocabulary formal.Vocabulary) uint64 {
	switch vocabulary {
	case formal.Input:
		return uint64(body.shape.Params) + uint64(body.shape.Captures) + uint64(body.shape.Globals) + uint64(body.shape.Ambients)
	case formal.Middle:
		return body.middle + uint64(body.shape.HeapTemplates)
	case formal.Output:
		return uint64(body.shape.Results)
	default:
		return 0
	}
}

func formalRootOrdinal(body slotSpaceBody, root Root) (uint64, formal.Vocabulary, bool) {
	switch root.Kind {
	case RootParam:
		return uint64(root.Index) + 1, formal.Input, root.Index < body.shape.Params
	case RootCapture:
		return uint64(body.shape.Params) + uint64(root.Index) + 1, formal.Input, root.Index < body.shape.Captures
	case RootGlobal:
		return uint64(body.shape.Params) + uint64(body.shape.Captures) + uint64(root.Index) + 1, formal.Input, root.Index < body.shape.Globals
	case RootAmbient:
		return uint64(body.shape.Params) + uint64(body.shape.Captures) + uint64(body.shape.Globals) + uint64(root.Index) + 1, formal.Input, root.Index < body.shape.Ambients
	case RootMiddle:
		return uint64(root.Index) + 1, formal.Middle, uint64(root.Index) < body.middle
	case RootHeapTemplate:
		return body.middle + uint64(root.Index) + 1, formal.Middle, root.Index < body.shape.HeapTemplates
	case RootResult:
		return uint64(root.Index) + 1, formal.Output, root.Index < body.shape.Results
	default:
		return 0, formal.Invalid, false
	}
}

// Slot resolves one typed root through the forest's frozen body schema.
func (s *SlotSpace) Slot(body lexicalidentity.StableLexicalBodyID, root Root) (FormalSlot, bool) {
	if s == nil {
		return FormalSlot{}, false
	}
	relation, found := s.byBody[body]
	if !found || relation < 0 || relation >= len(s.bodies) {
		return FormalSlot{}, false
	}
	ordinal, vocabulary, valid := formalRootOrdinal(s.bodies[relation], root)
	if !valid {
		return FormalSlot{}, false
	}
	return s.SlotAt(body, ordinal, vocabulary)
}

// SlotAt resolves a frozen root ordinal without manufacturing a concrete
// state key. Root bounds are owned by the sealed forest body schema.
func (s *SlotSpace) SlotAt(body lexicalidentity.StableLexicalBodyID, root uint64, vocabulary formal.Vocabulary) (FormalSlot, bool) {
	if s == nil || !vocabulary.Valid() {
		return FormalSlot{}, false
	}
	relation, found := s.byBody[body]
	if !found || relation < 0 || relation >= len(s.bodies) || root == 0 || root > formalRootCount(s.bodies[relation], vocabulary) {
		return FormalSlot{}, false
	}
	descriptor := formal.NewRoot(body, root, vocabulary)
	if !descriptor.Valid() {
		return FormalSlot{}, false
	}
	return FormalSlot{space: s, relation: relation, root: descriptor}, true
}

// Import resolves source through its full lexical body ID and re-interns only
// the destination space's private dense relation coordinate.
func (s *SlotSpace) Import(source FormalSlot) (FormalSlot, bool) {
	if !source.Valid() {
		return FormalSlot{}, false
	}
	return s.SlotAt(source.root.Owner(), source.root.Ordinal(), source.root.Vocabulary())
}

func (s FormalSlot) Valid() bool {
	return s.space != nil && s.root.Valid() && s.relation >= 0 && s.relation < len(s.space.bodies) &&
		s.root.Owner() == s.space.bodies[s.relation].id &&
		s.root.Ordinal() <= formalRootCount(s.space.bodies[s.relation], s.root.Vocabulary())
}

func (s FormalSlot) Body() lexicalidentity.StableLexicalBodyID {
	if !s.Valid() {
		return lexicalidentity.StableLexicalBodyID{}
	}
	return s.root.Owner()
}

func (s FormalSlot) RootOrdinal() uint64 { return s.root.Ordinal() }

func (s FormalSlot) Vocabulary() formal.Vocabulary { return s.root.Vocabulary() }

// Root returns the one neutral structural descriptor shared by formal scalar
// slots, path roots, and identity terms. FormalSlot adds only forest-local
// ownership and acceleration coordinates; it never restates root identity.
func (s FormalSlot) Root() (formal.Root, bool) {
	if !s.Valid() {
		return formal.Root{}, false
	}
	return s.root, true
}

// relationRoot inverts the dense formal vocabulary through the owning frozen
// body schema. It is the structural publication edge; no symbol spelling or
// State inventory participates.
func (s FormalSlot) relationRoot() (Root, bool) {
	if !s.Valid() {
		return Root{}, false
	}
	body := s.space.bodies[s.relation]
	ordinal := s.root.Ordinal() - 1
	switch s.root.Vocabulary() {
	case formal.Input:
		for _, part := range []struct {
			kind  RootKind
			count uint32
		}{{RootParam, body.shape.Params}, {RootCapture, body.shape.Captures}, {RootGlobal, body.shape.Globals}, {RootAmbient, body.shape.Ambients}} {
			if ordinal < uint64(part.count) {
				return Root{Kind: part.kind, Index: uint32(ordinal)}, true
			}
			ordinal -= uint64(part.count)
		}
	case formal.Middle:
		if ordinal < body.middle {
			return Root{Kind: RootMiddle, Index: uint32(ordinal)}, true
		}
		ordinal -= body.middle
		if ordinal < uint64(body.shape.HeapTemplates) {
			return Root{Kind: RootHeapTemplate, Index: uint32(ordinal)}, true
		}
	case formal.Output:
		if ordinal < uint64(body.shape.Results) {
			return Root{Kind: RootResult, Index: uint32(ordinal)}, true
		}
	}
	return Root{}, false
}

// CanonicalBytes excludes the forest-local dense relation index. Equal
// structural slots from independently frozen spaces therefore encode alike,
// while the full body identity prevents hash collisions by construction.
func (s FormalSlot) CanonicalBytes() ([formalSlotCanonicalSize]byte, bool) {
	var out [formalSlotCanonicalSize]byte
	if !s.Valid() {
		return out, false
	}
	body := s.Body()
	copy(out[:len(body)], body[:])
	binary.BigEndian.PutUint64(out[len(body):len(body)+8], s.root.Ordinal())
	out[len(out)-1] = byte(s.root.Vocabulary())
	return out, true
}
