import type { Metadata } from "next"
import { Provider } from "@/components/ui/provider"
import { Toaster } from "@/components/ui/toaster"
import { QueryProvider } from "./query-provider"

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
        <QueryProvider>
          <Provider>
            {children}
            <Toaster />
          </Provider>
        </QueryProvider>
      </body>
    </html>
  )
}
