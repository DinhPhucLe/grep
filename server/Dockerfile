# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cortisol-api ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=build /out/cortisol-api /app/cortisol-api
ENV HTTP_ADDR=:8080
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/cortisol-api"]
