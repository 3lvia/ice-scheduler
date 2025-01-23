package config

import (
	"context"
	"log/slog"

	"github.com/3lvia/libraries-go/pkg/hashivault"
)

type Secrets struct {
	NatsToken string
}

func NewVault(ctx context.Context) (hashivault.SecretsManager, error) {
	vault, errChan, err := hashivault.New(ctx, hashivault.WithOIDC())
	if err != nil {
		return nil, err
	}

	go func(ec <-chan error) {
		for err := range ec {
			slog.ErrorContext(ctx, "vault error", "error", err)
		}
	}(errChan)

	return vault, nil
}

func NewSecrets(ctx context.Context, vault hashivault.SecretsManager) (*Secrets, error) {
	natsToken, err := vault.GetStaticSecretAtKey(ctx, "ice/kv/data/nats", "token")
	if err != nil {
		return nil, err
	}

	return &Secrets{
		NatsToken: natsToken,
	}, nil
}