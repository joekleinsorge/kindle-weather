package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
	secretMountPath       = "/etc/secrets"
	weatherAPIURLTemplate = "https://api.openweathermap.org/data/3.0/onecall?lat=29.65&lon=-81.20&exclude=minutely&appid=%s&units=imperial"
	noaaAPIURLTemplate    = "https://api.tidesandcurrents.noaa.gov/api/prod/datagetter?product=predictions&application=NOS.COOPS.TAC.WL&datum=MLLW&station=8720218&time_zone=lst_ldt&units=english&interval=hilo&format=json&date=today"
	spacedevsAPIURL       = "https://ll.thespacedevs.com/2.3.0/launches/upcoming/?location__ids=27&format=json"
)

//go:embed templates/*.html
var templatesFS embed.FS

var (
	weatherAPIURL       string
	noaaAPIURL          string
	weatherCache        *cache.Cache
	tideCache           *cache.Cache
	launchCache         *cache.Cache
	httpClient          *http.Client
	launchHTTPClient    *http.Client
	tmpl                *template.Template
	autoRefresh         time.Duration
	enableRocketPreview bool

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
)

type WeatherData struct {
	Current        CurrentWeather  `json:"current"`
	Hourly         []HourlyWeather `json:"hourly"`
	Daily          []DailyWeather  `json:"daily"`
	Timezone       string          `json:"timezone"`
	TimezoneOffset int             `json:"timezone_offset"`
}

type CurrentWeather struct {
	Dt               int64              `json:"dt"`
	Sunrise          int64              `json:"sunrise"`
	Sunset           int64              `json:"sunset"`
	Temp             float64            `json:"temp"`
	FeelsLike        float64            `json:"feels_like"`
	Pressure         int                `json:"pressure"`
	Humidity         int                `json:"humidity"`
	DewPoint         float64            `json:"dew_point"`
	Uvi              float64            `json:"uvi"`
	Clouds           float64            `json:"clouds"`
	Visibility       int                `json:"visibility"`
	WindSpeed        float64            `json:"wind_speed"`
	WindDeg          int                `json:"wind_deg"`
	WindGust         float64            `json:"wind_gust"`
	Weather          []WeatherCondition `json:"weather"`
	SunriseFormatted string
	SunsetFormatted  string
}

type HourlyWeather struct {
	Dt          int64              `json:"dt"`
	DtFormatted string             `json:"-"`
	Temp        float64            `json:"temp"`
	FeelsLike   float64            `json:"feels_like"`
	Pressure    int                `json:"pressure"`
	Humidity    int                `json:"humidity"`
	DewPoint    float64            `json:"dew_point"`
	Uvi         float64            `json:"uvi"`
	Clouds      float64            `json:"clouds"`
	Visibility  int                `json:"visibility"`
	WindSpeed   float64            `json:"wind_speed"`
	WindGust    float64            `json:"wind_gust"`
	WindDeg     int                `json:"wind_deg"`
	Weather     []WeatherCondition `json:"weather"`
	Pop         float64            `json:"pop"`
	Rain        Rain               `json:"rain"`
}

type DailyWeather struct {
	Moonrise  int64   `json:"moonrise"`
	Moonset   int64   `json:"moonset"`
	MoonPhase float64 `json:"moon_phase"`
	Summary   string  `json:"summary"`
}

