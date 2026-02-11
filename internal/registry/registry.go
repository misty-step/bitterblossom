package registry

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Registry is a TOML-backed phonebook that maps sprite names to Fly.io machine IDs.
//
// Registry file location (default): ~/.config/bb/registry.toml
//
// Note: The original issue preferred github.com/BurntSushi/toml, but this project
// runs in a network-restricted environment. The parser/encoder below implements
// only the narrow subset of TOML that the registry format requires.
type Registry struct {
	Meta    Meta                   `json:"meta"`
	Sprites map[string]SpriteEntry `json:"sprites"`
}

type Meta struct {
	Account string    `json:"account"`
	App     string    `json:"app"`
	InitAt  time.Time `json:"init_at"`
}

type SpriteEntry struct {
	MachineID     string    `json:"machine_id"`
	CreatedAt     time.Time `json:"created_at"`
	AssignedIssue int       `json:"assigned_issue,omitempty"`
	AssignedRepo  string    `json:"assigned_repo,omitempty"`
	AssignedAt    time.Time `json:"assigned_at,omitempty"`
}

// DefaultPath returns the default registry path: ~/.config/bb/registry.toml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Best-effort fallback: keep things relative rather than returning "".
		return filepath.Join(".config", "bb", "registry.toml")
	}
	return filepath.Join(home, ".config", "bb", "registry.toml")
}

func newRegistry() *Registry {
	return &Registry{
		Sprites: make(map[string]SpriteEntry),
	}
}

// blockedDirs are system directories where registry files must never be written.
var blockedDirs = []string{"/etc", "/usr", "/bin", "/sbin", "/dev", "/proc", "/sys"}

// isBlockedPath checks whether path falls under any blocked system directory,
// accounting for platform symlinks on the blocked dirs themselves
// (e.g. macOS /etc -> /private/etc).
func isBlockedPath(path string) bool {
	for _, dir := range blockedDirs {
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
		if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
			if strings.HasPrefix(path, resolved+"/") {
				return true
			}
		}
	}
	return false
}

// resolveExistingAncestor walks up the path to find the longest existing
// ancestor, resolves symlinks on that ancestor, then re-appends the
// non-existing tail components. This prevents symlink bypass attacks where
// a symlink in an existing prefix points into a blocked directory but
// EvalSymlinks fails on the full path because the tail doesn't exist yet.
func resolveExistingAncestor(path string) (string, error) {
	current := filepath.Clean(path)
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Hit filesystem root without finding existing ancestor.
			return filepath.Join(append([]string{current}, tail...)...), nil
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
}

// validateRegistryPath ensures the resolved path is safe for file operations.
// Prevents path traversal attacks when paths come from untrusted input.
// Symlinks are resolved via ancestor-walking before checking blocked prefixes.
func validateRegistryPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("registry path: cannot resolve %q: %w", path, err)
	}

	// Must have .toml extension.
	if filepath.Ext(abs) != ".toml" {
		return "", fmt.Errorf("registry path: %q must have .toml extension", abs)
	}

	// Resolve symlinks by walking up to the longest existing ancestor.
	// This prevents bypass via partially-existing paths where EvalSymlinks
	// on the full parent would fail and fall back to the unresolved path.
	resolved, err := resolveExistingAncestor(abs)
	if err != nil {
		return "", fmt.Errorf("registry path: cannot resolve symlinks in %q: %w", abs, err)
	}

	if isBlockedPath(abs) {
		return "", fmt.Errorf("registry path: %q is in a protected system directory", abs)
	}
	if isBlockedPath(resolved) {
		return "", fmt.Errorf("registry path: %q resolves to protected system directory %q", abs, resolved)
	}

	return abs, nil
}

// Load loads a TOML registry file from disk.
//
// If the file does not exist, Load returns an empty registry and a nil error.
// If the file exists but is corrupt, Load returns a clear error describing the
// failure.
func Load(path string) (*Registry, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("reading registry: path is empty")
	}

	validated, err := validateRegistryPath(path)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(validated)
	if err != nil {
		if os.IsNotExist(err) {
			return newRegistry(), nil
		}
		return nil, fmt.Errorf("reading registry %q: %w", validated, err)
	}

	r := newRegistry()
	if err := parseTOMLRegistry(bytes.NewReader(raw), r); err != nil {
		return nil, fmt.Errorf("parsing registry %q: %w", path, err)
	}
	return r, nil
}

