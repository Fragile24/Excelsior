package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"excelsior/pkg/llm"
)

const summaryPrompt = `You are compacting a coding agent's conversation to save context. You receive the oldest complete interaction units and may receive a previous running summary.

Return text only, without tools. Update the previous summary: retain current facts, remove stale ones, and preserve exact paths, symbols, errors, event ids, and unfinished work. Describe results only when supported by the transcript. Be concise and omit empty sections.

Use these headings:
## Goal
## Files touched
## Done
## Pending
## Errors & fixes
## Current state
## Next step`

// ContextStatus describes the exact assembled model input, independently of
// cumulative provider usage. Counts are sampled before a main model call.
type ContextStatus struct {
	Model         string          `json:"model"`
	State         string          `json:"state"`  // ready | compacting | compacted | unchanged | error
	Reason        string          `json:"reason"` // automatic | manual
	InputTokens   int             `json:"inputTokens"`
	InputBudget   int             `json:"inputBudget"`
	OutputReserve int             `json:"outputReserve"`
	WindowTokens  int             `json:"windowTokens"`
	SoftLimit     int             `json:"softLimit"`
	HardLimit     int             `json:"hardLimit"`
	Tier          ContextTier     `json:"tier"`
	CanCompact    bool            `json:"canCompact"`
	BeforeTokens  int             `json:"beforeTokens"`
	Compactions   int             `json:"compactions"`
	Breakdown     *TokenBreakdown `json:"breakdown,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type contextManager struct {
	provider        llm.ChatProvider
	model           string
	reasoningEffort llm.ReasoningEffort
	config          HarnessConfig
	usage           *usageAcc
	summary         string
	store           *contextStore
	nextID          int
	elided          map[string]string // stub -> archived observation id
	counts          map[[32]byte]int
	emit            func(StreamEvent)
	manual          bool
	changes         int
	compactions     int
}

type contextStore struct {
	directory string
	files     map[string]string
}

func newContextStore() (*contextStore, error) {
	directory, err := os.MkdirTemp("", "excelsior-context-")
	if err != nil {
		return nil, fmt.Errorf("create context store: %w", err)
	}
	return &contextStore{directory: directory, files: make(map[string]string)}, nil
}

func (s *contextStore) save(id, content string) error {
	path := filepath.Join(s.directory, id+".txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	s.files[id] = path
	return nil
}

func (s *contextStore) load(id string) (string, error) {
	path, ok := s.files[id]
	if !ok {
		return "", fmt.Errorf("unknown event %q", id)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read stored event %q: %w", id, err)
	}
	return string(content), nil
}

func (s *contextStore) close() {
	if s != nil && s.directory != "" {
		_ = os.RemoveAll(s.directory)
	}
}

func newContextManager(provider llm.ChatProvider, model string, config HarnessConfig, reasoningEffort llm.ReasoningEffort, usage *usageAcc) (*contextManager, error) {
	manager := &contextManager{
		provider:        provider,
		model:           model,
		reasoningEffort: reasoningEffort,
		config:          config,
		usage:           usage,
	}
	if usesRecall(config.ContextTier) {
		store, err := newContextStore()
		if err != nil {
			return nil, err
		}
		manager.store = store
		manager.elided = make(map[string]string)
	}
	return manager, nil
}

func (m *contextManager) manage(ctx context.Context, history []llm.Message, injected []llm.Message, toolDefs []llm.ToolDefinition, manual bool) (managed []llm.Message, err error) {
	m.manual = manual
	defer func() { m.manual = false }()
	inputSize := func() (int, error) {
		return m.count(ctx, assembleMessages(history, m.summaryForInput(), injected), toolDefs, m.config.MaxOutputTokens)
	}
	size, err := inputSize()
	if err != nil {
		return nil, err
	}
	budget := m.config.inputBudget()
	soft := int(float64(budget) * softContextRatio)
	hard := int(float64(budget) * hardContextRatio)
	if m.config.ContextTier != ContextT4 {
		soft = hard
	}
	status := ContextStatus{
		Model: m.model, State: "ready", Reason: "automatic", InputTokens: size,
		InputBudget: budget, OutputReserve: m.config.MaxOutputTokens, WindowTokens: m.config.ContextWindowTokens,
		SoftLimit: soft, HardLimit: hard, Tier: m.config.ContextTier, CanCompact: m.canCompact(history),
		BeforeTokens: size, Compactions: m.compactions,
	}
	if manual {
		status.Reason = "manual"
	}
	report := func() {
		if m.emit != nil {
			value := status
			m.emit(StreamEvent{Type: "context", Context: &value})
		}
	}
	defer func() {
		if err != nil {
			status.State, status.Error = "error", err.Error()
			status.InputTokens = size
			report()
		}
	}()
	if (manual || size >= soft) && status.CanCompact {
		status.State = "compacting"
		report()
	}
	beforeChanges := m.changes
	if (manual || size >= soft) && usesElision(m.config.ContextTier) {
		history, err = m.elideMiddle(ctx, history)
		if err != nil {
			return nil, err
		}
		size, err = inputSize()
		if err != nil {
			return nil, err
		}
	}

	if usesSummarization(m.config.ContextTier) {
		for attempts := 0; (manual || size >= hard) && attempts < maxSummaryCalls; attempts++ {
			previousLength := len(history)
			history, err = m.summarizeOldest(ctx, history)
			if err != nil {
				return nil, err
			}
			if len(history) == previousLength {
				break
			}
			size, err = inputSize()
			if err != nil {
				return nil, err
			}
		}
	}

	status.State = "ready"
	if m.changes > beforeChanges {
		m.compactions++
		status.State = "compacted"
	} else if manual {
		status.State = "unchanged"
	}
	status.InputTokens = size
	status.Compactions = m.compactions
	status.CanCompact = m.canCompact(history)
	status.Breakdown = m.tokenBreakdown(ctx, assembleMessages(history, m.summaryForInput(), injected), toolDefs, size)
	if size > budget {
		return nil, contextBudgetError(size, budget, m.config.ContextTier)
	}
	report()
	return history, nil
}

// Manual compaction keeps the last two complete model turns, regardless of
// their size. Automatic management retains the paper's 30% recent window.
func (m *contextManager) canCompact(history []llm.Message) bool {
	_, turns := splitHistory(history)
	end := len(turns) - minimumRecentTurns
	if end <= 0 || m.config.ContextTier == ContextT0 {
		return false
	}
	if usesSummarization(m.config.ContextTier) {
		return true
	}
	for _, turn := range turns[:end] {
		for _, message := range turn {
			if message.Role == "tool" && utf8.RuneCountInString(message.Content) > 500 && !strings.HasPrefix(message.Content, "[tool output elided:") {
				return true
			}
		}
	}
	return false
}

func (m *contextManager) summaryForInput() string {
	if m.summary == "" || m.store == nil {
		return m.summary
	}
	return m.summary + fmt.Sprintf("\nArchived observations and turns are available through recall_event: event_1 through event_%d. These ids are local to this run.", m.nextID)
}

func usesElision(tier ContextTier) bool {
	return tier == ContextT1 || tier == ContextT2 || tier == ContextT4
}
func usesRecall(tier ContextTier) bool { return tier == ContextT2 || tier == ContextT4 }
func usesSummarization(tier ContextTier) bool {
	return tier == ContextT3 || tier == ContextT4
}

func contextBudgetError(tokens, budget int, tier ContextTier) error {
	return fmt.Errorf("%w: %d prompt tokens exceed the %d-token input allowance after reserving output (tier %d); shorten the conversation or increase the window", ErrContextBudget, tokens, budget, tier)
}

func (m *contextManager) count(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, outputTokens int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	req := llm.ChatRequest{
		Model: m.model, Messages: messages, Tools: tools,
		ContextWindowTokens: m.config.ContextWindowTokens,
		MaxTokens:           intPointer(outputTokens), Temperature: floatPointer(m.config.Temperature), TopP: floatPointer(m.config.TopP),
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	key := sha256.Sum256(encoded)
	if count, ok := m.counts[key]; ok {
		return count, nil
	}
	count, err := m.provider.CountTokens(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("count context tokens: %w", err)
	}
	if count < 0 {
		return 0, errors.New("token counter returned a negative count")
	}
	if m.counts == nil {
		m.counts = make(map[[32]byte]int)
	}
	m.counts[key] = count
	return count, nil
}

func assembleMessages(history []llm.Message, summary string, injected []llm.Message) []llm.Message {
	out := append([]llm.Message(nil), history...)
	if summary != "" {
		index := preambleEnd(out)
		message := llm.Message{Role: "system", Content: "<context-summary>\n" + summary + "\n</context-summary>"}
		out = append(out, llm.Message{})
		copy(out[index+1:], out[index:])
		out[index] = message
	}
	out = append(out, injected...)
	return out
}

func preambleEnd(history []llm.Message) int {
	index := 0
	for index < len(history) && history[index].Role == "system" {
		index++
	}
	if index < len(history) && history[index].Role == "user" {
		index++
	}
	return index
}

// A paper turn is one model invocation and all of its tool observations.
// Keep call/result groups intact even during a single long autonomous task.
func splitHistory(history []llm.Message) ([]llm.Message, [][]llm.Message) {
	index := preambleEnd(history)
	preamble := history[:index]
	var turns [][]llm.Message
	for index < len(history) {
		start := index
		for index < len(history) && history[index].Role != "assistant" {
			index++
		}
		if index == len(history) && len(turns) > 0 {
			// A follow-up waiting for its first response must remain verbatim.
			turns[len(turns)-1] = append(turns[len(turns)-1], history[start:]...)
			break
		}
		if index < len(history) {
			index++ // assistant
		}
		for index < len(history) && history[index].Role == "tool" {
			index++
		}
		turns = append(turns, append([]llm.Message(nil), history[start:index]...))
	}
	return preamble, turns
}

func joinHistory(preamble []llm.Message, turns [][]llm.Message) []llm.Message {
	length := len(preamble)
	for _, turn := range turns {
		length += len(turn)
	}
	out := make([]llm.Message, 0, length)
	out = append(out, preamble...)
	for _, turn := range turns {
		out = append(out, turn...)
	}
	return out
}

// recentStart counts each candidate tail as a complete prompt. Preserve at
// least two whole turns even when they exceed the 30% target.
func (m *contextManager) recentStart(ctx context.Context, turns [][]llm.Message) (int, error) {
	if len(turns) <= minimumRecentTurns {
		return 0, nil
	}
	budget := int(float64(m.config.inputBudget()) * recentContextRatio)
	start := len(turns) - minimumRecentTurns
	if m.manual {
		return start, nil
	}
	for start > 0 {
		tokens, err := m.count(ctx, joinHistory(nil, turns[start-1:]), nil, m.config.MaxOutputTokens)
		if err != nil {
			return 0, err
		}
		if tokens > budget {
			break
		}
		start--
	}
	return start, nil
}

func (m *contextManager) elideMiddle(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	preamble, turns := splitHistory(history)
	middleEnd, err := m.recentStart(ctx, turns)
	if err != nil {
		return nil, err
	}
	for turnIndex := 0; turnIndex < middleEnd; turnIndex++ {
		for messageIndex := range turns[turnIndex] {
			message := &turns[turnIndex][messageIndex]
			if message.Role != "tool" || utf8.RuneCountInString(message.Content) <= 500 || strings.HasPrefix(message.Content, "[tool output elided:") {
				continue
			}
			original := message.Content
			stub := fmt.Sprintf("[tool output elided: %d lines / %d chars; re-read or re-run to recover it", strings.Count(original, "\n")+1, utf8.RuneCountInString(original))
			if usesRecall(m.config.ContextTier) {
				id, err := m.archive(original)
				if err != nil {
					return nil, fmt.Errorf("store elided tool output: %w", err)
				}
				stub += "; call recall_event with id " + id
				m.elided[stub+".]"] = id
			}
			message.Content = stub + ".]"
			m.changes++
		}
	}
	return joinHistory(preamble, turns), nil
}

func (m *contextManager) summarizeOldest(ctx context.Context, history []llm.Message) ([]llm.Message, error) {
	preamble, turns := splitHistory(history)
	middleEnd, err := m.recentStart(ctx, turns)
	if err != nil {
		return nil, err
	}
	if middleEnd == 0 {
		return history, nil // immutable preamble and recent turns cannot be compacted
	}
	var excerpt strings.Builder
	for _, message := range preamble {
		if message.Role == "user" {
			excerpt.WriteString("Original task (reference):\n" + message.Content + "\n\n")
		}
	}
	if m.summary != "" {
		excerpt.WriteString("Previous running summary:\n")
		excerpt.WriteString(m.summary)
		excerpt.WriteString("\n\n")
	}
	excerpt.WriteString("Oldest complete interaction units:\n")
	summaryOutput := min(2048, m.config.MaxOutputTokens)
	summaryBudget := m.config.ContextWindowTokens - summaryOutput
	maxSummaryInputTokens := int(float64(summaryBudget) * 0.4)
	count := 0
	for _, turn := range turns[:middleEnd] {
		var unit strings.Builder
		for _, message := range turn {
			unit.WriteString(message.Role)
			if message.Name != "" {
				unit.WriteString(" " + message.Name + " " + message.ToolCallID)
			}
			unit.WriteString(": ")
			unit.WriteString(message.Content)
			if message.ReasoningContent != "" {
				unit.WriteString("\nreasoning: " + message.ReasoningContent)
			}
			unit.WriteString("\n")
			for _, call := range message.ToolCalls {
				unit.WriteString("tool_call ")
				unit.WriteString(call.Function.Name)
				unit.WriteString(" ")
				unit.WriteString(call.Function.Arguments)
				unit.WriteString("\n")
			}
		}
		candidate := excerpt.String() + unit.String()
		candidateTokens, err := m.count(ctx, []llm.Message{{Role: "system", Content: summaryPrompt}, {Role: "user", Content: candidate}}, nil, summaryOutput)
		if err != nil {
			return nil, err
		}
		if count > 0 && (candidateTokens > maxSummaryInputTokens || len(candidate) > llm.MaxProviderInputBytes/2) {
			break
		}
		if candidateTokens > summaryBudget {
			return nil, contextBudgetError(candidateTokens, summaryBudget, m.config.ContextTier)
		}
		if len(candidate) > llm.MaxProviderInputBytes/2 {
			return nil, fmt.Errorf("summary input exceeds the %d-byte safety limit", llm.MaxProviderInputBytes/2)
		}
		excerpt.WriteString(unit.String())
		count++
		if candidateTokens >= maxSummaryInputTokens || len(candidate) >= llm.MaxProviderInputBytes/2 {
			break
		}
	}
	if count == 0 {
		return nil, errors.New("no complete middle turn fits the summary input budget")
	}

	request := llm.ChatRequest{
		ContextWindowTokens: m.config.ContextWindowTokens,
		Model:               m.model,
		ReasoningEffort:     m.reasoningEffort,
		Messages:            []llm.Message{{Role: "system", Content: summaryPrompt}, {Role: "user", Content: excerpt.String()}},
		MaxTokens:           intPointer(summaryOutput),
		Temperature:         floatPointer(m.config.Temperature),
		TopP:                floatPointer(m.config.TopP),
	}
	message, usage, err := m.provider.StreamChat(ctx, request, nil)
	if usage != nil {
		m.usage.Add(*usage)
	}
	if err != nil {
		return nil, fmt.Errorf("context summary failed: %w", err)
	}
	if message == nil || strings.TrimSpace(message.Content) == "" {
		return nil, errors.New("context summary failed: model returned an empty summary")
	}
	if len(message.ToolCalls) != 0 {
		return nil, errors.New("context summary failed: expected text without tool calls")
	}
	if m.store != nil {
		for _, turn := range turns[:count] {
			original := append([]llm.Message(nil), turn...)
			for i := range original {
				if id, ok := m.elided[original[i].Content]; ok {
					content, err := m.store.load(id)
					if err != nil {
						return nil, err
					}
					original[i].Content = content
				}
			}
			encoded, err := json.Marshal(original)
			if err != nil {
				return nil, err
			}
			if _, err := m.archive(string(encoded)); err != nil {
				return nil, fmt.Errorf("archive summarized turn: %w", err)
			}
		}
	}
	m.summary = strings.TrimSpace(message.Content)
	m.changes += count
	remaining := append([][]llm.Message(nil), turns[count:]...)
	return joinHistory(preamble, remaining), nil
}

func (m *contextManager) archive(content string) (string, error) {
	m.nextID++
	id := fmt.Sprintf("event_%d", m.nextID)
	return id, m.store.save(id, content)
}

func intPointer(value int) *int           { return &value }
func floatPointer(value float64) *float64 { return &value }
