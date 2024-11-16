package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/joekleinsorge/kindle-weather/internal/weather"
	"github.com/joekleinsorge/kindle-weather/pkg/logger"
)

func main() {
	router := mux.NewRouter()
	router.Use(weather.LoggingMiddleware)

	router.HandleFunc("/", weather.Handler).Methods("GET")

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}

