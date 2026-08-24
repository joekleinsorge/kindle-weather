package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

type WeatherSnapshot struct {
	Data      WeatherData
	FetchedAt time.Time
	Stale     bool
}

type TideSnapshot struct {
	Data      TideData
	FetchedAt time.Time
	Stale     bool
}

type SurfSnapshot struct {
	Data      SurfForecast
	FetchedAt time.Time
	Stale     bool
}

type LaunchSnapshot struct {
	Data      *LaunchInfo
	FetchedAt time.Time
	Stale     bool
}

type DashboardSnapshot struct {
	Weather WeatherSnapshot
	Tide    TideSnapshot
	Surf    SurfSnapshot
	Launch  LaunchSnapshot
}

type ForecastView struct {
	Time        string
	Temperature string
	Description string
	IconClass   string
}

type Notice struct {
	Kind string
	Text string
}

type DashboardView struct {
	CurrentTemperature string
	CurrentIconClass   string
	Summary            string
	Forecast           []ForecastView
	ForecastSlots      [4]ForecastView
	TideSVG            template.HTML
	MoonPhaseIcon      string
	Sunrise            string
	Sunset             string
	Horizontal         bool
	KennedyLaunch      *LaunchInfo
	BeachStatus        *BeachStatus
	Notices            []Notice
	NoticeSlots        [2]Notice
	Stale              bool
	AutoRefreshSeconds int
	AutoRefreshURL     string
}

func loadDashboardSnapshot(ctx context.Context, now time.Time) (DashboardSnapshot, error) {
	var snapshot DashboardSnapshot
	var weatherErr, tideErr, surfErr, launchErr error

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		snapshot.Weather, weatherErr = getWeatherSnapshot(ctx, now)
	}()
	go func() {
		defer wg.Done()
		snapshot.Tide, tideErr = getTideSnapshot(ctx, now)
	}()
	go func() {
		defer wg.Done()
		snapshot.Surf, surfErr = getSurfSnapshot(ctx, now)
	}()
	go func() {
		defer wg.Done()
		snapshot.Launch, launchErr = getTodayKennedyLaunchSnapshot(ctx, now)
	}()
	wg.Wait()

	if weatherErr != nil {
		return DashboardSnapshot{}, weatherErr
	}
	logOptionalSourceError("tide", tideErr, now)
	logOptionalSourceError("surf", surfErr, now)
	logOptionalSourceError("launch", launchErr, now)
	return snapshot, nil
}

func logOptionalSourceError(source string, err error, now time.Time) {
	if err == nil {
		return
	}
	logJSON(logEntry{
		Timestamp: now.Format(time.RFC3339),
		Level:     "WARN",
		Message:   fmt.Sprintf("Error getting %s data: %v", source, err),
	})
}

func buildDashboardView(snapshot DashboardSnapshot, r *http.Request, now time.Time) DashboardView {
	weather := snapshot.Weather.Data
	currentCondition := weather.Current.Weather[0]
	forecastHours := getForecastHoursAt(weather.Hourly, now)

	view := DashboardView{
		CurrentTemperature: fmt.Sprintf("%.0f°", weather.Current.Temp),
		CurrentIconClass:   getIconClassName(currentCondition.Icon, currentCondition.ID),
		Summary:            weather.Daily[0].Summary,
		MoonPhaseIcon:      getMoonPhaseIcon(weather.Daily[0].MoonPhase),
		Sunrise:            weather.Current.SunriseFormatted,
		Sunset:             weather.Current.SunsetFormatted,
		Horizontal:         r.URL.Query().Has("h"),
		KennedyLaunch:      snapshot.Launch.Data,
		Notices:            buildActionNotices(weather, now),
		Stale:              snapshot.Weather.Stale || snapshot.Tide.Stale || snapshot.Surf.Stale,
		AutoRefreshURL:     buildAutoRefreshURL(r, now.Unix()),
	}

	if view.Summary == "" {
		view.Summary = currentCondition.Description
	}
	for _, hour := range forecastHours {
		condition := hour.Weather[0]
		item := ForecastView{
			Time:        hour.DtFormatted,
			Temperature: fmt.Sprintf("%.0f°", hour.Temp),
			Description: condition.Description,
			IconClass:   getIconClassName(condition.Icon, condition.ID),
		}
		view.Forecast = append(view.Forecast, item)
	}
	for i := range view.ForecastSlots {
		if i < len(view.Forecast) {
			view.ForecastSlots[i] = view.Forecast[i]
		}
	}
	for i := range view.NoticeSlots {
		if i < len(view.Notices) {
			view.NoticeSlots[i] = view.Notices[i]
		}
	}

	view.TideSVG, _ = generateTideSVG(snapshot.Tide.Data.Predictions)
	surfWindow := bestSurfWindow(snapshot.Surf.Data, weather, now)
	view.BeachStatus = getBeachStatusWithWindow(snapshot.Tide.Data.Predictions, surfWindow, now)
	view.AutoRefreshSeconds = int(adaptiveRefreshInterval(snapshot, now).Seconds())
	recordSourceAges(snapshot, now)
	return view
}

func recordSourceAges(snapshot DashboardSnapshot, now time.Time) {
	ages := map[string]time.Time{
		"weather":  snapshot.Weather.FetchedAt,
		"tide":     snapshot.Tide.FetchedAt,
		"surf":     snapshot.Surf.FetchedAt,
		"launches": snapshot.Launch.FetchedAt,
	}
	for source, fetchedAt := range ages {
		if fetchedAt.IsZero() {
			continue
		}
		sourceDataAgeSeconds.WithLabelValues(source).Set(max(0, now.Sub(fetchedAt).Seconds()))
	}
}

