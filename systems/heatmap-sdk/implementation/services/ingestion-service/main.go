package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

type BatchRequest struct {
	ClientID string  `json:"client_id"`
	ScreenID string  `json:"screen_id"`
	BatchID  string  `json:"batch_id,omitempty"`
	Events   []Event `json:"events"`
}

type Event struct {
	EventTime string  `json:"event_time"` // RFC3339
	XNorm     float64 `json:"x_norm"`
	YNorm     float64 `json:"y_norm"`
	EventType string  `json:"event_type"`

	ReceivedAt string `json:"received_at,omitempty"`
}

type KafkaEvent struct {
	ClientID  string  `json:"client_id"`
	ScreenID  string  `json:"screen_id"`
	BatchID   string  `json:"batch_id,omitempty"`
	EventTime string  `json:"event_time"`
	XNorm     float64 `json:"x_norm"`
	YNorm     float64 `json:"y_norm"`
	EventType string  `json:"event_type"`
	ReceivedAt string `json:"received_at,omitempty"`
}

func main() {
	port := env("PORT", "8088")
	kafkaBroker := env("KAFKA_BROKER", "kafka:9092")
	topic := env("KAFKA_TOPIC", "heatmap.events.raw")

	writer := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBroker),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		BatchSize:    1000,
		BatchTimeout: 200 * time.Millisecond,
	}
	defer func() {
		_ = writer.Close()
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /events/batch", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		var req BatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "invalid_json")
			return
		}

		if err := validateBatch(req); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		msgs := make([]kafka.Message, 0, len(req.Events))
		key := []byte(req.ClientID + "|" + req.ScreenID)
		for i := range req.Events {
			req.Events[i].ReceivedAt = now
			ke := KafkaEvent{
				ClientID:   req.ClientID,
				ScreenID:   req.ScreenID,
				BatchID:    req.BatchID,
				EventTime:  req.Events[i].EventTime,
				XNorm:      req.Events[i].XNorm,
				YNorm:      req.Events[i].YNorm,
				EventType:  req.Events[i].EventType,
				ReceivedAt: req.Events[i].ReceivedAt,
			}
			b, err := json.Marshal(ke)
			if err != nil {
				httpError(w, http.StatusInternalServerError, "marshal_failed")
				return
			}
			msgs = append(msgs, kafka.Message{
				Key:   key,
				Value: b,
				Time:  time.Now(),
			})
		}

		if err := writer.WriteMessages(ctx, msgs...); err != nil {
			log.Printf("kafka write failed: %v", err)
			httpError(w, http.StatusServiceUnavailable, "kafka_write_failed")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accepted": len(req.Events),
		})
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("ingestion-service listening on :%s (topic=%s broker=%s)", port, topic, kafkaBroker)
	log.Fatal(srv.ListenAndServe())
}

func validateBatch(req BatchRequest) error {
	if strings.TrimSpace(req.ClientID) == "" {
		return errors.New("missing_client_id")
	}
	if strings.TrimSpace(req.ScreenID) == "" {
		return errors.New("missing_screen_id")
	}
	if len(req.Events) == 0 {
		return errors.New("empty_events")
	}
	if len(req.Events) > 5000 {
		return errors.New("too_many_events")
	}
	for _, e := range req.Events {
		if strings.TrimSpace(e.EventType) == "" {
			return errors.New("missing_event_type")
		}
		if e.XNorm < 0 || e.XNorm > 1 || e.YNorm < 0 || e.YNorm > 1 {
			return errors.New("invalid_normalized_coords")
		}
		if _, err := time.Parse(time.RFC3339Nano, e.EventTime); err != nil {
			if _, err2 := time.Parse(time.RFC3339, e.EventTime); err2 != nil {
				return errors.New("invalid_event_time")
			}
		}
	}
	return nil
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
	})
}

func env(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

