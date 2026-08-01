FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server .

RUN apk add --no-cache ca-certificates

EXPOSE 5050

CMD [ "./server" ]