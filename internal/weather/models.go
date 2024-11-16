package weather

import "time"

// WeatherResponse represents the structure of the weather API response.
type WeatherResponse struct {
	Current  CurrentWeather   `json:"current"`
	Hourly   []HourlyWeather   `json:"hourly"`
	Daily    []DailyWeather    `json:"daily"`
}

// CurrentWeather represents the current weather conditions.
type CurrentWeather struct {
	Temperature float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
	Description string  `json:"weather_description"`
	Sunrise     int64   `json:"sunrise"`
	Sunset      int64   `json:"sunset"`
}

// HourlyWeather represents the weather conditions for each hour.
type HourlyWeather struct {
	Time        int64   `json:"dt"`
	Temperature float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Description string  `json:"weather_description"`
}

// DailyWeather represents the weather conditions for each day.
type DailyWeather struct {
	Date        int64   `json:"dt"`
	Temperature struct {
		Day   float64 `json:"day"`
		Night float64 `json:"night"`
	} `json:"temp"`
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
}

// TideResponse represents the structure of the tide API response.
type TideResponse struct {
	Predictions []TidePrediction `json:"predictions"`
}

// TidePrediction represents the tide data for a specific time.
type TidePrediction struct {
	Time  string  `json:"t"`
	Level float64 `json:"v"`
}

// MoonPhaseResponse represents the structure of the moon phase response.
type MoonPhaseResponse struct {
	Phase       string `json:"phase"`
	Description string `json:"description"`
	Date        string `json:"date"`
}

// WeatherCache is a struct that holds cached weather and tide data.
type WeatherCache struct {
	Weather WeatherResponse
	Tides   TideResponse
	LastFetched time.Time
}

