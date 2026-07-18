import { describe, expect, it } from "vitest"
import { render, screen } from "@/test/render"
import type { UiContainer } from "@/features/auth/utils/kratos-flow"
import { SocialLoginButtons } from "./SocialLoginButtons"

/** A minimal registration-flow-shaped `UiContainer` with `oidc` nodes for dex/google/github plus the usual `default`-group csrf node. */
function makeFlowWithOidcNodes(): UiContainer {
  return {
    action: "http://localhost:4433/self-service/registration?flow=flow-id",
    method: "POST",
    nodes: [
      {
        type: "input",
        group: "default",
        attributes: { name: "csrf_token", type: "hidden", value: "test-csrf-token", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "oidc",
        attributes: { name: "provider", type: "submit", value: "dex" },
        messages: [],
      },
      {
        type: "input",
        group: "oidc",
        attributes: { name: "provider", type: "submit", value: "google" },
        messages: [],
      },
      {
        type: "input",
        group: "oidc",
        attributes: { name: "provider", type: "submit", value: "github" },
        messages: [],
      },
    ],
  }
}

/** The same shape, but with no `oidc`-group nodes at all (OIDC disabled server-side). */
function makeFlowWithoutOidcNodes(): UiContainer {
  const flow = makeFlowWithOidcNodes()
  return { ...flow, nodes: flow.nodes.filter((node) => node.group !== "oidc") }
}

describe("SocialLoginButtons", () => {
  it("renders one button per oidc-group node, labeled by provider", () => {
    render(<SocialLoginButtons flow={makeFlowWithOidcNodes()} />)

    expect(screen.getByRole("button", { name: "Continue with Dex" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Continue with Google" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Continue with GitHub" })).toBeInTheDocument()
  })

  it("posts the form to the same-origin /api/kratos proxy path, not Kratos's own absolute URL", () => {
    render(<SocialLoginButtons flow={makeFlowWithOidcNodes()} />)

    const form = screen.getByRole("button", { name: "Continue with Dex" }).closest("form")
    expect(form).toHaveAttribute("action", "/api/kratos/self-service/registration?flow=flow-id")
    expect(form).toHaveAttribute("method", "POST")
  })

  it("submits each button with name=provider and the node's own value", () => {
    render(<SocialLoginButtons flow={makeFlowWithOidcNodes()} />)

    const googleButton = screen.getByRole("button", { name: "Continue with Google" })
    expect(googleButton).toHaveAttribute("name", "provider")
    expect(googleButton).toHaveAttribute("value", "google")
    expect(googleButton).toHaveAttribute("type", "submit")
  })

  it("echoes the flow's csrf_token as a hidden field", () => {
    render(<SocialLoginButtons flow={makeFlowWithOidcNodes()} />)

    const form = screen.getByRole("button", { name: "Continue with Dex" }).closest("form")
    const hiddenInput = form?.querySelector('input[name="csrf_token"]')
    expect(hiddenInput).toHaveAttribute("value", "test-csrf-token")
  })

  it("renders nothing when the flow has no oidc nodes", () => {
    render(<SocialLoginButtons flow={makeFlowWithoutOidcNodes()} />)

    expect(screen.queryByRole("button")).not.toBeInTheDocument()
    expect(screen.queryByRole("form")).not.toBeInTheDocument()
  })
})
