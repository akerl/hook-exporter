package main

import (
	"fmt"

	"github.com/akerl/go-lambda/s3"
)

type config struct {
	AuthToken    string `json:"auth_token"`
	MetricBucket string `json:"metric_bucket"`
}

var c *config

func loadConfig() error {
	cf, err := s3.GetConfigFromEnv(&c)
	if err != nil {
		return err
	}
	cf.OnError = func(_ *s3.ConfigFile, err error) {
		fmt.Println(err)
	}
	cf.Autoreload(60)

	return nil
}
