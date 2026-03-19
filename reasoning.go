package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxToolLoops = 20

// ExecuteReasoningBlock is the pure executor for reasoning Blocks (runtime: reasoning).
// It reads prompt.md, constructs an LLM API call, handles tool-use loops,
// and returns the structured output.
//
// Outer-level logging (block.start, block.complete, block.error) is NOT done here —
// that's the wrapper's job. However, tool-level logging (logToolCall, logToolResult)
// IS done here because these are execution-level events that happen mid-reasoning,
// inside the tool-use loop.
func ExecuteReasoningBlock(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte) *ExecutionResult {
	// Resolve model
	model, err := ResolveModel(block, rootDomain)
	if err != nil {
		return &ExecutionResult{Error: err, Meta: map[string]interface{}{}}
	}

	// Resolve provider
	provider, err := ResolveProvider(model, block.Config.Provider, rootDomain.Providers)
	if err != nil {
		return &ExecutionResult{Error: err, Meta: map[string]interface{}{"model": model}}
	}

	// Read prompt.md
	promptPath := filepath.Join(block.Dir, strings.TrimPrefix(block.Config.Prompt, "./"))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return &ExecutionResult{
			Error: fmt.Errorf("failed to read prompt.md for Block '%s': %w", block.Config.Name, err),
			Meta:  map[string]interface{}{"model": model, "provider": provider.Name},
		}
	}

	// Read output schema from block.yaml (already parsed from YAML)
	outSchemaObj := block.Config.Schema.Out
	if outSchemaObj == nil {
		return &ExecutionResult{
			Error: fmt.Errorf("no output schema defined in block.yaml for Block '%s'", block.Config.Name),
			Meta:  map[string]interface{}{"model": model, "provider": provider.Name},
		}
	}

	// Resolve tool Blocks (if any)
	var toolBlocks []*DiscoveredBlock
	var toolNames []string
	for _, toolName := range block.Config.Tools {
		toolBlock, err := FindBlock(projectRoot, toolName)
		if err != nil {
			return &ExecutionResult{
				Error: fmt.Errorf("tool Block '%s' referenced by '%s': %w", toolName, block.Config.Name, err),
				Meta:  map[string]interface{}{"model": model, "provider": provider.Name},
			}
		}
		if toolBlock.Config.Runtime == "embedded" {
			return &ExecutionResult{
				Error: fmt.Errorf("tool Block '%s' has runtime 'embedded' — only process and reasoning Blocks can be tools", toolName),
				Meta:  map[string]interface{}{"model": model, "provider": provider.Name},
			}
		}
		toolBlocks = append(toolBlocks, toolBlock)
		toolNames = append(toolNames, toolName)
	}

	// Dispatch to the correct provider format
	var output []byte
	var totalInputTokens, totalOutputTokens, toolLoops int

	switch provider.Format {
	case "anthropic":
		output, totalInputTokens, totalOutputTokens, toolLoops, err = runAnthropicReasoning(block, provider, model, string(prompt), input, outSchemaObj, toolBlocks, rootDomain, projectRoot)
	case "openai":
		output, err = runOpenAIReasoning(block, provider, model, string(prompt), input, outSchemaObj, toolBlocks, rootDomain, projectRoot)
	default:
		err = fmt.Errorf("unknown provider format '%s' for provider '%s'", provider.Format, provider.Name)
	}

	// Build metadata for the wrapper to include in log entries
	meta := map[string]interface{}{
		"model":         model,
		"provider":      provider.Name,
		"input_tokens":  totalInputTokens,
		"output_tokens": totalOutputTokens,
		"tool_loops":    toolLoops,
	}
	if len(toolNames) > 0 {
		meta["tools"] = toolNames
	}

	return &ExecutionResult{
		Output: output,
		Stderr: "", // Reasoning blocks don't produce stderr (no subprocess)
		Error:  err,
		Meta:   meta,
	}
}

