package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/ice-scheduler/scheduler/internal/runtime"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
)

type Config struct {
	Env       runtime.Env
	VaultAddr string
	NatsAddr  string
	ApiAddr   string
}

func New() (*Config, error) {
	_ = godotenv.Load(".env")

	// log all environment variables
	for k, e := range os.Environ() {
		fmt.Printf("%s=%s\n", k, e)
	}

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