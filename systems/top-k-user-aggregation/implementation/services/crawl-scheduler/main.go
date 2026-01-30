package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	_ "github.com/lib/pq"
)

const (
	TypeCrawlUser = "crawl:user"
)

// CrawlUserPayload matches the crawl-worker's expected payload
type CrawlUserPayload struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
	Since    int64  `json:"since"`
}

// CrawlSchedule represents a row in user_crawl_schedule
type CrawlSchedule struct {
	UserID      string
	Provider    string
	NextCrawlAt time.Time
	Status      string
	UpdatedAt   time.Time
}

func main() {
	postgresURL := getEnv("POSTGRES_URL", "postgres://topk:topk@localhost:5432/topk?sslmode=disable")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	pollInterval := getEnvDuration("POLL_INTERVAL", 10*time.Second)
	stuckThreshold := getEnvDuration("STUCK_THRESHOLD", 1*time.Hour)

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping postgres: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// Create Asynq client
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer asynqClient.Close()

	log.Printf("Starting crawl-scheduler: poll=%v, stuck_threshold=%v", pollInterval, stuckThreshold)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Main scheduler loop
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopped")
			return
		case <-ticker.C:
			// 1. Process ready jobs (IDLE + next_crawl_at <= now)
			processedReady := processReadyJobs(ctx, db, asynqClient)

			// 2. Process stuck jobs (ENQUEUED too long - reconciliation)
			processedStuck := processStuckJobs(ctx, db, asynqClient, stuckThreshold)

			if processedReady > 0 || processedStuck > 0 {
				log.Printf("Processed: ready=%d, stuck=%d", processedReady, processedStuck)
			}
		}
	}
}

// processReadyJobs finds IDLE jobs ready to run and enqueues them
func processReadyJobs(ctx context.Context, db *sql.DB, client *asynq.Client) int {
	query := `
		UPDATE user_crawl_schedule
		SET status = 'ENQUEUED'
		WHERE (user_id, provider) IN (
			SELECT user_id, provider 
			FROM user_crawl_schedule
			WHERE next_crawl_at <= NOW() 
			  AND status = 'IDLE'
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		)
		RETURNING user_id, provider
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error querying ready jobs: %v", err)
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, provider string
		if err := rows.Scan(&userID, &provider); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		if err := enqueueJob(client, userID, provider); err != nil {
			log.Printf("Error enqueueing job for user=%s provider=%s: %v", userID, provider, err)
			// Revert status to IDLE so it can be retried
			revertToIdle(db, userID, provider)
			continue
		}

		count++
		log.Printf("Enqueued: user=%s provider=%s", userID, provider)
	}

	return count
}

// processStuckJobs finds ENQUEUED jobs that are stuck and re-enqueues them
func processStuckJobs(ctx context.Context, db *sql.DB, client *asynq.Client, threshold time.Duration) int {
	cutoff := time.Now().Add(-threshold)

	query := `
		UPDATE user_crawl_schedule
		SET status = 'ENQUEUED', updated_at = NOW()
		WHERE (user_id, provider) IN (
			SELECT user_id, provider 
			FROM user_crawl_schedule
			WHERE status = 'ENQUEUED'
			  AND updated_at < $1
			LIMIT 50
			FOR UPDATE SKIP LOCKED
		)
		RETURNING user_id, provider
	`

	rows, err := db.QueryContext(ctx, query, cutoff)
	if err != nil {
		log.Printf("Error querying stuck jobs: %v", err)
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, provider string
		if err := rows.Scan(&userID, &provider); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		if err := enqueueJob(client, userID, provider); err != nil {
			log.Printf("Error re-enqueueing stuck job for user=%s provider=%s: %v", userID, provider, err)
			continue
		}

		count++
		log.Printf("Re-enqueued stuck job: user=%s provider=%s", userID, provider)
	}

	return count
}

// enqueueJob creates and enqueues an Asynq task
func enqueueJob(client *asynq.Client, userID, provider string) error {
	payload, err := json.Marshal(CrawlUserPayload{
		UserID:   userID,
		Provider: provider,
		Since:    time.Now().Add(-24 * time.Hour).Unix(), // last 24 hours
	})
	if err != nil {
		return err
	}

	task := asynq.NewTask(TypeCrawlUser, payload)
	_, err = client.Enqueue(task, asynq.Queue("crawl"))
	return err
}

// revertToIdle sets status back to IDLE if enqueue fails
func revertToIdle(db *sql.DB, userID, provider string) {
	_, err := db.Exec(`
		UPDATE user_crawl_schedule 
		SET status = 'IDLE' 
		WHERE user_id = $1 AND provider = $2
	`, userID, provider)
	if err != nil {
		log.Printf("Error reverting status for user=%s provider=%s: %v", userID, provider, err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
