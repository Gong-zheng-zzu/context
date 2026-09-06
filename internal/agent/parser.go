package agent

import (
	"regexp"
	"strings"
)

var (
	thoughtRe  = regexp.MustCompile(`(?i)Thought:\s*(.+?)(?:\nAction:|\nFinalAnswer:|\z)`)
	actionRe   = regexp.MustCompile(`(?i)Action:\s*(.+?)\s*\n`)
	inputRe    = regexp.MustCompile(`(?i)ActionInput:\s*(.+?)(?:\nAction:|\nFinalAnswer:|\z)`)
	answerRe   = regexp.MustCompile(`(?i)FinalAnswer:\s*(.+)`)
	multiSpace = regexp.MustCompile(`\n{3,}`)
)

// ParseOutput 解析LLM的ReAct格式输出
func ParseOutput(text string) ParsedLLMOutput {
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedLLMOutput{}
	}

	result := ParsedLLMOutput{}

	if m := answerRe.FindStringSubmatch(text); len(m) > 1 {
		result.FinalAnswer = strings.TrimSpace(m[1])
		result.FinalAnswer = multiSpace.ReplaceAllString(result.FinalAnswer, "\n")
		return result
	}

	if m := thoughtRe.FindStringSubmatch(text); len(m) > 1 {
		result.Thought = strings.TrimSpace(m[1])
	}

	if m := actionRe.FindStringSubmatch(text); len(m) > 1 {
		result.Action = strings.TrimSpace(m[1])
		result.HasAction = true
	}

	if m := inputRe.FindStringSubmatch(text); len(m) > 1 {
		result.ActionInput = strings.TrimSpace(m[1])
		result.ActionInput = multiSpace.ReplaceAllString(result.ActionInput, "\n")
	}

	return result
}
