package provider

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	configs := []Config{
		{Name: "a", Type: TypeFake},
		{Name: "b", Type: TypeFake},
	}
	aliases := map[string]string{"cheap": "a", "strong": "b", "default": "cheap"}
	r, err := NewRegistry(configs, aliases)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	t.Run("resolve", func(t *testing.T) {
		for nameOrAlias, want := range map[string]string{
			"a": "a", "b": "b", "cheap": "a", "strong": "b", "default": "a",
		} {
			got, err := r.Resolve(nameOrAlias)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", nameOrAlias, err)
			}
			if got != want {
				t.Fatalf("Resolve(%q) = %q, want %q", nameOrAlias, got, want)
			}
		}
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := r.Resolve("nope")
		if err == nil {
			t.Fatal("Resolve(nope) succeeded")
		}
		for _, want := range []string{`unknown provider or alias "nope"`, "known: a, b, cheap, default, strong"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want it to contain %q", err, want)
			}
		}
	})

	t.Run("names", func(t *testing.T) {
		want := []string{"a", "b"}
		got := r.Names()
		if len(got) != len(want) {
			t.Fatalf("Names() = %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Names() = %q, want %q", got, want)
			}
		}
		got[0] = "mutated"
		if r.Names()[0] != "a" {
			t.Fatal("Names() exposes the registry's own slice")
		}
	})

	t.Run("aliases", func(t *testing.T) {
		if got := strings.Join(r.Aliases("a"), ","); got != "cheap,default" {
			t.Fatalf("Aliases(a) = %q, want \"cheap,default\"", got)
		}
		if got := strings.Join(r.Aliases("b"), ","); got != "strong" {
			t.Fatalf("Aliases(b) = %q, want \"strong\"", got)
		}
		if got := r.Aliases("nope"); len(got) != 0 {
			t.Fatalf("Aliases(nope) = %q, want none", got)
		}
	})

	t.Run("get caches", func(t *testing.T) {
		p1, cfg1, err := r.Get("default")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if cfg1.Name != "a" || cfg1.Type != TypeFake {
			t.Fatalf("config = %+v, want provider a", cfg1)
		}
		p2, _, err := r.Get("a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if p1 != p2 {
			t.Fatalf("Get returned two providers: %v and %v", p1, p2)
		}
		if p1.Name() != "a" {
			t.Fatalf("Name() = %q, want \"a\"", p1.Name())
		}
	})

	t.Run("set replaces", func(t *testing.T) {
		before, _, err := r.Get("b")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		injected := &Fake{ProviderName: "b", Responses: []string{"injected"}}
		if err := r.Set("b", injected); err != nil {
			t.Fatalf("Set: %v", err)
		}
		after, _, err := r.Get("b")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if after != injected || after == before {
			t.Fatalf("Get after Set = %v, want the injected provider", after)
		}
		text, _, err := after.Complete(context.Background(), "x")
		if err != nil || text != "injected" {
			t.Fatalf("injected provider answered %q, %v", text, err)
		}
	})

	t.Run("set unknown name", func(t *testing.T) {
		if err := r.Set("nope", &Fake{}); err == nil {
			t.Fatal("Set(nope) succeeded")
		}
		if err := r.Set("cheap", &Fake{}); err == nil {
			t.Fatal("Set(cheap) succeeded; an alias is not a provider name")
		}
	})
}

func TestRegistryErrors(t *testing.T) {
	fake := func(names ...string) []Config {
		out := make([]Config, 0, len(names))
		for _, n := range names {
			out = append(out, Config{Name: n, Type: TypeFake})
		}
		return out
	}
	tests := []struct {
		name    string
		configs []Config
		aliases map[string]string
		want    string
	}{
		{
			name:    "empty name",
			configs: []Config{{Name: "", Type: TypeFake}},
			want:    `provider "": name is empty`,
		},
		{
			name:    "duplicate provider",
			configs: fake("a", "a"),
			want:    `duplicate provider "a"`,
		},
		{
			name:    "alias shadows a provider",
			configs: fake("a"),
			aliases: map[string]string{"a": "a"},
			want:    `alias "a" shadows a provider of the same name`,
		},
		{
			name:    "alias to an unknown provider",
			configs: fake("a"),
			aliases: map[string]string{"cheap": "haiku"},
			want:    `alias "cheap": unknown provider "haiku"`,
		},
		{
			name:    "alias to an unknown alias",
			configs: fake("a"),
			aliases: map[string]string{"cheap": "cheaper"},
			want:    `alias "cheap": unknown provider "cheaper"`,
		},
		{
			name:    "two alias cycle",
			configs: fake("a"),
			aliases: map[string]string{"x": "y", "y": "x"},
			want:    `alias "x": cycle or chain longer than 4`,
		},
		{
			name:    "self cycle",
			configs: fake("a"),
			aliases: map[string]string{"loop": "loop"},
			want:    `alias "loop": cycle or chain longer than 4`,
		},
		{
			name:    "chain of five hops",
			configs: fake("a"),
			aliases: map[string]string{"a1": "a2", "a2": "a3", "a3": "a4", "a4": "a5", "a5": "a"},
			want:    `alias "a1": cycle or chain longer than 4`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewRegistry(tc.configs, tc.aliases)
			if err == nil {
				t.Fatalf("NewRegistry = %v, want an error", r)
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestRegistryAliasChainOfFourIsAllowed(t *testing.T) {
	configs := []Config{{Name: "a", Type: TypeFake}}
	aliases := map[string]string{"a1": "a2", "a2": "a3", "a3": "a4", "a4": "a"}
	r, err := NewRegistry(configs, aliases)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if got, err := r.Resolve("a1"); err != nil || got != "a" {
		t.Fatalf("Resolve(a1) = %q, %v; want a", got, err)
	}
	if got := strings.Join(r.Aliases("a"), ","); got != "a1,a2,a3,a4" {
		t.Fatalf("Aliases(a) = %q, want every alias in the chain", got)
	}
}

func TestRegistryEmptyAliasTargetIsIgnored(t *testing.T) {
	configs := []Config{{Name: "a", Type: TypeFake}}
	r, err := NewRegistry(configs, map[string]string{"cheap": ""})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, err := r.Resolve("cheap"); err == nil {
		t.Fatal("an alias with an empty target was kept")
	}
}

func TestRegistryConcurrentGet(t *testing.T) {
	r, err := NewRegistry([]Config{{Name: "a", Type: TypeFake}}, map[string]string{"cheap": "a"})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const n = 20
	got := make([]Provider, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, _, err := r.Get("a")
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			got[i] = p
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if got[i] == nil || got[i] != got[0] {
			t.Fatalf("Get %d returned %v, want the same provider as %v", i, got[i], got[0])
		}
	}
}
