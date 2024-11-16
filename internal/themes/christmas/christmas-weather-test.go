package christmas

import (
	"strings"
	"testing"
)

func TestThemeWeather(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "Sunny day",
			input:    "Expect a sunny day",
			contains: []string{"Santa", "sleigh", "snowmen"},
		},
		{
			name:     "Rainy day",
			input:    "Expect a day with rain",
			contains: []string{"tinsel", "ornament"},
		},
		{
			name:     "Cloudy and windy",
			input:    "Expect a cloudy and windy day",
			contains: []string{"cotton candy", "North Pole", "jingle bells"},
		},
		{
			name:     "Snowy day",
			input:    "Expect a snowy day",
			contains: []string{"Christmas powder", "Frosty"},
		},
		{
			name:     "Unknown weather",
			input:    "Expect a day",
			contains: []string{"magically merry Christmas forecast", "Santa"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ThemeWeather(tc.input)
			for _, phrase := range tc.contains {
				if !strings.Contains(strings.ToLower(result), strings.ToLower(phrase)) {
					t.Errorf("Expected result to contain '%s', but got: %s", phrase, result)
				}
			}
			if !strings.Contains(result, "Ho ho ho!") {
				t.Errorf("Expected result to start with 'Ho ho ho!', but got: %s", result)
			}
		})
	}
}

func TestThemeWeatherRandomness(t *testing.T) {
	input := "Expect a sunny and snowy day"
	results := make(map[string]bool)

	for i := 0; i < 100; i++ {
		result := ThemeWeather(input)
		results[result] = true
	}

	if len(results) < 2 {
		t.Errorf("Expected multiple unique results due to randomness, but got %d unique results", len(results))
	}
}
