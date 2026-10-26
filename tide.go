package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

type TideData struct {
	Predictions []TidePrediction `json:"predictions"`
}

type TidePrediction struct {
	Time   string  `json:"t"`
	Type   string  `json:"type"`
	Height float64 `json:"v"`
	Date   string  `json:"-"`
}

type tideCacheEntry struct {
	Tide      TideData
	FetchedAt time.Time
}

func getTide(ctx context.Context) (TideData, error) {
	snapshot, err := getTideSnapshot(ctx, time.Now())
	return snapshot.Data, err
}

func getTideSnapshot(ctx context.Context, now time.Time) (TideSnapshot, error) {
	localNow := now.In(easternLocation())
	cacheKey := localNow.Format("2006-01-02")
	if cachedData, found := tideCache.Get(cacheKey); found {
		cacheRequestsTotal.WithLabelValues("tide", "hit").Inc()
		entry := cachedData.(tideCacheEntry)
		return TideSnapshot{Data: entry.Tide, FetchedAt: entry.FetchedAt}, nil
	}

	tideRefreshMu.Lock()
	defer tideRefreshMu.Unlock()
	if cachedData, found := tideCache.Get(cacheKey); found {
		cacheRequestsTotal.WithLabelValues("tide", "hit_after_wait").Inc()
		entry := cachedData.(tideCacheEntry)
		return TideSnapshot{Data: entry.Tide, FetchedAt: entry.FetchedAt}, nil
	}

	tide, err := fetchTideFromAPI(ctx)
	if err == nil && !tideMatchesDate(tide, cacheKey) {
		err = &APIError{URL: noaaAPIURL, Operation: "validate tide data", Err: fmt.Errorf("predictions are not for %s", cacheKey)}
	}
	if err != nil {
		apiRequestErrors.WithLabelValues("tide").Inc()
		if cachedData, found := tideCache.Get(tideCacheKeyLatest); found {
			entry := cachedData.(tideCacheEntry)
			// Yesterday's clock-only tide predictions must never be presented as
			// today's tides.
			if entry.FetchedAt.In(easternLocation()).Format("2006-01-02") != cacheKey {
				return TideSnapshot{}, err
			}
			logJSON(logEntry{
				Timestamp: now.Format(time.RFC3339),
				Level:     "WARN",
				Message:   fmt.Sprintf("Using cached tide data after refresh failure: %v", err),
			})
			cacheRequestsTotal.WithLabelValues("tide", "stale").Inc()
			return TideSnapshot{Data: entry.Tide, FetchedAt: entry.FetchedAt, Stale: true}, nil
		}
		return TideSnapshot{}, err
	}
	cacheRequestsTotal.WithLabelValues("tide", "miss").Inc()
	entry := tideCacheEntry{Tide: tide, FetchedAt: now}
	tideCache.Set(cacheKey, entry, cache.DefaultExpiration)
	tideCache.Set(tideCacheKeyLatest, entry, cache.NoExpiration)

	return TideSnapshot{Data: tide, FetchedAt: now}, nil
}

func tideMatchesDate(tide TideData, date string) bool {
	if len(tide.Predictions) == 0 {
		return false
	}
	for _, prediction := range tide.Predictions {
		if prediction.Date != "" && prediction.Date != date {
			return false
		}
	}
	return true
}

