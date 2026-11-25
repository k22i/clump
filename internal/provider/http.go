package provider

import "fmt"

// newOpenAI builds the OpenAI-compatible provider (TypeOpenAI). It is
// implemented by the provider-http task; until then it reports ErrConfig.
func newOpenAI(c Config) (Provider, error) {
	return nil, fmt.Errorf("%w: provider %q: type openai is not built yet", ErrConfig, c.Name)
}

// newAnthropic builds the Anthropic Messages provider (TypeAnthropic). It is
// implemented by the provider-http task; until then it reports ErrConfig.
func newAnthropic(c Config) (Provider, error) {
	return nil, fmt.Errorf("%w: provider %q: type anthropic is not built yet", ErrConfig, c.Name)
}
