package ai

import (
	"strings"
	"testing"
)

// TestBuildSummarizationPrompt_PlainTextTranscript verifies that a
// plain-text-only history is rendered as a two-message prompt (a
// system-role instruction followed by a single user-role transcript
// message), the transcript contains every source message's "role: content"
// line, and the user message carries no Parts.
func TestBuildSummarizationPrompt_PlainTextTranscript(t *testing.T) {
	history := []ChatMessage{
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "A programming language."},
	}

	prompt := BuildSummarizationPrompt(history, true)

	if len(prompt) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(prompt))
	}
	if prompt[0].Role != "system" {
		t.Fatalf("expected first message to be system-role, got %q", prompt[0].Role)
	}
	if prompt[1].Role != "user" {
		t.Fatalf("expected second message to be user-role, got %q", prompt[1].Role)
	}
	for _, m := range history {
		want := m.Role + ": " + m.Content
		if !contains(prompt[1].Content, want) {
			t.Errorf("expected transcript to contain %q, got %q", want, prompt[1].Content)
		}
	}
	if len(prompt[1].Parts) != 0 {
		t.Errorf("expected no Parts for a plain-text-only transcript, got %+v", prompt[1].Parts)
	}
}

// TestBuildSummarizationPrompt_IncludeImagesTrue verifies that with
// includeImages=true, a message with image Parts is rendered as
// role-prefixed text plus an attribution line plus the passed-through
// image part, the plain-text Content fallback never leaks the raw image
// URL (using imageAttachmentPlaceholder instead), and the system
// instruction mentions describing images.
func TestBuildSummarizationPrompt_IncludeImagesTrue(t *testing.T) {
	history := []ChatMessage{
		{
			Role: "user",
			Parts: []ContentPart{
				{Type: ContentPartTypeText, Text: "check this out"},
				{Type: ContentPartTypeImageURL, ImageURL: "https://example.com/image.png"},
			},
		},
	}

	prompt := BuildSummarizationPrompt(history, true)
	userMsg := prompt[1]

	if len(userMsg.Parts) != 3 {
		t.Fatalf("expected 3 parts (text + attribution + image), got %d: %+v", len(userMsg.Parts), userMsg.Parts)
	}
	if userMsg.Parts[0].Type != ContentPartTypeText || userMsg.Parts[0].Text != "user: check this out" {
		t.Errorf("expected first part to be the role-prefixed text (mirroring the no-Parts \"role: content\" shape), got %+v", userMsg.Parts[0])
	}
	if userMsg.Parts[1].Type != ContentPartTypeText || userMsg.Parts[1].Text != "user sent the following image:" {
		t.Errorf("expected second part to be the image attribution text, got %+v", userMsg.Parts[1])
	}
	if userMsg.Parts[2].Type != ContentPartTypeImageURL || userMsg.Parts[2].ImageURL != "https://example.com/image.png" {
		t.Errorf("expected third part to be the passed-through image, got %+v", userMsg.Parts[2])
	}

	// The plain-text Content fallback must never carry the raw URL.
	if contains(userMsg.Content, "https://example.com/image.png") {
		t.Errorf("expected Content fallback to never contain the raw image URL, got %q", userMsg.Content)
	}
	if !contains(userMsg.Content, imageAttachmentPlaceholder) {
		t.Errorf("expected Content fallback to contain the image placeholder, got %q", userMsg.Content)
	}

	if !contains(prompt[0].Content, "Describe the content of any images") {
		t.Errorf("expected system instruction to mention describing images when includeImages=true, got %q", prompt[0].Content)
	}
}

// TestBuildSummarizationPrompt_IncludeImagesFalse verifies that with
// includeImages=false, a message with image Parts is flattened to
// Parts-less plain text containing imageAttachmentPlaceholder (never the
// raw image URL), and the system instruction omits the image-description
// sentence.
func TestBuildSummarizationPrompt_IncludeImagesFalse(t *testing.T) {
	history := []ChatMessage{
		{
			Role: "user",
			Parts: []ContentPart{
				{Type: ContentPartTypeText, Text: "check this out"},
				{Type: ContentPartTypeImageURL, ImageURL: "https://example.com/image.png"},
			},
		},
	}

	prompt := BuildSummarizationPrompt(history, false)
	userMsg := prompt[1]

	if len(userMsg.Parts) != 0 {
		t.Fatalf("expected no Parts when includeImages=false, got %+v", userMsg.Parts)
	}
	if contains(userMsg.Content, "https://example.com/image.png") {
		t.Errorf("expected no URL anywhere in the transcript when includeImages=false, got %q", userMsg.Content)
	}
	if !contains(userMsg.Content, imageAttachmentPlaceholder) {
		t.Errorf("expected the image placeholder in the transcript, got %q", userMsg.Content)
	}
	if contains(prompt[0].Content, "Describe the content of any images") {
		t.Errorf("expected system instruction to omit the image-description sentence when includeImages=false, got %q", prompt[0].Content)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
