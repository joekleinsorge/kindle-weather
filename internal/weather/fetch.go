package weather

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
)

const (
	secretMountPath = "/etc/secrets"
	weatherAPIURLTemplate = "https://api.openweathermap.org/data/3.0/onecall?lat=29.65&lon=-81.20&exclude=minutely&appid=%s&units=imperial"
	noaaAPIURLTemplate = "https://api.tidesandcurrents.noaa.gov/api/prod/datagetter?product=predictions&application=NOS.COOPS.TAC.WL&datum=MLLW&station=8720218&time_zone=lst_ldt&units=english&interval=hilo&format=json&date=today"
)

var (
	weatherAPIURL string
	noaaAPIURL    string
	weatherCache  *cache.Cache
)

func init() {
	openWeatherAPIKey, err := readSecret("openweather-api-key")
	if err != nil {
		log.Fatalf("Failed to read OpenWeather API key: %v", err)
	}
	weatherAPIURL = fmt.Sprintf(weatherAPIURLTemplate, openWeatherAPIKey)
	noaaAPIURL = noaaAPIURLTemplate

	cacheExpiration, _ := strconv.Atoi(os.Getenv("CACHE_EXPIRATION"))
	if cacheExpiration == 0 {
		cacheExpiration = 1800 // default to 30 minutes
	}

	cacheCleanupInterval, _ := strconv.Atoi(os.Getenv("CACHE_CLEANUP_INTERVAL"))
	if cacheCleanupInterval == 0 {
		cacheCleanupInterval = 3600 // default to 1 hour
	}

	weatherCache = cache.New(time.Duration(cacheExpiration)*time.Second, time.Duration(cacheCleanupInterval)*time.Second)
}

func readSecret(secretName string) (string, error) {
	secretPath := filepath.Join(secretMountPath, secretName)
	secretValue, err := ioutil.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %v", err

