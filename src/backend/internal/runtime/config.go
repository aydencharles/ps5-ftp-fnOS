package runtime

import (
	"os"
	"path/filepath"
)

type Config struct {
	AppDir  string
	DataDir string
	TempDir string
	UIDir   string
	Listen  string
	Port    string
	DBPath  string
	KeyPath string
	LogPath string
}

func env(def string, keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return def
}

func Load() Config {
	exe, _ := os.Executable()
	appDir := env(filepath.Dir(exe), "TRIM_APPDEST", "APP_DIR")
	dataDir := env(filepath.Join(appDir, "var"), "TRIM_PKGVAR", "DATA_DIR")
	tempDir := env(filepath.Join(dataDir, "tmp"), "TRIM_PKGTMP", "TEMP_DIR")
	return Config{
		AppDir:  appDir,
		DataDir: dataDir,
		TempDir: tempDir,
		UIDir:   env(filepath.Join(appDir, "ui"), "UI_DIR"),
		Listen:  env("0.0.0.0", "LISTEN_ADDR", "BIND_ADDR"),
		Port:    env("8100", "TRIM_SERVICE_PORT", "PORT", "SERVICE_PORT"),
		DBPath:  filepath.Join(dataDir, "ps5-ftp-manager.db"),
		KeyPath: filepath.Join(dataDir, "profile.key"),
		LogPath: filepath.Join(dataDir, "server.log"),
	}
}

func (c Config) Prepare() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(c.TempDir, 0o700)
}
