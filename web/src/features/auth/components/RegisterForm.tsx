"use client"

import { useState } from "react"
import Link from "next/link"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { useQuery } from "@tanstack/react-query"
import { Button, Card, Field, Flex, Heading, Input, Text } from "@chakra-ui/react"
import { Pen } from "lucide-react"
import { PasswordInput, PasswordStrengthMeter } from "@/components/ui/password-input"
import { toaster } from "@/components/ui/toaster"
import { getErrorMessage } from "@/lib/get-error-message"
import { useRegister } from "@/features/auth/hooks/use-register"
import { getRegistrationFlow } from "@/features/auth/api/registration-flow"
import { SocialLoginButtons } from "@/features/auth/components/SocialLoginButtons"
import { getPasswordStrength } from "@/features/auth/utils/password-strength"
import {
  getFormMessages,
  getNodeMessages,
  getNodeValue,
  type UiContainer,
} from "@/features/auth/utils/kratos-flow"
import { registerSchema, type RegisterFormValues } from "@/features/auth/utils/schemas"

export function RegisterForm() {
  const registerMutation = useRegister()

  // `staleTime: 0` + `gcTime: 0` guarantee a fresh Kratos flow (and its
  // one-time-use CSRF token) is fetched every time this component mounts,
  // rather than reusing a flow left over from a previous mount that may
  // already have been submitted or expired.
  const flowQuery = useQuery({
    queryKey: ["auth", "registration-flow"],
    queryFn: getRegistrationFlow,
    staleTime: 0,
    gcTime: 0,
  })

  // Copied into local state (rather than read directly from the query) so
  // a failed submission can swap in the flow Kratos re-renders with
  // validation errors (e.g. "email already in use") without that being
  // treated as new query data (which `staleTime: 0` would otherwise
  // immediately refetch away).
  const [flow, setFlow] = useState<UiContainer | null>(null)
  // Adopt freshly fetched flow data during render (React's "adjust state
  // while rendering" pattern, same as LoginForm/MessageInput) rather than
  // in a `useEffect`-that-calls-`setState` -- exactly once per actual data
  // change, without clobbering an error flow a failed submission swapped in
  // via `setFlow(result.flow)`.
  const [lastQueryFlow, setLastQueryFlow] = useState<UiContainer | null>(null)
  if (flowQuery.data && flowQuery.data !== lastQueryFlow) {
    setLastQueryFlow(flowQuery.data)
    setFlow(flowQuery.data)
  }

  const {
    register,
    handleSubmit,
    control,
    formState: { errors },
  } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    mode: "onChange",
  })

  // `useWatch` (rather than the `watch()` function returned by `useForm`)
  // subscribes only this component to `password` changes and is
  // React-Compiler-memoizable, avoiding a `react-hooks/incompatible-library`
  // lint warning.
  const password = useWatch({ control, name: "password" }) ?? ""

  const onSubmit = handleSubmit(async (values) => {
    if (!flow) return

    try {
      const result = await registerMutation.mutateAsync({
        flow,
        values: {
          email: values.email,
          username: values.username,
          password: values.password,
          csrf_token: getNodeValue(flow.nodes, "csrf_token"),
        },
      })

      if (!result.ok) {
        setFlow(result.flow)
        const messages = getFormMessages(result.flow)
        toaster.create({
          type: "error",
          title: "Registration failed",
          description: messages[0] ?? "Please try again.",
        })
      }
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Registration failed",
        description: getErrorMessage(err, "Please try again."),
      })
    }
  })

  const emailMessages = flow ? getNodeMessages(flow.nodes, "traits.email") : []
  const usernameMessages = flow ? getNodeMessages(flow.nodes, "traits.username") : []
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
          <Card.Title fontSize="2xl">Create an account</Card.Title>
          <Card.Description>
            Get started with team AI collaboration
          </Card.Description>
        </Card.Header>
        <Card.Body>
          <form onSubmit={onSubmit} noValidate>
            <Flex direction="column" gap={4}>
              <Field.Root invalid={!!errors.email || emailMessages.length > 0}>
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
                  emailMessages.map((message) => (
                    <Field.ErrorText role="alert" key={message}>
                      {message}
                    </Field.ErrorText>
                  ))}
              </Field.Root>
              <Field.Root invalid={!!errors.username || usernameMessages.length > 0}>
                <Field.Label>Username</Field.Label>
                <Input
                  type="text"
                  placeholder="johndoe"
                  {...register("username")}
                />
                {errors.username && (
                  <Field.ErrorText role="alert">
                    {errors.username.message}
                  </Field.ErrorText>
                )}
                {!errors.username &&
                  usernameMessages.map((message) => (
                    <Field.ErrorText role="alert" key={message}>
                      {message}
                    </Field.ErrorText>
                  ))}
              </Field.Root>
              <Field.Root invalid={!!errors.password || passwordMessages.length > 0}>
                <Field.Label>Password</Field.Label>
                <PasswordInput
                  placeholder="Create a password"
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
                {password.length > 0 && (
                  <PasswordStrengthMeter
                    mt={2}
                    value={getPasswordStrength(password)}
                  />
                )}
              </Field.Root>
              <Field.Root invalid={!!errors.confirmPassword}>
                <Field.Label>Confirm Password</Field.Label>
                <PasswordInput
                  placeholder="Confirm your password"
                  {...register("confirmPassword")}
                />
                {errors.confirmPassword && (
                  <Field.ErrorText role="alert">
                    {errors.confirmPassword.message}
                  </Field.ErrorText>
                )}
              </Field.Root>
              <Button
                type="submit"
                colorPalette="blue"
                size="lg"
                w="full"
                // Disabled until the Kratos registration flow has loaded:
                // onSubmit silently no-ops while `flow` is null, so a click
                // in that window would otherwise be dropped without any
                // feedback (caught live by the wave-7 integration run as
                // registrations stuck on /register under load).
                disabled={!flow}
                loading={registerMutation.isPending}
                loadingText="Creating account..."
              >
                Create account
              </Button>
            </Flex>
          </form>
          {flow && <SocialLoginButtons flow={flow} />}
          <Text mt={6} textAlign="center" fontSize="sm" color="fg.muted">
            Already have an account?{" "}
            <Link href="/login">
              <Text
                as="span"
                color="blue.500"
                fontWeight="medium"
                _hover={{ textDecoration: "underline" }}
              >
                Sign in
              </Text>
            </Link>
          </Text>
        </Card.Body>
      </Card.Root>
    </Flex>
  )
}
