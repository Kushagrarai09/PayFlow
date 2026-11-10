FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/service ./cmd/service && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway

FROM alpine:3.20
RUN adduser -D -H app
WORKDIR /app
COPY --from=build /out/service /app/service
COPY --from=build /out/gateway /app/gateway
USER app
EXPOSE 8080 8081 8082 8083 8084
ENTRYPOINT ["/app/service"]
