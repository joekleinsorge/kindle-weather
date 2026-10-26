package main

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"
	"time"
)

// The overview answers three questions in large type: when does rain start,
// when is the next tide, and is there good surf. Each answer is a "band" with
// a short label, one big value, and a one-line caption.

const (
	rainBandHours     = 12
	rainLikelyPercent = 50
	rainMaybePercent  = 25
	// Average time between a high and the following low (or vice versa) for a
	// semidiurnal tide. Used only to estimate the next event after today's last one.
	halfTidalCycle = 6*time.Hour + 12*time.Minute + 30*time.Second
)

type RainBar struct {
	Bucket int    // 0–10, chance of rain in tens of percent
	Level  string // "dry", "maybe" or "wet"
}

type RainBand struct {
	Label      string
	Big        string
	Suffix     string
	Caption    string
	Bars       []RainBar
	Likely     bool // rain is likely within the window; only then is the band drawn in solid black
	StartLabel string
	EndLabel   string
}

type TideBand struct {
	Label   string
	Big     string
	Suffix  string
	Caption string
	Spark   template.HTML
}

type tideEvent struct {
	Time   time.Time
	Type   string
	Height float64
}

func buildRainBand(hourly []HourlyWeather, now time.Time, loc *time.Location) *RainBand {
	start := now.Truncate(time.Hour)
	var hours []HourlyWeather
	for _, hour := range hourly {
		if time.Unix(hour.Dt, 0).Before(start) {
			continue
		}
		hours = append(hours, hour)
		if len(hours) == rainBandHours {
			break
		}
	}
	if len(hours) == 0 {
		return nil
	}

	band := &RainBand{StartLabel: shortClock(time.Unix(hours[0].Dt, 0).In(loc)), EndLabel: shortClock(time.Unix(hours[len(hours)-1].Dt, 0).In(loc))}
	wet := func(h HourlyWeather) bool { return h.Pop >= rainLikelyPercent || h.Rain.OneH > 0 }
	startIdx, peakIdx := -1, 0
	for i, hour := range hours {
		level := "dry"
		switch {
		case wet(hour):
			level = "wet"
		case hour.Pop >= rainMaybePercent:
			level = "maybe"
		}
		bucket := int(math.Round(hour.Pop / 10))
		band.Bars = append(band.Bars, RainBar{Bucket: min(max(bucket, 0), 10), Level: level})
		if startIdx < 0 && wet(hour) {
			startIdx = i
		}
		if hour.Pop > hours[peakIdx].Pop {
			peakIdx = i
		}
	}

	if startIdx < 0 {
		band.Label = fmt.Sprintf("NEXT %d HOURS", len(hours))
		band.Big = "Dry"
		band.Caption = "no rain expected"
		if peak := hours[peakIdx].Pop; peak > 0 {
			band.Caption = fmt.Sprintf("%.0f%% at most", peak)
			if peak >= rainMaybePercent {
				band.Big = "Possible"
				band.Caption = fmt.Sprintf("%.0f%% at %s", peak, shortClock(time.Unix(hours[peakIdx].Dt, 0).In(loc)))
			}
		}
		return band
	}

	band.Likely = true
	endIdx := -1
	peakIdx = startIdx
	for i := startIdx; i < len(hours); i++ {
		if !wet(hours[i]) {
			endIdx = i
			break
		}
		if hours[i].Pop > hours[peakIdx].Pop {
			peakIdx = i
		}
	}
	startTime := time.Unix(hours[startIdx].Dt, 0).In(loc)
	peak := fmt.Sprintf("%.0f%% at %s", hours[peakIdx].Pop, shortClock(time.Unix(hours[peakIdx].Dt, 0).In(loc)))
	lastTime := time.Unix(hours[len(hours)-1].Dt, 0).In(loc).Add(time.Hour)

	if !startTime.After(now) {
		// Raining (or very likely) in the current hour: the useful answer is when it stops.
		band.Label = "RAIN LIKELY UNTIL"
		if endIdx < 0 {
			band.Big, band.Suffix = hourParts(lastTime)
			band.Label = "RAIN LIKELY THROUGH"
			band.Caption = peak
			return band
		}
		band.Big, band.Suffix = hourParts(time.Unix(hours[endIdx].Dt, 0).In(loc))
		band.Caption = peak
		return band
	}

	band.Label = "RAIN LIKELY FROM"
	band.Big, band.Suffix = hourParts(startTime)
	if endIdx < 0 {
		band.Caption = fmt.Sprintf("%s · past %s", peak, shortClock(lastTime))
	} else {
		band.Caption = fmt.Sprintf("%s · easing %s", peak, shortClock(time.Unix(hours[endIdx].Dt, 0).In(loc)))
	}
	return band
}

