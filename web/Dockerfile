# === Build stage ===
FROM oven/bun:1-alpine AS build

ARG NEXT_PUBLIC_API_URL=http://localhost:8080
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

WORKDIR /app
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

COPY . .
RUN bun run build

# === Runtime stage ===
FROM oven/bun:1-alpine

WORKDIR /app

# Copy Next.js standalone output
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public

EXPOSE 3000
ENV HOSTNAME=0.0.0.0
CMD ["bun", "server.js"]
