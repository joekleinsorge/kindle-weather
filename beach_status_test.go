package main

import (
	"testing"
	"time"
)

func TestUpcomingSuperLowTide(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.August, 11, 8, 0, 0, 0, loc)

	tests := []struct {
		name        string
		predictions []TidePrediction
		wantTime    string
	}{
		{
			name: "next negative daytime low tide",
			predictions: []TidePrediction{
				{Time: "6:30 AM", Type: "L", Height: -0.4},
				{Time: "1:45 PM", Type: "L", Height: -0.2},
			},
			wantTime: "1:45 PM",
		},
		{
			name: "zero feet qualifies",
			predictions: []TidePrediction{
				{Time: "10:15 AM", Type: "L", Height: 0},
			},
			wantTime: "10:15 AM",
		},
		{
			name: "ordinary low tide does not qualify",
			predictions: []TidePrediction{
				{Time: "10:15 AM", Type: "L", Height: 0.1},
			},
		},
		{
			name: "past low tide does not qualify",
			predictions: []TidePrediction{
				{Time: "7:30 AM", Type: "L", Height: -0.3},
			},
		},
		{
			name: "low tide after waking hours does not qualify",
			predictions: []TidePrediction{
				{Time: "7:30 PM", Type: "L", Height: -0.3},
			},
		},
		{
			name: "high tide does not qualify",
			predictions: []TidePrediction{
				{Time: "10:15 AM", Type: "H", Height: -0.2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upcomingSuperLowTide(tt.predictions, now)
			if tt.wantTime == "" {
				if got != nil {
					t.Fatalf("upcomingSuperLowTide() = %+v; want nil", got)
				}
				return
			}
			if got == nil || got.Time != tt.wantTime {
				t.Fatalf("upcomingSuperLowTide() = %+v; want time %q", got, tt.wantTime)
			}
		})
	}
}

func TestGetBeachStatus_TideTakesPrecedenceOverSurf(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.August, 11, 8, 0, 0, 0, loc)
	predictions := []TidePrediction{{Time: "1:45 PM", Type: "L", Height: -0.2}}

	status := getBeachStatus(predictions, true, now)
	if status == nil || status.Kind != "tide" || status.Text != "Super low tide at 1:45 PM" {
		t.Fatalf("getBeachStatus() = %+v; want upcoming tide status", status)
	}
}

func TestGetBeachStatus_FallsBackToSurf(t *testing.T) {
	status := getBeachStatus(nil, true, time.Now())
	if status == nil || status.Kind != "surf" || status.Text != "Good surf today" {
		t.Fatalf("getBeachStatus() = %+v; want surf status", status)
	}
}
