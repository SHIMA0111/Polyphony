import type { Metadata } from "next"
import { Provider } from "@/components/ui/provider"
import { MockBadge } from "@/components/mock-badge"

export const metadata: Metadata = {
  title: "Polyphony - Team AI Chat",
  description: "Collaborative AI chat for teams",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Provider>
          {children}
          <MockBadge />
        </Provider>
      </body>
    </html>
  )
}