// Save writes the registry to the provided path as TOML, creating parent
// directories if needed.
func (r *Registry) Save(path string) error {
	if r == nil {
		return errors.New("saving registry: nil receiver")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("saving registry: path is empty")
	}

	validated, err := validateRegistryPath(path)
	if err != nil {
		return err
	}
	path = validated

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating registry dir %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "registry-*.toml")
	if err != nil {
		return fmt.Errorf("creating temp registry file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanupTmp := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := writeTOMLRegistry(tmp, r); err != nil {
		cleanupTmp()
		return fmt.Errorf("writing registry %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return fmt.Errorf("closing temp registry file %q: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		cleanupTmp()
		return fmt.Errorf("chmod temp registry file %q: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		cleanupTmp()
		return fmt.Errorf("replacing registry %q: %w", path, err)
	}
	return nil
}

// LookupMachine returns the machine ID for the named sprite.
func (r *Registry) LookupMachine(name string) (string, bool) {
	if r == nil {
		return "", false
	}
	entry, ok := r.Sprites[name]
	if !ok {
		return "", false
	}
	return entry.MachineID, true
}

// LookupName returns the sprite name that matches the given machine ID.
func (r *Registry) LookupName(machineID string) (string, bool) {
	if r == nil {
		return "", false
	}
	// Deterministic iteration for stable behavior/tests.
	for _, name := range r.Names() {
		if r.Sprites[name].MachineID == machineID {
			return name, true
		}
	}
	return "", false
}

// Names returns a sorted list of sprite names present in the registry.
func (r *Registry) Names() []string {
	if r == nil || len(r.Sprites) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.Sprites))
	for name := range r.Sprites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count returns the number of registered sprites.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.Sprites)
}

// Register adds or updates a sprite entry for name -> machineID.
//
// If the name is new, CreatedAt is set to time.Now().
// If the name already exists, CreatedAt is preserved.
func (r *Registry) Register(name, machineID string) {
	if r == nil {
		return
	}
	if r.Sprites == nil {
		r.Sprites = make(map[string]SpriteEntry)
	}
	entry, ok := r.Sprites[name]
	if !ok {
		entry.CreatedAt = time.Now()
	}
	entry.MachineID = machineID
	r.Sprites[name] = entry
}

// Unregister removes a sprite from the registry.
func (r *Registry) Unregister(name string) {
	if r == nil || r.Sprites == nil {
		return
	}
	delete(r.Sprites, name)
}

type registrySection int

const (
	sectionUnknown registrySection = iota
	sectionMeta
	sectionSprite
)

type parseState struct {
	section    registrySection
	spriteName string
	lineNo     int
}

func parseTOMLRegistry(r io.Reader, reg *Registry) error {
	scanner := bufio.NewScanner(r)

	state := parseState{}
	for scanner.Scan() {
		state.lineNo++
		line := stripComments(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return state.errf("invalid section header %q (missing ']')", line)
			}
			header := strings.TrimSpace(line[1 : len(line)-1])
			switch {
			case header == "meta":
				state.section = sectionMeta
				state.spriteName = ""
			case strings.HasPrefix(header, "sprites."):
				state.section = sectionSprite
				namePart := strings.TrimSpace(strings.TrimPrefix(header, "sprites."))
				if namePart == "" {
					return state.errf("invalid sprite section header %q", line)
				}
				spriteName, err := parseBareOrQuotedKey(namePart)
				if err != nil {
					return state.errf("invalid sprite section header %q: %v", line, err)
				}
				state.spriteName = spriteName
				if reg.Sprites == nil {
					reg.Sprites = make(map[string]SpriteEntry)
				}
				if _, ok := reg.Sprites[spriteName]; !ok {
					reg.Sprites[spriteName] = SpriteEntry{}
				}
			default:
				// Forward-compatible: ignore unknown tables.
				state.section = sectionUnknown
				state.spriteName = ""
			}
			continue
		}

		key, value, ok := splitKeyValue(line)
		if !ok {
			return state.errf("invalid key/value line %q (expected key = value)", line)
		}
		switch state.section {
		case sectionMeta:
			if err := applyMetaKV(&reg.Meta, key, value); err != nil {
				return state.errf("%v", err)
			}
		case sectionSprite:
			if state.spriteName == "" {
				return state.errf("sprite key/value outside a [sprites.<name>] table")
			}
			entry := reg.Sprites[state.spriteName]
			if err := applySpriteKV(&entry, key, value); err != nil {
				return state.errf("%v", err)
			}
			reg.Sprites[state.spriteName] = entry
		default:
			// Ignore.
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning: %w", err)
	}
	if reg.Sprites == nil {
		reg.Sprites = make(map[string]SpriteEntry)
	}
	return nil
}

func (s parseState) errf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", s.lineNo, fmt.Sprintf(format, args...))
}

func stripComments(line string) string {
	// Remove # comments, respecting "quoted strings".
	inQuotes := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if inQuotes && ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && ch == '#' {
			return line[:i]
		}
	}
	return line
}

