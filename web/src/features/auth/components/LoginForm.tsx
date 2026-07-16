"use client"

import Link from "next/link"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Button, Card, Field, Flex, Heading, Input, Text } from "@chakra-ui/react"
import { Pen } from "lucide-react"
import { PasswordInput } from "@/components/ui/password-input"
import { toaster } from "@/components/ui/toaster"
import { useLogin } from "@/features/auth/hooks/use-login"
import { loginSchema, type LoginFormValues } from "@/features/auth/utils/schemas"

export function LoginForm() {
  const loginMutation = useLogin()
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    mode: "onBlur",
  })

  const onSubmit = handleSubmit(async (values) => {
    try {
      await loginMutation.mutateAsync(values)
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Sign in failed",
        description: err instanceof Error ? err.message : "Please try again.",
      })
    }
  })

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
              <Field.Root invalid={!!errors.email}>
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
              </Field.Root>
              <Field.Root invalid={!!errors.password}>
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
              </Field.Root>
              <Button
                type="submit"
                colorPalette="blue"
                size="lg"
                w="full"
                loading={loginMutation.isPending}
                loadingText="Signing in..."
              >
                Sign in
              </Button>
            </Flex>
          </form>
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
