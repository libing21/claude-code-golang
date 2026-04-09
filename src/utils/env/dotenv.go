package env

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// LoadDotEnv tries candidate dotenv paths in order and loads the first existing one.
// This intentionally supports the current workspace layout where the Go port lives
// beside the TS repo (`claude-code-runing/.env`).
func LoadDotEnv(paths ...string) error {
	var lastErr error
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			lastErr = err
			continue
		}
		if err := godotenv.Overload(path); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("no dotenv file found")
}
