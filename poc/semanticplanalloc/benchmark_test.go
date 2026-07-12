package semanticplanalloc

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	legacy "github.com/wippyai/go-lua/poc/semanticplan"
)

var (
	benchmarkTransformer Transformer
	benchmarkEffect      BoundEffect
	benchmarkLegacyRows  []legacy.BoundRow
)

func benchmarkPaths() (pathdom.Path, pathdom.Path) {
	return pathdom.NewPath(1, "target").Field("field"), pathdom.NewPath(2, "source").Field("nested")
}

func BenchmarkSymbolicRepresentation(b *testing.B) {
	target, source := benchmarkPaths()

	b.Run("legacy-lift", func(b *testing.B) {
		op := legacy.PathAssignmentOp{Target: target, SourcePath: source, HasSourcePath: true}
		registry := legacy.DefaultPathAssignmentRegistry()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = registry.Lift(op)
		}
	})

	b.Run("packed-compile", func(b *testing.B) {
		registry := DefaultRegistry()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var ok bool
			benchmarkTransformer, ok = registry.CompilePathAssignment(target, source)
			if !ok {
				b.Fatal("compile failed")
			}
		}
	})

	legacyTransformer := legacy.DefaultPathAssignmentRegistry().Lift(legacy.PathAssignmentOp{
		Target: target, SourcePath: source, HasSourcePath: true,
	})
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "bound")
	legacyBindings := legacy.Bindings{
		Roots:  map[symbol.ID]pathdom.Path{},
		Values: map[pathdom.PathKey]product.Value{source.Key(): value},
	}
	// symbol.ID currently has uint64 underlying representation, but assign via
	// indexing so this benchmark follows the public Bindings shape.
	legacyBindings.Roots[target.Symbol] = pathdom.NewPath(10, "caller-target")
	legacyBindings.Roots[source.Symbol] = pathdom.NewPath(11, "caller-source")
	b.Run("legacy-substitute", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var ok bool
			benchmarkLegacyRows, ok = legacyTransformer.SubstituteTerms(legacyBindings)
			if !ok {
				b.Fatal("substitution failed")
			}
		}
	})

	packed, ok := DefaultRegistry().CompilePathAssignment(target, source)
	if !ok {
		b.Fatal("compile failed")
	}
	bindings := Bindings{
		Roots:  []pathdom.Path{pathdom.NewPath(10, "caller-target"), pathdom.NewPath(11, "caller-source")},
		Values: []product.Value{{}, value}, ValueMask: uint64(1) << sourceTerm,
	}
	b.Run("packed-bind-and-iterate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cursor, bound := packed.Bind(&bindings)
			if !bound {
				b.Fatal("bind failed")
			}
			for {
				effect, more := cursor.Next()
				if !more {
					break
				}
				benchmarkEffect = effect
			}
		}
	})
}
