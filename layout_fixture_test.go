package main

import (
	"net/http"
	"os"
	"testing"
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
			CurrentTemperature: "99°", CurrentIconClass: "wi wi-day-cloudy",
			Summary:    "You can expect partly cloudy in the morning, with rain in the afternoon",
			Horizontal: r.URL.Query().Has("h"), Sunrise: "7:20 AM", Sunset: "7:06 PM",
			MoonPhaseIcon: "wi-moon-waning-crescent-3", AutoRefreshSeconds: 3600, AutoRefreshURL: r.URL.String(),
			KennedyLaunch: &LaunchInfo{Scheduled: "11:53 PM"},
			BeachStatus:   &BeachStatus{Kind: "surf", Text: "Best surf 9 AM–11 AM · 3.0 ft @ 10s"},
			Notices:       []Notice{{Text: "Rain likely around 1:00 PM (80%)"}, {Kind: "alert", Text: "Very high UV · use sun protection"}},
		}
		if r.URL.Query().Has("long") {
			view.Summary = "You can expect partly cloudy skies throughout the morning, followed by scattered thunderstorms and periods of heavy rain during the afternoon and evening."
		}
		if r.URL.Query().Has("minimal") {
			view.KennedyLaunch = nil
			view.BeachStatus = nil
			view.Notices = nil
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
