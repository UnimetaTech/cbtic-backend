FROM golang:1.26.2-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cbtic-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cbtic-worker ./cmd/worker

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/cbtic-api /app/cbtic-api
COPY --from=builder /out/cbtic-worker /app/cbtic-worker
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY migrations /app/migrations

EXPOSE 8080

CMD ["sh", "-c", "goose -dir /app/migrations postgres \"$DATABASE_URL\" up && exec /app/cbtic-api"]
