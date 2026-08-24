package main

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func validWeather(now time.Time) WeatherData {
	condition := []WeatherCondition{{ID: 800, Description: "clear sky", Icon: "01d"}}
	return WeatherData{
		Timezone: "America/New_York",
		Current: CurrentWeather{
			Dt: now.Unix(), Sunrise: now.Add(-time.Hour).Unix(), Sunset: now.Add(8 * time.Hour).Unix(),
			Temp: 82, FeelsLike: 88, Uvi: 8, WindGust: 12, Weather: condition,
		},
		Hourly: []HourlyWeather{
			{Dt: now.Add(time.Hour).Unix(), Temp: 83, Pop: 65, Weather: condition},
			{Dt: now.Add(2 * time.Hour).Unix(), Temp: 84, Weather: condition},
			{Dt: now.Add(4 * time.Hour).Unix(), Temp: 85, Weather: condition},
			{Dt: now.Add(6 * time.Hour).Unix(), Temp: 82, Weather: condition},
			{Dt: now.Add(8 * time.Hour).Unix(), Temp: 78, Weather: condition},
		},
		Daily: []DailyWeather{
			{Summary: "Sunny", MoonPhase: .5, Uvi: 8},
			{Summary: "Clouds late", Temp: DailyTemperature{Min: 69, Max: 84}, Pop: .35},
		},
	}
}

func TestRedactURL(t *testing.T) {
	got := redactURL("https://example.com/weather?lat=29.65&appid=secret-value&token=also-secret")
	if strings.Contains(got, "secret") {
		t.Fatalf("redactURL() exposed a secret: %s", got)
	}
	if !strings.Contains(got, "lat=29.65") || strings.Count(got, "REDACTED") != 2 {
		t.Fatalf("redactURL() = %q", got)
	}
}

func TestValidateWeatherDataRejectsIncompletePayloads(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		data WeatherData
	}{
		{name: "empty payload"},
		{name: "missing current condition", data: WeatherData{Current: CurrentWeather{Dt: now.Unix()}, Hourly: []HourlyWeather{{Dt: now.Unix()}}, Daily: []DailyWeather{{}}}},
		{name: "incomplete hourly condition", data: WeatherData{Current: CurrentWeather{Dt: now.Unix(), Weather: []WeatherCondition{{Icon: "01d"}}}, Hourly: []HourlyWeather{{Dt: now.Unix()}}, Daily: []DailyWeather{{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateWeatherData(tt.data); err == nil {
				t.Fatal("validateWeatherData() expected an error")
			}
		})
	}
	if err := validateWeatherData(validWeather(now)); err != nil {
		t.Fatalf("valid weather rejected: %v", err)
	}
}

func TestBuildActionNotices(t *testing.T) {
	now := time.Now()
	weather := validWeather(now)
	for i := range weather.Hourly {
		weather.Hourly[i].DtFormatted = time.Unix(weather.Hourly[i].Dt, 0).Format("3:04 PM")
	}
	notices := buildActionNotices(weather, now)
	if len(notices) != 2 || !strings.Contains(notices[0].Text, "Rain likely") || !strings.Contains(notices[1].Text, "Very high UV") {
		t.Fatalf("buildActionNotices() = %+v", notices)
	}
}

func TestBestSurfWindowReportsExactWindow(t *testing.T) {
	loc := easternLocation()
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, loc)
	weather := validWeather(now)
	weather.Current.Sunrise = now.Add(-2 * time.Hour).Unix()
	weather.Current.Sunset = now.Add(10 * time.Hour).Unix()
	weather.Hourly = []HourlyWeather{
		{Dt: now.Add(time.Hour).Unix(), WindSpeed: 4, WindDeg: 90},
		{Dt: now.Add(2 * time.Hour).Unix(), WindSpeed: 8, WindDeg: 270},
	}
	forecast := SurfForecast{Hourly: SurfHourlyForecast{
		Time:          []int64{now.Add(time.Hour).Unix(), now.Add(2 * time.Hour).Unix()},
		WaveHeight:    []float64{2.0, 3.0},
		WaveDirection: []float64{90, 90},
		WavePeriod:    []float64{8, 10},
	}}
	window := bestSurfWindow(forecast, weather, now)
	if window == nil || !window.Start.Equal(now.Add(time.Hour)) || !window.End.Equal(now.Add(3*time.Hour)) || window.WavePeriod != 10 {
		t.Fatalf("bestSurfWindow() = %+v", window)
	}
	status := getBeachStatusWithWindow(nil, window, now)
	if status == nil || status.Text != "Best surf 9 AM–11 AM · 3.0 ft @ 10s" {
		t.Fatalf("surf beach status = %+v", status)
	}
}

