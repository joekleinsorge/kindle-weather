package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type SurfHourView struct {
	Time, Height, Period, Direction, Wind string
}

type RainHourView struct {
	Time, Chance, Amount string
}

type TideEventView struct {
	Time, Type, Height string
}

// Ordinary links work without JavaScript. The section in the query lets the
// server include the same anchor in the automatic refresh URL.
func detailLinks(r *http.Request) map[string]string {
	links := make(map[string]string)
	for _, section := range []string{"overview", "surf", "tides", "rain"} {
		q := r.URL.Query()
		q.Del("refresh")
		q.Set("section", section)
		links[section] = (&url.URL{Path: r.URL.Path, RawQuery: q.Encode(), Fragment: section}).String()
	}
	return links
}

func detailSection(section string) string {
	switch section {
	case "overview", "surf", "tides", "rain":
		return section
	default:
		return ""
	}
}

func populateDetails(view *DashboardView, snapshot DashboardSnapshot, now time.Time) {
	weather := snapshot.Weather.Data
	loc := surfLocation(weather)
	// Include the current forecast hour, even partway through it.
	start := now.Truncate(time.Hour)
	hourly := snapshot.Surf.Data.Hourly
	for i := 0; i < usableSurfHours(hourly) && len(view.SurfHours) < 12; i++ {
		t := time.Unix(hourly.Time[i], 0)
		if t.Before(start) {
			continue
		}
		wind := "Unavailable"
		if speed, direction, ok := nearestWind(weather, t); ok {
			wind = fmt.Sprintf("%.0f mph %s", speed, compassDirection(float64(direction)))
		}
		view.SurfHours = append(view.SurfHours, SurfHourView{
			Time: t.In(loc).Format("Mon 3 PM"), Height: fmt.Sprintf("%.1f ft", hourly.WaveHeight[i]),
			Period: fmt.Sprintf("%.0f s", hourly.WavePeriod[i]), Direction: compassDirection(hourly.WaveDirection[i]), Wind: wind,
		})
	}
	view.SurfSummary = "Surf forecast temporarily unavailable."
	if len(view.SurfHours) > 0 {
		view.SurfSummary = "No remaining daylight surf window matches your preferences today."
		if window := bestSurfWindow(snapshot.Surf.Data, weather, now); window != nil {
			view.SurfSummary = fmt.Sprintf("Best surf today: %s–%s · %.1f ft at %.0f s · wind %.0f mph",
				window.Start.Format("3:04 PM"), window.End.Format("3:04 PM"), window.WaveHeight, window.WavePeriod, window.WindSpeed)
		}
	}
	for _, tide := range snapshot.Tide.Data.Predictions {
		kind := "Low"
		if tide.Type == "H" {
			kind = "High"
		}
		view.TideEvents = append(view.TideEvents, TideEventView{Time: tide.Time, Type: kind, Height: fmt.Sprintf("%.1f ft", tide.Height)})
	}
	for _, hour := range weather.Hourly {
		t := time.Unix(hour.Dt, 0)
		if t.Before(start) {
			continue
		}
		view.RainHours = append(view.RainHours, RainHourView{
			Time: t.In(loc).Format("Mon 3 PM"), Chance: fmt.Sprintf("%.0f%%", hour.Pop), Amount: fmt.Sprintf("%.2f in", hour.Rain.OneH/25.4),
		})
		if len(view.RainHours) == 12 {
			break
		}
	}
	view.SurfUpdated = detailUpdated(snapshot.Surf.FetchedAt, snapshot.Surf.Stale, loc)
	view.TideUpdated = detailUpdated(snapshot.Tide.FetchedAt, snapshot.Tide.Stale, loc)
	view.RainUpdated = detailUpdated(snapshot.Weather.FetchedAt, snapshot.Weather.Stale, loc)
}

func compassDirection(degrees float64) string {
	normalized := int(degrees + 22.5)
	return []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}[((normalized%360+360)%360)/45]
}

func detailUpdated(fetchedAt time.Time, stale bool, loc *time.Location) string {
	if fetchedAt.IsZero() {
		return ""
	}
	text := "Updated " + fetchedAt.In(loc).Format("Mon 3:04 PM")
	if stale {
		text += " · cached data; refresh unavailable"
	}
	return text
}
