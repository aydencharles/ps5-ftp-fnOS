package runtime

import "testing"

func TestLoadHonorsFnOSEnvironment(t *testing.T) {
	t.Setenv("TRIM_APPDEST", "/app")
	t.Setenv("TRIM_PKGVAR", "/var/app")
	t.Setenv("TRIM_PKGTMP", "/tmp/app")
	t.Setenv("TRIM_SERVICE_PORT", "9999")
	c := Load()
	if c.AppDir != "/app" || c.DataDir != "/var/app" || c.TempDir != "/tmp/app" || c.Port != "9999" {
		t.Fatalf("config=%+v", c)
	}
}
