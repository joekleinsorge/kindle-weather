package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Render the production template and assets without credentials or upstream APIs.
// Playwright owns this process; ordinary Go test runs skip the fixture server.
func TestLayoutFixtureServer(t *testing.T) {
	if os.Getenv("KINDLE_LAYOUT_SERVER") != "1" {
		t.Skip("only used by Playwright")
	}
	t.Fatal(http.ListenAndServe("127.0.0.1:18769", layoutFixtureHandler(t)))
}

// Export production HTML for browser inspection without opening a local port.
func TestLayoutFixtureFiles(t *testing.T) {
	dir := os.Getenv("KINDLE_LAYOUT_OUTPUT")
	if dir == "" {
		t.Skip("set KINDLE_LAYOUT_OUTPUT to export fixtures")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	handler := layoutFixtureHandler(t)
	for _, scenario := range []string{"", "long", "minimal", "paperwhite1", "paperwhite1&wide-time", "possible", "stale", "h", "h&long", "h&minimal", "h&paperwhite1", "h&paperwhite1&wide-time", "h&possible", "h&stale"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/?"+scenario, nil))
		name := scenario
		if name == "" {
			name = "default"
		}
		if err := os.WriteFile(filepath.Join(dir, name+".html"), recorder.Body.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func layoutFixtureHandler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/css/", http.FileServer(http.Dir(".")))
	mux.Handle("/font/", http.FileServer(http.Dir(".")))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		view := DashboardView{
			DetailLinks:        detailLinks(r),
			ActiveSection:      selectedSection(r),
			SurfSummary:        "Best surf today: 9:00 AM–11:00 AM · 3.0 ft at 10 s · wind 8 mph",
			CurrentTemperature: "84°", CurrentIconClass: "wi wi-day-cloudy",
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
		view.FeelsLike, view.NowLabel = "89", "THU 8:27 AM"
		view.Condition, view.DailyRange = "Partly cloudy", "High 91° · Low 76°"
		// Hours agree with the rain chances above: showers peak early afternoon.
		view.Forecast = []ForecastView{
			{Time: "11:00 AM", Temperature: "88°", Description: "overcast clouds", IconClass: "wi wi-day-cloudy"},
			{Time: "1:00 PM", Temperature: "85°", Description: "moderate rain", IconClass: "wi wi-day-rain"},
			{Time: "3:00 PM", Temperature: "83°", Description: "light rain", IconClass: "wi wi-day-showers"},
			{Time: "5:00 PM", Temperature: "82°", Description: "broken clouds", IconClass: "wi wi-day-cloudy"},
		}
		view.WeatherUpdated = "Updated Thu 8:20 AM"
		view.Rain = buildRainBand(hourly, now, loc)
		view.Tide = buildTideBand(tides, now, loc)
		view.MoonPercent, view.MoonName = moonDescription(0.9)
		view.SunEventLabel, view.SunEventTime = "sunset", "7:06 PM"
		if r.URL.Query().Has("paperwhite1") {
			// The physical 2012 Paperwhite photo exposed a time/caption collision.
			view.CurrentTemperature = "82°"
			view.FeelsLike, view.NowLabel = "87", "FRI 5:47 PM"
			view.Rain.Big, view.Rain.Suffix, view.Rain.Caption = "Dry", "", "no rain expected"
			view.Rain.Label, view.Rain.Likely = "NEXT 12 HOURS", false
			view.WeatherUpdated = "Updated Fri 5:40 PM"
			view.DailyRange = "High 86° · Low 74°"
			view.Forecast = nil
			for i, temp := range []string{"79°", "77°", "76°", "75°"} {
				view.Forecast = append(view.Forecast, ForecastView{Time: []string{"7:00 PM", "9:00 PM", "11:00 PM", "1:00 AM"}[i], Temperature: temp, Description: "clear sky", IconClass: "wi wi-night-clear"})
			}
			for i := range view.Rain.Bars {
				view.Rain.Bars[i] = RainBar{Level: "dry"}
			}
			view.Tide.Label, view.Tide.Big, view.Tide.Suffix = "LOW TIDE · FALLING", "7:32", "PM"
			view.Tide.Caption = "in 1½ hrs · high 1:44a"
			if r.URL.Query().Has("wide-time") {
				view.Tide.Big = "12:59"
				view.Tide.Caption = "in about 12½ hrs · high 12:59p"
			}
			view.BeachStatus, view.KennedyLaunch = nil, nil
			view.Notices = []Notice{{Kind: "alert", Text: "Gale Warning"}, {Text: "High UV today (7)"}}
		}
		if r.URL.Query().Has("long") {
			// Worst case: widest temperature, a fallback text band, and two alerts.
			view.CurrentTemperature = "104°"
			view.FeelsLike, view.DailyRange = "112", "High 104° · Low 81°"
			view.BeachStatus = &BeachStatus{Kind: "tide", Text: "Super low tide at 10:25 AM (-0.2 ft)"}
			view.Notices = append(view.Notices, Notice{Kind: "wind", Text: "Wind gusts up to 38 mph"})
		}
		if r.URL.Query().Has("minimal") {
			view.KennedyLaunch = nil
			view.BeachStatus = nil
			view.Notices = nil
			view.Tide = nil
		}
		if r.URL.Query().Has("possible") {
			view.Rain = buildRainBand(hourlyFromPops(now, 10, 35, 45, 20, 10), now, loc)
		}
		if r.URL.Query().Has("stale") {
			view.Stale = true
			view.StaleText = "Weather from Thu 4:05 AM · updates unavailable"
			view.WeatherAsOf = "as of 4:05 AM"
			view.WeatherUpdated = "Updated Thu 4:05 AM · cached data; refresh unavailable"
		}
		view.OverviewNotices = overviewNotices(view.Notices)
		if !r.URL.Query().Has("minimal") {
			waves := []float64{2.4, 2.8, 3.0, 3.1, 3.0, 2.9, 2.7, 2.6, 2.4, 2.3, 2.1, 2.0}
			periods := []int{9, 10, 10, 10, 9, 9, 8, 8, 8, 7, 7, 7}
			directions := []string{"NE", "NE", "NE", "E", "E", "E", "E", "E", "E", "SE", "SE", "SE"}
			winds := []string{"6 mph W", "8 mph W", "8 mph WNW", "11 mph N", "14 mph NE", "16 mph E", "15 mph E", "13 mph E", "12 mph SE", "10 mph SE", "9 mph S", "7 mph S"}
			for i, pop := range pops {
				hour := now.Truncate(time.Hour).Add(time.Duration(i) * time.Hour).Format("Mon 3 PM")
				view.SurfHours = append(view.SurfHours, SurfHourView{
					Time: hour, Height: fmt.Sprintf("%.1f ft", waves[i]), Period: fmt.Sprintf("%d s", periods[i]),
					Direction: directions[i], Wind: winds[i], Best: i >= 1 && i <= 2,
				})
				amount := 0.0
				if pop >= 50 {
					amount = pop / 400
				}
				view.RainHours = append(view.RainHours, RainHourView{Time: hour, Chance: fmt.Sprintf("%.0f%%", pop), Amount: fmt.Sprintf("%.2f in", amount)})
			}
			view.SurfBestShown = true
			view.TideEvents = []TideEventView{{Time: "4:38 AM", Type: "High", Height: "3.0 ft"}, {Time: "10:25 AM", Type: "Low", Height: "-0.2 ft"}, {Time: "5:13 PM", Type: "High", Height: "3.0 ft"}, {Time: "11:13 PM", Type: "Low", Height: "0.0 ft"}}
		} else {
			view.SurfSummary = "Surf forecast temporarily unavailable."
		}
		view.TideSVG, _ = generateTideSVG([]TidePrediction{
			{Time: "4:38 AM", Type: "H", Height: 3}, {Time: "10:25 AM", Type: "L", Height: 0},
			{Time: "5:13 PM", Type: "H", Height: 3}, {Time: "11:13 PM", Type: "L", Height: 0},
		})
		page := tmpl
		if r.URL.Path == "/dashboard.svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
			page = svgTmpl
		}
		if err := page.Execute(w, view); err != nil {
			t.Error(err)
		}
	})
	return mux
}
