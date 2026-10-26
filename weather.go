package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

type WeatherData struct {
	Current        CurrentWeather  `json:"current"`
	Hourly         []HourlyWeather `json:"hourly"`
	Daily          []DailyWeather  `json:"daily"`
	Alerts         []WeatherAlert  `json:"alerts"`
	Timezone       string          `json:"timezone"`
	TimezoneOffset int             `json:"timezone_offset"`
}

type CurrentWeather struct {
	Dt               int64              `json:"dt"`
	Sunrise          int64              `json:"sunrise"`
	Sunset           int64              `json:"sunset"`
	Temp             float64            `json:"temp"`
	FeelsLike        float64            `json:"feels_like"`
	Pressure         int                `json:"pressure"`
	Humidity         int                `json:"humidity"`
	DewPoint         float64            `json:"dew_point"`
	Uvi              float64            `json:"uvi"`
	Clouds           float64            `json:"clouds"`
	Visibility       int                `json:"visibility"`
	WindSpeed        float64            `json:"wind_speed"`
	WindDeg          int                `json:"wind_deg"`
	WindGust         float64            `json:"wind_gust"`
	Weather          []WeatherCondition `json:"weather"`
	SunriseFormatted string
	SunsetFormatted  string
}

type HourlyWeather struct {
	Dt          int64              `json:"dt"`
	DtFormatted string             `json:"-"`
	Temp        float64            `json:"temp"`
	FeelsLike   float64            `json:"feels_like"`
	Pressure    int                `json:"pressure"`
	Humidity    int                `json:"humidity"`
	DewPoint    float64            `json:"dew_point"`
	Uvi         float64            `json:"uvi"`
	Clouds      float64            `json:"clouds"`
	Visibility  int                `json:"visibility"`
	WindSpeed   float64            `json:"wind_speed"`
	WindGust    float64            `json:"wind_gust"`
	WindDeg     int                `json:"wind_deg"`
	Weather     []WeatherCondition `json:"weather"`
	Pop         float64            `json:"pop"`
	Rain        Rain               `json:"rain"`
}

type DailyWeather struct {
	Dt        int64              `json:"dt"`
	Sunrise   int64              `json:"sunrise"`
	Sunset    int64              `json:"sunset"`
	Moonrise  int64              `json:"moonrise"`
	Moonset   int64              `json:"moonset"`
	MoonPhase float64            `json:"moon_phase"`
	Summary   string             `json:"summary"`
	Temp      DailyTemperature   `json:"temp"`
	Pop       float64            `json:"pop"`
	Uvi       float64            `json:"uvi"`
	WindSpeed float64            `json:"wind_speed"`
	WindGust  float64            `json:"wind_gust"`
	Weather   []WeatherCondition `json:"weather"`
}

