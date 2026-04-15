FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o transaction-service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/transaction-service .
EXPOSE 8080
ENTRYPOINT ["./transaction-service"]
