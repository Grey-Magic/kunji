package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadFrom_MissingFileReturnsEmpty(t *testing.T) {
	c, err := LoadFrom("/no/such/path.yaml")
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Empty(t, c.ProfileNames())
}

func TestLoadFrom_DefaultsAndProfiles(t *testing.T) {
	path := writeConfig(t, `
threads: 40
format: text
profiles:
  prod:
    threads: 80
    proxy: socks5://localhost:9050
    format: jsonl
  paranoid:
    threads: 4
    only_valid: true
default_profile: prod
`)
	c, err := LoadFrom(path)
	require.NoError(t, err)

	v, ok := c.Lookup("", "threads")
	assert.True(t, ok)
	assert.Equal(t, 40, v)

	v, ok = c.Lookup("prod", "threads")
	assert.True(t, ok)
	assert.Equal(t, 80, v)

	v, ok = c.Lookup("prod", "proxy")
	assert.True(t, ok)
	assert.Equal(t, "socks5://localhost:9050", v)

	// Lookup with an unknown profile name falls back to the top-level
	// default block (so a mistyped profile still picks up reasonable
	// defaults). The fallback is intentional.
	v, ok = c.Lookup("missing-profile", "threads")
	assert.True(t, ok)
	assert.Equal(t, 40, v)

	// Unknown key in an unknown profile returns nothing.
	_, ok = c.Lookup("missing-profile", "no-such-key")
	assert.False(t, ok)
}

func TestActiveProfileName_Precedence(t *testing.T) {
	path := writeConfig(t, `
default_profile: from-config
profiles:
  from-config:
    threads: 1
`)
	c, err := LoadFrom(path)
	require.NoError(t, err)

	t.Run("cli wins", func(t *testing.T) {
		assert.Equal(t, "from-cli", c.ActiveProfileName("from-cli"))
	})
	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("KUNJI_PROFILE", "from-env")
		assert.Equal(t, "from-env", c.ActiveProfileName(""))
	})
	t.Run("config default when nothing else", func(t *testing.T) {
		assert.Equal(t, "from-config", c.ActiveProfileName(""))
	})
	t.Run("empty when nothing anywhere", func(t *testing.T) {
		c2, err := LoadFrom("/no/such/path.yaml")
		require.NoError(t, err)
		assert.Equal(t, "", c2.ActiveProfileName(""))
	})
}

func TestEnvNameFor(t *testing.T) {
	assert.Equal(t, "KUNJI_THREADS", envNameFor("threads"))
	assert.Equal(t, "KUNJI_WEBHOOK_RETRIES", envNameFor("webhook-retries"))
	assert.Equal(t, "KUNJI_FOO_BAR_BAZ", envNameFor("foo-bar-baz"))
}

func TestApply_OverridesDefaults(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	var threads int
	var proxy, format string
	var onlyValid bool
	root.Flags().IntVar(&threads, "threads", 40, "")
	root.Flags().StringVar(&proxy, "proxy", "", "")
	root.Flags().StringVar(&format, "format", "text", "")
	root.Flags().BoolVar(&onlyValid, "only-valid", false, "")

	path := writeConfig(t, `
default_profile: prod
profiles:
  prod:
    threads: 80
    proxy: socks5://localhost:9050
    format: jsonl
    only_valid: true
`)
	c, err := LoadFrom(path)
	require.NoError(t, err)

	c.Apply(root, "")

	threadsFlag := root.Flags().Lookup("threads")
	require.NotNil(t, threadsFlag)
	assert.Equal(t, "80", threadsFlag.DefValue)

	proxyFlag := root.Flags().Lookup("proxy")
	require.NotNil(t, proxyFlag)
	assert.Equal(t, "socks5://localhost:9050", proxyFlag.DefValue)

	onlyFlag := root.Flags().Lookup("only-valid")
	require.NotNil(t, onlyFlag)
	assert.Equal(t, "true", onlyFlag.DefValue)
}

func TestApply_EnvOverridesProfile(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	var threads int
	root.Flags().IntVar(&threads, "threads", 40, "")

	path := writeConfig(t, `
profiles:
  prod:
    threads: 80
`)
	c, err := LoadFrom(path)
	require.NoError(t, err)

	t.Setenv("KUNJI_THREADS", "12")

	c.Apply(root, "prod")

	threadsFlag := root.Flags().Lookup("threads")
	require.NotNil(t, threadsFlag)
	assert.Equal(t, "12", threadsFlag.DefValue)
}

func TestApply_NoProfileLeavesDefaults(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	var threads int
	root.Flags().IntVar(&threads, "threads", 40, "")

	path := writeConfig(t, `default_profile: does-not-exist`)
	c, err := LoadFrom(path)
	require.NoError(t, err)

	c.Apply(root, "")

	threadsFlag := root.Flags().Lookup("threads")
	require.NotNil(t, threadsFlag)
	assert.Equal(t, "40", threadsFlag.DefValue)
}

func TestProfileNames_Sorted(t *testing.T) {
	path := writeConfig(t, `
profiles:
  charlie: {}
  alpha: {}
  bravo: {}
`)
	c, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, c.ProfileNames())
}

// helpers ------------------------------------------------------------------