type WeatherCondition struct {
	ID          int    `json:"id"`
	Main        string `json:"main"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type Rain struct {
	OneH float64 `json:"1h"`
}

type TideData struct {
	Predictions []TidePrediction `json:"predictions"`
}

type TidePrediction struct {
	Time   string  `json:"t"`
	Type   string  `json:"type"`
	Height float64 `json:"v"`
}

type APIError struct {
	URL       string
	Operation string
	Err       error
}

type LaunchData struct {
	WindowStart string    `json:"window_start"`
	WindowEnd   string    `json:"window_end"`
	Name        string    `json:"name"`
	Pad         LaunchPad `json:"pad"`
}

type LaunchPad struct {
	Name     string         `json:"name"`
	Location LaunchLocation `json:"location"`
}

type LaunchLocation struct {
	Name string `json:"name"`
}

type LaunchInfo struct {
	Name      string
	Scheduled string
}

type launchCacheEntry struct {
	Launch *LaunchInfo
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
	launchHTTPClient = &http.Client{
		Timeout:   2 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	noaaAPIURL = noaaAPIURLTemplate
	weatherCache = cache.New(time.Hour, 2*time.Hour)
	tideCache = cache.New(30*time.Minute, time.Hour)
	launchCache = cache.New(15*time.Minute, time.Hour)
	autoRefresh = 30 * time.Minute
	enableRocketPreview = false

	var err error
	tmpl, err = template.New("index.html").Funcs(template.FuncMap{
		"getIconClassName": getIconClassName,
	}).ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}
}

func configureRuntime() error {
	openWeatherAPIKey := strings.TrimSpace(os.Getenv("OPENWEATHER_API_KEY"))
	if openWeatherAPIKey == "" {
		var err error
		openWeatherAPIKey, err = readSecret("openweather-api-key")
		if err != nil {
			return fmt.Errorf("failed to read OpenWeather API key: %w", err)
		}
	}
	weatherAPIURL = fmt.Sprintf(weatherAPIURLTemplate, openWeatherAPIKey)
	noaaAPIURL = noaaAPIURLTemplate

	exp := parseEnvDurationSeconds("CACHE_EXPIRATION", time.Hour)
	cleanup := parseEnvDurationSeconds("CACHE_CLEANUP_INTERVAL", 2*time.Hour)
	weatherCache = cache.New(exp, cleanup)
	tideCache = cache.New(parseEnvDurationSeconds("TIDE_CACHE_EXPIRATION", 30*time.Minute), cleanup)
	launchCache = cache.New(parseEnvDurationSeconds("LAUNCH_CACHE_EXPIRATION", 15*time.Minute), cleanup)
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
	}).String()
}

func getWeatherWithCache(ctx context.Context) (WeatherData, error) {
	// Check if weather data is in cache
	if cachedData, found := weatherCache.Get("weather"); found {
		return cachedData.(WeatherData), nil
	}

	// If not in cache, fetch from API
	data, err := fetchWeatherFromAPI(ctx)
	if err != nil {
		return WeatherData{}, err
	}

	// Store in cache
	weatherCache.Set("weather", data, cache.DefaultExpiration)

	return data, nil
}

func fetchWeatherFromAPI(ctx context.Context) (WeatherData, error) {
	apiRequestsTotal.WithLabelValues("weather").Inc()
	if strings.TrimSpace(weatherAPIURL) == "" {
		return WeatherData{}, fmt.Errorf("weather API URL is not configured")
	}

	req, err := http.NewRequest(http.MethodGet, weatherAPIURL, nil)
	if err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "build weather request", Err: err}
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "GET weather data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "GET weather data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var data WeatherData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "decode weather data", Err: err}
	}

	roundWeatherData(&data)
	formatWeatherTimes(&data)

	return data, nil
}

func formatLaunchTime(timestamp string) (string, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", fmt.Errorf("failed to parse time: %w", err)
	}

	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load location: %w", err)
	}

	etTime := parsedTime.In(etLocation)
	return etTime.Format("3:04pm"), nil
}

func buildTodayKennedyLaunchURL(now time.Time) (string, error) {
	baseURL, err := url.Parse(spacedevsAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load location: %w", err)
	}

	etNow := now.In(etLocation)
	startOfDay := time.Date(etNow.Year(), etNow.Month(), etNow.Day(), 0, 0, 0, 0, etLocation)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := baseURL.Query()
	query.Set("limit", "5")
	query.Set("ordering", "net")
	query.Set("net__gte", startOfDay.UTC().Format(time.RFC3339))
	query.Set("net__lt", endOfDay.UTC().Format(time.RFC3339))

	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

func getTodayKennedyLaunch(ctx context.Context) (*LaunchInfo, error) {
	cacheKey := todayLaunchCacheKey(time.Now())
	if cachedData, found := launchCache.Get(cacheKey); found {
		return cachedData.(launchCacheEntry).Launch, nil
	}

	launch, err := fetchTodayKennedyLaunch(ctx)
	if err != nil {
		return nil, err
	}
	launchCache.Set(cacheKey, launchCacheEntry{Launch: launch}, cache.DefaultExpiration)

	return launch, nil
}

func todayLaunchCacheKey(now time.Time) string {
	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return now.UTC().Format("2006-01-02")
	}
	return now.In(etLocation).Format("2006-01-02")
}

func fetchTodayKennedyLaunch(ctx context.Context) (*LaunchInfo, error) {
	apiRequestsTotal.WithLabelValues("launches").Inc()

	apiURL, err := buildTodayKennedyLaunchURL(time.Now())
	if err != nil {
		return nil, fmt.Errorf("error building API URL: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, &APIError{URL: apiURL, Operation: "build launch request", Err: err}
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := launchHTTPClient.Do(req)
	if err != nil {
		return nil, &APIError{URL: apiURL, Operation: "GET launch data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{URL: apiURL, Operation: "GET launch data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var data struct {
		Results []LaunchData `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, &APIError{URL: apiURL, Operation: "decode launch data", Err: err}
	}

	for _, launch := range data.Results {
		if !isKennedyLaunch(launch) || launch.WindowStart == "" {
			continue
		}

		formatted, err := formatLaunchTime(launch.WindowStart)
		if err != nil {
			log.Printf("Failed to format window_start for launch: %s", launch.Name)
			continue
		}

		return &LaunchInfo{
			Name:      launch.Name,
			Scheduled: formatted,
		}, nil
	}

	return nil, nil
}