// runAnthropicReasoning handles reasoning Blocks using the Anthropic API format.
// Returns output bytes, total input tokens, total output tokens, tool loop count, and error.
func runAnthropicReasoning(block *DiscoveredBlock, provider *ResolvedProvider, model, prompt string, input []byte, outSchema interface{}, toolBlocks []*DiscoveredBlock, rootDomain *DomainYaml, projectRoot string) ([]byte, int, int, int, error) {
	// Track token usage across all API calls in the loop
	var totalInputTokens, totalOutputTokens int

	// Build tool definitions for the API call
	var tools []AnthropicTool

	// Add tool Blocks as callable tools
	for _, tb := range toolBlocks {
		// Read input schema from block.yaml (already parsed from YAML)
		inSchemaObj := tb.Config.Schema.In
		if inSchemaObj == nil {
			return nil, 0, 0, 0, fmt.Errorf("no input schema defined in block.yaml for tool Block '%s'", tb.Config.Name)
		}

		tools = append(tools, AnthropicTool{
			Name:        tb.Config.Name,
			Description: tb.Config.Description,
			InputSchema: inSchemaObj,
		})
	}

	// Add the structured output tool — Anthropic uses tool_use pattern
	// to enforce structured output. The model is forced to call this tool
	// with the output schema, guaranteeing the response shape.
	outputToolName := "_aglet_output"
	tools = append(tools, AnthropicTool{
		Name:        outputToolName,
		Description: "Return the final structured output for this Block.",
		InputSchema: outSchema,
	})

	// Build initial messages
	messages := []AnthropicMessage{
		{Role: "user", Content: string(input)},
	}

	// Conversation loop — handle tool use
	for i := 0; i < maxToolLoops; i++ {
		req := AnthropicRequest{
			Model:     model,
			MaxTokens: 4096,
			System:    prompt,
			Messages:  messages,
			Tools:     tools,
		}

		// Make the API call
		resp, err := callAnthropic(provider, req)
		if err != nil {
			return nil, totalInputTokens, totalOutputTokens, i, fmt.Errorf("Anthropic API error for Block '%s': %w", block.Config.Name, err)
		}

		// Accumulate token usage from this API call
		if resp.Usage != nil {
			totalInputTokens += resp.Usage.InputTokens
			totalOutputTokens += resp.Usage.OutputTokens
		}

		// Append assistant response to conversation
		messages = append(messages, AnthropicMessage{
			Role:    "assistant",
			Content: resp.Content,
		})

		// Check for tool use in the response
		var hasToolUse bool
		var toolResults []AnthropicContentBlock

		for _, content := range resp.Content {
			if content.Type != "tool_use" {
				continue
			}
			hasToolUse = true

			// Check if this is the structured output tool
			if content.Name == outputToolName {
				// This is the final output — serialize and return
				outputData, err := json.Marshal(content.Input)
				if err != nil {
					return nil, totalInputTokens, totalOutputTokens, i, fmt.Errorf("failed to serialize output from Block '%s': %w", block.Config.Name, err)
				}
				return outputData, totalInputTokens, totalOutputTokens, i, nil
			}

			// This is a regular tool call — log it and execute the tool Block.
			// Tool-level logging stays here because it's part of the execution
			// flow: the reasoning block is mid-thought, calling tools, continuing.
			// The wrapper can't know about individual tool calls.
			logToolCall(block, content.Name, i, "")
			toolStart := time.Now()

			toolInput, err := json.Marshal(content.Input)
			if err != nil {
				return nil, totalInputTokens, totalOutputTokens, i, fmt.Errorf("failed to serialize tool input for '%s': %w", content.Name, err)
			}

			toolResult, err := executeToolBlock(content.Name, toolInput, rootDomain, projectRoot)
			toolDurationMs := time.Since(toolStart).Milliseconds()

			var resultContent interface{}
			if err != nil {
				logToolResult(block, content.Name, toolDurationMs, false, "")
				resultContent = fmt.Sprintf("Tool error: %s", err.Error())
			} else {
				logToolResult(block, content.Name, toolDurationMs, true, "")
				resultContent = string(toolResult)
			}

			toolResults = append(toolResults, AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: content.ID,
				Content:   resultContent,
			})
		}

		if !hasToolUse {
			// No tool use and no output tool — check for text response
			for _, content := range resp.Content {
				if content.Type == "text" && content.Text != "" {
					return []byte(content.Text), totalInputTokens, totalOutputTokens, i, nil
				}
			}
			return nil, totalInputTokens, totalOutputTokens, i, fmt.Errorf("Block '%s': no output produced by reasoning", block.Config.Name)
		}

		// Add tool results to the conversation and continue the loop
		messages = append(messages, AnthropicMessage{
			Role:    "user",
			Content: toolResults,
		})
	}

	return nil, totalInputTokens, totalOutputTokens, maxToolLoops, fmt.Errorf("Block '%s': exceeded maximum tool loops (%d)", block.Config.Name, maxToolLoops)
}

