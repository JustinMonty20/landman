package connector

// ResetRegistry clears all registered sources, translators, and mergers.
// This is intended for use in tests to ensure isolated test state.
// DO NOT use in production code.
func ResetRegistry() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.sources = make(map[string]DataSource)
	registry.translators = make(map[string][]Translator)
	registry.mergers = make(map[string]Merger)
}
