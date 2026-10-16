package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

// Render the production template and assets without credentials or upstream APIs.
// Playwright owns this process; ordinary Go test runs skip the fixture server.
func TestLayoutFixtureServer(t *testing.T) {
	if os.Getenv("KINDLE_LAYOUT_SERVER") != "1" {
		t.Skip("only used by Playwright")
	}
	mux := http.NewServeMux()
	mux.Handle("/css/", http.FileServer(http.Dir(".")))
	mux.Handle("/font/", http.FileServer(http.Dir(".")))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		view := DashboardView{
			DetailLinks:        detailLinks(r),
			SurfSummary:        "Best surf today: 9:00 AM–11:00 AM · 3.0 ft at 10 s · wind 8 mph",
			CurrentTemperature: "99°", CurrentIconClass: "wi wi-day-cloudy",
			Summary:    "You can expect partly cloudy in the morning, with rain in the afternoon",
			Horizontal: r.URL.Query().Has("h"), Sunrise: "7:20 AM", Sunset: "7:06 PM",
			MoonPhaseIcon: "wi-moon-waning-crescent-3", AutoRefreshSeconds: 3600, AutoRefreshURL: buildAutoRefreshURL(r, 123),
			KennedyLaunch: &LaunchInfo{Scheduled: "11:53 PM"},
			BeachStatus: &BeachStatus{
				Kind: "surf", Text: "Best surf 9 AM–11 AM · 3.0 ft @ 10s",
				Label: "GOOD SURF TODAY", Big: "9–11", Suffix: "AM", Detail: "3.0 ft @ 10 s\nwind 8 mph",
			},
			Notices: []Notice{{Kind: "rain", Text: "Rain likely around 1:00 PM (80%)"}, {Kind: "alert", Text: "Very high UV · use sun protection"}},
		}
		// A fixed time keeps the overview bands identical between runs.
		loc := easternLocation()
		now := time.Date(2026, 10, 15, 8, 27, 0, 0, loc)
		pops := []float64{5, 10, 20, 35, 80, 100, 90, 60, 30, 10, 5, 0}
		tides := []TidePrediction{
			{Time: "4:38 AM", Type: "H", Height: 3}, {Time: "10:25 AM", Type: "L", Height: -0.2},
			{Time: "5:13 PM", Type: "H", Height: 3}, {Time: "11:13 PM", Type: "L", Height: 0},
		}
		var hourly []HourlyWeather
		for i, pop := range pops {
			hourly = append(hourly, HourlyWeather{Dt: now.Truncate(time.Hour).Add(time.Duration(i) * time.Hour).Unix(), Pop: pop})
		}
		view.FeelsLike, view.NowLabel = "104", "THU 8:27 AM"
		view.Rain = buildRainBand(hourly, now, loc)
		view.Tide = buildTideBand(tides, now, loc)
		view.MoonPercent, view.MoonName = moonDescription(0.9)
		view.SunEventLabel, view.SunEventTime = "sunset", "7:06"
		if r.URL.Query().Has("long") {
			// Worst case: widest temperature, a fallback text band, and two alerts.
			view.CurrentTemperature = "104°"
			view.BeachStatus = &BeachStatus{Kind: "tide", Text: "Super low tide at 10:25 AM (-0.2 ft)"}
			view.Notices = append(view.Notices, Notice{Kind: "wind", Text: "Wind gusts up to 38 mph"})
		}
		if r.URL.Query().Has("minimal") {
			view.KennedyLaunch = nil
			view.BeachStatus = nil
			view.Notices = nil
			view.Tide = nil
		}
		view.OverviewNotices = overviewNotices(view.Notices)
		if !r.URL.Query().Has("minimal") {
			for i := 0; i < 12; i++ {
				view.SurfHours = append(view.SurfHours, SurfHourView{Time: "Thu 11 PM", Height: "3.0 ft", Period: "10 s", Direction: "NE", Wind: "12 mph NW"})
				view.RainHours = append(view.RainHours, RainHourView{Time: "Fri 12 AM", Chance: "100%", Amount: "0.25 in"})
			}
			view.TideEvents = []TideEventView{{Time: "4:38 AM", Type: "High", Height: "3.0 ft"}, {Time: "10:25 AM", Type: "Low", Height: "-0.2 ft"}, {Time: "5:13 PM", Type: "High", Height: "3.0 ft"}, {Time: "11:13 PM", Type: "Low", Height: "0.0 ft"}}
		} else {
			view.SurfSummary = "Surf forecast temporarily unavailable."
		}
		for _, time := range []string{"11:00 AM", "1:00 PM", "3:00 PM", "5:00 PM"} {
			view.Forecast = append(view.Forecast, ForecastView{Time: time, Temperature: "101°", Description: "heavy intensity rain", IconClass: "wi wi-day-rain"})
		}
		view.TideSVG, _ = generateTideSVG([]TidePrediction{
			{Time: "4:38 AM", Type: "H", Height: 3}, {Time: "10:25 AM", Type: "L", Height: 0},
			{Time: "5:13 PM", Type: "H", Height: 3}, {Time: "11:13 PM", Type: "L", Height: 0},
		})
		if err := tmpl.Execute(w, view); err != nil {
			t.Error(err)
		}
	})
	t.Fatal(http.ListenAndServe("127.0.0.1:18769", mux))
}
