"use client"

import { useState } from "react"
import Link from "next/link"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { useQuery } from "@tanstack/react-query"
import { Button, Card, Field, Flex, Heading, Input, Text } from "@chakra-ui/react"
import { Pen } from "lucide-react"
import { PasswordInput } from "@/components/ui/password-input"
import { toaster } from "@/components/ui/toaster"
import { useLogin } from "@/features/auth/hooks/use-login"
import { getLoginFlow } from "@/features/auth/api/login-flow"
import { SocialLoginButtons } from "@/features/auth/components/SocialLoginButtons"
import {
  getFormMessages,
  getNodeMessages,
  getNodeValue,
  type UiContainer,
} from "@/features/auth/utils/kratos-flow"
import { loginSchema, type LoginFormValues } from "@/features/auth/utils/schemas"

export function LoginForm() {
  const loginMutation = useLogin()

  // `staleTime: 0` + `gcTime: 0` guarantee a fresh Kratos flow (and its
  // one-time-use CSRF token) is fetched every time this component mounts,
  // rather than reusing a flow left over from a previous mount that may
  // already have been submitted or expired.
  const flowQuery = useQuery({
    queryKey: ["auth", "login-flow"],
    queryFn: getLoginFlow,
    staleTime: 0,
    gcTime: 0,
  })

  // Copied into local state (rather than read directly from the query) so
  // a failed submission can swap in the flow Kratos re-renders with
  // validation/credential errors without that being treated as new query
  // data (which `staleTime: 0` would otherwise immediately refetch away).
  const [flow, setFlow] = useState<UiContainer | null>(null)
  // Tracks the last query result this component has adopted, so freshly
  // fetched flow data is picked up during render (React's "adjust state
  // while rendering" pattern, same as MessageInput's aiError handling)
  // rather than in a `useEffect`-that-calls-`setState` -- exactly once per
  // actual data change, without clobbering an error flow a failed
  // submission swapped in via `setFlow(result.flow)`.
  const [lastQueryFlow, setLastQueryFlow] = useState<UiContainer | null>(null)
  if (flowQuery.data && flowQuery.data !== lastQueryFlow) {
    setLastQueryFlow(flowQuery.data)
    setFlow(flowQuery.data)
  }

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    mode: "onBlur",
  })

  const onSubmit = handleSubmit(async (values) => {
    if (!flow) return

    try {
      const result = await loginMutation.mutateAsync({
        flow,
        values: {
          identifier: values.email,
          password: values.password,
          csrf_token: getNodeValue(flow.nodes, "csrf_token"),
        },
      })

      if (!result.ok) {
        setFlow(result.flow)
        const messages = getFormMessages(result.flow)
        toaster.create({
          type: "error",
          title: "Sign in failed",
          description: messages[0] ?? "Please check your credentials and try again.",
        })
      }
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Sign in failed",
        description: err instanceof Error ? err.message : "Please try again.",
      })
    }
  })

  const identifierMessages = flow ? getNodeMessages(flow.nodes, "identifier") : []
  const passwordMessages = flow ? getNodeMessages(flow.nodes, "password") : []

  return (
    <Flex minH="100vh" align="center" justify="center" bg="bg.subtle" p={4}>
      <Card.Root w="full" maxW="420px" shadow="lg">
        <Card.Header spaceY={3} textAlign="center">
          <Flex align="center" justify="center" gap={2} mb={2}>
            <Flex
              h={10}
              w={10}
              rounded="lg"
              bg="colorPalette.solid"
              align="center"
              justify="center"
              colorPalette="blue"
            >
              <Pen size={24} color="white" />
            </Flex>
            <Heading size="3xl" fontWeight="bold" letterSpacing="tight">
              Polyphony
            </Heading>
          </Flex>
          <Card.Title fontSize="2xl">Welcome back</Card.Title>
          <Card.Description>
            Sign in to your account to continue
          </Card.Description>
        </Card.Header>
        <Card.Body>
          <form onSubmit={onSubmit} noValidate>
            <Flex direction="column" gap={4}>
              <Field.Root invalid={!!errors.email || identifierMessages.length > 0}>
                <Field.Label>Email</Field.Label>
                <Input
                  type="email"
                  placeholder="you@example.com"
                  {...register("email")}
                />
                {errors.email && (
                  <Field.ErrorText role="alert">
                    {errors.email.message}
                  </Field.ErrorText>
                )}
                {!errors.email &&
                  identifierMessages.map((message) => (
                    <Field.ErrorText role="alert" key={message}>
                      {message}
                    </Field.ErrorText>
                  ))}
              </Field.Root>
              <Field.Root invalid={!!errors.password || passwordMessages.length > 0}>
                <Field.Label>Password</Field.Label>
                <PasswordInput
                  placeholder="Enter your password"
                  {...register("password")}
                />
                {errors.password && (
                  <Field.ErrorText role="alert">
                    {errors.password.message}
                  </Field.ErrorText>
                )}
                {!errors.password &&
                  passwordMessages.map((message) => (
                    <Field.ErrorText role="alert" key={message}>
                      {message}
                    </Field.ErrorText>
                  ))}
              </Field.Root>
              <Button
                type="submit"
                colorPalette="blue"
                size="lg"
                w="full"
                // Disabled until the Kratos login flow has loaded: onSubmit
                // silently no-ops while `flow` is null, so a click in that
                // window would otherwise be dropped without any feedback
                // (caught live by the wave-7 integration run as a stuck
                // login under load).
                disabled={!flow}
                loading={loginMutation.isPending}
                loadingText="Signing in..."
              >
                Sign in
              </Button>
            </Flex>
          </form>
          {flow && <SocialLoginButtons flow={flow} />}
          <Text mt={6} textAlign="center" fontSize="sm" color="fg.muted">
            Don&apos;t have an account?{" "}
            <Link href="/register">
              <Text
                as="span"
                color="blue.500"
                fontWeight="medium"
                _hover={{ textDecoration: "underline" }}
              >
                Register
              </Text>
            </Link>
          </Text>
        </Card.Body>
      </Card.Root>
    </Flex>
  )
}
