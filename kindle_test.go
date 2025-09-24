package main

import (
	"testing"
	"time"
)

func TestGetMoonPhaseIcon(t *testing.T) {
	tests := []struct {
		phase    float64
		expected string
	}{
		{0, "wi-moon-new"},
		{0.2, "wi-moon-waxing-crescent-3"},
		{0.25, "wi-moon-first-quarter"},
		{0.4, "wi-moon-waxing-gibbous-3"},
		{0.5, "wi-moon-full"},
		{0.6, "wi-moon-waning-gibbous-3"},
		{0.75, "wi-moon-third-quarter"},
		{0.8, "wi-moon-waning-crescent-3"},
	}

	for _, tt := range tests {
		result := getMoonPhaseIcon(tt.phase)
		if result != tt.expected {
			t.Errorf("getMoonPhaseIcon(%f) = %s; want %s", tt.phase, result, tt.expected)
		}
	}
}

func TestRoundWeatherData(t *testing.T) {
	data := &WeatherData{
		Current: CurrentWeather{
			Temp:      72.6,
			FeelsLike: 73.4,
		},
		Hourly: []HourlyWeather{
			{
				Temp:      75.7,
				FeelsLike: 76.3,
				Pop:       0.456,
			},
		},
	}

	roundWeatherData(data)

	if data.Current.Temp != 73 {
		t.Errorf("Current.Temp = %f; want 73", data.Current.Temp)
	}
	if data.Current.FeelsLike != 73 {
		t.Errorf("Current.FeelsLike = %f; want 73", data.Current.FeelsLike)
	}
	if data.Hourly[0].Temp != 76 {
		t.Errorf("Hourly[0].Temp = %f; want 76", data.Hourly[0].Temp)
	}
	if data.Hourly[0].Pop != 46 {
		t.Errorf("Hourly[0].Pop = %f; want 46", data.Hourly[0].Pop)
	}
}

func TestGetForecastHours(t *testing.T) {
	now := time.Now()
	hourly := []HourlyWeather{
		{Dt: now.Add(2 * time.Hour).Unix()},
		{Dt: now.Add(4 * time.Hour).Unix()},
		{Dt: now.Add(6 * time.Hour).Unix()},
		{Dt: now.Add(8 * time.Hour).Unix()},
	}

	result := getForecastHours(hourly)

	if len(result) != 4 {
		t.Errorf("getForecastHours() returned %d items; want 4", len(result))
	}
}

func TestGetIconClassName(t *testing.T) {
	tests := []struct {
		name     string
		icon     string
		id       int
		expected string
	}{
		{"night clear sky", "01n", 800, "wi wi-night-clear"},
		{"night cloudy", "02n", 801, "wi wi-owm-night-801"},
		{"day clear sky", "01d", 800, "wi wi-owm-day-800"},
		{"day rain", "10d", 500, "wi wi-owm-day-500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIconClassName(tt.icon, tt.id)
			if result != tt.expected {
				t.Errorf("getIconClassName(%q, %d) = %q; want %q",
					tt.icon, tt.id, result, tt.expected)
			}
		})
	}
}

func TestProcessTideData(t *testing.T) {
	rawData := struct {
		Predictions []struct {
			Time   string  `json:"t"`
			Type   string  `json:"type"`
			Height float64 `json:"v"`
		} `json:"predictions"`
	}{
		Predictions: []struct {
			Time   string  `json:"t"`
			Type   string  `json:"type"`
			Height float64 `json:"v"`
		}{
			{Time: "2024-01-01 13:45", Type: "H", Height: 4.2},
			{Time: "2024-01-01 19:30", Type: "L", Height: 0.2},
		},
	}

	data, err := processTideData(rawData)
	if err != nil {
		t.Fatalf("processTideData() error = %v", err)
	}

	if len(data.Predictions) != 2 {
		t.Errorf("got %d predictions; want 2", len(data.Predictions))
	}

	tests := []struct {
		idx  int
		want string
		typ  string
	}{
		{0, "1:45 PM", "H"},
		{1, "7:30 PM", "L"},
	}

	for _, tt := range tests {
		if data.Predictions[tt.idx].Time != tt.want {
			t.Errorf("Prediction[%d].Time = %q; want %q",
				tt.idx, data.Predictions[tt.idx].Time, tt.want)
		}
		if data.Predictions[tt.idx].Type != tt.typ {
			t.Errorf("Prediction[%d].Type = %q; want %q",
				tt.idx, data.Predictions[tt.idx].Type, tt.typ)
		}
	}
}
