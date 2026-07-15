"use client"

import { useState } from "react"
import { Box, Flex, IconButton, Text } from "@chakra-ui/react"
import { Check, Copy } from "lucide-react"
import SyntaxHighlighter from "react-syntax-highlighter/dist/esm/prism"
import oneDark from "react-syntax-highlighter/dist/esm/styles/prism/one-dark"
import oneLight from "react-syntax-highlighter/dist/esm/styles/prism/one-light"
import { useColorModeValue } from "@/components/ui/color-mode"
import { Tooltip } from "@/components/ui/tooltip"

interface CodeBlockProps {
  /** Fenced-code-block language, taken from the markdown info string (e.g. `js` in ` ```js `). */
  language?: string
  /** The code content, without the surrounding fence. */
  children: string
}

/** How long the copy button shows its "copied" state before reverting. */
const COPY_FEEDBACK_MS = 2000

/**
 * Renders a fenced markdown code block with Prism syntax highlighting (theme
 * follows the app's color mode) and a copy-to-clipboard button.
 */
export function CodeBlock({ language, children }: CodeBlockProps) {
  const [copied, setCopied] = useState(false)
  const style = useColorModeValue(oneLight, oneDark)
  const resolvedLanguage = language || "text"

  const handleCopy = async () => {
    await navigator.clipboard.writeText(children)
    setCopied(true)
    setTimeout(() => setCopied(false), COPY_FEEDBACK_MS)
  }

  return (
    <Box
      position="relative"
      rounded="lg"
      overflow="hidden"
      my={2}
      borderWidth="1px"
      borderColor="border"
      role="group"
    >
      <Flex
        align="center"
        justify="space-between"
        px={3}
        py={1}
        bg="bg.subtle"
        borderBottomWidth="1px"
        borderColor="border"
      >
        <Text fontSize="xs" color="fg.muted" fontFamily="mono">
          {resolvedLanguage}
        </Text>
        <Tooltip content={copied ? "Copied!" : "Copy code"}>
          <IconButton
            aria-label="Copy code"
            size="xs"
            variant="ghost"
            onClick={handleCopy}
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
          </IconButton>
        </Tooltip>
      </Flex>
      <Box fontSize="13px" css={{ "& pre": { margin: 0 } }}>
        <SyntaxHighlighter
          language={resolvedLanguage}
          style={style}
          customStyle={{ margin: 0, padding: "12px 16px" }}
        >
          {children}
        </SyntaxHighlighter>
      </Box>
    </Box>
  )
}
