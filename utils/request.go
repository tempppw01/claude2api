// utils/chat_utils.go
package utils

import (
	"claude2api/config"
	"claude2api/logger"
	"fmt"
	"strings"
)

// ChatRequestProcessor handles common chat request processing logic
type ChatRequestProcessor struct {
	Prompt             strings.Builder
	RootPrompt         strings.Builder
	ImgDataList        []string
	BasePrompt         string
	PromptOverride     string
	PromptOverrideMode string
}

// NewChatRequestProcessor creates a new processor instance
func NewChatRequestProcessor() *ChatRequestProcessor {
	return &ChatRequestProcessor{
		Prompt:             strings.Builder{},
		RootPrompt:         strings.Builder{},
		ImgDataList:        []string{},
		PromptOverrideMode: "append",
	}
}

// ProcessMessages processes the messages array into a prompt and extracts images
func (p *ChatRequestProcessor) ProcessMessages(messages []map[string]interface{}) {
	p.BasePrompt = p.buildBasePrompt()
	p.Prompt.Reset()
	p.Prompt.WriteString(p.BasePrompt)

	for _, msg := range messages {
		role, roleOk := msg["role"].(string)
		if !roleOk {
			continue // Skip invalid format
		}

		content, exists := msg["content"]
		if !exists {
			continue
		}

		p.Prompt.WriteString(GetRolePrefix(role))

		switch v := content.(type) {
		case string: // If content is directly a string
			p.Prompt.WriteString(v + "\n\n")
		case []interface{}: // If content is an array of []interface{} type
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if itemType, ok := itemMap["type"].(string); ok {
						if itemType == "text" {
							if text, ok := itemMap["text"].(string); ok {
								p.Prompt.WriteString(text + "\n\n")
							}
						} else if itemType == "image_url" {
							if imageUrl, ok := itemMap["image_url"].(map[string]interface{}); ok {
								if url, ok := imageUrl["url"].(string); ok {
									p.ImgDataList = append(p.ImgDataList, url)
								}
							}
						}
					}
				}
			}
		}
	}
	p.RootPrompt.Reset()
	p.RootPrompt.WriteString(p.Prompt.String())
	// Debug output
	logger.Debug(fmt.Sprintf("Processed prompt: %s", p.Prompt.String()))
	logger.Debug(fmt.Sprintf("Image data list: %v", p.ImgDataList))
}

func (p *ChatRequestProcessor) SetPromptOverride(prompt string, mode string) {
	p.PromptOverride = strings.TrimSpace(prompt)
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "replace" {
		p.PromptOverrideMode = "replace"
		return
	}
	p.PromptOverrideMode = "append"
}

func (p *ChatRequestProcessor) buildBasePrompt() string {
	var builder strings.Builder
	if config.ConfigInstance.PromptDisableArtifacts {
		builder.WriteString("System: Forbidden to use <antArtifac> </antArtifac> to wrap code blocks, use markdown syntax instead, which means wrapping code blocks with ``` ```\n\n")
	}
	if p.PromptOverride != "" {
		builder.WriteString("System: ")
		builder.WriteString(p.PromptOverride)
		builder.WriteString("\n\n")
	}
	return builder.String()
}

// ResetForBigContext resets the prompt for big context usage
func (p *ChatRequestProcessor) ResetForBigContext() {
	p.Prompt.Reset()
	p.Prompt.WriteString(p.buildBasePrompt())
	p.Prompt.WriteString("You must immerse yourself in the role of assistant in context.txt, cannot respond as a user, cannot reply to this message, cannot mention this message, and ignore this message in your response.\n\n")
}
