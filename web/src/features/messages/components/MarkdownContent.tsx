"use client"

import { isValidElement } from "react"
import { Box } from "@chakra-ui/react"
import Markdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"
import { CodeBlock } from "./CodeBlock"

interface MarkdownContentProps {
  /** Raw markdown source (AI message content). */
  content: string
}

/**
 * Matches the `language-<lang>` class react-markdown/remark-gfm puts on a
 * fenced code block's `code` element, capturing `<lang>`. Hoisted to module
 * scope (rather than constructed inside the `code` renderer on every
 * render/keystroke) since it holds no per-call state; `[\w-]+` (rather than
 * `\w+`) so hyphenated language tags (e.g. `language-objective-c`) capture in
 * full instead of stopping at the first hyphen.
 */
const CODE_LANGUAGE_CLASS_PATTERN = /language-([\w-]+)/

/**
 * Renders markdown for an AI message body: GFM (tables, strikethrough, task
 * lists) via `remark-gfm`, with fenced code blocks delegated to `CodeBlock`
 * for syntax highlighting and a copy button. Human messages do not go
 * through this component — they keep the plain `whiteSpace="pre-wrap"` text
 * rendering in `MessageBubble`.
 *
 * Styling is done with a Chakra `Box` `css` prop targeting the rendered tag
 * names rather than the Chakra `Prose` snippet, since that snippet assumes
 * Tailwind's typography plugin, which this project does not use.
 */
export function MarkdownContent({ content }: MarkdownContentProps) {
  const components: Components = {
    // react-markdown always wraps a fenced code block's `code` element in
    // `pre`, and calls this renderer with that `code` element *unrendered*
    // (its type/props only -- the `code` function below hasn't run yet), so
    // its `className` can be inspected here to tell fence kinds apart before
    // `code` itself decides how to render:
    //  - language-labeled (` ```js `): `code` will return `CodeBlock`, which
    //    already renders its own container, so unwrap here to avoid
    //    double-wrapping it in another `pre`.
    //  - unlabeled (bare ` ``` `): `code` falls through to its plain inline
    //    `<code>` styling below, which has no block semantics of its own, so
    //    render a real block-level `pre` container here to preserve
    //    whitespace/newlines instead of letting it collapse as inline text.
    //    The nested `<code>` is reset to inherit this container's
    //    background/font instead of re-applying its own, so the block isn't
    //    double-styled.
    pre: ({ children }) => {
      const isLanguageLabeled =
        isValidElement<{ className?: string }>(children) &&
        (children.props.className ?? "").includes("language-")

      if (isLanguageLabeled) {
        return <>{children}</>
      }

      return (
        <Box
          as="pre"
          whiteSpace="pre-wrap"
          overflowX="auto"
          rounded="sm"
          bg="bg.emphasized"
          fontSize="0.9em"
          fontFamily="mono"
          px="3"
          py="2"
          css={{
            "& code": {
              background: "none",
              padding: 0,
              borderRadius: 0,
              fontSize: "inherit",
              fontFamily: "inherit",
            },
          }}
        >
          {children}
        </Box>
      )
    },
    code({ className, children }) {
      const match = CODE_LANGUAGE_CLASS_PATTERN.exec(className ?? "")
      const text = String(children).replace(/\n$/, "")

      if (match) {
        return <CodeBlock language={match[1]}>{text}</CodeBlock>
      }

      return (
        <Box
          as="code"
          className={className}
          px="1"
          py="0.5"
          rounded="sm"
          bg="bg.emphasized"
          fontSize="0.9em"
          fontFamily="mono"
        >
          {children}
        </Box>
      )
    },
  }

  return (
    <Box
      fontSize="15px"
      lineHeight="relaxed"
      css={{
        "& > *:first-of-type": { marginTop: 0 },
        "& > *:last-of-type": { marginBottom: 0 },
        "& p": { marginBottom: "0.5em" },
        "& h1, & h2, & h3, & h4": {
          fontWeight: "semibold",
          marginTop: "0.75em",
          marginBottom: "0.4em",
          lineHeight: "1.3",
        },
        "& h1": { fontSize: "1.3em" },
        "& h2": { fontSize: "1.2em" },
        "& h3": { fontSize: "1.1em" },
        "& ul, & ol": { paddingLeft: "1.4em", marginBottom: "0.5em" },
        "& li": { marginBottom: "0.15em" },
        "& a": {
          color: "blue.500",
          textDecoration: "underline",
          textUnderlineOffset: "2px",
        },
        "& blockquote": {
          borderLeftWidth: "3px",
          borderColor: "border",
          paddingLeft: "0.8em",
          color: "fg.muted",
          marginY: "0.5em",
        },
        "& table": {
          borderCollapse: "collapse",
          width: "100%",
          marginY: "0.5em",
        },
        "& th, & td": {
          borderWidth: "1px",
          borderColor: "border",
          padding: "0.35em 0.6em",
        },
        "& hr": { borderColor: "border", marginY: "0.75em" },
      }}
    >
      <Markdown remarkPlugins={[remarkGfm]} components={components}>
        {content}
      </Markdown>
    </Box>
  )
}
