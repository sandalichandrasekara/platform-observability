package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var serviceName = getenv("SERVICE_NAME", "demoapi")

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, partitioned by method, path and status code.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets, // .005 .. 10s
		},
		[]string{"method", "path"},
	)

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being served.",
	})
)

func main() {

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})).With("service", serviceName)
	slog.SetDefault(logger)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/work", handleWork)
	mux.HandleFunc("/error", handleError)

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           instrument(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if getenv("GENERATE_LOAD", "true") == "true" {
		go generateLoad(ctx, addr)
	}

	go func() {
		logger.Info("starting demo api", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		dur := time.Since(start)
		route := routeLabel(r.URL.Path)
		httpRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route).Observe(dur.Seconds())

		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		} else if rec.status >= 400 {
			level = slog.LevelWarn
		}
		slog.Default().Log(r.Context(), level, "request completed",
			"method", r.Method,
			"path", route,
			"status", rec.status,
			"duration_ms", float64(dur.Microseconds())/1000.0,
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func routeLabel(path string) string {
	switch path {
	case "/", "/work", "/error", "/healthz":
		return path
	default:
		return "/other"
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": serviceName,
		"message": "hello from the observability demo api",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleWork(w http.ResponseWriter, r *http.Request) {

	base := time.Duration(20+rand.Intn(80)) * time.Millisecond
	if rand.Float64() < 0.1 {
		base += time.Duration(300+rand.Intn(700)) * time.Millisecond
	}
	time.Sleep(base)
	writeJSON(w, http.StatusOK, map[string]any{
		"result":     "done",
		"latency_ms": base.Milliseconds(),
		"work_units": 1 + rand.Intn(5),
	})
}

func handleError(w http.ResponseWriter, r *http.Request) {
	if rand.Float64() < 0.35 {
		slog.Default().Error("simulated downstream failure", "path", "/error")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "simulated failure"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func generateLoad(ctx context.Context, addr string) {
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://localhost" + addr
	paths := []string{"/", "/work", "/work", "/work", "/error", "/healthz"}
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p := paths[rand.Intn(len(paths))]
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+p, nil)
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