// callAnthropic makes an HTTP request to the Anthropic API.
func callAnthropic(provider *ResolvedProvider, req AnthropicRequest) (*AnthropicResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", provider.URL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", provider.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AnthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	return &result, nil
}

// runOpenAIReasoning handles reasoning Blocks using the OpenAI-compatible API format.
func runOpenAIReasoning(block *DiscoveredBlock, provider *ResolvedProvider, model, prompt string, input []byte, outSchema interface{}, toolBlocks []*DiscoveredBlock, rootDomain *DomainYaml, projectRoot string) ([]byte, error) {
	// Build tool definitions
	var tools []OpenAITool
	for _, tb := range toolBlocks {
		// Read input schema from block.yaml (already parsed from YAML)
		inSchemaObj := tb.Config.Schema.In
		if inSchemaObj == nil {
			return nil, fmt.Errorf("no input schema defined in block.yaml for tool Block '%s'", tb.Config.Name)
		}

		tools = append(tools, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        tb.Config.Name,
				Description: tb.Config.Description,
				Parameters:  inSchemaObj,
			},
		})
	}

	// Build structured output format
	var responseFormat *OpenAIResponseFormat
	if outSchema != nil {
		responseFormat = &OpenAIResponseFormat{
			Type: "json_schema",
			JSONSchema: map[string]interface{}{
				"name":   block.Config.Name + "_output",
				"strict": true,
				"schema": outSchema,
			},
		}
	}

	// Build messages
	messages := []OpenAIMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: string(input)},
	}

	// Conversation loop — handle tool use
	for i := 0; i < maxToolLoops; i++ {
		req := OpenAIRequest{
			Model:    model,
			Messages: messages,
		}
		if len(tools) > 0 {
			req.Tools = tools
		}

		// Only apply response_format when no tools or when tools are done
		if len(tools) == 0 && responseFormat != nil {
			req.ResponseFormat = responseFormat
		}

		resp, err := callOpenAI(provider, req)
		if err != nil {
			return nil, fmt.Errorf("OpenAI API error for Block '%s': %w", block.Config.Name, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("Block '%s': no choices in API response", block.Config.Name)
		}

		choice := resp.Choices[0]

		// If no tool calls, this is the final response
		if len(choice.Message.ToolCalls) == 0 {
			if content, ok := choice.Message.Content.(string); ok && content != "" {
				return []byte(content), nil
			}
			return nil, fmt.Errorf("Block '%s': empty response from reasoning", block.Config.Name)
		}

		// Append assistant message with tool calls
		messages = append(messages, choice.Message)

		// Execute each tool call — tool-level logging stays here
		for _, tc := range choice.Message.ToolCalls {
			logToolCall(block, tc.Function.Name, i, "")
			toolStart := time.Now()

			toolResult, err := executeToolBlock(tc.Function.Name, []byte(tc.Function.Arguments), rootDomain, projectRoot)
			toolDurationMs := time.Since(toolStart).Milliseconds()

			var resultStr string
			if err != nil {
				logToolResult(block, tc.Function.Name, toolDurationMs, false, "")
				resultStr = fmt.Sprintf("Tool error: %s", err.Error())
			} else {
				logToolResult(block, tc.Function.Name, toolDurationMs, true, "")
				resultStr = string(toolResult)
			}

			messages = append(messages, OpenAIMessage{
				Role:       "tool",
				Content:    resultStr,
				ToolCallID: tc.ID,
			})
		}
	}

	return nil, fmt.Errorf("Block '%s': exceeded maximum tool loops (%d)", block.Config.Name, maxToolLoops)
}

// callOpenAI makes an HTTP request to an OpenAI-compatible API.
func callOpenAI(provider *ResolvedProvider, req OpenAIRequest) (*OpenAIResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	url := provider.URL + "/v1/chat/completions"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result OpenAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	return &result, nil
}

// executeToolBlock runs a tool Block (process or reasoning) and returns its output.
// Tool blocks are executed through WrapBlock so they get full observability too.
func executeToolBlock(name string, input []byte, rootDomain *DomainYaml, projectRoot string) ([]byte, error) {
	toolBlock, err := FindBlock(projectRoot, name)
	if err != nil {
		return nil, err
	}

	// Execute through the wrapper so the tool block gets its own
	// observability (logs, behavioral memory updates, etc.)
	return WrapBlock(toolBlock, rootDomain, projectRoot, input)
}
