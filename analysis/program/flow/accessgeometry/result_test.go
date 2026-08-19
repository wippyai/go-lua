package accessgeometry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
)

func TestAccessGeometryProvenanceFenceAndDenominators(t *testing.T) {
	result := accessGeometryTestResult()
	sourceID, flowID, staticID, moduleID := result.sourceID, result.flowID, result.staticID, result.moduleID
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("matching owner quartet was rejected")
	}
	if Matches(result, sourceID, flowID, staticID, identity.ContentID{}) ||
		Matches(result, sourceID, flowID, staticID, flowtest.ContentIDAt(9)) {
		t.Fatal("foreign or unavailable provenance matched")
	}
	if result.TableFields().Count() != 2 || result.ExactLenses().Count() != 1 || result.DynamicLenses().Count() != 1 {
		t.Fatal("typed views did not retain their exact authored denominators")
	}
	if _, ok := result.TableFields().At(2); ok {
		t.Fatal("TableField At escaped its denominator")
	}
}
