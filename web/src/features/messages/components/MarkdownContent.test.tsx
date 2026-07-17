import { describe, expect, it } from "vitest"
import { render, screen } from "@/test/render"
import { MarkdownContent } from "./MarkdownContent"

/**
 * Component coverage for the wave-3 review follow-up to `MarkdownContent`'s
 * `pre`/`code` renderers: a bare (unlabeled) fenced code block used to fall
 * into the same branch as *true* inline code (`pre` unconditionally
 * unwrapped to a `Fragment`, `code` only routing to `CodeBlock` when
 * `className` carried a `language-` tag), losing block semantics and
 * collapsing whitespace. These tests pin the three cases `pre` now
 * distinguishes between.
 */
describe("MarkdownContent code rendering", () => {
  it("renders true inline code (single backtick) inline, with no `pre` ancestor", () => {
    render(<MarkdownContent content="Call `foo()` to start." />)

    const code = screen.getByText("foo()")
    expect(code.tagName).toBe("CODE")
    expect(code.closest("pre")).toBeNull()
  })

  it("renders a language-labeled fenced block via CodeBlock without double-wrapping in an extra `pre`", () => {
    const { container } = render(
      <MarkdownContent content={"```js\nconst x = 1;\n```"} />,
    )

    // CodeBlock's header renders the resolved language as a label.
    expect(screen.getByText("js")).toBeInTheDocument()
    // CodeBlock's own copy-button wrapper (`role="group"`) is present...
    expect(screen.getByRole("group")).toBeInTheDocument()
    // ...and react-syntax-highlighter renders exactly one `<pre>` itself --
    // if `MarkdownContent`'s `pre` renderer failed to unwrap, there would be
    // two nested `<pre>` elements here instead of one.
    expect(container.querySelectorAll("pre")).toHaveLength(1)
    expect(container.textContent).toContain("const x = 1;")
  })

  it("renders a bare/unlabeled fenced block as a block container, preserving newlines instead of collapsing as inline text", () => {
    const { container } = render(
      <MarkdownContent content={"```\nline one\nline two\n```"} />,
    )

    // Must not be routed through CodeBlock (no language tag to show or
    // group wrapper to render).
    expect(screen.queryByRole("group")).not.toBeInTheDocument()

    const pre = container.querySelector("pre")
    expect(pre).not.toBeNull()
    expect(container.querySelectorAll("pre")).toHaveLength(1)
    expect(pre).toHaveStyle({ whiteSpace: "pre-wrap" })
    expect(pre?.textContent).toContain("line one")
    expect(pre?.textContent).toContain("line two")
  })
})
