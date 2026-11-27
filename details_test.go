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
	r := httptest.NewRequest("GET", "/details?h&refresh=9&section=surf", nil)
	links := detailLinks(r)
	if links["rain"] != "/details?h=#rain" {
		t.Fatalf("rain link = %q", links["rain"])
	}
	if links["overview"] != "/?h=" {
		t.Fatalf("overview link = %q", links["overview"])
	}
	// Refreshing from the details page returns to the overview.
	if got := buildAutoRefreshURL(r, 123); got != "/?h=&refresh=123" {
		t.Fatalf("refresh URL = %q", got)
	}
}

func TestDetailsTemplateRendersTablesAndReturnsToOverview(t *testing.T) {
	view := DashboardView{
		DetailLinks:    detailLinks(httptest.NewRequest("GET", "/details", nil)),
		AutoRefreshURL: "/?refresh=1",
		Summary:        "Clear skies",
		TideEvents:     []TideEventView{{Time: "3:17 AM", Type: "Low", Height: "0.1 ft"}},
		MoonPercent:    "26%", MoonName: "crescent",
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "details.html", view); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`content="600;url=/?refresh=1"`, `id="surf"`, `id="tides"`, `id="rain"`, "3:17 AM", "Clear skies", "Moon 26% lit · crescent"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("details page missing %q", want)
		}
	}
}

func TestMissingDetailSources(t *testing.T) {
	view := DashboardView{}
	populateDetails(&view, DashboardSnapshot{}, time.Now())
	if len(view.SurfHours) != 0 || len(view.TideEvents) != 0 || len(view.RainHours) != 0 || view.SurfSummary != "Surf forecast temporarily unavailable." {
		t.Fatalf("missing sources: %+v", view)
	}
}
