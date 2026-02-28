FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY . .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /backend .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /backend .
EXPOSE 8000
CMD ["./backend"]
