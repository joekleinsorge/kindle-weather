package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

const (
	surfAPIURLDefault = "https://marine-api.open-meteo.com/v1/marine?latitude=29.65&longitude=-81.20&hourly=wave_height,wave_direction,wave_period&forecast_hours=24&length_unit=imperial&timeformat=unixtime&cell_selection=sea"

	// Crescent Beach surf preferences. These intentionally describe a friendly,
	// broadly surfable day rather than large or expert-only conditions.
	surfMinWaveHeightFeet       = 1.5
	surfMaxWaveHeightFeet       = 6.0
	surfMinWavePeriodSeconds    = 7.0
	surfMinWaveDirectionDegrees = 20.0
	surfMaxWaveDirectionDegrees = 160.0
	surfLightWindMPH            = 5.0
	surfMaxWindMPH              = 12.0
	surfMinOffshoreWindDegrees  = 195
	surfMaxOffshoreWindDegrees  = 315
	surfWindMatchWindow         = 90 * time.Minute
	surfAPITimeout              = 3 * time.Second
	surfMaxStaleAge             = 6 * time.Hour

	surfCacheKey       = "forecast"
	surfLatestCacheKey = "latest-successful"
)

var (
	surfAPIURL    = surfAPIURLDefault
	surfCache     = cache.New(30*time.Minute, time.Hour)
	surfRefreshMu sync.Mutex
)

type surfCacheEntry struct {
	Forecast  SurfForecast
	FetchedAt time.Time
}

type SurfForecast struct {
	Hourly SurfHourlyForecast `json:"hourly"`
}

type SurfHourlyForecast struct {
	Time          []int64   `json:"time"`
	WaveHeight    []float64 `json:"wave_height"`
	WaveDirection []float64 `json:"wave_direction"`
	WavePeriod    []float64 `json:"wave_period"`
}

type SurfWindow struct {
	Start      time.Time
	End        time.Time
	WaveHeight float64
	WavePeriod float64
	WindSpeed  float64
}

func configureSurfRuntime(cleanup time.Duration) {
	surfAPIURL = strings.TrimSpace(os.Getenv("SURF_API_URL"))
	if surfAPIURL == "" {
		surfAPIURL = surfAPIURLDefault
	}
	surfCache = cache.New(parseEnvDurationSeconds("SURF_CACHE_EXPIRATION", 30*time.Minute), cleanup)
}

func getSurfForecast(ctx context.Context) (SurfForecast, error) {
	snapshot, err := getSurfSnapshot(ctx, time.Now())
	return snapshot.Data, err
}

func getSurfSnapshot(ctx context.Context, now time.Time) (SurfSnapshot, error) {
	if cachedData, found := surfCache.Get(surfCacheKey); found {
		cacheRequestsTotal.WithLabelValues("surf", "hit").Inc()
		entry := cachedData.(surfCacheEntry)
		return SurfSnapshot{Data: entry.Forecast, FetchedAt: entry.FetchedAt}, nil
	}

	surfRefreshMu.Lock()
	defer surfRefreshMu.Unlock()
	if cachedData, found := surfCache.Get(surfCacheKey); found {
		cacheRequestsTotal.WithLabelValues("surf", "hit_after_wait").Inc()
		entry := cachedData.(surfCacheEntry)
		return SurfSnapshot{Data: entry.Forecast, FetchedAt: entry.FetchedAt}, nil
	}

	forecast, err := fetchSurfForecast(ctx)
	if err != nil {
		apiRequestErrors.WithLabelValues("surf").Inc()
		if cachedData, found := surfCache.Get(surfLatestCacheKey); found {
			entry := cachedData.(surfCacheEntry)
			localNow := now.In(easternLocation())
			if now.Sub(entry.FetchedAt) > surfMaxStaleAge || !sameDate(entry.FetchedAt.In(easternLocation()), localNow) {
				return SurfSnapshot{}, err
			}
			logJSON(logEntry{
				Timestamp: now.Format(time.RFC3339),
				Level:     "WARN",
				Message:   fmt.Sprintf("Using cached surf data after refresh failure: %v", err),
			})
			cacheRequestsTotal.WithLabelValues("surf", "stale").Inc()
			return SurfSnapshot{Data: entry.Forecast, FetchedAt: entry.FetchedAt, Stale: true}, nil
		}
		return SurfSnapshot{}, err
	}
	cacheRequestsTotal.WithLabelValues("surf", "miss").Inc()

	entry := surfCacheEntry{Forecast: forecast, FetchedAt: now}
	surfCache.Set(surfCacheKey, entry, cache.DefaultExpiration)
	surfCache.Set(surfLatestCacheKey, entry, cache.NoExpiration)
	return SurfSnapshot{Data: forecast, FetchedAt: now}, nil
}

func fetchSurfForecast(ctx context.Context) (SurfForecast, error) {
	apiRequestsTotal.WithLabelValues("surf").Inc()
	requestStarted := time.Now()
	defer func() { apiRequestDuration.WithLabelValues("surf").Observe(time.Since(requestStarted).Seconds()) }()
	if strings.TrimSpace(surfAPIURL) == "" {
		return SurfForecast{}, fmt.Errorf("surf API URL is not configured")
	}

	requestContext, cancel := context.WithTimeout(ctx, surfAPITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, surfAPIURL, nil)
	if err != nil {
		return SurfForecast{}, &APIError{URL: surfAPIURL, Operation: "build surf request", Err: err}
	}
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return SurfForecast{}, &APIError{URL: surfAPIURL, Operation: "GET surf data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SurfForecast{}, &APIError{URL: surfAPIURL, Operation: "GET surf data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var forecast SurfForecast
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&forecast); err != nil {
		return SurfForecast{}, &APIError{URL: surfAPIURL, Operation: "decode surf data", Err: err}
	}
	if usableSurfHours(forecast.Hourly) == 0 {
		return SurfForecast{}, &APIError{URL: surfAPIURL, Operation: "validate surf data", Err: fmt.Errorf("no complete hourly forecasts")}
	}

	return forecast, nil
}

