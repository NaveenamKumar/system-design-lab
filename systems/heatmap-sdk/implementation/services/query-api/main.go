package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	gridW = 100
	gridH = 200
)

type cell struct {
	CellX int32 `json:"cell_x"`
	CellY int32 `json:"cell_y"`
	Count int64 `json:"count"`
}

type hourlyResponse struct {
	GridW       int    `json:"grid_w"`
	GridH       int    `json:"grid_h"`
	ClientID    string `json:"client_id"`
	ScreenID    string `json:"screen_id"`
	BucketStart string `json:"bucket_start"`
	Cells       []cell `json:"cells"`
}

type clickhouseJSON struct {
	Data []map[string]any `json:"data"`
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)

func main() {
	port := env("PORT", "8091")
	chURL := env("CLICKHOUSE_URL", "http://clickhouse:8123")
	chUser := env("CLICKHOUSE_USER", "heatmap")
	chPass := env("CLICKHOUSE_PASSWORD", "heatmap")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /heatmap/hourly", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		clientID := strings.TrimSpace(q.Get("client_id"))
		screenID := strings.TrimSpace(q.Get("screen_id"))
		dt := strings.TrimSpace(q.Get("dt"))   // YYYY-MM-DD
		hour := strings.TrimSpace(q.Get("hour")) // HH

		if !safeID.MatchString(clientID) {
			httpError(w, http.StatusBadRequest, "invalid_client_id")
			return
		}
		if !safeID.MatchString(screenID) {
			httpError(w, http.StatusBadRequest, "invalid_screen_id")
			return
		}
		if _, err := time.Parse("2006-01-02", dt); err != nil {
			httpError(w, http.StatusBadRequest, "invalid_dt")
			return
		}
		hh, err := strconv.Atoi(hour)
		if err != nil || hh < 0 || hh > 23 {
			httpError(w, http.StatusBadRequest, "invalid_hour")
			return
		}

		bucketStart := fmt.Sprintf("%s %02d:00:00", dt, hh)

		// Note: this is a lab API. We still keep IDs constrained to reduce injection risk.
		sql := fmt.Sprintf(`
SELECT cell_x, cell_y, sum(count) AS count
FROM heatmap.heatmap_hourly
WHERE client_id = '%s'
  AND screen_id = '%s'
  AND bucket_start = toDateTime('%s')
GROUP BY cell_x, cell_y
FORMAT JSON
`, escapeSQL(clientID), escapeSQL(screenID), escapeSQL(bucketStart))

		body, err := clickhouseQuery(chURL, chUser, chPass, sql)
		if err != nil {
			log.Printf("clickhouse query failed: %v", err)
			httpError(w, http.StatusBadGateway, "clickhouse_query_failed")
			return
		}

		var parsed clickhouseJSON
		if err := json.Unmarshal(body, &parsed); err != nil {
			log.Printf("clickhouse json parse failed: %v", err)
			httpError(w, http.StatusBadGateway, "clickhouse_bad_json")
			return
		}

		out := hourlyResponse{
			GridW:       gridW,
			GridH:       gridH,
			ClientID:    clientID,
			ScreenID:    screenID,
			BucketStart: bucketStart,
			Cells:       make([]cell, 0, len(parsed.Data)),
		}

		for _, row := range parsed.Data {
			cx, ok1 := asInt32(row["cell_x"])
			cy, ok2 := asInt32(row["cell_y"])
			cnt, ok3 := asInt64(row["count"])
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			out.Cells = append(out.Cells, cell{CellX: cx, CellY: cy, Count: cnt})
		}

		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /buckets/hourly", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		clientID := strings.TrimSpace(q.Get("client_id"))
		screenID := strings.TrimSpace(q.Get("screen_id"))
		limitStr := strings.TrimSpace(q.Get("limit"))
		limit := 24
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 500 {
				limit = v
			}
		}

		if !safeID.MatchString(clientID) || !safeID.MatchString(screenID) {
			httpError(w, http.StatusBadRequest, "invalid_ids")
			return
		}

		sql := fmt.Sprintf(`
SELECT bucket_start, sum(count) AS events
FROM heatmap.heatmap_hourly
WHERE client_id = '%s'
  AND screen_id = '%s'
GROUP BY bucket_start
ORDER BY bucket_start DESC
LIMIT %d
FORMAT JSON
`, escapeSQL(clientID), escapeSQL(screenID), limit)

		body, err := clickhouseQuery(chURL, chUser, chPass, sql)
		if err != nil {
			log.Printf("clickhouse query failed: %v", err)
			httpError(w, http.StatusBadGateway, "clickhouse_query_failed")
			return
		}

		writeRawJSON(w, http.StatusOK, body)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("query-api listening on :%s (clickhouse=%s)", port, chURL)
	log.Fatal(srv.ListenAndServe())
}

func clickhouseQuery(baseURL, user, pass, sql string) ([]byte, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	// ClickHouse HTTP: POST body is the query. Auth via query params.
	q := u.Query()
	q.Set("user", user)
	q.Set("password", pass)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("POST", u.String(), bytes.NewBufferString(sql))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clickhouse status=%d body=%s", resp.StatusCode, string(b))
	}
	return b, nil
}

func asInt32(v any) (int32, bool) {
	switch t := v.(type) {
	case float64:
		return int32(t), true
	case int:
		return int32(t), true
	case int32:
		return t, true
	case int64:
		return int32(t), true
	case string:
		i, err := strconv.Atoi(t)
		return int32(i), err == nil
	default:
		return 0, false
	}
}

func asInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}

func escapeSQL(s string) string {
	// Minimal quoting to keep the lab safe-ish.
	return strings.ReplaceAll(s, "'", "''")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRawJSON(w http.ResponseWriter, code int, raw []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
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

