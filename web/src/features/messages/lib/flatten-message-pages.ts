import type { Message, MessagePage } from "../types"

/**
 * Flattens an infinite query's pages (`data.pages` from
 * `getMessagesInfiniteQueryOptions`) into a single, display-ready,
 * oldest-to-newest `Message[]`.
 *
 * `pages[0]` is the most recently fetched (newest) page, and each page's own
 * `messages` array is itself newest-first (the API's `ListByRoom` contract:
 * `ORDER BY sequence DESC`). To get a chronological list for rendering, each
 * page is reversed internally (oldest-first *within* that page) and the
 * pages themselves are walked oldest-page-first.
 *
 * @param pages - The infinite query's pages, newest page first.
 * @returns All messages across every page, oldest to newest.
 */
export function flattenMessagePages(pages: MessagePage[]): Message[] {
  return [...pages].reverse().flatMap((page) => [...page.messages].reverse())
}
