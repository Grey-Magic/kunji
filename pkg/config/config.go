// Package config loads user configuration from ~/.kunji/config.yaml and
// applies profile / environment overrides to cobra flag defaults.
//
// Resolution order, highest priority first:
//
//  1. CLI flag passed on the command line (handled by cobra during parsing)
//  2. KUNJI_<FLAG_UPPER> environment variable
//  3. The active profile's value for that flag
//  4. The built-in default registered with the flag
//
// "Active profile" is determined by:
//
//  1. --profile <name> on the command line
//  2. KUNJI_PROFILE environment variable
//  3. default_profile field in the config file
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// spec is the on-disk YAML shape. Profiles and the top-level "default" block
// both flatten into a map[string]interface{} so YAML's "any-key-at-root" form
// works without an exhaustive struct definition per flag.
type spec struct {
	Default        map[string]interface{}            `yaml:",inline"`
	Profiles       map[string]map[string]interface{} `yaml:"profiles"`
	DefaultProfile string                            `yaml:"default_profile"`
}

// Config holds the parsed config plus the path it was loaded from. A zero
// Config (Path == "") is valid and means "no config file present".
type Config struct {
	spec spec
	Path string
}

// DefaultPath returns the location we look for config at. Resolution order:
//
//	KUNJI_CONFIG env var (explicit override)
//	~/.kunji/config.yaml
//	.kunji/config.yaml (cwd fallback)
func DefaultPath() string {
	if p := strings.TrimSpace(os.Getenv("KUNJI_CONFIG")); p != "" {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".kunji", "config.yaml")
	}
	return filepath.Join(".kunji", "config.yaml")
}

// Load reads the config file at DefaultPath(). A missing file is not an
// error — it returns an empty Config so callers can use Apply unconditionally.
// Parse errors are returned so the user can fix a malformed config.
func Load() (*Config, error) {
	return LoadFrom(DefaultPath())
}

// LoadFrom is Load for a specific path. Useful in tests.
func LoadFrom(path string) (*Config, error) {
	c := &Config{Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return c, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &c.spec); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return c, nil
}

// ActiveProfileName returns the profile to use. Resolution order:
//
//	--profile flag value (passed in via cliProfile)
//	KUNJI_PROFILE env var
//	default_profile from the config
//	"" (caller treats this as "no profile")
func (c *Config) ActiveProfileName(cliProfile string) string {
	if v := strings.TrimSpace(cliProfile); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("KUNJI_PROFILE")); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.spec.DefaultProfile); v != "" {
		return v
	}
	return ""
}

// Lookup returns the value for key from the active profile (or the top-level
// default block if profileName is ""), and a found flag.
// Lookup returns the value for key from the active profile (or the top-level
// default block if profileName is ""), and a found flag.
//
// Keys are normalized: dashes and underscores are interchangeable so users
// can write either `only_valid: true` or `only-valid: true` in YAML.
func (c *Config) Lookup(profileName, key string) (interface{}, bool) {
	normalized := normalizeFlagName(key)
	if profileName != "" {
		if p, ok := c.spec.Profiles[profileName]; ok {
			if v, ok := lookupNormalized(p, normalized); ok {
				return v, true
			}
		}
	}
	if v, ok := lookupNormalized(c.spec.Default, normalized); ok {
		return v, true
	}
	return nil, false
}

func lookupNormalized(m map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	// Try the alternate spelling (dashes <-> underscores) so users can
	// write either form in their config.
	for k, v := range m {
		if normalizeFlagName(k) == key {
			return v, true
		}
	}
	return nil, false
}

func normalizeFlagName(s string) string {
	return strings.ReplaceAll(s, "_", "-")
}

// ProfileNames returns the configured profile names sorted alphabetically.
func (c *Config) ProfileNames() []string {
	out := make([]string, 0, len(c.spec.Profiles))
	for n := range c.spec.Profiles {
		out = append(out, n)
	}
	// tiny sort
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Apply walks the cobra command tree (rootCmd and every descendant) and
// overrides each flag's default with the value from the active profile or
// the environment. CLI flags passed on the command line still win because
// cobra only falls back to the default when a flag was not provided.
//
// The env-var-name for a flag "foo-bar" is "KUNJI_FOO_BAR".
func (c *Config) Apply(root *cobra.Command, cliProfile string) {
	profileName := c.ActiveProfileName(cliProfile)

	walk(root, func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			// Don't change flags the user explicitly changed on the
			// command line. Cobra's Changed() is set after parsing,
			// so during Apply (pre-parse) everything looks unchanged.
			// We rely on cobra's parse-time semantics: defaults are
			// only consulted for flags the user did not provide.

			envName := envNameFor(f.Name)
			if v, ok := os.LookupEnv(envName); ok {
				if err := f.Value.Set(v); err == nil {
					f.DefValue = f.Value.String()
					return
				}
			}

			val, ok := c.Lookup(profileName, f.Name)
			if !ok {
				return
			}
			if err := setFlagFromValue(f, val); err == nil {
				f.DefValue = f.Value.String()
			}
		})
	})
}

func envNameFor(flagName string) string {
	return "KUNJI_" + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

func setFlagFromValue(f *pflag.Flag, val interface{}) error {
	switch v := val.(type) {
	case string:
		return f.Value.Set(v)
	case bool:
		return f.Value.Set(strconvFormatBool(v))
	case int:
		return f.Value.Set(strconvFormatInt(v))
	case int64:
		return f.Value.Set(strconvFormatInt64(v))
	case float64:
		return f.Value.Set(strconvFormatFloat(v))
	case []interface{}:
		// YAML list → stringSlice / []string flag.
		parts := make([]string, 0, len(v))
		for _, p := range v {
			s, ok := p.(string)
			if !ok {
				return fmt.Errorf("flag %q: list element is not a string", f.Name)
			}
			parts = append(parts, s)
		}
		return f.Value.Set(strings.Join(parts, ","))
	case []string:
		return f.Value.Set(strings.Join(v, ","))
	default:
		return fmt.Errorf("flag %q: unsupported value type %T", f.Name, val)
	}
}

func strconvFormatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func strconvFormatInt(i int) string     { return fmt.Sprintf("%d", i) }
func strconvFormatInt64(i int64) string { return fmt.Sprintf("%d", i) }
func strconvFormatFloat(f float64) string {
	// %g keeps the textual representation short and round-trippable.
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%g", f), "0"), ".")
}

func walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, c := range cmd.Commands() {
		walk(c, fn)
	}
}

// Cached is a process-wide convenience wrapper for the common case. It loads
// the config once and re-uses it on subsequent calls. The first call returns
// (cfg, err) where err is non-nil only for parse errors; a missing file is
// not an error.
var (
	cachedOnce sync.Once
	cachedCfg  *Config
	cachedErr  error
)

func Cached() (*Config, error) {
	cachedOnce.Do(func() {
		cachedCfg, cachedErr = Load()
	})
	return cachedCfg, cachedErr
}