func usableSurfHours(hourly SurfHourlyForecast) int {
	return min(len(hourly.Time), len(hourly.WaveHeight), len(hourly.WaveDirection), len(hourly.WavePeriod))
}

func isGoodSurfToday(forecast SurfForecast, weather WeatherData, now time.Time) bool {
	return bestSurfWindow(forecast, weather, now) != nil
}

func bestSurfWindow(forecast SurfForecast, weather WeatherData, now time.Time) *SurfWindow {
	loc := surfLocation(weather)
	localNow := now.In(loc)
	sunrise, sunset := surfDaylightWindow(weather, localNow, loc)
	if localNow.After(sunset) {
		return nil
	}

	windowStart := localNow
	if windowStart.Before(sunrise) {
		windowStart = sunrise
	}

	var windows []SurfWindow
	for i := 0; i < usableSurfHours(forecast.Hourly); i++ {
		forecastTime := time.Unix(forecast.Hourly.Time[i], 0).In(loc)
		if !sameDate(forecastTime, localNow) || forecastTime.Before(windowStart) || !forecastTime.Before(sunset) {
			continue
		}
		if !isSurfableWave(
			forecast.Hourly.WaveHeight[i],
			forecast.Hourly.WavePeriod[i],
			forecast.Hourly.WaveDirection[i],
		) {
			continue
		}

		windSpeed, windDirection, ok := nearestWind(weather, forecastTime)
		if !ok || !isSurfableWind(windSpeed, windDirection) {
			continue
		}

		windowEnd := forecastTime.Add(time.Hour)
		if windowEnd.After(sunset) {
			windowEnd = sunset
		}
		candidate := SurfWindow{
			Start:      forecastTime,
			End:        windowEnd,
			WaveHeight: forecast.Hourly.WaveHeight[i],
			WavePeriod: forecast.Hourly.WavePeriod[i],
			WindSpeed:  windSpeed,
		}
		if len(windows) > 0 && forecastTime.Sub(windows[len(windows)-1].End) <= 30*time.Minute {
			current := &windows[len(windows)-1]
			current.End = candidate.End
			if candidate.WavePeriod > current.WavePeriod {
				current.WaveHeight = candidate.WaveHeight
				current.WavePeriod = candidate.WavePeriod
				current.WindSpeed = candidate.WindSpeed
			}
			continue
		}
		windows = append(windows, candidate)
	}

	if len(windows) == 0 {
		return nil
	}
	best := windows[0]
	for _, window := range windows[1:] {
		if window.End.Sub(window.Start) > best.End.Sub(best.Start) ||
			(window.End.Sub(window.Start) == best.End.Sub(best.Start) && window.WavePeriod > best.WavePeriod) {
			best = window
		}
	}
	return &best
}

func isSurfableWave(height, period, direction float64) bool {
	if math.IsNaN(height) || math.IsNaN(period) || math.IsNaN(direction) {
		return false
	}
	return height >= surfMinWaveHeightFeet &&
		height <= surfMaxWaveHeightFeet &&
		period >= surfMinWavePeriodSeconds &&
		direction >= surfMinWaveDirectionDegrees &&
		direction <= surfMaxWaveDirectionDegrees
}

func isSurfableWind(speed float64, direction int) bool {
	if speed < 0 || speed > surfMaxWindMPH {
		return false
	}
	if speed <= surfLightWindMPH {
		return true
	}
	return direction >= surfMinOffshoreWindDegrees && direction <= surfMaxOffshoreWindDegrees
}

func nearestWind(weather WeatherData, target time.Time) (float64, int, bool) {
	bestDifference := time.Duration(math.MaxInt64)
	var bestSpeed float64
	var bestDirection int

	consider := func(timestamp int64, speed float64, direction int) {
		if timestamp == 0 {
			return
		}
		difference := time.Unix(timestamp, 0).Sub(target).Abs()
		if difference < bestDifference {
			bestDifference = difference
			bestSpeed = speed
			bestDirection = direction
		}
	}

	consider(weather.Current.Dt, weather.Current.WindSpeed, weather.Current.WindDeg)
	for _, hour := range weather.Hourly {
		consider(hour.Dt, hour.WindSpeed, hour.WindDeg)
	}

	if bestDifference > surfWindMatchWindow {
		return 0, 0, false
	}
	return bestSpeed, bestDirection, true
}

func surfLocation(weather WeatherData) *time.Location {
	if weather.Timezone != "" {
		if loc, err := time.LoadLocation(weather.Timezone); err == nil {
			return loc
		}
	}
	if loc, err := time.LoadLocation("America/New_York"); err == nil {
		return loc
	}
	return time.FixedZone("Eastern", weather.TimezoneOffset)
}

func surfDaylightWindow(weather WeatherData, localNow time.Time, loc *time.Location) (time.Time, time.Time) {
	sunrise := time.Unix(weather.Current.Sunrise, 0).In(loc)
	sunset := time.Unix(weather.Current.Sunset, 0).In(loc)
	if weather.Current.Sunrise == 0 || !sameDate(sunrise, localNow) {
		sunrise = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 6, 0, 0, 0, loc)
	}
	if weather.Current.Sunset == 0 || !sameDate(sunset, localNow) {
		sunset = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 20, 0, 0, 0, loc)
	}
	return sunrise, sunset
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
