package main

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGetMoonPhaseIcon(t *testing.T) {
	tests := []struct {
		phase    float64
		expected string
	}{
		{0, "wi-moon-new"},
		{0.2, "wi-moon-waxing-crescent-3"},
		{0.25, "wi-moon-first-quarter"},
		{0.4, "wi-moon-waxing-gibbous-3"},
		{0.5, "wi-moon-full"},
		{0.6, "wi-moon-waning-gibbous-3"},
		{0.75, "wi-moon-third-quarter"},
		{0.8, "wi-moon-waning-crescent-3"},
	}

	for _, tt := range tests {
		result := getMoonPhaseIcon(tt.phase)
		if result != tt.expected {
			t.Errorf("getMoonPhaseIcon(%f) = %s; want %s", tt.phase, result, tt.expected)
		}
	}
}

func TestRoundWeatherData(t *testing.T) {
	data := &WeatherData{
		Current: CurrentWeather{
			Temp:      72.6,
			FeelsLike: 73.4,
		},
		Hourly: []HourlyWeather{
			{
				Temp:      75.7,
				FeelsLike: 76.3,
				Pop:       0.456,
			},
		},
	}

	roundWeatherData(data)

	if data.Current.Temp != 73 {
		t.Errorf("Current.Temp = %f; want 73", data.Current.Temp)
	}
	if data.Current.FeelsLike != 73 {
		t.Errorf("Current.FeelsLike = %f; want 73", data.Current.FeelsLike)
	}
	if data.Hourly[0].Temp != 76 {
		t.Errorf("Hourly[0].Temp = %f; want 76", data.Hourly[0].Temp)
	}
	if data.Hourly[0].Pop != 46 {
		t.Errorf("Hourly[0].Pop = %f; want 46", data.Hourly[0].Pop)
	}
}

func TestGetForecastHours(t *testing.T) {
	now := time.Now()
	hourly := []HourlyWeather{
		{Dt: now.Add(2 * time.Hour).Unix()},
		{Dt: now.Add(4 * time.Hour).Unix()},
		{Dt: now.Add(6 * time.Hour).Unix()},
		{Dt: now.Add(8 * time.Hour).Unix()},
	}

	result := getForecastHours(hourly)

	if len(result) != 4 {
		t.Errorf("getForecastHours() returned %d items; want 4", len(result))
	}
}

func TestGetIconClassName(t *testing.T) {
	tests := []struct {
		name     string
		icon     string
		id       int
		expected string
	}{
		{"night clear sky", "01n", 800, "wi wi-night-clear"},
		{"night cloudy", "02n", 801, "wi wi-owm-night-801"},
		{"day clear sky", "01d", 800, "wi wi-owm-day-800"},
		{"day rain", "10d", 500, "wi wi-owm-day-500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIconClassName(tt.icon, tt.id)
			if result != tt.expected {
				t.Errorf("getIconClassName(%q, %d) = %q; want %q",
					tt.icon, tt.id, result, tt.expected)
			}
		})
	}
}

func TestProcessTideData(t *testing.T) {
	rawData := struct {
		Predictions []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		} `json:"predictions"`
	}{
		Predictions: []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		}{
			{Time: "2024-01-01 13:45", Type: "H", Height: "4.2"},
			{Time: "2024-01-01 19:30", Type: "L", Height: "0.2"},
		},
	}

	data, err := processTideData(rawData)
	if err != nil {
		t.Fatalf("processTideData() error = %v", err)
	}

	if len(data.Predictions) != 2 {
		t.Errorf("got %d predictions; want 2", len(data.Predictions))
	}

	tests := []struct {
		idx  int
		want string
		typ  string
	}{
		{0, "1:45 PM", "H"},
		{1, "7:30 PM", "L"},
	}

	for _, tt := range tests {
		if data.Predictions[tt.idx].Time != tt.want {
			t.Errorf("Prediction[%d].Time = %q; want %q",
				tt.idx, data.Predictions[tt.idx].Time, tt.want)
		}
		if data.Predictions[tt.idx].Type != tt.typ {
			t.Errorf("Prediction[%d].Type = %q; want %q",
				tt.idx, data.Predictions[tt.idx].Type, tt.typ)
		}
	}
}

