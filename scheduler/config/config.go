package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/3lvia/libraries-go/pkg/elvia/runtime"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
)

const (
	TracerName = "scheduler"
)

type Config struct {
	Env       runtime.Env
	VaultAddr string
	NatsAddr  string
	ApiAddr   string
}

func New() (*Config, error) {
	_ = godotenv.Load(".env")

	return &Config{
		Env:       runtime.Env(get("ENVIRONMENT", string(runtime.Production))),
		VaultAddr: get("VAULT_ADDR", ""),
		NatsAddr:  get("NATS_ADDR", nats.DefaultURL),
		ApiAddr:   get("API_ADDR", ":8080"),
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