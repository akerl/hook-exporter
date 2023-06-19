package main

import (
	"regexp"

	"github.com/akerl/go-lambda/mux"
)

var (
	metricRegex = regexp.MustCompile(`^/metric$`)
	indexRegex  = regexp.MustCompile(`^/(metrics)?$`)
)

func main() {
	err := loadConfig()
	if err != nil {
		panic(err)
	}

	d := mux.NewDispatcher(
		mux.NewRouteWithAuth(metricRegex, metricHandler, metricAuth),
		mux.NewRoute(indexRegex, indexHandler),
	)
	mux.Start(d)
}
