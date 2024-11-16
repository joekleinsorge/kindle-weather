package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
)

const (
	secretMountPath       = "/etc/secrets"
	weatherAPIURLTemplate = "https://api.openweathermap.org/data/3.0/onecall?lat=29.65&lon=-81.20&exclude=minutely&appid=%s&units=imperial"
	noaaAPIURLTemplate    = "https://api.tidesandcurrents.noaa.gov/api/prod/datagetter?product=predictions&application=NOS.COOPS.TAC.WL&datum=MLLW&station=8720218&time_zone=lst_ldt&units=english&interval=hilo&format=json&date=today"
	spacedevsAPIURL       = "https://ll.thespacedevs.com/2.3.0/launches/upcoming/?location__ids=27&format=json"
)

var (
	weatherAPIURL string
	noaaAPIURL    string
	weatherCache  *cache.Cache
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
	Time string `json:"t"`
	Type string `json:"type"`
}

type APIError struct {
	URL       string
	Operation string
	Err       error
}

type LaunchData struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
  Name        string `json:"name"`
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
	// Initialize API URLs
	openWeatherAPIKey, err := readSecret("openweather-api-key")
	if err != nil {
		log.Fatalf("Failed to read OpenWeather API key: %v", err)
	}
	weatherAPIURL = fmt.Sprintf(weatherAPIURLTemplate, openWeatherAPIKey)
	noaaAPIURL = noaaAPIURLTemplate

	// Initialize cache
	cacheExpiration, _ := strconv.Atoi(os.Getenv("CACHE_EXPIRATION"))
	if cacheExpiration == 0 {
		cacheExpiration = 1800 // default to 30 minutes
	}

	cacheCleanupInterval, _ := strconv.Atoi(os.Getenv("CACHE_CLEANUP_INTERVAL"))
	if cacheCleanupInterval == 0 {
		cacheCleanupInterval = 3600 // default to 1 hour
	}

	weatherCache = cache.New(time.Duration(cacheExpiration)*time.Second, time.Duration(cacheCleanupInterval)*time.Second)
}

func readSecret(secretName string) (string, error) {
	secretPath := filepath.Join(secretMountPath, secretName)
	secretValue, err := ioutil.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %v", err)
	}
	return string(secretValue), nil
}

func getWeatherWithCache() (WeatherData, error) {
	// Check if weather data is in cache
	if cachedData, found := weatherCache.Get("weather"); found {
		return cachedData.(WeatherData), nil
	}

	// If not in cache, fetch from API
	data, err := fetchWeatherFromAPI()
	if err != nil {
		return WeatherData{}, err
	}

	// Store in cache
	weatherCache.Set("weather", data, cache.DefaultExpiration)

	return data, nil
}

func fetchWeatherFromAPI() (WeatherData, error) {
	resp, err := http.Get(weatherAPIURL)
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

func ConvertToETTime(timestamp string) (string, error) {
	// Parse the timestamp into a time.Time object
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", fmt.Errorf("failed to parse time: %w", err)
	}

	// Load the Eastern Time location
	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load location: %w", err)
	}

	// Convert the time to Eastern Time
	etTime := parsedTime.In(etLocation)

	// Format the time as 4:04pm
	return etTime.Format("3:04pm"), nil
}

