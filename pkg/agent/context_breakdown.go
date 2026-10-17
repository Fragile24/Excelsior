package agent

import (
	"context"

	"excelsior/pkg/llm"
)

// TokenBreakdown allocates the exact assembled prompt size across message
// roles. Role-level values are estimates because chat-template overhead is
// shared across messages and cannot be attributed to a single role precisely.
type TokenBreakdown struct {
	System    int `json:"system"`
	Tools     int `json:"tools"`
	User      int `json:"user"`
	Assistant int `json:"assistant"`
}

func (m *contextManager) tokenBreakdown(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, total int) *TokenBreakdown {
	var groups [4][]llm.Message
	var tools []llm.ToolDefinition
	for _, message := range messages {
		switch message.Role {
		case "system":
			groups[0] = append(groups[0], message)
		case "user":
			groups[2] = append(groups[2], message)
		case "assistant":
			if len(message.ToolCalls) == 0 {
				groups[3] = append(groups[3], message)
				continue
			}
			callMessage := message
			callMessage.Content = ""
			callMessage.ReasoningContent = ""
			groups[1] = append(groups[1], callMessage)
			if message.Content != "" || message.ReasoningContent != "" {
				message.ToolCalls = nil
				groups[3] = append(groups[3], message)
			}
		case "tool":
			groups[1] = append(groups[1], message)
		default:
			groups[0] = append(groups[0], message)
		}
	}
	tools = append(tools, toolDefs...)

	var raw [4]int
	for index, group := range groups {
		definitions := []llm.ToolDefinition(nil)
		if index == 1 {
			definitions = tools
		}
		if len(group) == 0 && len(definitions) == 0 {
			continue
		}
		count, err := m.count(ctx, group, definitions, m.config.MaxOutputTokens)
		if err != nil {
			return nil
		}
		raw[index] = count
	}
	return apportionTokens(total, raw)
}

func apportionTokens(total int, raw [4]int) *TokenBreakdown {
	var sum int64
	for _, count := range raw {
		if count > 0 {
			sum += int64(count)
		}
	}
	if total < 0 || sum == 0 {
		return nil
	}

	values := [4]int{}
	var remainders [4]int64
	assigned := 0
	for index, count := range raw {
		if count <= 0 {
			continue
		}
		product := int64(total) * int64(count)
		values[index] = int(product / sum)
		remainders[index] = product % sum
		assigned += values[index]
	}
	for assigned < total {
		best := 0
		for index := 1; index < len(remainders); index++ {
			if remainders[index] > remainders[best] {
				best = index
			}
		}
		values[best]++
		remainders[best] = -1
		assigned++
	}
	return &TokenBreakdown{System: values[0], Tools: values[1], User: values[2], Assistant: values[3]}
}
