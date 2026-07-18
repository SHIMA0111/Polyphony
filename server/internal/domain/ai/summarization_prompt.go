package ai

import "strings"

// imageAttachmentPlaceholder replaces an image ContentPart in the
// transcript whenever the real image is not being passed through to the
// summarizer -- either because includeImages is false, or in the plain-text
// Content fallback that BuildSummarizationPrompt always populates (see its
// doc comment: Content must never itself carry raw image data, even when
// includeImages is true and the real image is passed through via Parts).
const imageAttachmentPlaceholder = "[image attachment]"

// summarizationSystemInstructionBase is the summarization instruction sent
// with every summarization request, regardless of includeImages.
const summarizationSystemInstructionBase = "Summarize the following conversation concisely, preserving key facts, decisions, and open questions."

// summarizationImageInstruction is appended to
// summarizationSystemInstructionBase only when includeImages is true: it
// asks the model to describe images so their content survives in the
// cached text summary after the images themselves age out of the live
// context window (see step50.md's Vision-interplay note).
const summarizationImageInstruction = " Describe the content of any images so the description can stand in for them."

// BuildSummarizationPrompt renders history (already filtered by
// ai.ContextBuilder.Build and enriched by the usecase's attachment-
// enrichment step) into the two-message request MessageUsecase sends to
// ai.LLMGateway.Complete to produce a cached ContextSummary.SummaryText: a
// system-role instruction message, followed by a single user-role message
// carrying the transcript.
//
// This is a pure, I/O-free helper: it never calls Complete itself (the
// usecase layer does that) and never persists anything -- it only decides
// what to *ask* the model.
//
// Rendering rules, applied per entry of history in order:
//   - An entry with no Parts (plain-Content) renders as one
//     "{role}: {content}" line.
//   - An entry with Parts (Step 39's multimodal shape) renders each part in
//     order: a text part's Text is carried through verbatim, except that the
//     entry's first text part is prefixed with "{role}: " -- mirroring the
//     "{role}: {content}" shape of the no-Parts case, so a reader (human or
//     model) sees the same "who said this" framing regardless of which
//     branch rendered a given message; an image part
//     (ContentPartTypeImageURL/ContentPartTypeImageBase64) renders
//     depending on includeImages -- see below.
//   - A "\n" separator part is inserted between consecutive entries (but
//     not before the first), again mirroring the "\n"-joined textLines
//     transcript, so the Parts transcript does not run every message's
//     content together with no boundary between them.
//
// The returned user message's Content field is always the full plain-text
// transcript (every image part rendered as the fixed
// "[image attachment]" placeholder, regardless of includeImages) -- Content
// never carries a URL or base64 blob. Its Parts field is populated
// (overriding Content on the wire per ChatMessage's own doc comment) only
// when includeImages is true and history contains at least one image part:
// in that case, each image part is preceded by a
// "{role} sent the following image:" attribution text part and then passed
// through unchanged, so a Vision-capable target model receives the actual
// image data through the same provider mapping any other Vision request
// uses and can describe it in its response. When includeImages is false (or
// history has no images at all), Parts is left nil and the plain-text
// Content transcript (with its placeholders) is all that is sent.
//
// Either way, only the model's resulting plain-text Content is ever cached
// as ContextSummary.SummaryText -- no URL or base64 image data from history
// is ever persisted, regardless of includeImages.
func BuildSummarizationPrompt(history []ChatMessage, includeImages bool) []ChatMessage {
	systemContent := summarizationSystemInstructionBase
	if includeImages {
		systemContent += summarizationImageInstruction
	}

	var textLines []string
	var parts []ContentPart
	hasImagePart := false

	for i, m := range history {
		if i > 0 {
			// Mirrors textLines' "\n"-joined transcript: without this, every
			// message's Parts would run together with no boundary between
			// them.
			parts = append(parts, ContentPart{Type: ContentPartTypeText, Text: "\n"})
		}

		if len(m.Parts) == 0 {
			textLines = append(textLines, m.Role+": "+m.Content)
			parts = append(parts, ContentPart{Type: ContentPartTypeText, Text: m.Role + ": " + m.Content})
			continue
		}

		var line strings.Builder
		line.WriteString(m.Role + ": ")
		firstTextPart := true
		for _, part := range m.Parts {
			if part.Type == ContentPartTypeText {
				line.WriteString(part.Text)
				text := part.Text
				if firstTextPart {
					// Mirrors the no-Parts branch's "{role}: {content}"
					// shape: only the entry's first text part carries the
					// role prefix, not every text part.
					text = m.Role + ": " + text
					firstTextPart = false
				}
				parts = append(parts, ContentPart{Type: ContentPartTypeText, Text: text})
				continue
			}

			// Image part (ContentPartTypeImageURL or ContentPartTypeImageBase64).
			hasImagePart = true
			line.WriteString(imageAttachmentPlaceholder)
			if includeImages {
				parts = append(parts,
					ContentPart{Type: ContentPartTypeText, Text: m.Role + " sent the following image:"},
					part,
				)
			} else {
				parts = append(parts, ContentPart{Type: ContentPartTypeText, Text: imageAttachmentPlaceholder})
			}
		}
		textLines = append(textLines, line.String())
	}

	userMsg := ChatMessage{Role: "user", Content: strings.Join(textLines, "\n")}
	if includeImages && hasImagePart {
		userMsg.Parts = parts
	}

	return []ChatMessage{
		{Role: "system", Content: systemContent},
		userMsg,
	}
}
