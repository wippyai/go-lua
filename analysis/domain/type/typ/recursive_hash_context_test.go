package typ

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
)

func TestEqualityHashContextCancelsRecursiveTraversal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fields := make([]Field, 128)
	for i := range fields {
		fields[i] = Field{Name: fmt.Sprintf("field%d", i), Type: &cancelHashType{cancel: cancel}}
	}
	// A Generic forces EqualityHash to use the recursive traversal. Its fields
	// cancel after the traversal begins, proving the traversal itself—not only
	// the entry check—observes cancellation.
	value := &Generic{Name: "ManyFields", Body: &Record{Fields: fields}}
	started := time.Now()
	_, err := EqualityHashContext(ctx, value)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EqualityHashContext error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("EqualityHashContext cancellation took %s, want prompt return", elapsed)
	}
}

type cancelHashType struct {
	once   sync.Once
	cancel context.CancelFunc
}

func (*cancelHashType) Kind() kind.Kind { return kind.Record }
func (*cancelHashType) String() string  { return "cancel-hash" }
func (t *cancelHashType) Hash() uint64 {
	t.once.Do(t.cancel)
	return 1
}
func (t *cancelHashType) Equals(other Type) bool {
	_, ok := other.(*cancelHashType)
	return ok
}
