package christmas

import (
    "strings"
    "math/rand"
)

var (
    weatherConditions = map[string][]string{
        "sunny":   {"clear skies for Santa's sleigh ride", "perfect weather for snowmen to wear sunglasses"},
        "cloudy":  {"sky full of cotton candy clouds", "overcast with a chance of falling snowflakes"},
        "rainy":   {"drizzle of liquid tinsel", "ornament-polishing shower"},
        "windy":   {"North Pole breeze carrying jingle bells", "gusts of peppermint-scented air"},
        "foggy":   {"thick mist from Mrs. Claus's cookie steam", "foggy enough to guide Rudolph's nose"},
        "stormy":  {"snowglobe-shaking weather", "perfect blizzard for building an igloo"},
        "cold":    {"Jack Frost nipping at your nose", "weather cold enough to freeze a sugar plum fairy"},
        "hot":     {"unusually warm for the elves' liking", "Santa's "beach vacation" weather"},
        "clear":   {"sky as clear as an icicle", "perfect visibility for spotting flying reindeer"},
        "snowy":   {"blanket of fresh Christmas powder", "flurry of Frosty's cousins falling"},
    }
)

func ThemeWeather(summary string) string {
    summary = strings.ToLower(summary)
    themedParts := []string{}

    for condition, themes := range weatherConditions {
        if strings.Contains(summary, condition) {
            themedParts = append(themedParts, themes[rand.Intn(len(themes))])
        }
    }

    if len(themedParts) == 0 {
        return "A magically merry Christmas forecast, details known only to Santa"
    }

    return "Ho ho ho! Expect a day with " + strings.Join(themedParts, " and ")
}