func TestProcessTideData_InvalidHeight(t *testing.T) {
	rawData := struct {
		Predictions []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		} `json:"predictions"`
	}{
		Predictions: []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		}{
			{Time: "2024-01-01 13:45", Type: "H", Height: ""},
		},
	}

	_, err := processTideData(rawData)
	if err == nil {
		t.Fatal("processTideData() expected error for invalid height")
	}
}

func TestProcessTideData_SkipsInvalidPredictions(t *testing.T) {
	rawData := struct {
		Predictions []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		} `json:"predictions"`
	}{
		Predictions: []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		}{
			{Time: "2024-01-01 13:45", Type: "H", Height: "4.2"},
			{Time: "bad-time", Type: "L", Height: "0.2"},
			{Time: "2024-01-01 19:30", Type: "X", Height: "0.2"},
		},
	}

	data, err := processTideData(rawData)
	if err != nil {
		t.Fatalf("processTideData() error = %v", err)
	}
	if len(data.Predictions) != 1 {
		t.Fatalf("got %d valid predictions; want 1", len(data.Predictions))
	}
	if data.Predictions[0].Time != "1:45 PM" || data.Predictions[0].Type != "H" {
		t.Fatalf("unexpected prediction: %+v", data.Predictions[0])
	}
}

func TestGenerateTideSVG_CompactLayout(t *testing.T) {
	svg, err := generateTideSVG([]TidePrediction{
		{Time: "3:17 AM", Type: "L", Height: 0.1},
		{Time: "9:24 AM", Type: "H", Height: 4.2},
	})
	if err != nil {
		t.Fatalf("generateTideSVG() error = %v", err)
	}

	rendered := string(svg)
	for _, want := range []string{
		`viewBox="0 0 600 95"`,
		`<path`,
		` C `,
		`x="35"`,
		`x="565"`,
		`>L</text>`,
		`>H</text>`,
		`>3:17 AM</text>`,
		`>9:24 AM</text>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("generated tide SVG missing %q: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, "<polyline") {
		t.Fatalf("generated tide SVG should use a curved path, got: %s", rendered)
	}
}

func TestGenerateTideSVG_NoPredictionsRendersFallback(t *testing.T) {
	svg, err := generateTideSVG(nil)
	if err != nil {
		t.Fatalf("generateTideSVG() error = %v", err)
	}

	if !strings.Contains(string(svg), "Tide data unavailable") {
		t.Fatalf("generated tide fallback missing unavailable text: %s", svg)
	}
}

func TestFetchTideFromAPI_RetriesTransientFailure(t *testing.T) {
	oldNOAAURL := noaaAPIURL
	oldHTTPClient := httpClient
	defer func() {
		noaaAPIURL = oldNOAAURL
		httpClient = oldHTTPClient
	}()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[{"t":"2024-01-01 13:45","type":"H","v":"4.2"}]}`))
	}))
	defer server.Close()

	noaaAPIURL = server.URL + "?station=8720218"
	httpClient = server.Client()

	data, err := fetchTideFromAPI(context.Background())
	if err != nil {
		t.Fatalf("fetchTideFromAPI() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("fetchTideFromAPI() attempts = %d; want 2", attempts)
	}
	if len(data.Predictions) != 1 {
		t.Fatalf("got %d predictions; want 1", len(data.Predictions))
	}
}

