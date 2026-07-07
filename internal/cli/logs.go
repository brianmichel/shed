package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/brianmichel/shed/internal/model"
	"github.com/fatih/color"
)

// logRenderer turns a job's agent.* event stream (relayed by internal/agent
// from pi's RPC protocol) into a readable transcript: streamed assistant
// text, tool calls, and their results. With raw set, it instead prints the
// agent process's untouched JSONL stdout, for debugging the driver itself.
type logRenderer struct {
	raw          bool
	thinkingOpen bool
}

func newLogRenderer(raw bool) *logRenderer {
	return &logRenderer{raw: raw}
}

func (r *logRenderer) handle(ev model.Event) {
	if r.raw {
		if ev.Type == "command.stdout" || ev.Type == "command.stderr" {
			if chunk, ok := ev.Data["chunk"].(string); ok {
				fmt.Print(chunk)
			}
		}
		return
	}
	if !strings.HasPrefix(ev.Type, "agent.") {
		return
	}
	switch ev.Type {
	case "agent.message_update":
		r.renderMessageUpdate(ev.Data)
	case "agent.tool_execution_start":
		r.closeThinking()
		r.renderToolStart(ev.Data)
	case "agent.tool_execution_end":
		r.renderToolEnd(ev.Data)
	case "agent.stderr":
		if text, ok := ev.Data["text"].(string); ok {
			fmt.Print(color.HiBlackString(text))
		}
	case "agent.agent_end":
		r.closeThinking()
	}
}

func (r *logRenderer) renderMessageUpdate(data map[string]any) {
	amEvent := nestedMap(data, "assistantMessageEvent")
	if amEvent == nil {
		return
	}
	switch str(amEvent, "type") {
	case "text_delta":
		r.closeThinking()
		fmt.Print(str(amEvent, "delta"))
	case "text_end":
		fmt.Println()
	case "thinking_delta":
		if !r.thinkingOpen {
			fmt.Print(color.HiBlackString("\n[thinking] "))
			r.thinkingOpen = true
		}
		fmt.Print(color.HiBlackString(str(amEvent, "delta")))
	case "thinking_end":
		r.closeThinking()
	}
}

func (r *logRenderer) closeThinking() {
	if r.thinkingOpen {
		fmt.Println()
		r.thinkingOpen = false
	}
}

func (r *logRenderer) renderToolStart(data map[string]any) {
	toolName := str(data, "toolName")
	args := nestedMap(data, "args")
	fmt.Println()
	fmt.Println(color.CyanString(summarizeToolCall(toolName, args)))
}

func (r *logRenderer) renderToolEnd(data map[string]any) {
	result := nestedMap(data, "result")
	isError, _ := data["isError"].(bool)
	text := resultText(result)
	if text == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if isError {
			fmt.Println(color.RedString("  " + line))
		} else {
			fmt.Println("  " + line)
		}
	}
}

func summarizeToolCall(toolName string, args map[string]any) string {
	if toolName == "bash" {
		if cmd, ok := args["command"].(string); ok {
			return "$ " + cmd
		}
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, args[k]))
	}
	return fmt.Sprintf("-> %s(%s)", toolName, strings.Join(parts, ", "))
}

func resultText(result map[string]any) string {
	content, _ := result["content"].([]any)
	var sb strings.Builder
	for _, c := range content {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := cm["text"].(string); t != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(t)
		}
	}
	return sb.String()
}

func str(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func nestedMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}
