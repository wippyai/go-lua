package transformer

import "testing"

var (
	benchmarkOutputRegistry   *OutputCapabilityRegistry
	benchmarkSemanticRegistry *SemanticCapabilityRegistry
	benchmarkEffectCatalog    *EffectCatalog
)

func BenchmarkDefaultOutputCapabilityRegistry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkOutputRegistry = DefaultOutputCapabilityRegistry()
	}
}

func BenchmarkDefaultSemanticCapabilityRegistry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkSemanticRegistry = DefaultSemanticCapabilityRegistry()
	}
}

func BenchmarkDefaultEffectCatalog(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkEffectCatalog = DefaultEffectCatalog()
	}
}