func TestAdaptiveRefreshInterval(t *testing.T) {
	oldRefresh := autoRefresh
	autoRefresh = 30 * time.Minute
	defer func() { autoRefresh = oldRefresh }()

	loc := easternLocation()
	now := time.Date(2026, time.August, 23, 9, 0, 0, 0, loc)
	weather := validWeather(now)
	weather.Hourly[0].Pop = 70
	snapshot := DashboardSnapshot{Weather: WeatherSnapshot{Data: weather}, Launch: LaunchSnapshot{Data: &LaunchInfo{Start: now.Add(time.Hour)}}}
	if got := adaptiveRefreshInterval(snapshot, now); got != 5*time.Minute {
		t.Fatalf("adaptive refresh near launch = %v; want 5m", got)
	}

	night := time.Date(2026, time.August, 23, 23, 0, 0, 0, loc)
	snapshot.Launch.Data = nil
	snapshot.Weather.Data.Hourly = nil
	if got := adaptiveRefreshInterval(snapshot, night); got != 90*time.Minute {
		t.Fatalf("adaptive overnight refresh = %v; want 90m", got)
	}
}

func TestWeatherCacheCoalescesConcurrentRefreshes(t *testing.T) {
	oldURL, oldClient, oldCache := weatherAPIURL, weatherHTTPClient, weatherCache
	defer func() {
		weatherAPIURL, weatherHTTPClient, weatherCache = oldURL, oldClient, oldCache
	}()

	now := time.Now()
	payload := `{"timezone":"America/New_York","current":{"dt":` + fmtInt(now.Unix()) + `,"sunrise":` + fmtInt(now.Add(-time.Hour).Unix()) + `,"sunset":` + fmtInt(now.Add(8*time.Hour).Unix()) + `,"temp":72,"weather":[{"id":800,"description":"clear","icon":"01d"}]},"hourly":[{"dt":` + fmtInt(now.Add(time.Hour).Unix()) + `,"temp":73,"weather":[{"id":800,"description":"clear","icon":"01d"}]}],"daily":[{"summary":"Clear","moon_phase":0.5}]}`
	var calls atomic.Int32
	weatherAPIURL = "https://example.com/weather?appid=secret"
	weatherCache = cache.New(time.Hour, time.Hour)
	weatherHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
	})}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := getWeatherSnapshot(context.Background(), now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent weather refresh: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("weather API calls = %d; want 1", calls.Load())
	}
}

func TestTideCacheRejectsPreviousDayFallback(t *testing.T) {
	oldURL, oldClient, oldCache := noaaAPIURL, httpClient, tideCache
	defer func() { noaaAPIURL, httpClient, tideCache = oldURL, oldClient, oldCache }()

	loc := easternLocation()
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, loc)
	tideCache = cache.New(time.Minute, time.Minute)
	tideCache.Set(tideCacheKeyLatest, tideCacheEntry{
		Tide:      TideData{Predictions: []TidePrediction{{Time: "1:00 PM", Type: "L", Height: -0.2}}},
		FetchedAt: now.Add(-24 * time.Hour),
	}, cache.NoExpiration)
	noaaAPIURL = "https://example.com/tides"
	httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := getTideSnapshot(ctx, now); err == nil {
		t.Fatal("getTideSnapshot() used a previous day's fallback")
	}
}

func TestDashboardSVGTemplateRenders(t *testing.T) {
	view := DashboardView{
		CurrentTemperature: "72°", Summary: "Clear",
		TideSVG: template.HTML(`<svg viewBox="0 0 600 95"></svg>`),
	}
	var output bytes.Buffer
	if err := svgTmpl.Execute(&output, view); err != nil {
		t.Fatalf("SVG template: %v", err)
	}
	if !strings.Contains(output.String(), `viewBox="0 0 758 1024"`) || !strings.Contains(output.String(), "72°") {
		t.Fatalf("unexpected SVG: %s", output.String())
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d; want 404", recorder.Code)
	}
}

func fmtInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
