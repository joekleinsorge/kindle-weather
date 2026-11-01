package main

import (
	"strings"
	"testing"
	"time"
)

func overviewTestTime(t *testing.T, hour, minute int) (time.Time, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return time.Date(2026, 10, 15, hour, minute, 0, 0, loc), loc
}

func hourlyFromPops(start time.Time, pops ...float64) []HourlyWeather {
	var hours []HourlyWeather
	for i, pop := range pops {
		hours = append(hours, HourlyWeather{Dt: start.Truncate(time.Hour).Add(time.Duration(i) * time.Hour).Unix(), Pop: pop})
	}
	return hours
}

func TestBuildRainBand(t *testing.T) {
	now, loc := overviewTestTime(t, 15, 27)
	tests := []struct {
		name                        string
		pops                        []float64
		label, big, suffix, caption string
	}{
		{"rain later", []float64{5, 5, 10, 10, 15, 20, 30, 55, 70, 65, 40, 25}, "RAIN STARTS", "10", "PM", "70% at 11p · dry by 1a"},
		{"raining now", []float64{80, 90, 60, 30, 10, 5, 5, 5, 5, 5, 5, 5}, "RAIN UNTIL", "6", "PM", "90% at 4p"},
		{"dry", []float64{0, 5, 10, 20, 10, 0, 0, 0, 0, 0, 0, 0}, "NEXT 12 HOURS", "Dry", "", "20% at most"},
		{"rain all window", []float64{10, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60}, "RAIN STARTS", "4", "PM", "60% at 4p · past 3a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			band := buildRainBand(hourlyFromPops(now, tt.pops...), now, loc)
			if band == nil {
				t.Fatal("expected a rain band")
			}
			if band.Label != tt.label || band.Big != tt.big || band.Suffix != tt.suffix || band.Caption != tt.caption {
				t.Fatalf("got %q %q %q %q", band.Label, band.Big, band.Suffix, band.Caption)
			}
			if len(band.Bars) != len(tt.pops) {
				t.Fatalf("got %d bars, want %d", len(band.Bars), len(tt.pops))
			}
		})
	}
	if band := buildRainBand(nil, now, loc); band != nil {
		t.Fatalf("expected no band without hourly data, got %+v", band)
	}
}

func TestBuildRainBandBarsSkipPastHoursAndClassifyChance(t *testing.T) {
	now, loc := overviewTestTime(t, 15, 27)
	hours := hourlyFromPops(now.Add(-2*time.Hour), 90, 90, 0, 30, 70)
	band := buildRainBand(hours, now, loc)
	if len(band.Bars) != 3 {
		t.Fatalf("expected past hours to be skipped, got %d bars", len(band.Bars))
	}
	want := []RainBar{{Bucket: 0, Level: "dry"}, {Bucket: 3, Level: "maybe"}, {Bucket: 7, Level: "wet"}}
	for i := range want {
		if band.Bars[i] != want[i] {
			t.Fatalf("bar %d = %+v, want %+v", i, band.Bars[i], want[i])
		}
	}
}

func todayTides() []TidePrediction {
	return []TidePrediction{
		{Time: "12:16 AM", Type: "H", Height: 3.5, Date: "2026-10-15"},
		{Time: "5:57 AM", Type: "L", Height: 0.4, Date: "2026-10-15"},
		{Time: "12:49 PM", Type: "H", Height: 3.7, Date: "2026-10-15"},
		{Time: "6:56 PM", Type: "L", Height: 0.3, Date: "2026-10-15"},
	}
}

func TestBuildTideBand(t *testing.T) {
	now, loc := overviewTestTime(t, 15, 27)
	band := buildTideBand(todayTides(), now, loc)
	if band == nil {
		t.Fatal("expected a tide band")
	}
	if band.Label != "LOW TIDE · FALLING" || band.Big != "6:56" || band.Suffix != "PM" {
		t.Fatalf("got %q %q %q", band.Label, band.Big, band.Suffix)
	}
	if band.Caption != "in 3½ hrs · high 1:08a" {
		t.Fatalf("caption = %q", band.Caption)
	}
	spark := string(band.Spark)
	if !strings.HasPrefix(spark, `<svg viewBox="0 0 220 64"`) || !strings.Contains(spark, "<circle") {
		t.Fatalf("unexpected sparkline: %s", spark)
	}
}

func TestBuildTideBandEstimatesAfterLastTideOfDay(t *testing.T) {
	now, loc := overviewTestTime(t, 21, 40)
	band := buildTideBand(todayTides(), now, loc)
	if band == nil || band.Label != "HIGH TIDE · RISING" || band.Big != "1:08" || band.Suffix != "AM" {
		t.Fatalf("got %+v", band)
	}
	if !strings.HasPrefix(band.Caption, "in about 3½ hrs") {
		t.Fatalf("expected an estimated caption, got %q", band.Caption)
	}
}

func TestBuildTideBandWithoutData(t *testing.T) {
	now, loc := overviewTestTime(t, 15, 27)
	if band := buildTideBand(nil, now, loc); band != nil {
		t.Fatalf("expected no tide band, got %+v", band)
	}
}