func buildTodayLaunchURL() (string, error) {
	baseURL, err := url.Parse(spacedevsAPIBaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	query := url.Values{}

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour).Add(-time.Second)

	query.Set("net__gte", start.Format(time.RFC3339))
	query.Set("net__lt", end.Format(time.RFC3339))

	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

func getUpcomingLaunches() ([]LaunchData, error) {
	apiURL, err := buildTodayLaunchURL()
	if err != nil {
		return nil, fmt.Errorf("error building API URL: %w", err)
	}

	resp, err := http.Get(apiURL)
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

	// Convert timestamps to ET
	for i, launch := range data.Results {
		if launch.WindowStart != "" {
			data.Results[i].WindowStart, err = ConvertToETTime(launch.WindowStart)
			if err != nil {
				log.Printf("Failed to convert window_start for launch: %s", launch.Name)
			}
		}

		if launch.WindowEnd != "" {
			data.Results[i].WindowEnd, err = ConvertToETTime(launch.WindowEnd)
			if err != nil {
				log.Printf("Failed to convert window_end for launch: %s", launch.Name)
			}
		}
	}

	return data.Results, nil
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

func (e *APIError) Error() string {
	return fmt.Sprintf("error during %s: %v (url: %s)", e.Operation, e.Err, e.URL)
}

func getWeather() (WeatherData, error) {
	resp, err := http.Get(weatherAPIURL)
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

func getTide() (TideData, error) {
	resp, err := http.Get(noaaAPIURL)
	if err != nil {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "GET tide data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "GET tide data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var rawData struct {
		Predictions []struct {
			Time string `json:"t"`
			Type string `json:"type"`
		} `json:"predictions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "decode tide data", Err: err}
	}

	tideData, err := processTideData(rawData)
	if err != nil {
		return TideData{}, err
	}

	return tideData, nil
}

func processTideData(rawData struct {
	Predictions []struct {
		Time string `json:"t"`
		Type string `json:"type"`
	} `json:"predictions"`
}) (TideData, error) {
	var tideData TideData
	for _, p := range rawData.Predictions {
		itemTime, err := time.Parse("2006-01-02 15:04", p.Time)
		if err != nil {
			return TideData{}, &APIError{URL: noaaAPIURL, Operation: "parse tide time", Err: err}
		}
		tideData.Predictions = append(tideData.Predictions, TidePrediction{
			Time: itemTime.Format("3:04 PM"),
			Type: p.Type,
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
	weather, err := getWeatherWithCache()
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error getting weather data: %v", err),
		})
		http.Error(w, "Could not get weather data", http.StatusInternalServerError)
		return
	}

	tide, err := getTide()
	if err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Error getting tide data: %v", err),
		})
		http.Error(w, "Could not get tide data", http.StatusInternalServerError)
		return
	}

	forecastHours := getForecastHours(weather.Hourly)
	moonPhaseIcon := getMoonPhaseIcon(weather.Daily[0].MoonPhase)

	tmpl := template.Must(template.New("index").Funcs(template.FuncMap{
		"getIconClassName": getIconClassName,
		"add": func(a, b int) int {
			return a + b
		},
	}).Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Weather & Tide</title>
    <meta http-equiv="Content-Type" content="text/html;charset=utf-8">
    <meta name="viewport"
          content="width=758, initial-scale=1, maximum-scale=1, user-scalable=no">
    <link rel="stylesheet" href="/css/kindle.css">
    <link rel="stylesheet" href="/css/weather-icons.min.css">
    <link rel="stylesheet" href="/css/weather-icons-wind.min.css">
    <link rel="icon" href="data:,">
</head>
<body>
    <div id="page">
        <!-- Current Weather Icon -->
        <div id="iconWrapper">
            <i id="icon" class="{{ getIconClassName (index .Weather.Current.Weather 0).Icon (index .Weather.Current.Weather 0).ID }}"></i>
        </div>
        
        <!-- Current Temperature -->
        <div class="tempWrapper">
            <div id="temp">{{ .Weather.Current.Temp }}</div>
        </div>
        
        <!-- Weather Description -->
        <div id="description">
            <p>{{ (index .Weather.Daily 0).Summary }}</p>
        </div>

        <!-- Upcoming Launches -->
        <div id="launches">
          <p>{{ (index .Launches 0).Name }}</p>
          <p>{{ (index .Launches 0).WindowStart }}</p>
        </div>
        
        
        <!-- Hourly Forecast -->
        <div class="forecast">
            {{ range $index, $hour := .ForecastHours }}
            <div class="col">
                <div class="colTime">{{ $hour.DtFormatted }}</div>
                <div class="forecastIconWrapper">
                    <i class="colIcon {{ getIconClassName (index $hour.Weather 0).Icon (index $hour.Weather 0).ID }}"></i>
                </div>
                <div class="colTemp">{{ $hour.Temp }}</div>
                <div class="colDesc">{{ (index $hour.Weather 0).Description }}</div>
            </div>
            {{ end }}
        </div>
        
        <!-- Tide Data -->
        <div class="tide-section">
            {{ range .Tide.Predictions }}
            <div class="tide-item"> {{ .Type }} at {{ .Time }} </div>
            {{ end }}
        </div>

        <!-- Moonphase Icon -->
        <div id="moon">
            <i class="{{ .MoonPhaseIcon }}"></i>
        </div>

        <!-- Sunrise and Sunset Times -->
        <div id="sun">
            <i class="wi wi-sunrise"></i> {{ .Weather.Current.SunriseFormatted }}
            <i class="wi wi-sunset"></i> {{ .Weather.Current.SunsetFormatted }}
        </div>
    </div>
</body>
</html>
`))

	data := struct {
		Weather       WeatherData
		Tide          TideData
		ForecastHours []HourlyWeather
		MoonPhaseIcon string
    Launches      LaunchData
	}{weather, tide, forecastHours, moonPhaseIcon, launches}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not render template: %v", err), http.StatusInternalServerError)
	}
}

func main() {
	http.Handle("/", loggingMiddleware(http.HandlerFunc(handler)))
	http.Handle("/css/", loggingMiddleware(http.StripPrefix("/css/", http.FileServer(http.Dir("css")))))
	http.Handle("/font/", loggingMiddleware(http.StripPrefix("/font/", http.FileServer(http.Dir("font")))))

	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	logJSON(logEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "INFO",
		Message:   "Server started at http://localhost:8080",
	})
	if err := server.ListenAndServe(); err != nil {
		logJSON(logEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     "FATAL",
			Message:   fmt.Sprintf("Server failed to start: %v", err),
		})
		os.Exit(1)
	}
}
