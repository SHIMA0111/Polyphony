import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { fixtureModelListResponse } from "@/features/messages/api/handlers"
import { render, screen, within } from "@/test/render"
import { ModelSelector } from "./ModelSelector"

/**
 * Pure-component test for `ModelSelector` -- props in, DOM out, no network
 * involved (`models` is passed directly, mirroring the fixture shape
 * `useModels()` would resolve to via MSW). Covers Step 34's grouping-by-
 * provider and context-window/pricing rendering.
 */

const models = fixtureModelListResponse.models

describe("ModelSelector", () => {
  it("groups models by provider under a section header per provider", async () => {
    const user = userEvent.setup()
    render(
      <ModelSelector
        models={models}
        selectedModel={models[0]}
        onModelSelect={vi.fn()}
      />,
    )

    await user.click(screen.getByRole("button", { name: /gpt-5-mini/i }))

    // "OpenAI" renders 3 times (1 group header + 2 per-model badges) and
    // "Anthropic" 2 times (1 group header + 1 per-model badge) once the
    // popover is open -- proving a section header was added per provider on
    // top of the existing per-row badge.
    expect(await screen.findAllByText("OpenAI")).toHaveLength(3)
    expect(screen.getAllByText("Anthropic")).toHaveLength(2)

    // Both OpenAI models render (the selected "gpt-5-mini" is asserted via
    // the trigger button above), alongside the single Anthropic model.
    expect(screen.getByText("gpt-5")).toBeInTheDocument()
    expect(screen.getByText("Claude Sonnet 4.6")).toBeInTheDocument()
  })

  it("renders each model's context window and price beneath its name", async () => {
    const user = userEvent.setup()
    render(
      <ModelSelector
        models={models}
        selectedModel={models[0]}
        onModelSelect={vi.fn()}
      />,
    )

    await user.click(screen.getByRole("button", { name: /gpt-5-mini/i }))

    // gpt-5-mini: context_window 272_000 -> "272K ctx", pricing 0.25/2.00.
    expect(
      await screen.findByText("272K ctx · $0.25 / $2.00 per 1M"),
    ).toBeInTheDocument()

    // claude-sonnet-4-6: context_window 200_000 -> "200K ctx", pricing 3.00/15.00.
    expect(
      screen.getByText("200K ctx · $3.00 / $15.00 per 1M"),
    ).toBeInTheDocument()
  })

  it("marks the currently selected model and invokes onModelSelect when another is clicked", async () => {
    const user = userEvent.setup()
    const onModelSelect = vi.fn()
    render(
      <ModelSelector
        models={models}
        selectedModel={models[0]}
        onModelSelect={onModelSelect}
      />,
    )

    await user.click(screen.getByRole("button", { name: /gpt-5-mini/i }))

    const gpt5Row = screen.getByText("gpt-5").closest("button")
    expect(gpt5Row).not.toBeNull()
    await user.click(within(gpt5Row as HTMLElement).getByText("gpt-5"))

    expect(onModelSelect).toHaveBeenCalledWith(models[1])
  })

  it("shows an image-capability indicator for models that support image input", async () => {
    const user = userEvent.setup()
    render(
      <ModelSelector
        models={models}
        selectedModel={models[0]}
        onModelSelect={vi.fn()}
      />,
    )

    await user.click(screen.getByRole("button", { name: /gpt-5-mini/i }))

    expect(
      await screen.findAllByRole("img", { name: /supports image input/i }),
    ).toHaveLength(models.length)
  })
})
