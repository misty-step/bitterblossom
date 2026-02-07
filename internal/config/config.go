package config

// Config represents the parsed fleet composition input.
type Config struct {
	FleetName string         `json:"fleet_name" yaml:"fleet_name"`
	Sprites   []SpriteConfig `json:"sprites" yaml:"sprites"`
}

// SpriteConfig defines a single sprite entry in a composition file.
type SpriteConfig struct {
	Name            string `json:"name" yaml:"name"`
	CompositionFile string `json:"composition_file" yaml:"composition_file"`
}
