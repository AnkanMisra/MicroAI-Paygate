FROM oven/bun:1 AS deps
WORKDIR /app
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

FROM deps AS build
WORKDIR /app
# Next.js inlines NEXT_PUBLIC_* at build time. Every public variable the
# frontend reads must be declared as an ARG here AND wired via build args
# in docker-compose / CI; otherwise the container ships the source defaults
# regardless of the gateway's CHAIN_ID / PAYMENT_AMOUNT config.
ARG NEXT_PUBLIC_GATEWAY_URL=http://localhost:3000
ARG NEXT_PUBLIC_EXPECTED_CHAIN_ID=84532
ARG NEXT_PUBLIC_EXPECTED_CHAIN_NAME="Base Sepolia"
ARG NEXT_PUBLIC_PAYMENT_AMOUNT=0.001
ARG NEXT_PUBLIC_PAYMENT_TOKEN=USDC
ENV NEXT_PUBLIC_GATEWAY_URL=$NEXT_PUBLIC_GATEWAY_URL
ENV NEXT_PUBLIC_EXPECTED_CHAIN_ID=$NEXT_PUBLIC_EXPECTED_CHAIN_ID
ENV NEXT_PUBLIC_EXPECTED_CHAIN_NAME=$NEXT_PUBLIC_EXPECTED_CHAIN_NAME
ENV NEXT_PUBLIC_PAYMENT_AMOUNT=$NEXT_PUBLIC_PAYMENT_AMOUNT
ENV NEXT_PUBLIC_PAYMENT_TOKEN=$NEXT_PUBLIC_PAYMENT_TOKEN
COPY . .
RUN bun run build

FROM oven/bun:1 AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3001
COPY --from=deps /app/package.json /app/bun.lock* ./
COPY --from=deps /app/node_modules /app/node_modules
COPY --from=build /app/.next /app/.next
COPY --from=build /app/public /app/public
EXPOSE 3001
CMD ["bun", "start"]