func buildActionNotices(weather WeatherData, now time.Time) []Notice {
	var notices []Notice
	if len(weather.Alerts) > 0 && strings.TrimSpace(weather.Alerts[0].Event) != "" {
		notices = append(notices, Notice{Kind: "alert", Text: weather.Alerts[0].Event})
	}

	for _, hour := range weather.Hourly {
		forecastTime := time.Unix(hour.Dt, 0)
		if forecastTime.Before(now) || forecastTime.After(now.Add(12*time.Hour)) {
			continue
		}
		if hour.Pop >= 50 || hour.Rain.OneH > 0 {
			notices = append(notices, Notice{
				Kind: "rain",
				Text: fmt.Sprintf("Rain likely around %s (%.0f%%)", hour.DtFormatted, hour.Pop),
			})
			break
		}
	}

	uvi := weather.Current.Uvi
	if len(weather.Daily) > 0 && weather.Daily[0].Uvi > uvi {
		uvi = weather.Daily[0].Uvi
	}
	if uvi >= 8 {
		notices = append(notices, Notice{Kind: "uv", Text: fmt.Sprintf("Very high UV today (%.0f)", uvi)})
	} else if uvi >= 6 {
		notices = append(notices, Notice{Kind: "uv", Text: fmt.Sprintf("High UV today (%.0f)", uvi)})
	}

	maxGust := weather.Current.WindGust
	for _, hour := range weather.Hourly {
		if time.Unix(hour.Dt, 0).After(now.Add(12 * time.Hour)) {
			break
		}
		maxGust = math.Max(maxGust, hour.WindGust)
	}
	if maxGust >= 25 {
		notices = append(notices, Notice{Kind: "wind", Text: fmt.Sprintf("Wind gusts up to %.0f mph", maxGust)})
	}

	if weather.Current.FeelsLike >= 95 {
		notices = append(notices, Notice{Kind: "temperature", Text: fmt.Sprintf("Feels like %.0f° — limit midday heat", weather.Current.FeelsLike)})
	} else if weather.Current.FeelsLike <= 40 {
		notices = append(notices, Notice{Kind: "temperature", Text: fmt.Sprintf("Feels like %.0f° — dress warmly", weather.Current.FeelsLike)})
	}

	if len(notices) > 2 {
		return notices[:2]
	}
	return notices
}

func adaptiveRefreshInterval(snapshot DashboardSnapshot, now time.Time) time.Duration {
	interval := autoRefresh
	localNow := now.In(surfLocation(snapshot.Weather.Data))
	if localNow.Hour() >= 22 || localNow.Hour() < 5 {
		interval = max(interval*3, 90*time.Minute)
		if interval > 2*time.Hour {
			interval = 2 * time.Hour
		}
	}

	if launch := snapshot.Launch.Data; launch != nil && !launch.Start.IsZero() {
		untilLaunch := launch.Start.Sub(now)
		if untilLaunch > 0 && untilLaunch <= 2*time.Hour {
			interval = min(interval, 5*time.Minute)
		}
	}
	for _, hour := range snapshot.Weather.Data.Hourly {
		until := time.Unix(hour.Dt, 0).Sub(now)
		if until > 0 && until <= 2*time.Hour && (hour.Pop >= 50 || hour.Rain.OneH > 0) {
			interval = min(interval, 10*time.Minute)
			break
		}
	}
	if tide := upcomingSuperLowTide(snapshot.Tide.Data.Predictions, now); tide != nil {
		if tideTime, ok := tideTimeToday(*tide, now); ok && tideTime.Sub(now) <= 2*time.Hour {
			interval = min(interval, 15*time.Minute)
		}
	}
	if interval < time.Minute {
		return time.Minute
	}
	return interval
}

func tideTimeToday(tide TidePrediction, now time.Time) (time.Time, bool) {
	clock, err := time.Parse("3:04 PM", tide.Time)
	if err != nil {
		return time.Time{}, false
	}
	localNow := now.In(easternLocation())
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), clock.Hour(), clock.Minute(), 0, 0, localNow.Location()), true
}

func handler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()

	snapshot, err := loadDashboardSnapshot(ctx, now)
	if err != nil {
		logJSON(logEntry{Timestamp: now.Format(time.RFC3339), Level: "ERROR", Message: fmt.Sprintf("Error getting weather data: %v", err)})
		http.Error(w, "Weather data is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if enableRocketPreview && r.URL.Query().Has("rocketPreview") {
		snapshot.Launch.Data = &LaunchInfo{Scheduled: "4:30pm"}
	}
	view := buildDashboardView(snapshot, r, now)

	var output bytes.Buffer
	if err := tmpl.Execute(&output, view); err != nil {
		logJSON(logEntry{Timestamp: now.Format(time.RFC3339), Level: "ERROR", Message: fmt.Sprintf("Could not render template: %v", err)})
		http.Error(w, "Could not render dashboard", http.StatusInternalServerError)
		return
	}

	setDashboardHeaders(w, "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
}

func dashboardSVGHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	snapshot, err := loadDashboardSnapshot(ctx, now)
	if err != nil {
		http.Error(w, "Weather data is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	view := buildDashboardView(snapshot, r, now)
	var output bytes.Buffer
	if err := svgTmpl.Execute(&output, view); err != nil {
		http.Error(w, "Could not render dashboard image", http.StatusInternalServerError)
		return
	}
	setDashboardHeaders(w, "image/svg+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
}

func setDashboardHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
