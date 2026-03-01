package runner

import (
	"os"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAndFilterKeys_SingleKey(t *testing.T) {
	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("sk-test-key-123", "")

	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "sk-test-key-123", keys[0])
}

func TestLoadAndFilterKeys_KeyFile(t *testing.T) {
	// Create temp file with keys
	content := "sk-key-1\nsk-key-2\nsk-key-3\n"
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 3)
}

func TestLoadAndFilterKeys_Deduplication(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("sk-key-1\nsk-key-1\nsk-key-2\nsk-key-1\n")
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 2, "should deduplicate keys")
}

func TestLoadAndFilterKeys_Whitespace(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("  sk-key-1  \n  sk-key-2  \n")
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Equal(t, "sk-key-1", keys[0], "should trim whitespace")
	assert.Equal(t, "sk-key-2", keys[1], "should trim whitespace")
}

func TestLoadAndFilterKeys_ShortKeysFiltered(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("ab\nsk-valid-key\nc\nsk-another\n")
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 2, "should filter out keys shorter than MinKeyLength")
}

func TestLoadAndFilterKeys_KeysWithSpaces(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("sk valid key\nsk-good-key\n")
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 1, "should filter out keys with spaces")
	assert.Equal(t, "sk-good-key", keys[0])
}

func TestLoadAndFilterKeys_Resume(t *testing.T) {
	// Create resume file with existing keys
	resumeFile, err := os.CreateTemp("", "results*.txt")
	require.NoError(t, err)
	defer os.Remove(resumeFile.Name())

	_, err = resumeFile.WriteString("sk-already-processed\n")
	require.NoError(t, err)
	resumeFile.Close()

	// Create keys file
	keysFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(keysFile.Name())

	_, err = keysFile.WriteString("sk-already-processed\nsk-new-key-1\nsk-new-key-2\n")
	require.NoError(t, err)
	keysFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		OutFile:      resumeFile.Name(),
		Resume:       true,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", keysFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 2, "should skip already processed keys")
}

func TestLoadAndFilterKeys_NoValidKeys(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "keys*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("ab\nc\n")
	require.NoError(t, err)
	tmpFile.Close()

	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, keys, 0, "should return empty slice when no valid keys")
}

func TestLoadAndFilterKeys_NonExistentFile(t *testing.T) {
	r := &Runner{
		MinKeyLength: 4,
		Validators:   make(map[string]validators.Validator),
		Detector:     validators.NewDetector(),
	}

	keys, err := r.LoadAndFilterKeys("", "/non/existent/file.txt")

	assert.Error(t, err)
	assert.Nil(t, keys)
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short key", "abc", "******"},
		{"exactly 12 chars", "123456789012", "123456.....9012"},
		{"long key", "sk-test-key-12345", "sk-tes.....2345"},
		{"empty", "", "******"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRunner_OpenResultFile_JSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "results*.json")
	require.NoError(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	r := &Runner{
		OutFile: tmpFile.Name(),
	}

	file, err := r.openResultFile()
	require.NoError(t, err)
	assert.NotNil(t, file)
	file.Close()
}

func TestRunner_ShouldWriteHeader(t *testing.T) {
	tests := []struct {
		name     string
		outFile  string
		setup    func(string) error
		expected bool
	}{
		{
			name:     "new csv file",
			outFile:  "new_results.csv",
			setup:    nil,
			expected: true,
		},
		{
			name:    "existing file",
			outFile: "test.csv",
			setup: func(f string) error {
				file, err := os.Create(f)
				if err != nil {
					return err
				}
				file.WriteString("header1,header2\n")
				file.WriteString("data1,data2\n")
				return file.Close()
			},
			expected: false,
		},
		{
			name:     "empty output",
			outFile:  "",
			setup:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				err := tt.setup(tt.outFile)
				require.NoError(t, err)
				defer os.Remove(tt.outFile)
			}

			r := &Runner{
				OutFile: tt.outFile,
			}

			result := r.shouldWriteHeader()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTolower(t *testing.T) {
	assert.Equal(t, "hello", tolower("HELLO"))
	assert.Equal(t, "hello world", tolower("Hello World"))
	assert.Equal(t, "123", tolower("123"))
}
