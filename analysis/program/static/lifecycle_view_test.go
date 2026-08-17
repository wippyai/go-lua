package static

import "testing"

func TestLifecycleViewExpiresEveryTypedProjectionAfterCommit(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View()
	if !construction.Available() || construction.Types().Primitives().Count() != 1 {
		t.Fatal("claimed construction View did not expose its authored component")
	}
	if construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("construction View exposed unexpected rows")
	}
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Available() || construction.Types().Primitives().Count() != 0 ||
		construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("expired construction View retained a typed projection")
	}
	if !component.View().Available() || component.View().Types().Primitives().Count() != 1 {
		t.Fatal("published Component View lost its typed projection")
	}
}
