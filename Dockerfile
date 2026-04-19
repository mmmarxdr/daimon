FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o daimon ./cmd/daimon

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/daimon /usr/local/bin/daimon
RUN adduser -D -h /home/daimon daimon
USER daimon
WORKDIR /home/daimon
ENTRYPOINT ["daimon"]
