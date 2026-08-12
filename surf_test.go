package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsGoodSurfToday(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.August, 11, 8, 0, 0, 0, loc)
	goodHour := now.Add(2 * time.Hour)

	weather := WeatherData{
		Timezone: "America/New_York",
		Current: CurrentWeather{
			Dt:      now.Unix(),
			Sunrise: time.Date(2026, time.August, 11, 6, 45, 0, 0, loc).Unix(),
			Sunset:  time.Date(2026, time.August, 11, 20, 5, 0, 0, loc).Unix(),
		},
		Hourly: []HourlyWeather{{
			Dt:        goodHour.Unix(),
			WindSpeed: 8,
			WindDeg:   270,
		}},
	}

	tests := []struct {
		name      string
		height    float64
		period    float64
		direction float64
		windSpeed float64
		windDeg   int
		want      bool
	}{
		{name: "good swell and offshore wind", height: 2.5, period: 9, direction: 90, windSpeed: 8, windDeg: 270, want: true},
		{name: "small waves", height: 1.0, period: 9, direction: 90, windSpeed: 8, windDeg: 270},
		{name: "short period", height: 2.5, period: 5, direction: 90, windSpeed: 8, windDeg: 270},
		{name: "swell misses beach", height: 2.5, period: 9, direction: 190, windSpeed: 8, windDeg: 270},
		{name: "moderate onshore wind", height: 2.5, period: 9, direction: 90, windSpeed: 8, windDeg: 90},
		{name: "light onshore wind", height: 2.5, period: 9, direction: 90, windSpeed: 4, windDeg: 90, want: true},
		{name: "strong offshore wind", height: 2.5, period: 9, direction: 90, windSpeed: 15, windDeg: 270},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weather.Hourly[0].WindSpeed = tt.windSpeed
			weather.Hourly[0].WindDeg = tt.windDeg
			forecast := SurfForecast{Hourly: SurfHourlyForecast{
				Time:          []int64{goodHour.Unix()},
				WaveHeight:    []float64{tt.height},
				WavePeriod:    []float64{tt.period},
				WaveDirection: []float64{tt.direction},
			}}

			if got := isGoodSurfToday(forecast, weather, now); got != tt.want {
				t.Fatalf("isGoodSurfToday() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestIsGoodSurfToday_IgnoresTomorrowAndPastDaylight(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.August, 11, 21, 0, 0, 0, loc)
	tomorrow := now.Add(12 * time.Hour)
	weather := WeatherData{
		Timezone: "America/New_York",
		Current: CurrentWeather{
			Sunrise: time.Date(2026, time.August, 11, 6, 45, 0, 0, loc).Unix(),
			Sunset:  time.Date(2026, time.August, 11, 20, 5, 0, 0, loc).Unix(),
		},
		Hourly: []HourlyWeather{{Dt: tomorrow.Unix(), WindSpeed: 4, WindDeg: 90}},
	}
	forecast := SurfForecast{Hourly: SurfHourlyForecast{
		Time:          []int64{tomorrow.Unix()},
		WaveHeight:    []float64{3},
		WavePeriod:    []float64{10},
		WaveDirection: []float64{90},
	}}

	if isGoodSurfToday(forecast, weather, now) {
		t.Fatal("isGoodSurfToday() = true after today's daylight window")
	}
}

func TestFetchSurfForecast(t *testing.T) {
	oldURL := surfAPIURL
	oldHTTPClient := httpClient
	defer func() {
		surfAPIURL = oldURL
		httpClient = oldHTTPClient
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hourly":{"time":[1786464000],"wave_height":[2.5],"wave_direction":[90],"wave_period":[9]}}`))
	}))
	defer server.Close()

	surfAPIURL = server.URL
	httpClient = server.Client()

	forecast, err := fetchSurfForecast(context.Background())
	if err != nil {
		t.Fatalf("fetchSurfForecast() error = %v", err)
	}
	if got := forecast.Hourly.WaveHeight[0]; got != 2.5 {
		t.Fatalf("wave height = %v; want 2.5", got)
	}
}

func TestFetchSurfForecast_RejectsIncompleteData(t *testing.T) {
	oldURL := surfAPIURL
	oldHTTPClient := httpClient
	defer func() {
		surfAPIURL = oldURL
		httpClient = oldHTTPClient
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hourly":{"time":[1786464000],"wave_height":[],"wave_direction":[90],"wave_period":[9]}}`))
	}))
	defer server.Close()

	surfAPIURL = server.URL
	httpClient = server.Client()

	if _, err := fetchSurfForecast(context.Background()); err == nil {
		t.Fatal("fetchSurfForecast() expected validation error")
	}
}
