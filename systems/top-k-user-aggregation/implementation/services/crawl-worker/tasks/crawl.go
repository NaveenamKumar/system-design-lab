package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hibiken/asynq"
	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
)

const TypeCrawlUser = "crawl:user"

// DB connection (initialized once)
var db *sql.DB

func init() {
	postgresURL := getEnv("POSTGRES_URL", "")
	if postgresURL == "" {
		log.Println("POSTGRES_URL not set, DB updates disabled")
		return
	}

	var err error
	db, err = sql.Open("postgres", postgresURL)
	if err != nil {
		log.Printf("Warning: failed to connect to postgres: %v", err)
		return
	}

	if err := db.Ping(); err != nil {
		log.Printf("Warning: failed to ping postgres: %v", err)
		db = nil
		return
	}
	log.Println("Connected to PostgreSQL for status updates")
}

// CrawlUserPayload is the job payload
type CrawlUserPayload struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
	Since    int64  `json:"since"` // unix timestamp
}

// ListenEvent is the normalized event we publish to Kafka
type ListenEvent struct {
	EventID    string `json:"event_id"`
	UserID     string `json:"user_id"`
	SongID     string `json:"song_id"`
	Provider   string `json:"provider"`
	ListenedAt int64  `json:"listened_at"`
}

// NewCrawlUserTask creates a new crawl task
func NewCrawlUserTask(userID, provider string, since time.Time) (*asynq.Task, error) {
	payload, err := json.Marshal(CrawlUserPayload{
		UserID:   userID,
		Provider: provider,
		Since:    since.Unix(),
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCrawlUser, payload), nil
}

// HandleCrawlUserTask processes the crawl job
func HandleCrawlUserTask(ctx context.Context, t *asynq.Task) error {
	var p CrawlUserPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	log.Printf("Crawling user=%s provider=%s since=%d", p.UserID, p.Provider, p.Since)

	// 1. Update status to RUNNING (if DB available)
	updateStatus(p.UserID, p.Provider, "RUNNING", "")

	// 2. Fetch listen history from provider (simulated for now)
	events := fetchListenHistory(p.UserID, p.Provider, p.Since)

	// 3. Publish events to Kafka
	if err := publishEvents(ctx, events); err != nil {
		// Mark as IDLE so scheduler can retry
		updateStatusWithError(p.UserID, p.Provider, "IDLE", fmt.Sprintf("publish error: %v", err))
		return fmt.Errorf("publish events: %w", err)
	}

	// 4. Update DB: status=IDLE, next_crawl_at=tomorrow
	//    Scheduler will pick it up tomorrow
	markCrawlComplete(p.UserID, p.Provider)

	log.Printf("Crawl complete: user=%s events=%d", p.UserID, len(events))
	return nil
}

// fetchListenHistory simulates fetching from a provider API
// TODO: replace with real provider API calls
func fetchListenHistory(userID, provider string, since int64) []ListenEvent {
	// Simulated: generate some fake events
	var events []ListenEvent
	now := time.Now().Unix()
	for i := 0; i < 10; i++ {
		events = append(events, ListenEvent{
			EventID:    fmt.Sprintf("%s-%s-%d-%d", userID, provider, now, i),
			UserID:     userID,
			SongID:     fmt.Sprintf("song-%d", i%100),
			Provider:   provider,
			ListenedAt: since + int64(i*3600), // 1 hour apart
		})
	}
	return events
}

// publishEvents sends events to Kafka topic user.listen.raw
func publishEvents(ctx context.Context, events []ListenEvent) error {
	kafkaBroker := getEnv("KAFKA_BROKER", "localhost:29092")
	topic := "user.listen.raw"

	w := &kafka.Writer{
		Addr:     kafka.TCP(kafkaBroker),
		Topic:    topic,
		Balancer: &kafka.Hash{}, // partition by key (user_id)
	}
	defer w.Close()

	var msgs []kafka.Message
	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		msgs = append(msgs, kafka.Message{
			Key:   []byte(e.UserID),
			Value: data,
		})
	}

	return w.WriteMessages(ctx, msgs...)
}

// updateStatus updates the job status in PostgreSQL
func updateStatus(userID, provider, status, lastError string) {
	if db == nil {
		return
	}

	_, err := db.Exec(`
		UPDATE user_crawl_schedule 
		SET status = $1, last_error = $2
		WHERE user_id = $3 AND provider = $4
	`, status, sql.NullString{String: lastError, Valid: lastError != ""}, userID, provider)

	if err != nil {
		log.Printf("Warning: failed to update status: %v", err)
	}
}

// updateStatusWithError updates status and records error
func updateStatusWithError(userID, provider, status, lastError string) {
	if db == nil {
		return
	}

	_, err := db.Exec(`
		UPDATE user_crawl_schedule 
		SET status = $1, last_error = $2
		WHERE user_id = $3 AND provider = $4
	`, status, lastError, userID, provider)

	if err != nil {
		log.Printf("Warning: failed to update status with error: %v", err)
	}
}

// markCrawlComplete sets status=IDLE and schedules next crawl for tomorrow
func markCrawlComplete(userID, provider string) {
	if db == nil {
		return
	}

	tomorrow := time.Now().Add(24 * time.Hour)

	_, err := db.Exec(`
		UPDATE user_crawl_schedule 
		SET status = 'IDLE', 
		    next_crawl_at = $1,
		    last_error = NULL
		WHERE user_id = $2 AND provider = $3
	`, tomorrow, userID, provider)

	if err != nil {
		log.Printf("Warning: failed to mark crawl complete: %v", err)
	} else {
		log.Printf("Scheduled next crawl for user=%s provider=%s at %v", userID, provider, tomorrow)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
