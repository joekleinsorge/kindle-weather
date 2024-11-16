package halloween

import (
    "strings"
    "math/rand"
)

var (
    weatherConditions = map[string][]string{
        "sunny":   {"clear skies for vampire sunbathing", "perfect weather for zombie tanning"},
        "cloudy":  {"overcast with a chance of werewolf sightings", "gloomy atmosphere fit for a haunted house"},
        "rain":   {"witch's brew falling from the sky", "ghostly tears showering the earth"},
        "windy":   {"banshees howling through the air", "skeleton leaves dancing in the breeze"},
        "foggy":   {"thick mist from the witch's cauldron", "spooky fog rolling in from the graveyard"},
        "stormy":  {"thunder and lightning summoned by mad scientists", "perfect weather for raising the dead"},
        "cold":    {"chilling touch of the undead", "frosty breath of ice zombies"},
        "hot":     {"hellfire temperatures", "weather hot enough to melt a witch's face"},
        "clear":   {"transparent as a ghost", "visibility clear enough to spot distant vampires"},
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
        return "A mysteriously nondescript Halloween forecast"
    }

    return "Expect a day with " + strings.Join(themedParts, " and ")
}
