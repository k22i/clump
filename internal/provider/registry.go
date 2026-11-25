package provider

// Registry holds the configured providers and the aliases (cheap, strong,
// default) that point at them.
type Registry struct {
	configs map[string]Config
	order   []string
	aliases map[string]string
	built   map[string]Provider
}

// NewRegistry builds a registry. configs keep their order for Names.
// aliases maps an alias ("cheap", "strong", "default") to a provider name or
// to another alias; chains are followed up to 4 hops. It errors on a
// duplicate or empty config name, an alias that shadows a config name, an
// alias pointing at nothing, or an alias cycle; errors name the alias or
// provider at fault, e.g. `alias "cheap": unknown provider "haiku"`.
func NewRegistry(configs []Config, aliases map[string]string) (*Registry, error) {
	panic("provider: NewRegistry not implemented")
}

// Resolve maps a provider name or alias to a provider name.
func (r *Registry) Resolve(nameOrAlias string) (string, error) {
	panic("provider: Registry.Resolve not implemented")
}

// Get resolves nameOrAlias and returns the provider (built once, then
// reused) and its config.
func (r *Registry) Get(nameOrAlias string) (Provider, Config, error) {
	panic("provider: Registry.Get not implemented")
}

// Names returns the provider names (not aliases) in config order.
func (r *Registry) Names() []string {
	panic("provider: Registry.Names not implemented")
}

// Aliases returns the aliases that resolve to the provider name, sorted.
func (r *Registry) Aliases(name string) []string {
	panic("provider: Registry.Aliases not implemented")
}

// Set replaces the built provider for a name (tests inject a *Fake). The
// name must already exist.
func (r *Registry) Set(name string, p Provider) error {
	panic("provider: Registry.Set not implemented")
}
