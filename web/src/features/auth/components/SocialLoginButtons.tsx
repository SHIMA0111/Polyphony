"use client"

import { Button, Flex, Separator, Text } from "@chakra-ui/react"
import { Chrome, Github, Lock, type LucideIcon } from "lucide-react"
import { getNodeValue, toRelativeKratosAction, type UiContainer } from "@/features/auth/utils/kratos-flow"

interface SocialLoginButtonsProps {
  /** The currently-rendered Kratos login or registration flow (see `LoginForm`/`RegisterForm`). */
  flow: UiContainer
}

/** Icon shown per OIDC provider id; unrecognized/mock providers (e.g. `dex`) fall back to a generic lock icon. */
const PROVIDER_ICONS: Record<string, LucideIcon> = {
  google: Chrome,
  github: Github,
}

/** Human-readable label for a provider id Kratos doesn't itself title-case. */
function labelFor(provider: string): string {
  switch (provider) {
    case "github":
      return "GitHub"
    case "google":
      return "Google"
    default:
      return provider.length > 0 ? provider[0].toUpperCase() + provider.slice(1) : provider
  }
}

/**
 * Renders one "Continue with {Provider}" button per `oidc`-group node found
 * in a Kratos self-service login/registration flow (Step 44), below a
 * "or continue with" divider. Renders nothing (`null`) when the flow has no
 * `oidc` nodes at all, so the UI degrades gracefully if OIDC is ever
 * disabled server-side — callers don't need to check this themselves.
 *
 * Submission is a real, full-page HTML `<form>` POST — deliberately not a
 * `fetch`/XHR call, unlike this app's password-method submissions — because
 * Kratos's OIDC method responds to this exact submission with a genuine
 * HTTP redirect out to the provider (dex/Google/GitHub) and eventually back;
 * a `fetch`-based submission would only ever observe that redirect chain's
 * *final* resolved response in JS, never get the actual browser to
 * navigate through it. `flow.action` is passed through
 * {@link toRelativeKratosAction} exactly like every other flow submission
 * in this app, so the POST stays same-origin through `/api/kratos/*` (see
 * that route's docstring for how it preserves this redirect untouched).
 *
 * @param flow - The currently-rendered Kratos login or registration flow.
 * @returns The provider buttons, or `null` if the flow has no `oidc` nodes.
 */
export function SocialLoginButtons({ flow }: SocialLoginButtonsProps) {
  const oidcNodes = flow.nodes.filter((node) => node.group === "oidc")

  if (oidcNodes.length === 0) {
    return null
  }

  const csrfToken = getNodeValue(flow.nodes, "csrf_token")
  const action = toRelativeKratosAction(flow.action)

  return (
    <Flex direction="column" gap={4} mt={6}>
      <Flex align="center" gap={3}>
        <Separator flex="1" />
        <Text fontSize="sm" color="fg.muted" whiteSpace="nowrap">
          or continue with
        </Text>
        <Separator flex="1" />
      </Flex>
      <form action={action} method={flow.method}>
        <input type="hidden" name="csrf_token" value={csrfToken} readOnly />
        <Flex direction="column" gap={3}>
          {oidcNodes.map((node) => {
            const provider = String(node.attributes.value ?? "")
            const Icon = PROVIDER_ICONS[provider] ?? Lock
            return (
              <Button key={provider} type="submit" name="provider" value={provider} variant="outline" w="full">
                <Icon size={18} />
                {`Continue with ${labelFor(provider)}`}
              </Button>
            )
          })}
        </Flex>
      </form>
    </Flex>
  )
}