func splitKeyValue(line string) (key string, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

func applyMetaKV(meta *Meta, key, value string) error {
	switch key {
	case "account":
		v, err := parseStringValue(value)
		if err != nil {
			return fmt.Errorf("meta.account: %w", err)
		}
		meta.Account = v
	case "app":
		v, err := parseStringValue(value)
		if err != nil {
			return fmt.Errorf("meta.app: %w", err)
		}
		meta.App = v
	case "init_at":
		v, err := parseTimeValue(value)
		if err != nil {
			return fmt.Errorf("meta.init_at: %w", err)
		}
		meta.InitAt = v
	default:
		// Ignore unknown meta keys for forward-compatibility.
	}
	return nil
}

func applySpriteKV(entry *SpriteEntry, key, value string) error {
	switch key {
	case "machine_id":
		v, err := parseStringValue(value)
		if err != nil {
			return fmt.Errorf("sprites.machine_id: %w", err)
		}
		entry.MachineID = v
	case "created_at":
		v, err := parseTimeValue(value)
		if err != nil {
			return fmt.Errorf("sprites.created_at: %w", err)
		}
		entry.CreatedAt = v
	case "assigned_issue":
		v, err := parseIntValue(value)
		if err != nil {
			return fmt.Errorf("sprites.assigned_issue: %w", err)
		}
		entry.AssignedIssue = v
	case "assigned_repo":
		v, err := parseStringValue(value)
		if err != nil {
			return fmt.Errorf("sprites.assigned_repo: %w", err)
		}
		entry.AssignedRepo = v
	case "assigned_at":
		v, err := parseTimeValue(value)
		if err != nil {
			return fmt.Errorf("sprites.assigned_at: %w", err)
		}
		entry.AssignedAt = v
	default:
		// Ignore unknown sprite keys for forward-compatibility.
	}
	return nil
}

func parseStringValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty value")
	}
	if strings.HasPrefix(value, "\"") {
		s, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid quoted string %q: %w", value, err)
		}
		return s, nil
	}
	// Minimal support for single-quoted literal strings.
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2 {
		return value[1 : len(value)-1], nil
	}
	return "", fmt.Errorf("expected quoted string, got %q", value)
}

func parseIntValue(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}
	// Tolerate quoted ints for forward-compat with sloppy writers.
	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		s, err := parseStringValue(value)
		if err != nil {
			return 0, err
		}
		value = s
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid int %q: %w", value, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid int %q: must be >= 0", value)
	}
	return n, nil
}

func parseTimeValue(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty value")
	}
	// TOML permits RFC3339 timestamps without quotes, but tolerate quoted too.
	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		s, err := parseStringValue(value)
		if err != nil {
			return time.Time{}, err
		}
		value = s
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp %q: %w", value, err)
	}
	return t, nil
}

var bareKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func parseBareOrQuotedKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("empty key")
	}
	if bareKeyRE.MatchString(key) {
		return key, nil
	}
	// Allow quoted keys like "my sprite".
	if strings.HasPrefix(key, "\"") || strings.HasPrefix(key, "'") {
		return parseStringValue(key)
	}
	return "", fmt.Errorf("expected bare key or quoted key, got %q", key)
}

func writeTOMLRegistry(w io.Writer, reg *Registry) error {
	// Meta
	if reg.Meta.Account != "" || reg.Meta.App != "" || !reg.Meta.InitAt.IsZero() {
		if _, err := io.WriteString(w, "[meta]\n"); err != nil {
			return err
		}
		if reg.Meta.Account != "" {
			if _, err := fmt.Fprintf(w, "account = %s\n", strconv.Quote(reg.Meta.Account)); err != nil {
				return err
			}
		}
		if reg.Meta.App != "" {
			if _, err := fmt.Fprintf(w, "app = %s\n", strconv.Quote(reg.Meta.App)); err != nil {
				return err
			}
		}
		if !reg.Meta.InitAt.IsZero() {
			if _, err := fmt.Fprintf(w, "init_at = %s\n", reg.Meta.InitAt.Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}

	// Sprites
	names := reg.Names()
	for i, name := range names {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		headerName := name
		if !bareKeyRE.MatchString(name) {
			headerName = strconv.Quote(name)
		}
		if _, err := fmt.Fprintf(w, "[sprites.%s]\n", headerName); err != nil {
			return err
		}
		entry := reg.Sprites[name]
		if entry.MachineID != "" {
			if _, err := fmt.Fprintf(w, "machine_id = %s\n", strconv.Quote(entry.MachineID)); err != nil {
				return err
			}
		}
		if !entry.CreatedAt.IsZero() {
			if _, err := fmt.Fprintf(w, "created_at = %s\n", entry.CreatedAt.Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		if entry.AssignedIssue > 0 {
			if _, err := fmt.Fprintf(w, "assigned_issue = %d\n", entry.AssignedIssue); err != nil {
				return err
			}
		}
		if strings.TrimSpace(entry.AssignedRepo) != "" {
			if _, err := fmt.Fprintf(w, "assigned_repo = %s\n", strconv.Quote(entry.AssignedRepo)); err != nil {
				return err
			}
		}
		if !entry.AssignedAt.IsZero() {
			if _, err := fmt.Fprintf(w, "assigned_at = %s\n", entry.AssignedAt.Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
	}
	return nil
}
