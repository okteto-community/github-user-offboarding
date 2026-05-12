FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o offboard .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/offboard /offboard
ENTRYPOINT ["/offboard"]
