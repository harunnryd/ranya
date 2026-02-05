package llm

type ToolRegistry interface {
	Tools() []Tool
	HandleTool(name string, args map[string]any) (string, error)
}
