package config

import (
	"errors"
	"fmt"
	"os"

	"elvia.io/scheduler/internal/runtime"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
)

type Config struct {
	Env      runtime.Env
	NatsConn string
	ApiAddr  string
}

func NewConfig() (*Config, error) {
	_ = godotenv.Load(".Env")

	return &Config{
		Env:      runtime.Env(get("ENVIRONMENT", string(runtime.Production))),
		NatsConn: get("NATS_URL", nats.DefaultURL),
		ApiAddr:  get("API_ADDR", ":8080"),
	}, nil
}

func get(name string, alt string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		return alt
	}
	return v
}

func required(name string) (string, error) {
	v := get(name, "")
	if v == "" {
		return "", errors.New(fmt.Sprintf("missing environment variable %s", name))
	}
	return v, nil
}