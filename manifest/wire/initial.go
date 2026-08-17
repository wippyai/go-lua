package wire

import (
	"fmt"
	"sort"
)

const GlobalEnvironmentRoot = "GlobalEnvRoot"

func ModuleRootIdentity(providerIdentity string) string {
	return "ModuleRoot:" + providerIdentity
}

// InitialAggregate is the portable aggregate class of a provider-declared
// initial root. Dynamic tables and every later mutation remain runtime state.
type InitialAggregate uint8

const (
	InitialAggregateInvalid InitialAggregate = iota
	InitialAggregateTable
	InitialAggregateMetatable
)

// InitialMutability is the policy of one exact initial root/key entry.
type InitialMutability uint8

const (
	InitialMutabilityInvalid InitialMutability = iota
	InitialMutable
	InitialFrozen
)

// InitialRootReference selects either the declaring provider's ordinary
// module root or an explicitly declared initial root.
type InitialRootReference struct {
	Module   bool   `json:"module,omitempty"`
	Identity string `json:"identity,omitempty"`
}

func ProviderModuleRoot() InitialRootReference { return InitialRootReference{Module: true} }

func DeclaredInitialRoot(identity string) InitialRootReference {
	return InitialRootReference{Identity: identity}
}

// InitialValueKind is the closed reference vocabulary needed by provider
// bootstrap structure. Ordinary exported literals are derived from Export;
// only root aliases and callable placements need explicit declaration here.
type InitialValueKind uint8

const (
	InitialValueInvalid InitialValueKind = iota
	InitialValueRoot
	InitialValueFunction
)

type InitialValue struct {
	Kind     InitialValueKind     `json:"kind"`
	Root     InitialRootReference `json:"root,omitempty"`
	Function string               `json:"function,omitempty"`
}

func InitialRootValue(root InitialRootReference) InitialValue {
	return InitialValue{Kind: InitialValueRoot, Root: root}
}

func InitialFunctionValue(local string) InitialValue {
	return InitialValue{Kind: InitialValueFunction, Function: local}
}

// InitialRoot declares provider-owned bootstrap structure not implied by the
// provider's ordinary module mount.
type InitialRoot struct {
	Identity  string           `json:"identity"`
	Aggregate InitialAggregate `json:"aggregate"`
	Immutable bool             `json:"immutable,omitempty"`
}

// InitialEntry places an exact root alias or callable into the initial
// environment. Root and Value function names are provider-local references.
type InitialEntry struct {
	Root       InitialRootReference `json:"root"`
	Key        string               `json:"key"`
	Value      InitialValue         `json:"value"`
	Mutability InitialMutability    `json:"mutability"`
}

type InitialPrimitive uint8

const (
	InitialPrimitiveInvalid InitialPrimitive = iota
	InitialPrimitiveString
)

// InitialMetatableAttachment declares one primitive bootstrap attachment.
type InitialMetatableAttachment struct {
	Primitive InitialPrimitive     `json:"primitive"`
	Metatable InitialRootReference `json:"metatable"`
}

func (m *Manifest) DefineInitialRoot(root InitialRoot) {
	if m == nil {
		return
	}
	m.InitialRoots = append(m.InitialRoots, root)
}

func (m *Manifest) DefineInitialEntry(entry InitialEntry) {
	if m == nil {
		return
	}
	m.InitialEntries = append(m.InitialEntries, entry)
}

func (m *Manifest) DefineInitialMetatable(attachment InitialMetatableAttachment) {
	if m == nil {
		return
	}
	m.InitialMetatables = append(m.InitialMetatables, attachment)
}

func validateInitialEnvironment(m *Manifest) error {
	if m == nil {
		return nil
	}
	roots := make(map[string]struct{}, len(m.InitialRoots))
	for _, root := range m.InitialRoots {
		if root.Identity == "" || root.Aggregate < InitialAggregateTable || root.Aggregate > InitialAggregateMetatable {
			return fmt.Errorf("manifest: invalid initial root %q", root.Identity)
		}
		if _, exists := roots[root.Identity]; exists {
			return fmt.Errorf("manifest: duplicate initial root %q", root.Identity)
		}
		roots[root.Identity] = struct{}{}
	}
	rootValid := func(ref InitialRootReference) bool {
		if ref.Module {
			return ref.Identity == "" && m.Path != ""
		}
		_, ok := roots[ref.Identity]
		return ref.Identity != "" && ok
	}
	entries := make([]string, 0, len(m.InitialEntries))
	for _, entry := range m.InitialEntries {
		if !rootValid(entry.Root) || entry.Key == "" || entry.Mutability < InitialMutable || entry.Mutability > InitialFrozen {
			return fmt.Errorf("manifest: invalid initial entry %q", entry.Key)
		}
		switch entry.Value.Kind {
		case InitialValueRoot:
			if !rootValid(entry.Value.Root) || entry.Value.Function != "" {
				return fmt.Errorf("manifest: invalid initial root value for %q", entry.Key)
			}
		case InitialValueFunction:
			if entry.Value.Function == "" || entry.Value.Root != (InitialRootReference{}) {
				return fmt.Errorf("manifest: invalid initial function value for %q", entry.Key)
			}
			if _, direct := m.FunctionSignatures[entry.Value.Function]; !direct {
				return fmt.Errorf("manifest: initial entry %q names unknown function %q", entry.Key, entry.Value.Function)
			}
		default:
			return fmt.Errorf("manifest: invalid initial value kind for %q", entry.Key)
		}
		entries = append(entries, initialRootKey(entry.Root, entry.Key))
	}
	sort.Strings(entries)
	for index := 1; index < len(entries); index++ {
		if entries[index-1] == entries[index] {
			return fmt.Errorf("manifest: duplicate initial entry %q", entries[index])
		}
	}
	primitives := make(map[InitialPrimitive]struct{}, len(m.InitialMetatables))
	for _, attachment := range m.InitialMetatables {
		if attachment.Primitive != InitialPrimitiveString || !rootValid(attachment.Metatable) || attachment.Metatable.Module {
			return fmt.Errorf("manifest: invalid initial metatable attachment")
		}
		if _, exists := primitives[attachment.Primitive]; exists {
			return fmt.Errorf("manifest: duplicate initial metatable attachment")
		}
		primitives[attachment.Primitive] = struct{}{}
	}
	return nil
}

func initialRootKey(root InitialRootReference, key string) string {
	if root.Module {
		return "module\x00" + key
	}
	return root.Identity + "\x00" + key
}
