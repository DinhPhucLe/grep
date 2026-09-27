# syntax=docker/dockerfile:1

FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY contracts ./contracts
COPY fixtures ./fixtures
COPY src ./src
RUN npm run build && npm prune --omit=dev

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
ENV MCP_HTTP_ADDR=:3100
ENV MCP_BACKEND=mock
COPY --from=build /app/package.json /app/package-lock.json ./
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY --from=build /app/fixtures ./fixtures
COPY --from=build /app/contracts ./contracts
EXPOSE 3100
USER node
CMD ["node", "dist/src/index.js"]