type DailyTemperature struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type WeatherCondition struct {
	ID          int    `json:"id"`
	Main        string `json:"main"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type Rain struct {
	OneH float64 `json:"1h"`
}

type WeatherAlert struct {
	SenderName  string   `json:"sender_name"`
	Event       string   `json:"event"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type weatherCacheEntry struct {
	Weather   WeatherData
	FetchedAt time.Time
}

func getWeatherWithCache(ctx context.Context) (WeatherData, error) {
	snapshot, err := getWeatherSnapshot(ctx, time.Now())
	return snapshot.Data, err
}

func getWeatherSnapshot(ctx context.Context, now time.Time) (WeatherSnapshot, error) {
	if cachedData, found := weatherCache.Get("weather"); found {
		cacheRequestsTotal.WithLabelValues("weather", "hit").Inc()
		entry := cachedData.(weatherCacheEntry)
		return WeatherSnapshot{Data: entry.Weather, FetchedAt: entry.FetchedAt}, nil
	}

	weatherRefreshMu.Lock()
	defer weatherRefreshMu.Unlock()
	if cachedData, found := weatherCache.Get("weather"); found {
		cacheRequestsTotal.WithLabelValues("weather", "hit_after_wait").Inc()
		entry := cachedData.(weatherCacheEntry)
		return WeatherSnapshot{Data: entry.Weather, FetchedAt: entry.FetchedAt}, nil
	}

	data, err := fetchWeatherFromAPI(ctx)
	if err != nil {
		apiRequestErrors.WithLabelValues("weather").Inc()
		if cachedData, found := weatherCache.Get(weatherCacheKeyLatest); found {
			entry := cachedData.(weatherCacheEntry)
			if now.Sub(entry.FetchedAt) <= maxWeatherStaleAge {
				cacheRequestsTotal.WithLabelValues("weather", "stale").Inc()
				logJSON(logEntry{Timestamp: now.Format(time.RFC3339), Level: "WARN", Message: fmt.Sprintf("Using cached weather data after refresh failure: %v", err)})
				return WeatherSnapshot{Data: entry.Weather, FetchedAt: entry.FetchedAt, Stale: true}, nil
			}
		}
		return WeatherSnapshot{}, err
	}
	cacheRequestsTotal.WithLabelValues("weather", "miss").Inc()

	entry := weatherCacheEntry{Weather: data, FetchedAt: now}
	weatherCache.Set("weather", entry, cache.DefaultExpiration)
	weatherCache.Set(weatherCacheKeyLatest, entry, cache.NoExpiration)
	return WeatherSnapshot{Data: data, FetchedAt: now}, nil
}

func fetchWeatherFromAPI(ctx context.Context) (WeatherData, error) {
	apiRequestsTotal.WithLabelValues("weather").Inc()
	requestStarted := time.Now()
	defer func() { apiRequestDuration.WithLabelValues("weather").Observe(time.Since(requestStarted).Seconds()) }()
	if strings.TrimSpace(weatherAPIURL) == "" {
		return WeatherData{}, fmt.Errorf("weather API URL is not configured")
	}

	req, err := http.NewRequest(http.MethodGet, weatherAPIURL, nil)
	if err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "build weather request", Err: err}
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "kindle-weather/1.0")

	resp, err := weatherHTTPClient.Do(req)
	if err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "GET weather data", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "GET weather data", Err: fmt.Errorf("status code %d", resp.StatusCode)}
	}

	var data WeatherData
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&data); err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "decode weather data", Err: err}
	}
	if err := validateWeatherData(data); err != nil {
		return WeatherData{}, &APIError{URL: weatherAPIURL, Operation: "validate weather data", Err: err}
	}

	roundWeatherData(&data)
	formatWeatherTimes(&data)

	return data, nil
}

func validateWeatherData(data WeatherData) error {
	if data.Current.Dt == 0 {
		return fmt.Errorf("current forecast is missing a timestamp")
	}
	if len(data.Current.Weather) == 0 || strings.TrimSpace(data.Current.Weather[0].Icon) == "" {
		return fmt.Errorf("current forecast is missing a weather condition")
	}
	if len(data.Daily) == 0 {
		return fmt.Errorf("daily forecast is empty")
	}
	if len(data.Hourly) == 0 {
		return fmt.Errorf("hourly forecast is empty")
	}
	for i, hour := range data.Hourly {
		if hour.Dt == 0 || len(hour.Weather) == 0 || strings.TrimSpace(hour.Weather[0].Icon) == "" {
			return fmt.Errorf("hourly forecast %d is incomplete", i)
		}
	}
	return nil
}

func getWeather(ctx context.Context) (WeatherData, error) {
	// Deprecated: use getWeatherWithCache/fetchWeatherFromAPI
	return fetchWeatherFromAPI(ctx)
}

