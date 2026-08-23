package validators

import "os"

// readWholeFile is the default implementation of readFile.
func readWholeFile(path string) ([]byte, error) { return os.ReadFile(path) }
