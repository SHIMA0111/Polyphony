"use client"

import { useState } from "react"
import Link from "next/link"
import {
  Box,
  Button,
  Card,
  Field,
  Flex,
  Heading,
  Input,
  Text,
} from "@chakra-ui/react"
import { Pen } from "lucide-react"
import { useAuth } from "@/hooks/use-auth"

export function RegisterForm() {
  const [email, setEmail] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [error, setError] = useState("")
  const [isSubmitting, setIsSubmitting] = useState(false)
  const { register } = useAuth()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")

    if (password !== confirmPassword) {
      setError("Passwords do not match")
      return
    }

    setIsSubmitting(true)

    try {
      await register(email, username, password)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Registration failed")
    } finally {
      setIsSubmitting(false)
    }
  }

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
          <form onSubmit={handleSubmit}>
            <Flex direction="column" gap={4}>
              {error && (
                <Box
                  bg="red.50"
                  color="red.600"
                  p={3}
                  rounded="md"
                  fontSize="sm"
                >
                  {error}
                </Box>
              )}
              <Field.Root>
                <Field.Label>Email</Field.Label>
                <Input
                  type="email"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </Field.Root>
              <Field.Root>
                <Field.Label>Username</Field.Label>
                <Input
                  type="text"
                  placeholder="johndoe"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                />
              </Field.Root>
              <Field.Root>
                <Field.Label>Password</Field.Label>
                <Input
                  type="password"
                  placeholder="Create a password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </Field.Root>
              <Field.Root>
                <Field.Label>Confirm Password</Field.Label>
                <Input
                  type="password"
                  placeholder="Confirm your password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                />
              </Field.Root>
              <Button
                type="submit"
                colorPalette="blue"
                size="lg"
                w="full"
                loading={isSubmitting}
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
