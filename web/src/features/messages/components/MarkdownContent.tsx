"use client"

import { Box } from "@chakra-ui/react"
import Markdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"
import { CodeBlock } from "./CodeBlock"

interface MarkdownContentProps {
  /** Raw markdown source (AI message content). */
  content: string
}

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
    // `pre` is unwrapped: `CodeBlock` (returned by the `code` renderer below)
    // already renders its own container, so keeping the default `pre`
    // wrapper here would double-wrap fenced blocks.
    pre: ({ children }) => <>{children}</>,
    code({ className, children }) {
      const match = /language-(\S+)/.exec(className ?? "")
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