func buildTideBand(predictions []TidePrediction, now time.Time, loc *time.Location) *TideBand {
	events := parseTideEvents(predictions, now, loc)
	if len(events) == 0 {
		return nil
	}

	next, following, estimated := nextTideEvents(events, now)
	if next == nil {
		return nil
	}

	band := &TideBand{Label: "LOW TIDE · FALLING"}
	if next.Type == "H" {
		band.Label = "HIGH TIDE · RISING"
	}
	band.Big, band.Suffix = next.Time.Format("3:04"), next.Time.Format("PM")
	band.Caption = untilText(next.Time.Sub(now))
	if estimated {
		band.Caption = strings.Replace(band.Caption, "in ", "in about ", 1)
		band.Label = "EST. " + band.Label
	}
	if following != nil {
		kind := "low"
		if following.Type == "H" {
			kind = "high"
		}
		marker := ""
		if estimated || !containsTideEvent(events, following.Time) {
			marker = "~"
		}
		band.Caption += fmt.Sprintf(" · %s %s%s", kind, marker, shortClock(following.Time))
	}
	band.Spark = tideSparkline(events, now, loc)
	return band
}

func parseTideEvents(predictions []TidePrediction, now time.Time, loc *time.Location) []tideEvent {
	today := now.In(loc).Format("2006-01-02")
	var events []tideEvent
	for _, p := range predictions {
		date := p.Date
		if date == "" {
			date = today
		}
		t, err := time.ParseInLocation("2006-01-02 3:04 PM", date+" "+p.Time, loc)
		if err != nil {
			continue
		}
		events = append(events, tideEvent{Time: t, Type: p.Type, Height: p.Height})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events
}

// nextTideEvents returns the next tide after now and the one after it. NOAA is
// queried for today only, so after the last tide of the day the next ones are
// estimated from the average half tidal cycle.
func containsTideEvent(events []tideEvent, t time.Time) bool {
	for _, event := range events {
		if event.Time.Equal(t) {
			return true
		}
	}
	return false
}

func nextTideEvents(events []tideEvent, now time.Time) (*tideEvent, *tideEvent, bool) {
	if len(events) == 0 {
		return nil, nil, false
	}
	for i := range events {
		if events[i].Time.After(now) {
			if i+1 < len(events) {
				return &events[i], &events[i+1], false
			}
			following := opposite(events[i])
			return &events[i], &following, false
		}
	}
	last := events[len(events)-1]
	next := opposite(last)
	for !next.Time.After(now) {
		next = opposite(next)
	}
	following := opposite(next)
	return &next, &following, true
}

func opposite(e tideEvent) tideEvent {
	next := tideEvent{Time: e.Time.Add(halfTidalCycle), Type: "H", Height: e.Height}
	if e.Type == "H" {
		next.Type = "L"
	}
	return next
}

// tideSparkline draws today's tide curve (midnight to midnight) with a marker
// at the current time. Heights between NOAA highs and lows follow a cosine.
func tideSparkline(events []tideEvent, now time.Time, loc *time.Location) template.HTML {
	if len(events) < 2 {
		return ""
	}
	localNow := now.In(loc)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	dayLength := dayEnd.Sub(dayStart)

	// Extend the curve past both ends of the day by mirroring the nearest cycle.
	points := append([]tideEvent(nil), events...)
	for points[0].Time.After(dayStart) {
		gap := points[1].Time.Sub(points[0].Time)
		points = append([]tideEvent{{Time: points[0].Time.Add(-gap), Height: points[1].Height}}, points...)
	}
	for points[len(points)-1].Time.Before(dayEnd) {
		n := len(points)
		gap := points[n-1].Time.Sub(points[n-2].Time)
		points = append(points, tideEvent{Time: points[n-1].Time.Add(gap), Height: points[n-2].Height})
	}

	lo, hi := points[0].Height, points[0].Height
	for _, p := range points {
		lo, hi = math.Min(lo, p.Height), math.Max(hi, p.Height)
	}
	if hi-lo < 0.1 {
		hi = lo + 0.1
	}
	heightAt := func(t time.Time) float64 {
		for i := 1; i < len(points); i++ {
			a, b := points[i-1], points[i]
			if t.After(b.Time) {
				continue
			}
			f := t.Sub(a.Time).Seconds() / b.Time.Sub(a.Time).Seconds()
			return a.Height + (b.Height-a.Height)*(1-math.Cos(math.Pi*f))/2
		}
		return points[len(points)-1].Height
	}

	const width, height, pad = 220.0, 64.0, 6.0
	y := func(h float64) float64 { return pad + (height-2*pad)*(1-(h-lo)/(hi-lo)) }
	var path strings.Builder
	const samples = 48
	for i := 0; i <= samples; i++ {
		t := dayStart.Add(time.Duration(i) * dayLength / samples)
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&path, "%s%.1f,%.1f ", cmd, width*float64(i)/samples, y(heightAt(t)))
	}
	fmt.Fprintf(&path, "L%.0f,%.0f L0,%.0f Z", width, height, height)

	nowX := width * float64(localNow.Sub(dayStart)) / float64(dayLength)
	nowY := y(heightAt(localNow))
	return template.HTML(fmt.Sprintf(
		`<svg viewBox="0 0 220 64" width="220" height="64" preserveAspectRatio="none" aria-hidden="true">`+
			`<path d="%s" fill="#aaa" stroke="#000" stroke-width="2.5" stroke-linejoin="round"></path>`+
			`<line x1="%.1f" y1="0" x2="%.1f" y2="64" stroke="#000" stroke-width="2.5"></line>`+
			`<circle cx="%.1f" cy="%.1f" r="5" fill="#fff" stroke="#000" stroke-width="2.5"></circle>`+
			`</svg>`,
		path.String(), nowX, nowX, nowX, nowY))
}

