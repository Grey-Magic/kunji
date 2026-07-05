package sink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// jsonMarshal is a thin wrapper used by FileSink so sink.go doesn't need to
// import encoding/json directly.
func jsonMarshal(r *models.ValidationResult) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// sha256Sum wraps crypto/sha256 for FileSink.
func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// sanitize replaces characters that would be unsafe in filenames. Exposed for
// any future sink that wants to derive a filename from a string.
func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-' || c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

// ensure os/filepath/hex are referenced even if a sink is removed later
var (
	_ = os.WriteFile
	_ = filepath.Join
	_ = hex.EncodeToString
)
