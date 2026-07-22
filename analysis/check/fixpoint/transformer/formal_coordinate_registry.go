package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// freezeFormalCoordinateRegistry records the complete root vocabulary before
// any coordinate selector is built. Classes are read only from the sealed
// rekey tags; this registry never derives identity from selector adjacency.
func freezeFormalCoordinateRegistry(domain state.ProductDomain, rekey state.CoordinateFormalRootRekey) (*formalCoordinateRegistry, error) {
	roots, err := domain.CoordinateFormalRoots(rekey)
	if err != nil {
		return nil, fmt.Errorf("formal coordinate registry: root vocabulary: %w", err)
	}
	// A sealed relation may lawfully have no coordinate roots: for example, a
	// body with no inputs, middle registers, or results.  It still owns a
	// complete (empty) vocabulary, so retain an explicit empty registry rather
	// than treating that absence as a malformed rekey.
	if len(roots) == 0 {
		return &formalCoordinateRegistry{
			classes:   make(map[formal.Root]formal.LexicalClassID),
			members:   make(map[formal.LexicalClassID][]formal.Root),
			alphabets: make(map[formal.OccurrenceID]formalWriteAlphabet),
		}, nil
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Less(roots[j]) })
	builder := newFormalCoordinateRegistryBuilder(roots[0].Owner())
	for _, root := range roots {
		class, tagged := domain.CoordinateFormalRootClass(rekey, root)
		if !tagged {
			return nil, fmt.Errorf("formal coordinate registry: root has no lexical class")
		}
		if err := builder.addClass(root, class); err != nil {
			return nil, err
		}
	}
	return builder.freeze()
}

// formalCoordinateRegistry is the inert, freeze-time contract shared by the
// later epoch, alias, and provider stages. Its three relations have different
// laws: classes are immutable, aliases require an exact guard, and alphabets
// require an exact occurrence.
type formalCoordinateRegistry struct {
	owner     lexicalidentity.StableLexicalBodyID
	classes   map[formal.Root]formal.LexicalClassID
	members   map[formal.LexicalClassID][]formal.Root
	aliases   []formalGuardedAliasFact
	alphabets map[formal.OccurrenceID]formalWriteAlphabet
}

type formalGuardScope struct {
	occurrence formal.OccurrenceID
	branch     uint32
}

func (s formalGuardScope) valid() bool { return s.occurrence.Valid() && s.branch != 0 }

type formalGuardedAliasFact struct {
	left, right formal.Root
	guard       formalGuardScope
}
type formalWriteAlphabet struct {
	occurrence formal.OccurrenceID
	roots      []formal.Root
}

func (a formalWriteAlphabet) contains(root formal.Root) bool {
	index := sort.Search(len(a.roots), func(index int) bool { return !a.roots[index].Less(root) })
	return index < len(a.roots) && a.roots[index] == root
}

type formalCoordinateRegistryBuilder struct {
	owner     lexicalidentity.StableLexicalBodyID
	classes   map[formal.Root]formal.LexicalClassID
	members   map[formal.LexicalClassID][]formal.Root
	aliases   []formalGuardedAliasFact
	alphabets map[formal.OccurrenceID]formalWriteAlphabet
}

func newFormalCoordinateRegistryBuilder(owner lexicalidentity.StableLexicalBodyID) *formalCoordinateRegistryBuilder {
	return &formalCoordinateRegistryBuilder{owner: owner, classes: make(map[formal.Root]formal.LexicalClassID), members: make(map[formal.LexicalClassID][]formal.Root), alphabets: make(map[formal.OccurrenceID]formalWriteAlphabet)}
}

func (b *formalCoordinateRegistryBuilder) addClass(root formal.Root, class formal.LexicalClassID) error {
	if b == nil || b.owner == (lexicalidentity.StableLexicalBodyID{}) || !root.Valid() || !class.Valid() || root.Owner() != b.owner || class.Owner() != b.owner {
		return fmt.Errorf("formal coordinate registry: invalid lexical class binding")
	}
	if prior, exists := b.classes[root]; exists {
		if prior != class {
			return fmt.Errorf("formal coordinate registry: root has two lexical classes")
		}
		return nil
	}
	b.classes[root] = class
	b.members[class] = append(b.members[class], root)
	return nil
}

func (b *formalCoordinateRegistryBuilder) addAlias(left, right formal.Root, guard formalGuardScope) error {
	if b == nil || !left.Valid() || !right.Valid() || left.Owner() != b.owner || right.Owner() != b.owner || !guard.valid() || guard.occurrence.Owner() != b.owner {
		return fmt.Errorf("formal coordinate registry: invalid guarded alias")
	}
	if right.Less(left) {
		left, right = right, left
	}
	b.aliases = append(b.aliases, formalGuardedAliasFact{left: left, right: right, guard: guard})
	return nil
}

