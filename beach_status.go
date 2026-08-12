package main

import (
	"fmt"
	"time"
)

const (
	superLowTideMaxFeet = 0.0
	wakingHoursStart    = 7
	wakingHoursEnd      = 19
)

type BeachStatus struct {
	Kind string
	Text string
}

func getBeachStatus(predictions []TidePrediction, goodSurfToday bool, now time.Time) *BeachStatus {
	if tide := upcomingSuperLowTide(predictions, now); tide != nil {
		return &BeachStatus{
			Kind: "tide",
			Text: fmt.Sprintf("Super low tide at %s", tide.Time),
		}
	}
	if goodSurfToday {
		return &BeachStatus{Kind: "surf", Text: "Good surf today"}
	}
	return nil
}

func upcomingSuperLowTide(predictions []TidePrediction, now time.Time) *TidePrediction {
	loc := easternLocation()
	localNow := now.In(loc)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), wakingHoursStart, 0, 0, 0, loc)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), wakingHoursEnd, 0, 0, 0, loc)

	var next *TidePrediction
	var nextTime time.Time
	for i := range predictions {
		prediction := predictions[i]
		if prediction.Type != "L" || prediction.Height > superLowTideMaxFeet {
			continue
		}

		clockTime, err := time.Parse("3:04 PM", prediction.Time)
		if err != nil {
			continue
		}
		predictionTime := time.Date(
			localNow.Year(), localNow.Month(), localNow.Day(),
			clockTime.Hour(), clockTime.Minute(), 0, 0, loc,
		)
		if predictionTime.Before(localNow) || predictionTime.Before(start) || predictionTime.After(end) {
			continue
		}
		if next == nil || predictionTime.Before(nextTime) {
			next = &predictions[i]
			nextTime = predictionTime
		}
	}

	return next
}

func easternLocation() *time.Location {
	if loc, err := time.LoadLocation("America/New_York"); err == nil {
		return loc
	}
	return time.FixedZone("Eastern", -5*60*60)
}
