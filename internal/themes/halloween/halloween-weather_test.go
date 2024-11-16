package halloween

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
			contains: []string{"vampire", "zombie"},
		},
		{
			name:     "Rainy day",
			input:    "Expect a day with rain",
			contains: []string{"witch", "ghost"},
		},
		{
			name:     "Cloudy and windy",
			input:    "Expect a cloudy and windy day",
			contains: []string{"werewolf", "banshee", "skeleton"},
		},
		{
			name:     "Unknown weather",
			input:    "Expect a day",
			contains: []string{"mysteriously nondescript halloween forecast"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ThemeWeather(tc.input)
			found := false
			for _, phrase := range tc.contains {
				if strings.Contains(strings.ToLower(result), phrase) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected result to contain one of '%v', but got: %s", tc.contains, result)
			}
		})
	}
}
func TestThemeWeatherRandomness(t *testing.T) {
	input := "Expect a sunny and rainy day"
	results := make(map[string]bool)

	for i := 0; i < 100; i++ {
		result := ThemeWeather(input)
		results[result] = true
	}

	if len(results) < 2 {
		t.Errorf("Expected multiple unique results due to randomness, but got %d unique results", len(results))
	}
}
