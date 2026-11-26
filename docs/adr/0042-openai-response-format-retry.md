# One retry when a server rejects `response_format`

`openAIProvider` sends `response_format: {"type":"json_object"}` and, when a server rejects the
field, retries the same request once without it and reports that attempt.

`response_format` is what makes the JSON contract reliable on OpenAI itself and on DeepSeek/OpenRouter,
but some OpenAI-compatible servers (older Ollama, some LiteLLM fronts) return 400 for it. Rather
than dropping the field for everyone, it is attempted and then removed once — `responseFormat` is a
pointer so its absence is expressible.
