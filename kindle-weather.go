package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	secretMountPath        = "/etc/secrets"
	weatherAPIURLTemplate  = "https://api.openweathermap.org/data/3.0/onecall?lat=29.65&lon=-81.20&exclude=minutely&appid=%s&units=imperial"
	noaaAPIURLTemplate     = "https://api.tidesandcurrents.noaa.gov/api/prod/datagetter?product=predictions&application=NOS.COOPS.TAC.WL&datum=MLLW&station=8720218&time_zone=lst_ldt&units=english&interval=hilo&format=json&date=today"
	spacedevsAPIURLDefault = "https://ll.thespacedevs.com/2.3.0/launches/upcoming/?location__ids=27&format=json"
	tideCacheKeyLatest     = "latest-successful"
	weatherCacheKeyLatest  = "latest-successful"
	tideAPIMaxAttempts     = 3
	maxAPIResponseBytes    = 2 << 20
	maxWeatherStaleAge     = 6 * time.Hour
)

//go:embed templates/*
var templatesFS embed.FS

var (
	weatherAPIURL       string
	noaaAPIURL          string
	spacedevsAPIURL     string
	weatherCache        *cache.Cache
	tideCache           *cache.Cache
	launchCache         *cache.Cache
	httpClient          *http.Client
	weatherHTTPClient   *http.Client
	launchHTTPClient    *http.Client
	tmpl                *template.Template
	svgTmpl             *template.Template
	autoRefresh         time.Duration
	enableRocketPreview bool
	weatherRefreshMu    sync.Mutex
	tideRefreshMu       sync.Mutex
	launchRefreshMu     sync.Mutex

	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "path"})

	apiRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_requests_total",
		Help: "Total number of external API requests",
	}, []string{"api"})

	apiRequestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_request_errors_total",
		Help: "Total number of external API refresh failures",
	}, []string{"api"})

	apiRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "api_request_duration_seconds",
		Help:    "External API request duration in seconds",
		Buckets: []float64{.05, .1, .25, .5, 1, 2, 4, 8},
	}, []string{"api"})

	cacheRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_requests_total",
		Help: "Cache request outcomes",
	}, []string{"source", "result"})

	sourceDataAgeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "source_data_age_seconds",
		Help: "Age of the data currently presented by source",
	}, []string{"source"})
)

type APIError struct {
	URL       string
	Operation string
	Err       error
}