func TestFormatLaunchTime(t *testing.T) {
	got, err := formatLaunchTime("2024-04-18T20:30:00Z")
	if err != nil {
		t.Fatalf("formatLaunchTime() unexpected error: %v", err)
	}

	if got != "4:30pm" {
		t.Fatalf("formatLaunchTime() = %q, want %q", got, "4:30pm")
	}

	if _, err := formatLaunchTime("not-a-time"); err == nil {
		t.Fatal("formatLaunchTime() expected error for invalid timestamp")
	}
}

func TestBuildTodayKennedyLaunchURL(t *testing.T) {
	now := time.Date(2024, time.April, 18, 15, 0, 0, 0, time.UTC)
	launchURL, err := buildTodayKennedyLaunchURL(now)
	if err != nil {
		t.Fatalf("buildTodayKennedyLaunchURL() error = %v", err)
	}

	parsedURL, err := url.Parse(launchURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	q := parsedURL.Query()
	if got := q.Get("net__gte"); got != "2024-04-18T04:00:00Z" {
		t.Fatalf("net__gte = %q; want %q", got, "2024-04-18T04:00:00Z")
	}
	if got := q.Get("net__lt"); got != "2024-04-19T04:00:00Z" {
		t.Fatalf("net__lt = %q; want %q", got, "2024-04-19T04:00:00Z")
	}
	if got := q.Get("location__ids"); got != "27" {
		t.Fatalf("location__ids = %q; want %q", got, "27")
	}
}

func TestBuildAutoRefreshURL_PreservesQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/?h&foo=bar", nil)
	refreshURL := buildAutoRefreshURL(req, 12345)

	parsedURL, err := url.Parse(refreshURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsedURL.Path != "/" {
		t.Fatalf("Path = %q; want %q", parsedURL.Path, "/")
	}

	q := parsedURL.Query()
	if got := q.Get("refresh"); got != strconv.FormatInt(12345, 10) {
		t.Fatalf("refresh = %q; want %q", got, strconv.FormatInt(12345, 10))
	}
	if got := q.Get("foo"); got != "bar" {
		t.Fatalf("foo = %q; want %q", got, "bar")
	}
	if _, ok := q["h"]; !ok {
		t.Fatal("expected query parameter h to be preserved")
	}
}

func TestIndexTemplate_NoLaunchHasNoLaunchMarkupAndMoonUsesIconFont(t *testing.T) {
	rendered := renderIndexTemplate(t, nil)

	if strings.Contains(rendered, `id="launches"`) {
		t.Fatalf("expected no launch markup when KennedyLaunch is nil: %s", rendered)
	}
	if strings.Contains(rendered, "No Kennedy") {
		t.Fatalf("expected no launch fallback text: %s", rendered)
	}
	if !strings.Contains(rendered, `class="wi wi-moon-full"`) {
		t.Fatalf("expected moon icon to include weather icon base class: %s", rendered)
	}
}

func TestIndexTemplate_LaunchPreviewRendersIconAndTime(t *testing.T) {
	rendered := renderIndexTemplate(t, &LaunchInfo{Scheduled: "4:30pm"})

	for _, want := range []string{
		`id="launches"`,
		`class="rocket-icon"`,
		`class="launch-time">4:30pm</span>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rendered launch template to contain %q: %s", want, rendered)
		}
	}
}

func renderIndexTemplate(t *testing.T, launch *LaunchInfo) string {
	t.Helper()

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
		Weather: WeatherData{
			Current: CurrentWeather{
				Temp:             72,
				SunriseFormatted: "6:25 AM",
				SunsetFormatted:  "8:17 PM",
				Weather:          []WeatherCondition{{Icon: "01d", ID: 800}},
			},
			Daily: []DailyWeather{{Summary: "Clear skies"}},
		},
		TideSVG:            template.HTML(`<svg></svg>`),
		MoonPhaseIcon:      "wi-moon-full",
		KennedyLaunch:      launch,
		AutoRefreshSeconds: 1800,
		AutoRefreshURL:     "/",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("tmpl.Execute() error = %v", err)
	}
	return buf.String()
}
