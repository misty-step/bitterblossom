package sprite

import (
	"context"

	"github.com/misty-step/bitterblossom/internal/contracts"
)

// Sprite describes a managed sprite in the fleet.
type Sprite struct {
	Name  string                `json:"name"`
	State contracts.SpriteState `json:"state"`
}

// SpriteClient defines the contract for querying and controlling sprites.
//
// Implementations should handle transport concerns and return shared contract
// types so callers can work consistently across local and remote backends.
type SpriteClient interface {
	Status(ctx context.Context, sprite string) (contracts.SpriteStatus, error)
	Dispatch(ctx context.Context, sprite, task string) (contracts.DispatchResult, error)
}
