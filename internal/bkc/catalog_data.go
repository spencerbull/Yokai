package bkc

// catalog is the in-memory registry of every known BKC entry. It is
// populated by init() functions in each catalog_<vendor>.go file via
// register() so that entries can be grouped by model provider.
var catalog []Config

// register appends one or more BKC configs to the package-level catalog.
// It panics on duplicate IDs to catch copy/paste errors at program start.
func register(configs ...Config) {
	seen := make(map[string]struct{}, len(catalog))
	for _, cfg := range catalog {
		seen[cfg.ID] = struct{}{}
	}
	for _, cfg := range configs {
		if cfg.ID == "" {
			panic("bkc: config with empty ID")
		}
		if _, dup := seen[cfg.ID]; dup {
			panic("bkc: duplicate config id: " + cfg.ID)
		}
		seen[cfg.ID] = struct{}{}
		catalog = append(catalog, cfg)
	}
}