func isKennedyLaunch(launch LaunchData) bool {
	locationName := strings.ToLower(launch.Pad.Location.Name)
	padName := strings.ToLower(launch.Pad.Name)

	if locationName == "" && padName == "" {
		return false
	}

	if strings.Contains(locationName, "kennedy space center") {
		return true
	}

	return strings.Contains(padName, "kennedy space center")
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

func (e *APIError) Error() string {
	return fmt.Sprintf("error during %s: %v (url: %s)", e.Operation, e.Err, e.URL)
}

func getWeather(ctx context.Context) (WeatherData, error) {
	// Deprecated: use getWeatherWithCache/fetchWeatherFromAPI
	return fetchWeatherFromAPI(ctx)
}

func roundWeatherData(data *WeatherData) {
	// Round current weather data
	data.Current.Temp = math.Round(data.Current.Temp)
	data.Current.FeelsLike = math.Round(data.Current.FeelsLike)
	data.Current.WindSpeed = math.Round(data.Current.WindSpeed)
	data.Current.DewPoint = math.Round(data.Current.DewPoint)

	// Round hourly data
	for i := range data.Hourly {
		data.Hourly[i].Temp = math.Round(data.Hourly[i].Temp)
		data.Hourly[i].FeelsLike = math.Round(data.Hourly[i].FeelsLike)
		data.Hourly[i].Pressure = int(math.Round(float64(data.Hourly[i].Pressure)))
		data.Hourly[i].Humidity = int(math.Round(float64(data.Hourly[i].Humidity)))
		data.Hourly[i].DewPoint = math.Round(data.Hourly[i].DewPoint)
		data.Hourly[i].WindSpeed = math.Round(data.Hourly[i].WindSpeed)
		data.Hourly[i].WindGust = math.Round(data.Hourly[i].WindGust)
		data.Hourly[i].Pop = math.Round(data.Hourly[i].Pop * 100) // Convert probability to percentage and round
		data.Hourly[i].Rain.OneH = math.Round(data.Hourly[i].Rain.OneH)
	}
}

func (w *WeatherData) convertTime(unixTime int64) string {
	loc, err := time.LoadLocation(w.Timezone)
	if err != nil {
		// Fallback to using the offset if loading the location fails
		loc = time.FixedZone(w.Timezone, w.TimezoneOffset)
	}
	return time.Unix(unixTime, 0).In(loc).Format("3:04 PM")
}

func formatWeatherTimes(data *WeatherData) {
	data.Current.SunriseFormatted = data.convertTime(data.Current.Sunrise)
	data.Current.SunsetFormatted = data.convertTime(data.Current.Sunset)

	for i := range data.Hourly {
		data.Hourly[i].DtFormatted = data.convertTime(data.Hourly[i].Dt)
	}
}

func getForecastHours(hourly []HourlyWeather) []HourlyWeather {
	var result []HourlyWeather
	now := time.Now()
	targetHours := []int{2, 4, 6, 8}

	for _, targetHour := range targetHours {
		targetTime := now.Add(time.Duration(targetHour) * time.Hour)
		var closestHour HourlyWeather
		smallestDiff := time.Duration(math.MaxInt64)

		for _, h := range hourly {
			forecastTime := time.Unix(h.Dt, 0)
			diff := forecastTime.Sub(targetTime).Abs()
			if diff < smallestDiff {
				smallestDiff = diff
				closestHour = h
			}
		}

		if closestHour.Dt != 0 { // Check if a valid hour was found
			result = append(result, closestHour)
		}
	}
	return result
}

func getTide(ctx context.Context) (TideData, error) {
	cacheKey := time.Now().Format("2006-01-02")
	if cachedData, found := tideCache.Get(cacheKey); found {
		return cachedData.(TideData), nil
	}

	tide, err := fetchTideFromAPI(ctx)
	if err != nil {
		return TideData{}, err
	}
	tideCache.Set(cacheKey, tide, cache.DefaultExpiration)

	return tide, nil
}

func fetchTideFromAPI(ctx context.Context) (TideData, error) {
	apiRequestsTotal.WithLabelValues("tide").Inc()

	// Add height to URL parameters
	noaaURLWithHeight := noaaAPIURL + "&datatype=hilo&datum=MLLW"

	req, err := http.NewRequest(http.MethodGet, noaaURLWithHeight, nil)
	if err != nil {
		return TideData{}, &APIError{URL: noaaURLWithHeight, Operation: "build tide request", Err: err}
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TideData{}, &APIError{URL: noaaURLWithHeight, Operation: "GET tide data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TideData{}, &APIError{URL: noaaURLWithHeight, Operation: "GET tide data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var rawData struct {
		Predictions []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		} `json:"predictions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return TideData{}, &APIError{URL: noaaURLWithHeight, Operation: "decode tide data", Err: err}
	}

	tideData, err := processTideData(rawData)
	if err != nil {
		return TideData{}, err
	}

	return tideData, nil
}

func processTideData(rawData struct {
	Predictions []struct {
		Time   string `json:"t"`
		Type   string `json:"type"`
		Height string `json:"v"`
	} `json:"predictions"`
}) (TideData, error) {
	var tideData TideData
	if len(rawData.Predictions) == 0 {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "process tide data", Err: fmt.Errorf("no predictions found")}
	}

	for _, p := range rawData.Predictions {
		if p.Type != "H" && p.Type != "L" {
			return TideData{}, &APIError{URL: noaaAPIURL, Operation: "process tide data", Err: fmt.Errorf("invalid tide type: %s", p.Type)}
		}

		itemTime, err := time.Parse("2006-01-02 15:04", p.Time)
		if err != nil {
			return TideData{}, &APIError{URL: noaaAPIURL, Operation: "parse tide time", Err: err}
		}

		height, err := strconv.ParseFloat(p.Height, 64)
		if err != nil {
			return TideData{}, &APIError{URL: noaaAPIURL, Operation: "parse tide height", Err: err}
		}
		tideData.Predictions = append(tideData.Predictions, TidePrediction{
			Time:   itemTime.Format("3:04 PM"),
			Type:   p.Type,
			Height: height,
		})
	}

	return tideData, nil
}

func getIconClassName(icon string, id int) string {
	isNight := string(icon[len(icon)-1]) == "n"
	if isNight && id == 800 {
		return "wi wi-night-clear"
	}
	if isNight {
		return fmt.Sprintf("wi wi-owm-night-%d", id)
	}
	return fmt.Sprintf("wi wi-owm-day-%d", id)
}

func getMoonPhaseIcon(moonPhase float64) string {
	switch {
	case moonPhase == 0:
		return "wi-moon-new"
	case moonPhase < 0.25:
		return "wi-moon-waxing-crescent-3"
	case moonPhase == 0.25:
		return "wi-moon-first-quarter"
	case moonPhase < 0.5:
		return "wi-moon-waxing-gibbous-3"
	case moonPhase == 0.5:
		return "wi-moon-full"
	case moonPhase < 0.75:
		return "wi-moon-waning-gibbous-3"
	case moonPhase == 0.75:
		return "wi-moon-third-quarter"
	default:
		return "wi-moon-waning-crescent-3"
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	weather, err := getWeatherWithCache(ctx)
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error getting weather data: %v", err),
		})
		http.Error(w, "Could not get weather data", http.StatusInternalServerError)
		return
	}

	tide, err := getTide(ctx)
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error getting tide data: %v", err),
		})
		http.Error(w, "Could not get tide data", http.StatusInternalServerError)
		return
	}

	kennedyLaunch, err := getTodayKennedyLaunch(ctx)
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error getting launch data: %v", err),
		})
	}
	if enableRocketPreview && r.URL.Query().Has("rocketPreview") {
		kennedyLaunch = &LaunchInfo{Scheduled: "4:30pm"}
	}

	forecastHours := getForecastHours(weather.Hourly)
	moonPhaseIcon := getMoonPhaseIcon(weather.Daily[0].MoonPhase)

	// Generate SVG from tide data
	tideSVG, err := generateTideSVG(tide.Predictions)
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error generating tide SVG: %v", err),
		})
		http.Error(w, "Could not generate tide chart", http.StatusInternalServerError)
		return
	}

	horizontal := r.URL.Query().Has("h")
	refreshURL := buildAutoRefreshURL(r, time.Now().Unix())
	autoRefreshSeconds := int(autoRefresh.Seconds())

	data := struct {
		Weather            WeatherData
		Tide               TideData
		TideSVG            template.HTML
		ForecastHours      []HourlyWeather
		MoonPhaseIcon      string
		Horizontal         bool
		KennedyLaunch      *LaunchInfo
		AutoRefreshSeconds int
		AutoRefreshURL     string
	}{
		Weather:            weather,
		Tide:               tide,
		TideSVG:            tideSVG,
		ForecastHours:      forecastHours,
		MoonPhaseIcon:      moonPhaseIcon,
		Horizontal:         horizontal,
		KennedyLaunch:      kennedyLaunch,
		AutoRefreshSeconds: autoRefreshSeconds,
		AutoRefreshURL:     refreshURL,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not render template: %v", err), http.StatusInternalServerError)
	}
}

func generateTideSVG(predictions []TidePrediction) (template.HTML, error) {
	if len(predictions) == 0 {
		return "", fmt.Errorf("no tide predictions to render")
	}
	const svgTemplate = `
    <svg width="600" height="95" viewBox="0 0 600 95">
        <line x1="35" y1="44" x2="565" y2="44" stroke="black" stroke-width="1" />
        <path
            fill="none" 
            stroke="black" 
            stroke-width="2"
            d="{{.Path}}"
        />
        {{range .Points}}
        <circle cx="{{.X}}" cy="{{.Y}}" r="4" fill="black" />
        {{end}}
        {{range .Labels}}
        <text x="{{.X}}" y="{{.Y}}" font-size="13" text-anchor="middle" font-weight="bold">{{.Type}}</text>
        <text x="{{.X}}" y="{{.TimeY}}" font-size="12" text-anchor="middle">{{.Time}}</text>
        {{end}}
    </svg>`

	type Point struct {
		X, Y float64
	}

	type Label struct {
		X, Y, TimeY float64
		Type        string
		Time        string
	}

	var points []Point
	var labels []Label
	buildPath := func(points []Point) string {
		if len(points) == 0 {
			return ""
		}
		if len(points) == 1 {
			return fmt.Sprintf("M %.1f %.1f", points[0].X, points[0].Y)
		}

		var path strings.Builder
		path.WriteString(fmt.Sprintf("M %.1f %.1f", points[0].X, points[0].Y))
		for i := 1; i < len(points); i++ {
			prev := points[i-1]
			curr := points[i]
			midX := prev.X + ((curr.X - prev.X) / 2)
			path.WriteString(fmt.Sprintf(" C %.1f %.1f, %.1f %.1f, %.1f %.1f", midX, prev.Y, midX, curr.Y, curr.X, curr.Y))
		}
		return path.String()
	}

	// Find min/max heights for scaling
	minHeight := predictions[0].Height
	maxHeight := predictions[0].Height
	for _, p := range predictions {
		if p.Height < minHeight {
			minHeight = p.Height
		}
		if p.Height > maxHeight {
			maxHeight = p.Height
		}
	}

	// Generate points and labels
	for i, p := range predictions {
		denom := float64(len(predictions) - 1)
		x := 300.0
		if denom > 0 {
			x = 35 + float64(i)*(530.0/denom)
		}
		scale := 1.0
		if maxHeight != minHeight {
			scale = (p.Height - minHeight) / (maxHeight - minHeight)
		}
		y := 58 - (scale * 28)

		points = append(points, Point{X: x, Y: y})
		labels = append(labels, Label{
			X:     x,
			Y:     78,
			TimeY: 91,
			Type:  p.Type,
			Time:  p.Time,
		})
	}

	// Render SVG
	tmpl := template.Must(template.New("svg").Parse(svgTemplate))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, struct {
		Points []Point
		Labels []Label
		Path   string
	}{
		Points: points,
		Labels: labels,
		Path:   buildPath(points),
	})
	if err != nil {
		return "", fmt.Errorf("error rendering SVG: %w", err)
	}

	return template.HTML(buf.String()), nil
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
	mux.Handle("/", loggingMiddleware(otelhttp.NewHandler(http.HandlerFunc(handler), "GET /")))
	mux.Handle("/css/", loggingMiddleware(otelhttp.NewHandler(http.StripPrefix("/css/", http.FileServer(http.Dir("css"))), "GET /css")))
	mux.Handle("/font/", loggingMiddleware(otelhttp.NewHandler(http.StripPrefix("/font/", http.FileServer(http.Dir("font"))), "GET /font")))
	mux.Handle("/metrics", otelhttp.NewHandler(promhttp.Handler(), "GET /metrics"))
	mux.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(healthHandler), "GET /health"))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server in background
	go func() {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "INFO",
			Message:   "Server started at http://localhost:8080",
		})
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logJSON(logEntry{
				Timestamp: time.Now().Format(time.RFC3339),
				Level:     "FATAL",
				Message:   fmt.Sprintf("Server failed to start: %v", err),
			})
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop
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
