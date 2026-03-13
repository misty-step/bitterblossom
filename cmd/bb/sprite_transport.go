package main

import (
	"context"
	"fmt"
	"time"

	sprites "github.com/superfly/sprites-go"
)

type spriteClientOptions struct {
	disableControl bool
}

type spriteSessionOptions struct {
	disableControl bool
	probeTimeout   time.Duration
}

type spriteSession struct {
	client *sprites.Client
	sprite *sprites.Sprite
}

func newSpritesClientFromEnv(opts spriteClientOptions) (*sprites.Client, error) {
	token, err := spriteToken()
	if err != nil {
		return nil, err
	}
	return newSpritesClient(token, opts), nil
}

func newSpritesClient(token string, opts spriteClientOptions) *sprites.Client {
	if opts.disableControl {
		return sprites.New(token, sprites.WithDisableControl())
	}
	return sprites.New(token)
}

func newSpriteSession(ctx context.Context, spriteName string, opts spriteSessionOptions) (*spriteSession, error) {
	client, err := newSpritesClientFromEnv(spriteClientOptions{disableControl: opts.disableControl})
	if err != nil {
		return nil, err
	}

	session := &spriteSession{
		client: client,
		sprite: client.Sprite(spriteName),
	}
	if err := probeSprite(ctx, session.sprite, spriteName, opts.probeTimeout); err != nil {
		_ = session.close()
		return nil, err
	}
	return session, nil
}

func (s *spriteSession) close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func probeSprite(ctx context.Context, s *sprites.Sprite, spriteName string, timeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}
	return nil
}
