package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

const maxAliasHops = 4

type Registry struct {
	configs map[string]Config
	order   []string
	aliases map[string]string

	mu    sync.Mutex
	built map[string]Provider
}

func NewRegistry(configs []Config, aliases map[string]string) (*Registry, error) {
	r := &Registry{
		configs: make(map[string]Config, len(configs)),
		order:   make([]string, 0, len(configs)),
		aliases: make(map[string]string, len(aliases)),
		built:   make(map[string]Provider, len(configs)),
	}
	for _, c := range configs {
		if c.Name == "" {
			return nil, fmt.Errorf("provider %q: name is empty", c.Name)
		}
		if _, ok := r.configs[c.Name]; ok {
			return nil, fmt.Errorf("duplicate provider %q", c.Name)
		}
		r.configs[c.Name] = c
		r.order = append(r.order, c.Name)
	}
	for name, target := range aliases {
		if target == "" {
			continue
		}
		if _, ok := r.configs[name]; ok {
			return nil, fmt.Errorf("alias %q shadows a provider of the same name", name)
		}
		r.aliases[name] = target
	}
	for _, name := range r.aliasNames() {
		if _, err := r.followAlias(name); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) aliasNames() []string {
	names := make([]string, 0, len(r.aliases))
	for name := range r.aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) followAlias(name string) (string, error) {
	cur := name
	for hop := 0; hop <= maxAliasHops; hop++ {
		if _, ok := r.configs[cur]; ok {
			return cur, nil
		}
		next, ok := r.aliases[cur]
		if !ok {
			return "", fmt.Errorf("alias %q: unknown provider %q", name, cur)
		}
		cur = next
	}
	return "", fmt.Errorf("alias %q: cycle or chain longer than %d", name, maxAliasHops)
}

func (r *Registry) Resolve(nameOrAlias string) (string, error) {
	if _, ok := r.configs[nameOrAlias]; ok {
		return nameOrAlias, nil
	}
	if _, ok := r.aliases[nameOrAlias]; ok {
		return r.followAlias(nameOrAlias)
	}
	return "", fmt.Errorf("unknown provider or alias %q (known: %s)",
		nameOrAlias, strings.Join(r.known(), ", "))
}

func (r *Registry) known() []string {
	names := make([]string, 0, len(r.configs)+len(r.aliases))
	names = append(names, r.order...)
	names = append(names, r.aliasNames()...)
	sort.Strings(names)
	return names
}

func (r *Registry) Get(nameOrAlias string) (Provider, Config, error) {
	name, err := r.Resolve(nameOrAlias)
	if err != nil {
		return nil, Config{}, err
	}
	cfg := r.configs[name]

	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.built[name]; ok {
		return p, cfg, nil
	}
	p, err := New(cfg)
	if err != nil {
		return nil, Config{}, err
	}
	r.built[name] = p
	return p, cfg, nil
}

func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

func (r *Registry) Aliases(name string) []string {
	var out []string
	for _, alias := range r.aliasNames() {
		if target, err := r.followAlias(alias); err == nil && target == name {
			out = append(out, alias)
		}
	}
	return out
}

func (r *Registry) Set(name string, p Provider) error {
	if _, ok := r.configs[name]; !ok {
		return fmt.Errorf("unknown provider %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.built[name] = p
	return nil
}
