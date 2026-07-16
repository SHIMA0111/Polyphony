import Link from "next/link"
import { Button, Card, Flex, Heading, Text } from "@chakra-ui/react"
import { SearchX } from "lucide-react"

/**
 * Rendered when Next's `notFound()` is triggered for this route segment —
 * i.e. `app/(main)/rooms/[roomId]/page.tsx`'s room prefetch resolves to a
 * 404 (the room ID doesn't exist or was deleted).
 *
 * Plain Server Component per the App Router `not-found.tsx` contract (no
 * props, no `"use client"` needed).
 */
export default function RoomNotFound() {
  return (
    <Flex minH="100vh" align="center" justify="center" bg="bg.subtle" p={4}>
      <Card.Root w="full" maxW="420px" shadow="lg">
        <Card.Header spaceY={3} textAlign="center">
          <Flex align="center" justify="center" mb={2}>
            <Flex
              h={14}
              w={14}
              rounded="full"
              bg="colorPalette.subtle"
              align="center"
              justify="center"
              colorPalette="gray"
            >
              <SearchX size={28} />
            </Flex>
          </Flex>
          <Heading size="2xl" fontWeight="bold">
            Room not found
          </Heading>
        </Card.Header>
        <Card.Body>
          <Text textAlign="center" color="fg.muted" mb={6}>
            This room doesn&apos;t exist or may have been deleted. Check the
            link, or head back to your rooms.
          </Text>
          <Link href="/rooms">
            <Button colorPalette="blue" size="lg" w="full">
              Back to rooms
            </Button>
          </Link>
        </Card.Body>
      </Card.Root>
    </Flex>
  )
}