func (b *formalCoordinateRegistryBuilder) addAlphabet(occurrence formal.OccurrenceID, roots []formal.Root) error {
	if b == nil || !occurrence.Valid() || occurrence.Owner() != b.owner {
		return fmt.Errorf("formal coordinate registry: invalid write alphabet")
	}
	if _, exists := b.alphabets[occurrence]; exists {
		return fmt.Errorf("formal coordinate registry: duplicate write alphabet")
	}
	owned := append([]formal.Root(nil), roots...)
	for _, root := range owned {
		if !root.Valid() || root.Owner() != b.owner {
			return fmt.Errorf("formal coordinate registry: foreign write alphabet root")
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Less(owned[j]) })
	for index := 1; index < len(owned); index++ {
		if owned[index-1] == owned[index] {
			return fmt.Errorf("formal coordinate registry: duplicate write alphabet root")
		}
	}
	b.alphabets[occurrence] = formalWriteAlphabet{occurrence: occurrence, roots: owned}
	return nil
}

func (b *formalCoordinateRegistryBuilder) freeze() (*formalCoordinateRegistry, error) {
	if b == nil || b.owner == (lexicalidentity.StableLexicalBodyID{}) || len(b.classes) == 0 {
		return nil, fmt.Errorf("formal coordinate registry: empty root registry")
	}
	classes := make(map[formal.Root]formal.LexicalClassID, len(b.classes))
	for root, class := range b.classes {
		classes[root] = class
	}
	members := make(map[formal.LexicalClassID][]formal.Root, len(b.members))
	for class, roots := range b.members {
		copyRoots := append([]formal.Root(nil), roots...)
		sort.Slice(copyRoots, func(i, j int) bool { return copyRoots[i].Less(copyRoots[j]) })
		members[class] = copyRoots
	}
	aliases := append([]formalGuardedAliasFact(nil), b.aliases...)
	sort.Slice(aliases, func(i, j int) bool {
		if aliases[i].guard.occurrence != aliases[j].guard.occurrence {
			return aliases[i].guard.occurrence.Ordinal() < aliases[j].guard.occurrence.Ordinal()
		}
		if aliases[i].guard.branch != aliases[j].guard.branch {
			return aliases[i].guard.branch < aliases[j].guard.branch
		}
		if aliases[i].left != aliases[j].left {
			return aliases[i].left.Less(aliases[j].left)
		}
		return aliases[i].right.Less(aliases[j].right)
	})
	alphabets := make(map[formal.OccurrenceID]formalWriteAlphabet, len(b.alphabets))
	for occurrence, alphabet := range b.alphabets {
		alphabet.roots = append([]formal.Root(nil), alphabet.roots...)
		alphabets[occurrence] = alphabet
	}
	return &formalCoordinateRegistry{owner: b.owner, classes: classes, members: members, aliases: aliases, alphabets: alphabets}, nil
}

func (r *formalCoordinateRegistry) class(root formal.Root) (formal.LexicalClassID, bool) {
	if r == nil || !root.Valid() || root.Owner() != r.owner {
		return formal.LexicalClassID{}, false
	}
	class, ok := r.classes[root]
	return class, ok
}
func (r *formalCoordinateRegistry) classMembers(class formal.LexicalClassID) []formal.Root {
	if r == nil || !class.Valid() || class.Owner() != r.owner {
		return nil
	}
	return append([]formal.Root(nil), r.members[class]...)
}
func (r *formalCoordinateRegistry) aliasesAt(guard formalGuardScope) []formalGuardedAliasFact {
	if r == nil || !guard.valid() || guard.occurrence.Owner() != r.owner {
		return nil
	}
	var out []formalGuardedAliasFact
	for _, alias := range r.aliases {
		if alias.guard == guard {
			out = append(out, alias)
		}
	}
	return out
}
func (r *formalCoordinateRegistry) alphabet(occurrence formal.OccurrenceID) (formalWriteAlphabet, bool) {
	if r == nil || !occurrence.Valid() || occurrence.Owner() != r.owner {
		return formalWriteAlphabet{}, false
	}
	alphabet, ok := r.alphabets[occurrence]
	if !ok {
		return formalWriteAlphabet{}, false
	}
	alphabet.roots = append([]formal.Root(nil), alphabet.roots...)
	return alphabet, true
}