func roundWeatherData(data *WeatherData) {
	// Round current weather data
	data.Current.Temp = math.Round(data.Current.Temp)
	data.Current.FeelsLike = math.Round(data.Current.FeelsLike)
	data.Current.WindSpeed = math.Round(data.Current.WindSpeed)
	data.Current.DewPoint = math.Round(data.Current.DewPoint)

	// Round hourly data
	for i := range data.Hourly {
		data.Hourly[i].Temp = math.Round(data.Hourly[i].Temp)
		data.Hourly[i].FeelsLike = math.Round(data.Hourly[i].FeelsLike)
		data.Hourly[i].Pressure = int(math.Round(float64(data.Hourly[i].Pressure)))
		data.Hourly[i].Humidity = int(math.Round(float64(data.Hourly[i].Humidity)))
		data.Hourly[i].DewPoint = math.Round(data.Hourly[i].DewPoint)
		data.Hourly[i].WindSpeed = math.Round(data.Hourly[i].WindSpeed)
		data.Hourly[i].WindGust = math.Round(data.Hourly[i].WindGust)
		data.Hourly[i].Pop = math.Round(data.Hourly[i].Pop * 100) // Convert probability to percentage and round
		// Rain remains in millimeters at full precision; convert when displaying.
	}
}

func (w *WeatherData) convertTime(unixTime int64) string {
	loc, err := time.LoadLocation(w.Timezone)
	if err != nil {
		// Fallback to using the offset if loading the location fails
		loc = time.FixedZone(w.Timezone, w.TimezoneOffset)
	}
	return time.Unix(unixTime, 0).In(loc).Format("3:04 PM")
}

func formatWeatherTimes(data *WeatherData) {
	data.Current.SunriseFormatted = data.convertTime(data.Current.Sunrise)
	data.Current.SunsetFormatted = data.convertTime(data.Current.Sunset)

	for i := range data.Hourly {
		data.Hourly[i].DtFormatted = data.convertTime(data.Hourly[i].Dt)
	}
}

func getForecastHours(hourly []HourlyWeather) []HourlyWeather {
	return getForecastHoursAt(hourly, time.Now())
}

func getForecastHoursAt(hourly []HourlyWeather, now time.Time) []HourlyWeather {
	var result []HourlyWeather
	targetHours := []int{2, 4, 6, 8}
	selected := make(map[int64]bool, len(targetHours))

	for _, targetHour := range targetHours {
		targetTime := now.Add(time.Duration(targetHour) * time.Hour)
		var closestHour HourlyWeather
		smallestDiff := time.Duration(math.MaxInt64)

		for _, h := range hourly {
			if selected[h.Dt] {
				continue
			}
			forecastTime := time.Unix(h.Dt, 0)
			diff := forecastTime.Sub(targetTime).Abs()
			if diff < smallestDiff {
				smallestDiff = diff
				closestHour = h
			}
		}

		if closestHour.Dt != 0 { // Check if a valid hour was found
			result = append(result, closestHour)
			selected[closestHour.Dt] = true
		}
	}
	return result
}

func getIconClassName(icon string, id int) string {
	if icon == "" {
		return "wi wi-na"
	}
	isNight := string(icon[len(icon)-1]) == "n"
	if isNight && id == 800 {
		return "wi wi-night-clear"
	}
	if isNight {
		return fmt.Sprintf("wi wi-owm-night-%d", id)
	}
	return fmt.Sprintf("wi wi-owm-day-%d", id)
}

func getMoonPhaseIcon(moonPhase float64) string {
	switch {
	case moonPhase == 0:
		return "wi-moon-new"
	case moonPhase < 0.25:
		return "wi-moon-waxing-crescent-3"
	case moonPhase == 0.25:
		return "wi-moon-first-quarter"
	case moonPhase < 0.5:
		return "wi-moon-waxing-gibbous-3"
	case moonPhase == 0.5:
		return "wi-moon-full"
	case moonPhase < 0.75:
		return "wi-moon-waning-gibbous-3"
	case moonPhase == 0.75:
		return "wi-moon-third-quarter"
	default:
		return "wi-moon-waning-crescent-3"
	}
}
