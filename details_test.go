package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestForecastDetails(t *testing.T) {
	now := time.Date(2026, 10, 15, 23, 30, 0, 0, easternLocation())
	weather := validWeather(now)
	weather.Hourly = []HourlyWeather{
		{Dt: now.Add(-2 * time.Hour).Unix(), Pop: .1},
		{Dt: now.Add(time.Hour).Unix(), Pop: .65, Rain: Rain{OneH: 2.54}, WindSpeed: 8, WindDeg: 270},
	}
	roundWeatherData(&weather)
	view := DashboardView{}
	forecast := SurfForecast{Hourly: SurfHourlyForecast{
		Time: []int64{now.Add(time.Hour).Unix()}, WaveHeight: []float64{3}, WavePeriod: []float64{9}, WaveDirection: []float64{90},
	}}
	populateDetails(&view, DashboardSnapshot{Weather: WeatherSnapshot{Data: weather}, Surf: SurfSnapshot{Data: forecast}}, now)
	if len(view.RainHours) != 1 || view.RainHours[0].Chance != "65%" || view.RainHours[0].Amount != "0.10 in" {
		t.Fatalf("rain details = %+v", view.RainHours)
	}
	if !strings.Contains(view.RainHours[0].Time, "Fri") {
		t.Fatalf("missing day across midnight: %+v", view.RainHours[0])
	}
	if len(view.SurfHours) != 1 || view.SurfHours[0].Wind != "8 mph W" || view.SurfHours[0].Direction != "E" {
		t.Fatalf("surf details = %+v", view.SurfHours)
	}
	if view.SurfSummary != "No remaining daylight surf window matches your preferences today." {
		t.Fatalf("surf summary = %q", view.SurfSummary)
	}
}

func TestDetailNavigationPreservesOrientationAndRefresh(t *testing.T) {
	r := httptest.NewRequest("GET", "/?h&section=surf", nil)
	links := detailLinks(r)
	if !strings.Contains(links["rain"], "h=") || !strings.HasSuffix(links["rain"], "section=rain#rain") {
		t.Fatalf("rain link = %q", links["rain"])
	}
	if got := buildAutoRefreshURL(r, 123); !strings.HasSuffix(got, "section=surf#surf") {
		t.Fatalf("refresh URL = %q", got)
	}
	r = httptest.NewRequest("GET", "/?section=unknown", nil)
	if strings.Contains(buildAutoRefreshURL(r, 123), "#") {
		t.Fatal("unknown section became a fragment")
	}
}

func TestMissingDetailSources(t *testing.T) {
	view := DashboardView{}
	populateDetails(&view, DashboardSnapshot{}, time.Now())
	if len(view.SurfHours) != 0 || len(view.TideEvents) != 0 || len(view.RainHours) != 0 || view.SurfSummary != "Surf forecast temporarily unavailable." {
		t.Fatalf("missing sources: %+v", view)
	}
}
