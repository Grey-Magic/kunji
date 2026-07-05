package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliceSource(t *testing.T) {
	s := NewSliceSource("a", "b", "c")
	assert.Equal(t, "slice", s.Name())
	assert.Equal(t, 3, s.Total())

	for _, want := range []string{"a", "b", "c"} {
		got, err := s.Next(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err := s.Next(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestRegistry_RegistersBuiltins(t *testing.T) {
	kinds := Default.Kinds()
	assert.Contains(t, kinds, "file")
	assert.Contains(t, kinds, "stdin")
}

func TestRegistry_RejectsUnknown(t *testing.T) {
	_, err := Default.Open("nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown source kind")
}

func TestRegistry_PanicsOnDoubleRegister(t *testing.T) {
	r := NewRegistry()
	r.Register("x", func(args []string) (Source, error) { return nil, nil })
	assert.Panics(t, func() {
		r.Register("x", func(args []string) (Source, error) { return nil, nil })
	})
}

func TestFileSource_SkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join([]string{
		"sk-aaa",
		"",
		"# comment",
		"  sk-bbb  ",
		"sk-ccc",
	}, "\n")), 0o600))

	s, err := Default.Open("file", []string{path})
	require.NoError(t, err)
	defer s.Close()

	assert.Equal(t, 3, s.Total(), "Total should count non-blank, non-comment lines")

	for _, want := range []string{"sk-aaa", "sk-bbb", "sk-ccc"} {
		got, err := s.Next(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err = s.Next(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestFileSource_RequiresPath(t *testing.T) {
	_, err := Default.Open("file", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a path")
}

func TestFileSource_FileMissing(t *testing.T) {
	_, err := Default.Open("file", []string{"/nonexistent/path/that/does/not/exist"})
	require.Error(t, err)
}
