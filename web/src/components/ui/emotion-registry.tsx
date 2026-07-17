"use client"

import createCache from "@emotion/cache"
import type { EmotionCache } from "@emotion/cache"
import { CacheProvider as EmotionCacheProvider } from "@emotion/react"
import { useServerInsertedHTML } from "next/navigation"
import * as React from "react"

/**
 * Fixes the React #418 hydration mismatch on dynamic (streamed) routes such
 * as `/rooms` and `/rooms/[roomId]`.
 *
 * Emotion (used internally by Chakra UI v3's `styled`/`css`/`Global`) inserts
 * its `<style data-emotion="...">` tags at the call site by default. During
 * Next.js App Router streaming SSR that means the style tag lands *inside*
 * the streamed subtree, at the exact DOM position where the client's first
 * render expects to find the styled element itself — React 19 cannot
 * reconcile the extra node and aborts hydration for that subtree.
 *
 * This is the standard fix for CSS-in-JS libraries under the App Router
 * (see https://nextjs.org/docs/app/guides/css-in-js#other-css-in-js-libraries
 * and MUI's `AppRouterCacheProvider`): give Emotion a cache whose `insert` is
 * intercepted so inserted rule names are tracked instead of rendered inline,
 * then flush them once per server-render pass via `useServerInsertedHTML`.
 * Next.js splices that returned markup into `<head>` out-of-band, so the
 * streamed body never contains the style tag and the client's DOM position
 * expectations match exactly.
 */
export function EmotionRegistry({ children }: { children: React.ReactNode }) {
  const [{ cache, flush }] = React.useState(() => {
    const cache = createCache({ key: "css" })
    cache.compat = true

    const prevInsert = cache.insert
    let insertedNames: string[] = []
    cache.insert = (...args) => {
      const serialized = args[1]
      if (cache.inserted[serialized.name] === undefined) {
        insertedNames.push(serialized.name)
      }
      return prevInsert(...args)
    }

    const flush = () => {
      const names = insertedNames
      insertedNames = []
      return names
    }

    return { cache, flush }
  })

  useServerInsertedHTML(() => {
    const names = flush()
    if (names.length === 0) {
      return null
    }

    let styles = ""
    for (const name of names) {
      styles += (cache as EmotionCache).inserted[name]
    }

    return (
      <style
        key={cache.key}
        data-emotion={`${cache.key} ${names.join(" ")}`}
        dangerouslySetInnerHTML={{ __html: styles }}
      />
    )
  })

  return (
    <EmotionCacheProvider value={cache}>{children}</EmotionCacheProvider>
  )
}
