package config

import "testing"

// env is a fake environment for tests.
func env(vars map[string]string) Env {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

// Variable names and a home directory shared by the cases below.
const (
	configHome = "XDG_CONFIG_HOME"
	dataHome   = "XDG_DATA_HOME"
	home       = "HOME"
	homeDir    = "/home/u"
)

func TestDefaultPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		vars map[string]string
		want Paths
	}{
		{
			name: "XDG set",
			vars: map[string]string{configHome: "/cfg", dataHome: "/data", home: homeDir},
			want: Paths{ConfigFile: "/cfg/kitsune/server.toml", DataDir: "/data/kitsune-server"},
		},
		{
			name: "HOME fallback",
			vars: map[string]string{home: homeDir},
			want: Paths{ConfigFile: "/home/u/.config/kitsune/server.toml", DataDir: "/home/u/.local/share/kitsune-server"},
		},
		{
			name: "relative XDG ignored",
			vars: map[string]string{configHome: "cfg", dataHome: "", home: homeDir},
			want: Paths{ConfigFile: "/home/u/.config/kitsune/server.toml", DataDir: "/home/u/.local/share/kitsune-server"},
		},
		{
			name: "nothing usable",
			vars: map[string]string{home: "relative"},
			want: Paths{},
		},
		{
			name: "config dir only",
			vars: map[string]string{configHome: "/cfg"},
			want: Paths{ConfigFile: "/cfg/kitsune/server.toml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DefaultPaths(env(tt.vars)); got != tt.want {
				t.Errorf("DefaultPaths() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
