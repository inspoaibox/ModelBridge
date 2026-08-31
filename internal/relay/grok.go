package relay

import "context"

// Grok uses xAI's OpenAI-compatible Chat Completions endpoint. The existing
// OpenAI SDK client preserves the same request/response and status semantics.
type GrokProvider struct{}

func (GrokProvider) ChatCompletions(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionResponse, error) {
	return (OpenAIProvider{}).ChatCompletions(ctx, upstream)
}

func (GrokProvider) NewChatCompletionStream(ctx context.Context, upstream UpstreamChatCompletionRequest) (ChatCompletionStream, error) {
	return (OpenAIProvider{}).NewChatCompletionStream(ctx, upstream)
}

func (GrokProvider) CreateEmbeddings(ctx context.Context, upstream UpstreamEmbeddingRequest) (EmbeddingResponse, error) {
	return (OpenAIProvider{}).CreateEmbeddings(ctx, upstream)
}
