FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o query-api .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/query-api .

ENV PORT=8091
ENV CLICKHOUSE_URL=http://clickhouse:8123
ENV CLICKHOUSE_USER=heatmap
ENV CLICKHOUSE_PASSWORD=heatmap

EXPOSE 8091
CMD ["./query-api"]

