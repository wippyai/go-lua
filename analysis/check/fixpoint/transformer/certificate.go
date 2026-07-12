package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SemanticCertificate proves that every present production plan cell and
// higher-layer extension has an explicit non-contextual verdict for every
// State lane. Its fields are private so a compiler cannot forge coverage.
type SemanticCertificate struct{ plan *operationplan.Plan }

// CertifyPlan validates source-operation coverage atomically. Unsupported
// means contextual; Unaffected and Supported are both explicit certificates.
func CertifyPlan(plan *operationplan.Plan, registry *SemanticCapabilityRegistry) (SemanticCertificate, error) {
	if plan == nil {
		return SemanticCertificate{}, fmt.Errorf("transformer: nil operation plan")
	}
	if registry == nil {
		return SemanticCertificate{}, fmt.Errorf("transformer: nil semantic capability registry")
	}
	if err := registry.Complete(stateCatalog()); err != nil {
		return SemanticCertificate{}, err
	}
	dependencies := plan.DependencyCursor()
	for kind, ok := dependencies.Next(); ok; kind, ok = dependencies.Next() {
		if unsupported := registry.UnsupportedFact(kind); len(unsupported) != 0 {
			return SemanticCertificate{}, fmt.Errorf("transformer: operation-plan dependency %d unsupported on lanes %v", kind, unsupported)
		}
	}
	for point := 0; point < plan.PointCount(); point++ {
		cursor := plan.Cursor(cfg.Point(point))
		for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
			if unsupported := registry.UnsupportedFact(cell.Kind()); len(unsupported) != 0 {
				return SemanticCertificate{}, fmt.Errorf("transformer: operation-plan kind %d unsupported on lanes %v", cell.Kind(), unsupported)
			}
		}
		extensions := plan.ExtensionCursor(cfg.Point(point))
		for cell, ok := extensions.Next(); ok; cell, ok = extensions.Next() {
			if unsupported := registry.UnsupportedExtension(cell.Kind()); len(unsupported) != 0 {
				return SemanticCertificate{}, fmt.Errorf("transformer: operation-plan extension %d unsupported on lanes %v", cell.Kind(), unsupported)
			}
		}
	}
	return SemanticCertificate{plan: plan}, nil
}