func fetchTideFromAPI(ctx context.Context) (TideData, error) {
	tideURL, err := buildTideURL(noaaAPIURL)
	if err != nil {
		return TideData{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= tideAPIMaxAttempts; attempt++ {
		tideData, retry, err := fetchTideAttempt(ctx, tideURL)
		if err == nil {
			return tideData, nil
		}
		lastErr = err
		if !retry || attempt == tideAPIMaxAttempts || ctx.Err() != nil {
			break
		}

		timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return TideData{}, &APIError{URL: tideURL, Operation: "GET tide data", Err: ctx.Err()}
		case <-timer.C:
		}
	}

	return TideData{}, lastErr
}

func buildTideURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", &APIError{URL: baseURL, Operation: "build tide request", Err: err}
	}
	q := u.Query()
	q.Set("product", "predictions")
	q.Set("datum", "MLLW")
	q.Set("units", "english")
	q.Set("time_zone", "lst_ldt")
	q.Set("interval", "hilo")
	q.Set("format", "json")
	if q.Get("date") == "" {
		q.Set("date", "today")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func fetchTideAttempt(ctx context.Context, tideURL string) (TideData, bool, error) {
	apiRequestsTotal.WithLabelValues("tide").Inc()
	requestStarted := time.Now()
	defer func() { apiRequestDuration.WithLabelValues("tide").Observe(time.Since(requestStarted).Seconds()) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tideURL, nil)
	if err != nil {
		return TideData{}, false, &APIError{URL: tideURL, Operation: "build tide request", Err: err}
	}
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TideData{}, true, &APIError{URL: tideURL, Operation: "GET tide data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= http.StatusInternalServerError
		return TideData{}, retry, &APIError{URL: tideURL, Operation: "GET tide data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var rawData struct {
		Predictions []struct {
			Time   string `json:"t"`
			Type   string `json:"type"`
			Height string `json:"v"`
		} `json:"predictions"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&rawData); err != nil {
		return TideData{}, true, &APIError{URL: tideURL, Operation: "decode tide data", Err: err}
	}

	tideData, err := processTideData(rawData)
	if err != nil {
		return TideData{}, false, err
	}

	return tideData, false, nil
}

func processTideData(rawData struct {
	Predictions []struct {
		Time   string `json:"t"`
		Type   string `json:"type"`
		Height string `json:"v"`
	} `json:"predictions"`
}) (TideData, error) {
	var tideData TideData
	if len(rawData.Predictions) == 0 {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "process tide data", Err: fmt.Errorf("no predictions found")}
	}

	var skipped []string
	for _, p := range rawData.Predictions {
		if p.Type != "H" && p.Type != "L" {
			skipped = append(skipped, fmt.Sprintf("invalid tide type %q", p.Type))
			continue
		}

		itemTime, err := time.Parse("2006-01-02 15:04", p.Time)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("invalid tide time %q", p.Time))
			continue
		}

		height, err := strconv.ParseFloat(p.Height, 64)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("invalid tide height %q", p.Height))
			continue
		}
		tideData.Predictions = append(tideData.Predictions, TidePrediction{
			Time:   itemTime.Format("3:04 PM"),
			Type:   p.Type,
			Height: height,
			Date:   itemTime.Format("2006-01-02"),
		})
	}
	if len(tideData.Predictions) == 0 {
		return TideData{}, &APIError{URL: noaaAPIURL, Operation: "process tide data", Err: fmt.Errorf("no valid predictions found: %s", strings.Join(skipped, "; "))}
	}

	return tideData, nil
}

func generateTideSVG(predictions []TidePrediction) (template.HTML, error) {
	if len(predictions) == 0 {
		return tideUnavailableSVG(), nil
	}
	const svgTemplate = `
    <svg width="600" height="125" viewBox="0 0 600 125">
		<line x1="65" y1="39" x2="535" y2="39" stroke="black" stroke-width="1.5" />
        <path
            fill="none" 
            stroke="black" 
            stroke-width="3"
            d="{{.Path}}"
        />
        {{range .Points}}
        <circle cx="{{.X}}" cy="{{.Y}}" r="5" fill="black" />
        {{end}}
        {{range .Labels}}
		<text x="{{.X}}" y="{{.Y}}" font-size="18" text-anchor="middle" font-weight="bold">{{.Type}}</text>
		<text x="{{.X}}" y="{{.TimeY}}" font-size="22" text-anchor="middle" font-weight="bold">{{.Time}}</text>
        {{end}}
    </svg>`

	type Point struct {
		X, Y float64
	}

	type Label struct {
		X, Y, TimeY float64
		Type        string
		Time        string
	}

	var points []Point
	var labels []Label
	buildPath := func(points []Point) string {
		if len(points) == 0 {
			return ""
		}
		if len(points) == 1 {
			return fmt.Sprintf("M %.1f %.1f", points[0].X, points[0].Y)
		}

		var path strings.Builder
		path.WriteString(fmt.Sprintf("M %.1f %.1f", points[0].X, points[0].Y))
		for i := 1; i < len(points); i++ {
			prev := points[i-1]
			curr := points[i]
			midX := prev.X + ((curr.X - prev.X) / 2)
			path.WriteString(fmt.Sprintf(" C %.1f %.1f, %.1f %.1f, %.1f %.1f", midX, prev.Y, midX, curr.Y, curr.X, curr.Y))
		}
		return path.String()
	}

	// Find min/max heights for scaling
	minHeight := predictions[0].Height
	maxHeight := predictions[0].Height
	for _, p := range predictions {
		if p.Height < minHeight {
			minHeight = p.Height
		}
		if p.Height > maxHeight {
			maxHeight = p.Height
		}
	}

	// Generate points and labels
	for i, p := range predictions {
		denom := float64(len(predictions) - 1)
		x := 300.0
		if denom > 0 {
			x = 65 + float64(i)*(470.0/denom)
		}
		scale := 1.0
		if maxHeight != minHeight {
			scale = (p.Height - minHeight) / (maxHeight - minHeight)
		}
		y := 54 - (scale * 30)
		tideType := "LOW"
		if p.Type == "H" {
			tideType = "HIGH"
		}

		points = append(points, Point{X: x, Y: y})
		labels = append(labels, Label{
			X:     x,
			Y:     82,
			TimeY: 112,
			Type:  tideType,
			Time:  p.Time,
		})
	}

	// Render SVG
	tmpl := template.Must(template.New("svg").Parse(svgTemplate))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, struct {
		Points []Point
		Labels []Label
		Path   string
	}{
		Points: points,
		Labels: labels,
		Path:   buildPath(points),
	})
	if err != nil {
		return "", fmt.Errorf("error rendering SVG: %w", err)
	}

	return template.HTML(buf.String()), nil
}

func tideUnavailableSVG() template.HTML {
	return template.HTML(`
    <svg width="600" height="125" viewBox="0 0 600 125">
		<line x1="65" y1="54" x2="535" y2="54" stroke="black" stroke-width="2" stroke-dasharray="6 6" />
		<text x="300" y="62" font-size="22" text-anchor="middle" font-weight="bold">Tide data unavailable</text>
    </svg>`)
}
