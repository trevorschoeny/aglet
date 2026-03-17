package main

// BlockYaml represents the parsed block.yaml for a Block.
type BlockYaml struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Domain      string   `yaml:"domain"`
	Role        string   `yaml:"role"`
	Runtime     string   `yaml:"runtime"`     // process | embedded | reasoning
	Model       string   `yaml:"model"`       // LLM model (reasoning only, inheritable)
	Provider    string   `yaml:"provider"`    // Explicit provider override (reasoning only)
	Prompt      string   `yaml:"prompt"`      // Path to prompt.md (reasoning only)
	Impl        string   `yaml:"impl"`        // Path to main.* (process/embedded only)
	Schema      struct {
		In  interface{} `yaml:"in"`
		Out interface{} `yaml:"out"`
	} `yaml:"schema"`
	Calls              []string          `yaml:"calls"`
	Tools              []string          `yaml:"tools"`              // Blocks callable during reasoning
	Execution          string            `yaml:"execution"`
	Error              string            `yaml:"error"`
	BehavioralMemory   *BehavioralMemory `yaml:"behavioral_memory,omitempty"` // Written by aglet stats --write
}

// DomainYaml represents the parsed domain.yaml.
type DomainYaml struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Parent      string            `yaml:"parent"`
	Entrypoints []string          `yaml:"entrypoints"`
	Runners     map[string]string `yaml:"runners"`
	Providers   map[string]ProviderConfig `yaml:"providers"`
	Defaults    DomainDefaults    `yaml:"defaults"`
}

// ProviderConfig holds LLM provider connection details.
type ProviderConfig struct {
	Env    string `yaml:"env"`    // Environment variable for API key
	URL    string `yaml:"url"`    // Custom API endpoint (optional)
	Format string `yaml:"format"` // "anthropic" or "openai" (optional, inferred for built-ins)
}

// DomainDefaults holds inheritable defaults.
type DomainDefaults struct {
	Execution string `yaml:"execution"`
	Error     string `yaml:"error"`
	Model     string `yaml:"model"`
}

// BehavioralMemory holds the AML-computed behavioral profile of a Block.
// This section is authored and maintained solely by `aglet stats --write`.
// It lives in block.yaml alongside developer-declared fields, forming the
// complete Semantic Overlay: declared meaning + learned behavior.
type BehavioralMemory struct {
	TotalCalls      int            `yaml:"total_calls" json:"total_calls"`
	AvgRuntimeMs    float64        `yaml:"avg_runtime_ms" json:"avg_runtime_ms"`
	ErrorRate       float64        `yaml:"error_rate" json:"error_rate"`
	WarmthScore     float64        `yaml:"warmth_score" json:"warmth_score"`
	WarmthLevel     string         `yaml:"warmth_level" json:"warmth_level"` // hot | warm | cold
	LastCalled      string         `yaml:"last_called,omitempty" json:"last_called,omitempty"`
	VersionSince    string         `yaml:"version_since,omitempty" json:"version_since,omitempty"`    // timestamp of the block.updated event that triggered the current measurement window
	TokenAvg        int            `yaml:"token_avg,omitempty" json:"token_avg,omitempty"`            // reasoning only
	ObservedCallees map[string]int `yaml:"observed_callees,omitempty" json:"observed_callees,omitempty"` // tool blocks invoked by this block (reasoning only)
	ObservedCallers map[string]int `yaml:"observed_callers,omitempty" json:"observed_callers,omitempty"` // blocks that invoked this block as a tool
	LastUpdated     string         `yaml:"last_updated" json:"last_updated"`
}

// DiscoveredBlock holds a parsed Block and its filesystem location.
type DiscoveredBlock struct {
	Config BlockYaml
	Dir    string // Absolute path to the Block directory
}

// SurfaceYaml represents the parsed surface.yaml.
type SurfaceYaml struct {
	ID          string                       `yaml:"id"`
	Name        string                       `yaml:"name"`
	Description string                       `yaml:"description"`
	Domain      string                       `yaml:"domain"`
	Version     string                       `yaml:"version"`
	Entry       string                       `yaml:"entry"`
	Framework   string                       `yaml:"framework"`
	Bundler     string                       `yaml:"bundler"`
	Dev         struct {
		Command string `yaml:"command"`
		Port    int    `yaml:"port"`
	} `yaml:"dev"`
	Contract    SurfaceContract              `yaml:"contract"`
}

// SurfaceContract holds the contract section of surface.yaml.
type SurfaceContract struct {
	Dependencies map[string]ContractDependency `yaml:"dependencies"`
	Events       map[string]interface{}        `yaml:"events"`
}

// ContractDependency represents a dependency in the Surface contract.
type ContractDependency struct {
	Block    string      `yaml:"block"`
	Pipeline string      `yaml:"pipeline"`
	Callers  []string    `yaml:"callers"`
	Input    interface{} `yaml:"input"`
	Output   interface{} `yaml:"output"`
	Intent   string      `yaml:"intent"`
	Trigger  string      `yaml:"trigger"`
}

// ComponentYaml represents the parsed component.yaml for a Component.
type ComponentYaml struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Domain      string   `yaml:"domain"`
	Role        string   `yaml:"role"`
	Consumes    []string `yaml:"consumes"`
}

// --- Anthropic API types ---

type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []AnthropicMessage `json:"messages"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

type AnthropicContentBlock struct {
	Type      string      `json:"type"`
	Text      string      `json:"text,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Input     interface{} `json:"input,omitempty"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"`
}

type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

type AnthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []AnthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      *AnthropicUsage         `json:"usage,omitempty"`
	Error      *AnthropicError         `json:"error,omitempty"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// --- OpenAI API types ---

type OpenAIRequest struct {
	Model          string          `json:"model"`
	Messages       []OpenAIMessage `json:"messages"`
	Tools          []OpenAITool    `json:"tools,omitempty"`
	ResponseFormat *OpenAIResponseFormat `json:"response_format,omitempty"`
}

type OpenAIMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
	Type     string         `json:"type"` // "function"
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAIResponseFormat struct {
	Type       string      `json:"type"` // "json_schema"
	JSONSchema interface{} `json:"json_schema,omitempty"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Error   *OpenAIError   `json:"error,omitempty"`
}

type OpenAIChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}