type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Method    string `json:"method,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    int    `json:"status,omitempty"`
	Duration  string `json:"duration,omitempty"`
	IP        string `json:"ip,omitempty"`
}

func init() {
	httpClient = &http.Client{
		Timeout:   8 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	// OpenWeather requires its API key in the URL query. Keep that request out
	// of automatic HTTP client tracing so query parameters cannot reach traces.
	weatherHTTPClient = &http.Client{
		Timeout:   8 * time.Second,
		Transport: http.DefaultTransport,
	}
	launchHTTPClient = &http.Client{
		Timeout:   2 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	noaaAPIURL = noaaAPIURLTemplate
	spacedevsAPIURL = spacedevsAPIURLDefault
	weatherCache = cache.New(time.Hour, 2*time.Hour)
	tideCache = cache.New(30*time.Minute, time.Hour)
	launchCache = cache.New(15*time.Minute, time.Hour)
	autoRefresh = 30 * time.Minute
	enableRocketPreview = false

	var err error
	funcs := template.FuncMap{
		"getIconClassName": getIconClassName,
		// SVG text cannot wrap, so multi-line captions are drawn one line at a time.
		"lines": func(s string) []string { return strings.Split(s, "\n") },
	}
	tmpl, err = template.New("index.html").Funcs(funcs).ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}
	svgTmpl, err = template.New("dashboard.svg").Funcs(funcs).ParseFS(templatesFS, "templates/dashboard.svg")
	if err != nil {
		log.Fatalf("failed to parse SVG template: %v", err)
	}
}

func configureRuntime() error {
	weatherAPIURL = strings.TrimSpace(os.Getenv("WEATHER_API_URL"))
	if weatherAPIURL == "" {
		openWeatherAPIKey := strings.TrimSpace(os.Getenv("OPENWEATHER_API_KEY"))
		if openWeatherAPIKey == "" {
			var err error
			openWeatherAPIKey, err = readSecret("openweather-api-key")
			if err != nil {
				return fmt.Errorf("failed to read OpenWeather API key: %w", err)
			}
		}
		weatherAPIURL = fmt.Sprintf(weatherAPIURLTemplate, openWeatherAPIKey)
	}

	noaaAPIURL = strings.TrimSpace(os.Getenv("NOAA_API_URL"))
	if noaaAPIURL == "" {
		noaaAPIURL = noaaAPIURLTemplate
	}

	spacedevsAPIURL = strings.TrimSpace(os.Getenv("SPACEDEVS_API_URL"))
	if spacedevsAPIURL == "" {
		spacedevsAPIURL = spacedevsAPIURLDefault
	}

	exp := parseEnvDurationSeconds("CACHE_EXPIRATION", time.Hour)
	cleanup := parseEnvDurationSeconds("CACHE_CLEANUP_INTERVAL", 2*time.Hour)
	weatherCache = cache.New(exp, cleanup)
	tideCache = cache.New(parseEnvDurationSeconds("TIDE_CACHE_EXPIRATION", 30*time.Minute), cleanup)
	launchCache = cache.New(parseEnvDurationSeconds("LAUNCH_CACHE_EXPIRATION", 15*time.Minute), cleanup)
	configureSurfRuntime(cleanup)
	autoRefresh = parseEnvDurationSeconds("AUTO_REFRESH_SECONDS", 30*time.Minute)
	launchHTTPClient.Timeout = parseEnvDurationSeconds("LAUNCH_API_TIMEOUT_SECONDS", 2*time.Second)
	enableRocketPreview = parseEnvBool("ENABLE_ROCKET_PREVIEW")

	return nil
}

func readSecret(secretName string) (string, error) {
	secretPath := filepath.Join(secretMountPath, secretName)
	secretValue, err := os.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %v", err)
	}
	return strings.TrimSpace(string(secretValue)), nil
}

func parseEnvDurationSeconds(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	// Interpret as integer seconds
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return def
	}
	return time.Duration(secs) * time.Second
}

func parseEnvBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func setupOpenTelemetry(ctx context.Context) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	tracesEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if endpoint == "" && tracesEndpoint == "" {
		return nil, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "kindle-weather"
	}

	res := resource.NewWithAttributes(
		"",
		attribute.String("service.name", serviceName),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)

	return tp.Shutdown, nil
}

func buildAutoRefreshURL(r *http.Request, refreshToken int64) string {
	q := r.URL.Query()
	q.Set("refresh", strconv.FormatInt(refreshToken, 10))

	return (&url.URL{
		Path:     r.URL.Path,
		RawQuery: q.Encode(),
		Fragment: detailSection(q.Get("section")),
	}).String()
}

func logJSON(entry logEntry) {
	jsonEntry, err := json.Marshal(entry)
	if err != nil {
		log.Println("Error marshalling log entry:", err)
		return
	}
	fmt.Println(string(jsonEntry))
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := &loggingResponseWriter{w, http.StatusOK}
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		// Record metrics
		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(lrw.statusCode)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())

		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "INFO",
			Message:   "Request completed",
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    lrw.statusCode,
			Duration:  duration.String(),
			IP:        r.RemoteAddr,
		})
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.TrimSpace(weatherAPIURL) == "" || tmpl == nil || svgTmpl == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logJSON(logEntry{Timestamp: time.Now().Format(time.RFC3339), Level: "ERROR", Message: fmt.Sprintf("Recovered request panic: %v", recovered)})
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (e *APIError) Error() string {
	return fmt.Sprintf("error during %s: %v (url: %s)", e.Operation, e.Err, redactURL(e.URL))
}

func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid URL]"
	}
	q := u.Query()
	for key := range q {
		switch strings.ToLower(key) {
		case "appid", "api_key", "apikey", "key", "token", "access_token":
			q.Set(key, "REDACTED")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		os.Exit(runHealthcheck())
	}

	if err := configureRuntime(); err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "FATAL",
			Message:   err.Error(),
		})
		os.Exit(1)
	}

	otelShutdown, err := setupOpenTelemetry(context.Background())
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to initialize OpenTelemetry: %v", err),
		})
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", loggingMiddleware(otelhttp.NewHandler(http.HandlerFunc(handler), "GET /")))
	mux.Handle("GET /dashboard.svg", loggingMiddleware(otelhttp.NewHandler(http.HandlerFunc(dashboardSVGHandler), "GET /dashboard.svg")))
	mux.Handle("GET /css/", loggingMiddleware(otelhttp.NewHandler(http.StripPrefix("/css/", http.FileServer(http.Dir("css"))), "GET /css")))
	mux.Handle("GET /font/", loggingMiddleware(otelhttp.NewHandler(http.StripPrefix("/font/", http.FileServer(http.Dir("font"))), "GET /font")))
	mux.Handle("GET /metrics", otelhttp.NewHandler(promhttp.Handler(), "GET /metrics"))
	mux.Handle("GET /health", otelhttp.NewHandler(http.HandlerFunc(healthHandler), "GET /health"))
	mux.Handle("GET /ready", otelhttp.NewHandler(http.HandlerFunc(readinessHandler), "GET /ready"))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           recoveryMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      12 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "INFO",
			Message:   "Server started at http://localhost:8080",
		})
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-shutdownSignal.Done():
	case err := <-serverErrors:
		logJSON(logEntry{Timestamp: time.Now().Format(time.RFC3339), Level: "FATAL", Message: fmt.Sprintf("Server failed: %v", err)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)

	if otelShutdown != nil {
		if err := otelShutdown(ctx); err != nil {
			logJSON(logEntry{
				Timestamp: time.Now().Format(time.RFC3339),
				Level:     "ERROR",
				Message:   fmt.Sprintf("Failed to shut down OpenTelemetry: %v", err),
			})
		}
	}
}

func runHealthcheck() int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8080/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