// moonDescription turns OpenWeather's 0–1 moon phase into "27%" and "crescent".
func moonDescription(phase float64) (string, string) {
	lit := (1 - math.Cos(2*math.Pi*phase)) / 2 * 100
	var name string
	switch {
	case phase < 0.03 || phase > 0.97:
		name = "new moon"
	case phase < 0.22 || phase > 0.78:
		name = "crescent"
	case phase < 0.28:
		name = "first quarter"
	case phase > 0.72:
		name = "last quarter"
	case phase < 0.47 || phase > 0.53:
		name = "gibbous"
	default:
		name = "full moon"
	}
	return fmt.Sprintf("%.0f%%", lit), name
}

// nextSunEvent picks whichever of sunrise or sunset comes next.
func nextSunEvent(weather WeatherData, now time.Time, loc *time.Location) (string, string) {
	sunrise := time.Unix(weather.Current.Sunrise, 0).In(loc)
	sunset := time.Unix(weather.Current.Sunset, 0).In(loc)
	switch {
	case weather.Current.Sunrise == 0 || weather.Current.Sunset == 0:
		return "", ""
	case now.Before(sunrise):
		return "sunrise", sunrise.Format("3:04 PM")
	case now.Before(sunset):
		return "sunset", sunset.Format("3:04 PM")
	case len(weather.Daily) > 1 && weather.Daily[1].Sunrise != 0:
		return "sunrise", time.Unix(weather.Daily[1].Sunrise, 0).In(loc).Format("3:04 PM")
	default:
		return "sunrise", sunrise.Format("3:04 PM")
	}
}

// hourParts splits a time into the big number and its AM/PM suffix: "10", "PM".
func hourParts(t time.Time) (string, string) {
	if t.Minute() == 0 {
		return t.Format("3"), t.Format("PM")
	}
	return t.Format("3:04"), t.Format("PM")
}

// shortClock formats compact caption times: "11p", "1:05a".
func shortClock(t time.Time) string {
	layout := "3PM"
	if t.Minute() != 0 {
		layout = "3:04PM"
	}
	s := strings.ToLower(t.Format(layout))
	return strings.TrimSuffix(s, "m")
}

func untilText(d time.Duration) string {
	if d < 55*time.Minute {
		minutes := max(5, int(math.Round(d.Minutes()/5))*5)
		return fmt.Sprintf("in %d min", minutes)
	}
	halves := int(math.Round(d.Hours() * 2))
	whole, half := halves/2, halves%2 == 1
	text := fmt.Sprintf("%d", whole)
	if half {
		text += "½"
	}
	if whole == 1 && !half {
		return "in 1 hr"
	}
	return "in " + text + " hrs"
}

func overviewNotices(notices []Notice) []Notice {
	var result []Notice
	for _, notice := range notices {
		// The rain band already answers "when will it rain".
		if notice.Kind == "rain" {
			continue
		}
		result = append(result, notice)
	}
	// Warnings precede routine UV and temperature advice.
	sort.SliceStable(result, func(i, j int) bool {
		priority := func(kind string) int {
			switch kind {
			case "alert":
				return 0
			case "wind":
				return 1
			case "temperature":
				return 2
			default:
				return 3
			}
		}
		return priority(result[i].Kind) < priority(result[j].Kind)
	})
	if len(result) > 2 {
		result = result[:2]
	}
	return result
}