func TestUntilText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		3 * time.Minute:              "in 5 min",
		42 * time.Minute:             "in 40 min",
		time.Hour:                    "in 1 hr",
		80 * time.Minute:             "in 1½ hrs",
		3*time.Hour + 29*time.Minute: "in 3½ hrs",
		6*time.Hour + 5*time.Minute:  "in 6 hrs",
	} {
		if got := untilText(d); got != want {
			t.Errorf("untilText(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestShortClockAndHourParts(t *testing.T) {
	now, _ := overviewTestTime(t, 23, 0)
	if got := shortClock(now); got != "11p" {
		t.Fatalf("shortClock = %q", got)
	}
	if got := shortClock(now.Add(65 * time.Minute)); got != "12:05a" {
		t.Fatalf("shortClock = %q", got)
	}
	if big, suffix := hourParts(now); big != "11" || suffix != "PM" {
		t.Fatalf("hourParts = %q %q", big, suffix)
	}
	if big, suffix := hourParts(now.Add(-30 * time.Minute)); big != "10:30" || suffix != "PM" {
		t.Fatalf("hourParts = %q %q", big, suffix)
	}
}

func TestMoonDescription(t *testing.T) {
	for phase, want := range map[float64][2]string{
		0:    {"0%", "new moon"},
		0.17: {"26%", "crescent"},
		0.25: {"50%", "first quarter"},
		0.5:  {"100%", "full moon"},
		0.62: {"86%", "gibbous"},
		0.9:  {"10%", "crescent"},
	} {
		lit, name := moonDescription(phase)
		if lit != want[0] || name != want[1] {
			t.Errorf("moonDescription(%v) = %q %q, want %q %q", phase, lit, name, want[0], want[1])
		}
	}
}

func TestNextSunEvent(t *testing.T) {
	_, loc := overviewTestTime(t, 0, 0)
	weather := WeatherData{
		Current: CurrentWeather{
			Sunrise: time.Date(2026, 10, 15, 7, 17, 0, 0, loc).Unix(),
			Sunset:  time.Date(2026, 10, 15, 19, 10, 0, 0, loc).Unix(),
		},
		Daily: []DailyWeather{{}, {Sunrise: time.Date(2026, 10, 16, 7, 18, 0, 0, loc).Unix()}},
	}
	for _, tt := range []struct {
		hour         int
		label, value string
	}{{5, "sunrise", "7:17"}, {12, "sunset", "7:10"}, {21, "sunrise", "7:18"}} {
		now, _ := overviewTestTime(t, tt.hour, 0)
		if label, value := nextSunEvent(weather, now, loc); label != tt.label || value != tt.value {
			t.Errorf("at %d:00 got %q %q, want %q %q", tt.hour, label, value, tt.label, tt.value)
		}
	}
}

func TestBeachStatusBandFields(t *testing.T) {
	now, loc := overviewTestTime(t, 6, 30)
	window := &SurfWindow{
		Start: time.Date(2026, 10, 15, 7, 17, 0, 0, loc), End: time.Date(2026, 10, 15, 11, 0, 0, 0, loc),
		WaveHeight: 3, WavePeriod: 9, WindSpeed: 5,
	}
	status := getBeachStatusWithWindow(nil, window, now)
	if status.Label != "GOOD SURF TODAY" || status.Big != "7–11" || status.Suffix != "AM" || status.Detail != "3.0 ft @ 9 s\nwind 5 mph" {
		t.Fatalf("upcoming window: %+v", status)
	}

	status = getBeachStatusWithWindow(nil, window, now.Add(2*time.Hour))
	if status.Label != "GOOD SURF UNTIL" || status.Big != "11" || status.Suffix != "AM" {
		t.Fatalf("current window: %+v", status)
	}
}

func TestOverviewNoticesDropRain(t *testing.T) {
	got := overviewNotices([]Notice{{Kind: "rain", Text: "Rain likely"}, {Kind: "uv", Text: "High UV"}})
	if len(got) != 1 || got[0].Kind != "uv" {
		t.Fatalf("got %+v", got)
	}
}

func TestIndexTemplateRendersOverviewBands(t *testing.T) {
	now, loc := overviewTestTime(t, 15, 27)
	view := DashboardView{
		CurrentTemperature: "89°",
		CurrentIconClass:   "wi wi-owm-day-802",
		FeelsLike:          "96",
		NowLabel:           "THU 3:27 PM",
		MoonPhaseIcon:      "wi-moon-waxing-crescent-3",
		Rain:               buildRainBand(hourlyFromPops(now, 5, 5, 10, 10, 15, 20, 30, 55, 70, 65, 40, 25), now, loc),
		Tide:               buildTideBand(todayTides(), now, loc),
		AutoRefreshURL:     "/",
	}
	view.MoonPercent, view.MoonName = moonDescription(0.17)
	var buf strings.Builder
	if err := tmpl.Execute(&buf, view); err != nil {
		t.Fatal(err)
	}
	rendered := buf.String()
	for _, want := range []string{
		`id="rain-band"`, `RAIN STARTS`, `<span class="band-suffix"> PM</span>`, `class="rain-bar h7 wet"`,
		`id="tide-band"`, `LOW TIDE · FALLING`, `6:56`, `<svg viewBox="0 0 220 64"`,
		`feels 96`, `THU 3:27 PM`, `26%<span class="foot-sub">crescent</span>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered overview missing %q", want)
		}
	}

	view.Tide = nil
	buf.Reset()
	if err := tmpl.Execute(&buf, view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Tide data unavailable") {
		t.Fatal("expected a tide fallback when tide data is missing")
	}
}
