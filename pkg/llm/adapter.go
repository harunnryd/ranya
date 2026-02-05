package llm

import "context"

type Tool struct {
	Name                         string
	Description                  string
	Schema                       any
	RequiresConfirmation         bool
	ConfirmationPrompt           string
	ConfirmationPromptByLanguage map[string]string
}

type Context struct {
	Messages []map[string]any
	Tools    []Tool
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Response struct {
	Text         string
	Tokens       int
	Usage        Usage
	FinishReason string
	ToolCalls    []ToolCall
	HandoffAgent string
}

type LLMAdapter interface {
	Generate(ctx context.Context, input Context) (Response, error)
	Stream(ctx context.Context, input Context) (<-chan string, error)
	MapTools(tools []Tool) (providerTools any, err error)
	ToProviderFormat(ctx Context) (any, error)
	FromProviderFormat(raw any) (Response, error)
	Name() string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}
