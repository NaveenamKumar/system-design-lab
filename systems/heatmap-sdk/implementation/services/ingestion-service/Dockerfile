FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o ingestion-service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/ingestion-service .

ENV PORT=8088
ENV KAFKA_BROKER=kafka:9092
ENV KAFKA_TOPIC=heatmap.events.raw

EXPOSE 8088
CMD ["./ingestion-service"]

