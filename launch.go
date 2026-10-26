package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

type LaunchData struct {
	WindowStart string    `json:"window_start"`
	WindowEnd   string    `json:"window_end"`
	Name        string    `json:"name"`
	Pad         LaunchPad `json:"pad"`
}

type LaunchPad struct {
	Name     string         `json:"name"`
	Location LaunchLocation `json:"location"`
}

type LaunchLocation struct {
	Name string `json:"name"`
}

type LaunchInfo struct {
	Name      string
	Scheduled string
	Start     time.Time
}

type launchCacheEntry struct {
	Launch    *LaunchInfo
	FetchedAt time.Time
}

func formatLaunchTime(timestamp string) (string, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", fmt.Errorf("failed to parse time: %w", err)
	}

	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load location: %w", err)
	}

	etTime := parsedTime.In(etLocation)
	return etTime.Format("3:04pm"), nil
}

func buildTodayKennedyLaunchURL(now time.Time) (string, error) {
	baseURL, err := url.Parse(spacedevsAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load location: %w", err)
	}

	etNow := now.In(etLocation)
	startOfDay := time.Date(etNow.Year(), etNow.Month(), etNow.Day(), 0, 0, 0, 0, etLocation)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := baseURL.Query()
	query.Set("limit", "5")
	query.Set("ordering", "net")
	query.Set("net__gte", startOfDay.UTC().Format(time.RFC3339))
	query.Set("net__lt", endOfDay.UTC().Format(time.RFC3339))

	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

func getTodayKennedyLaunch(ctx context.Context) (*LaunchInfo, error) {
	snapshot, err := getTodayKennedyLaunchSnapshot(ctx, time.Now())
	return snapshot.Data, err
}

func getTodayKennedyLaunchSnapshot(ctx context.Context, now time.Time) (LaunchSnapshot, error) {
	cacheKey := todayLaunchCacheKey(now)
	if cachedData, found := launchCache.Get(cacheKey); found {
		cacheRequestsTotal.WithLabelValues("launches", "hit").Inc()
		entry := cachedData.(launchCacheEntry)
		return LaunchSnapshot{Data: entry.Launch, FetchedAt: entry.FetchedAt}, nil
	}

	launchRefreshMu.Lock()
	defer launchRefreshMu.Unlock()
	if cachedData, found := launchCache.Get(cacheKey); found {
		cacheRequestsTotal.WithLabelValues("launches", "hit_after_wait").Inc()
		entry := cachedData.(launchCacheEntry)
		return LaunchSnapshot{Data: entry.Launch, FetchedAt: entry.FetchedAt}, nil
	}

	launch, err := fetchTodayKennedyLaunch(ctx)
	if err != nil {
		apiRequestErrors.WithLabelValues("launches").Inc()
		return LaunchSnapshot{}, err
	}
	cacheRequestsTotal.WithLabelValues("launches", "miss").Inc()
	entry := launchCacheEntry{Launch: launch, FetchedAt: now}
	launchCache.Set(cacheKey, entry, cache.DefaultExpiration)

	return LaunchSnapshot{Data: launch, FetchedAt: now}, nil
}

func todayLaunchCacheKey(now time.Time) string {
	etLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		return now.UTC().Format("2006-01-02")
	}
	return now.In(etLocation).Format("2006-01-02")
}

func fetchTodayKennedyLaunch(ctx context.Context) (*LaunchInfo, error) {
	apiRequestsTotal.WithLabelValues("launches").Inc()
	requestStarted := time.Now()
	defer func() { apiRequestDuration.WithLabelValues("launches").Observe(time.Since(requestStarted).Seconds()) }()

	apiURL, err := buildTodayKennedyLaunchURL(time.Now())
	if err != nil {
		return nil, fmt.Errorf("error building API URL: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, &APIError{URL: apiURL, Operation: "build launch request", Err: err}
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := launchHTTPClient.Do(req)
	if err != nil {
		return nil, &APIError{URL: apiURL, Operation: "GET launch data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{URL: apiURL, Operation: "GET launch data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var data struct {
		Results []LaunchData `json:"results"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&data); err != nil {
		return nil, &APIError{URL: apiURL, Operation: "decode launch data", Err: err}
	}

	for _, launch := range data.Results {
		if !isKennedyLaunch(launch) || launch.WindowStart == "" {
			continue
		}

		formatted, err := formatLaunchTime(launch.WindowStart)
		if err != nil {
			log.Printf("Failed to format window_start for launch: %s", launch.Name)
			continue
		}

		return &LaunchInfo{
			Name:      launch.Name,
			Scheduled: formatted,
			Start:     parseLaunchTime(launch.WindowStart),
		}, nil
	}

	return nil, nil
}

func parseLaunchTime(timestamp string) time.Time {
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func isKennedyLaunch(launch LaunchData) bool {
	locationName := strings.ToLower(launch.Pad.Location.Name)
	padName := strings.ToLower(launch.Pad.Name)

	if locationName == "" && padName == "" {
		return false
	}

	if strings.Contains(locationName, "kennedy space center") {
		return true
	}

	return strings.Contains(padName, "kennedy space center")
}
