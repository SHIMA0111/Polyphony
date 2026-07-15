"use client"

import Link from "next/link"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { Button, Card, Field, Flex, Heading, Input, Text } from "@chakra-ui/react"
import { Pen } from "lucide-react"
import { PasswordInput, PasswordStrengthMeter } from "@/components/ui/password-input"
import { toaster } from "@/components/ui/toaster"
import { useRegister } from "@/features/auth/hooks/use-register"
import { getPasswordStrength } from "@/features/auth/utils/password-strength"
import { registerSchema, type RegisterFormValues } from "@/features/auth/utils/schemas"

export function RegisterForm() {
  const registerMutation = useRegister()
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
    try {
      await registerMutation.mutateAsync({
        email: values.email,
        username: values.username,
        password: values.password,
      })
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Registration failed",
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
          <Card.Title fontSize="2xl">Create an account</Card.Title>
          <Card.Description>
            Get started with team AI collaboration
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
              <Field.Root invalid={!!errors.username}>
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
              </Field.Root>
              <Field.Root invalid={!!errors.password}>
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
                loading={registerMutation.isPending}
                loadingText="Creating account..."
              >
                Create account
              </Button>
            </Flex>
          </form>
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
