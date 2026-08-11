package architecture

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// CollectSurvey obtains exactly the declared source parent and fields from
// semantic.Session. It is read-only; an existing declared destination joins
// the typed frontier only so its package identity can be checked.
func CollectSurvey(ctx context.Context, session *semantic.Session, declaration Declaration) (Survey, error) {
	sources, err := declarationSources(declaration)
	if err != nil {
		return Survey{}, err
	}
	snapshot, err := session.SurveyBoundary(ctx, sources, declaration.Destination.Path)
	if err != nil {
		return Survey{}, fmt.Errorf("architecture survey: %w", err)
	}
	if err := validateSurvey(snapshot, declaration.Parent, declaration.Fields); err != nil {
		return Survey{}, err
	}
	return Survey{
		snapshot:    snapshot,
		declaration: canonicalDeclaration(declaration),
	}, nil
}

func declarationSources(declaration Declaration) ([]cutplan.SymbolRef, error) {
	if declaration.Parent.Object == "" {
		return nil, fmt.Errorf("architecture declaration requires a source parent")
	}
	if len(declaration.Fields) == 0 {
		return nil, fmt.Errorf("architecture declaration requires exact source fields")
	}
	seen := map[string]bool{declaration.Parent.Object: true}
	result := make([]cutplan.SymbolRef, 0, len(declaration.Fields)+1)
	result = append(result, declaration.Parent)
	for _, field := range declaration.Fields {
		if field.Object == "" || seen[field.Object] {
			return nil, fmt.Errorf("architecture declaration has duplicate or empty source identity %q", field.Object)
		}
		seen[field.Object] = true
		result = append(result, field)
	}
	return canonicalRefs(result), nil
}

func validateSurvey(snapshot semantic.Snapshot, parent cutplan.SymbolRef, fields []cutplan.SymbolRef) error {
	if snapshot.Workspace == nil {
		return fmt.Errorf("architecture survey has no typed workspace")
	}
	wanted, err := declarationSources(Declaration{Parent: parent, Fields: fields})
	if err != nil {
		return err
	}
	seen := make(map[string]cutplan.ObjectEvidence, len(snapshot.Objects))
	for _, object := range snapshot.Objects {
		if object.Role != cutplan.ObjectSource {
			return fmt.Errorf("architecture survey contains non-source evidence %s", object.Object.Object)
		}
		if _, exists := seen[object.Object.Object]; exists {
			return fmt.Errorf("architecture survey has duplicate evidence %s", object.Object.Object)
		}
		seen[object.Object.Object] = object
	}
	if len(seen) != len(wanted) {
		return fmt.Errorf("architecture survey source denominator does not equal declaration")
	}
	for _, source := range wanted {
		if _, exists := seen[source.Object]; !exists {
			return fmt.Errorf("architecture survey misses declared source %s", source.Object)
		}
	}
	return nil
}

func canonicalRefs(values []cutplan.SymbolRef) []cutplan.SymbolRef {
	result := append([]cutplan.SymbolRef(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		return result[left].Object < result[right].Object
	})
	return result
}

func canonicalDeclaration(value Declaration) Declaration {
	result := value
	result.Fields = canonicalRefs(value.Fields)
	result.Laws = append([]cutplan.Law(nil), value.Laws...)
	sort.Slice(result.Laws, func(left, right int) bool {
		if result.Laws[left].ID != result.Laws[right].ID {
			return result.Laws[left].ID < result.Laws[right].ID
		}
		if result.Laws[left].Package != result.Laws[right].Package {
			return result.Laws[left].Package < result.Laws[right].Package
		}
		return result.Laws[left].Test < result.Laws[right].Test
	})
	return result
}

func sameDeclaration(left, right Declaration) bool {
	return reflect.DeepEqual(canonicalDeclaration(left), canonicalDeclaration(right))
}
