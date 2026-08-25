package registry_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/authority"
	"github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	"github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// Structural declarations belong to registry. CheckView is the composition
// surface and must not project the same unavailable/duplicate finding into
// every proof report.
func TestCheckViewLeavesStructuralFindingsInRegistry(t *testing.T) {
	builder := plan.NewBuilder( /* deliberately unavailable schema identity */ model.SchemaID{})
	_ = builder.AddExpression(plan.DefineExpressionRef(model.ExpressionID{}, nil))
	_ = builder.AddExpression(plan.DefineExpressionRef(model.ExpressionID{}, nil))
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("unchecked schema build failed")
	}
	indexed := registry.Build(schema)
	issues := indexed.Issues()
	if len(issues) == 0 {
		t.Fatal("registry discarded structural findings")
	}
	var expressionUnavailable, expressionDuplicate int
	for _, issue := range issues {
		switch issue.Code {
		case registry.CodeExpressionUnavailable:
			expressionUnavailable++
		case registry.CodeExpressionDuplicate:
			expressionDuplicate++
		}
	}
	if expressionUnavailable != 2 || expressionDuplicate != 1 {
		t.Fatalf("registry findings=%v, unavailable=%d duplicate=%d", issues, expressionUnavailable, expressionDuplicate)
	}
	if report := typing.CheckView(indexed); !report.Valid() {
		for _, issue := range report.Issues() {
			if issue.Code == typing.CodeUnavailable || issue.Code == typing.CodeDuplicateIdentity || issue.Code == typing.CodeExpressionDigest {
				t.Fatalf("typing CheckView projected registry issue: %v", issue)
			}
		}
	}
	if report := authority.CheckView(indexed); !report.Valid() {
		t.Fatalf("authority CheckView projected registry issue: %s", report.Error())
	}
	if _, err := recurrence.CheckView(indexed); err != nil {
		t.Fatalf("recurrence CheckView projected registry issue: %v", err)
	}
	if report := typing.Check(schema); report.Valid() {
		t.Fatal("standalone typing Check dropped registry findings")
	}
	if report := authority.Check(schema); report.Valid() {
		t.Fatal("standalone authority Check dropped registry findings")
	}
	if _, err := recurrence.Check(schema); err == nil {
		t.Fatal("standalone recurrence Check dropped registry findings")
	}
}
