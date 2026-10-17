package llm

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/dandandujie/dsr-go/encoding/tokenizer"
	"github.com/dandandujie/dsr-go/encoding/v4/dsv4"
	"github.com/dandandujie/dsr-go/encoding/v4/dsv41"
	"github.com/dandandujie/dsr-go/recipe/openai/chatcompletion"
	"github.com/dandandujie/dsr-go/recipe/request"
)

//go:embed tokenizers/deepseek-v4.json.gz tokenizers/deepseek-v41-tokens.json
var tokenizerAssets embed.FS

var v4Tokenizer = sync.OnceValues(func() (*tokenizer.Tokenizer, error) { return loadTokenizer(false) })
var v41Tokenizer = sync.OnceValues(func() (*tokenizer.Tokenizer, error) { return loadTokenizer(true) })

// CountTokens tokenizes the complete model prompt, including the chat template,
// tool protocol, reasoning markers, and generation prefix. It never uses a
// text-size heuristic. The pinned profiles cover text-only DeepSeek V4/V4.1.
func (c *Client) CountTokens(ctx context.Context, req ChatRequest) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = c.Model
	}
	profile, err := c.tokenProfile(req.Model)
	if err != nil {
		return 0, err
	}

	// Mirror the text-only GoAI wire adapter. In particular, a tool result's
	// Name is not sent, and empty user/system/tool contents remain present.
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		wire := map[string]any{"role": message.Role, "content": message.Content}
		if message.Role == "assistant" {
			if message.ReasoningContent != "" {
				wire["reasoning_content"] = message.ReasoningContent
			}
			if len(message.ToolCalls) > 0 {
				calls := append([]ToolCall(nil), message.ToolCalls...)
				for index := range calls {
					calls[index].Type = "function"
				}
				wire["tool_calls"] = calls
			}
		}
		if message.Role == "tool" {
			wire["tool_call_id"] = message.ToolCallID
		}
		messages = append(messages, wire)
	}
	tools := append([]ToolDefinition(nil), req.Tools...)
	for index := range tools {
		tools[index].Type = "function"
	}
	body, err := json.Marshal(map[string]any{"model": req.Model, "messages": messages, "tools": tools})
	if err != nil {
		return 0, fmt.Errorf("count prompt tokens: %w", err)
	}
	var typed chatcompletion.ChatCompletionRequest
	if err := json.Unmarshal(body, &typed); err != nil {
		return 0, fmt.Errorf("count prompt tokens: %w", err)
	}
	converted, err := typed.Convert(request.NewConversionOptions())
	if err != nil {
		return 0, fmt.Errorf("count prompt tokens: %w", err)
	}
	var ids []uint32
	if profile == "deepseek-v4" {
		loaded, err := v4Tokenizer()
		if err != nil {
			return 0, err
		}
		ids, err = dsv4.New().WithTokenizer(loaded).Encode(converted.Conversation)
		if err != nil {
			return 0, err
		}
	} else {
		loaded, err := v41Tokenizer()
		if err != nil {
			return 0, err
		}
		ids, err = dsv41.New().WithTokenizer(loaded).Encode(converted.Conversation)
		if err != nil {
			return 0, err
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (c *Client) tokenProfile(model string) (string, error) {
	if c.TokenEncoding != "" {
		if c.TokenEncoding == "deepseek-v4" || c.TokenEncoding == "deepseek-v4.1" {
			return c.TokenEncoding, nil
		}
		return "", fmt.Errorf("unsupported token encoding %q; use deepseek-v4 or deepseek-v4.1", c.TokenEncoding)
	}
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "deepseek-flash", "deepseek-v4.1-flash", "deepseek-v4-flash-vision-exp", "deepseek-ai/deepseek-v4.1-flash":
		return "deepseek-v4.1", nil
	case "deepseek-v4-pro", "deepseek-v4-pro-0813", "deepseek-v4-pro-0731", "deepseek-v4-flash-0731", "deepseek-ai/deepseek-v4-pro", "deepseek-ai/deepseek-v4-flash":
		return "deepseek-v4", nil
	case "deepseek-v4-flash":
		base, err := url.Parse(c.baseURL())
		if err != nil {
			return "", err
		}
		// DeepSeek's hosted legacy Flash alias now routes to V4.1. A custom
		// server using the original V4 model keeps the literal V4 profile.
		if strings.EqualFold(base.Hostname(), "api.deepseek.com") {
			return "deepseek-v4.1", nil
		}
		return "deepseek-v4", nil
	default:
		return "", fmt.Errorf("no exact token encoding for model %q; set EXCELSIOR_TOKEN_ENCODING (or --token-encoding) to the DeepSeek format served by your endpoint, or supply a provider with CountTokens", model)
	}
}

func loadTokenizer(v41 bool) (*tokenizer.Tokenizer, error) {
	compressed, err := tokenizerAssets.ReadFile("tokenizers/deepseek-v4.json.gz")
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, err
	}
	if v41 {
		// The published pipelines share the entire BPE vocabulary and merges;
		// only eight added-token definitions differ. Embed that small delta.
		var definition map[string]json.RawMessage
		if err := json.Unmarshal(data, &definition); err != nil {
			return nil, err
		}
		var added []json.RawMessage
		if err := json.Unmarshal(definition["added_tokens"], &added); err != nil {
			return nil, err
		}
		patch, err := tokenizerAssets.ReadFile("tokenizers/deepseek-v41-tokens.json")
		if err != nil {
			return nil, err
		}
		var replacements []struct {
			Index int             `json:"index"`
			Token json.RawMessage `json:"token"`
		}
		if err := json.Unmarshal(patch, &replacements); err != nil {
			return nil, err
		}
		for _, replacement := range replacements {
			if replacement.Index < 0 || replacement.Index >= len(added) {
				return nil, fmt.Errorf("invalid bundled tokenizer delta index %d", replacement.Index)
			}
			added[replacement.Index] = replacement.Token
		}
		definition["added_tokens"], err = json.Marshal(added)
		if err != nil {
			return nil, err
		}
		data, err = json.Marshal(definition)
		if err != nil {
			return nil, err
		}
	}
	return tokenizer.LoadBytes(data)
}
