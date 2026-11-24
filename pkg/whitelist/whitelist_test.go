package whitelist

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitParseValid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"single channel", "C024BE91L", []string{"C024BE91L"}},
		{"channel, private channel and DM", "C0ALLOWED1,G0PRIVATE1,D0ALICE001", []string{"C0ALLOWED1", "G0PRIVATE1", "D0ALICE001"}},
		{"whitespace around entries", "  C0ALLOWED1 ,\tD0ALICE001\n", []string{"C0ALLOWED1", "D0ALICE001"}},
		{"duplicates", "C0ALLOWED1,C0ALLOWED1", []string{"C0ALLOWED1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := Parse(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, len(tt.want), w.Len())
			for _, id := range tt.want {
				assert.True(t, w.Allows(id), id)
			}
		})
	}
}

func TestUnitParseMalformed(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"empty", "", "set but empty"},
		{"whitespace only", "  \t ", "set but empty"},
		{"only a comma", ",", "entry 1 is empty"},
		{"stray comma", "C0ALLOWED1,,D0ALICE001", "entry 2 is empty"},
		{"trailing comma", "C0ALLOWED1,", "entry 2 is empty"},
		{"channel name", "general", `entry 1 ("general")`},
		{"hash name", "C0ALLOWED1,#general", `entry 2 ("#general")`},
		{"at user", "@alice", `entry 1 ("@alice")`},
		{"user ID", "U0ALICE001", `entry 1 ("U0ALICE001")`},
		{"lowercase ID", "c0allowed1", `entry 1 ("c0allowed1")`},
		{"too short", "C123", `entry 1 ("C123")`},
		{"inner space", "C0ALL OWED1", `entry 1 ("C0ALL OWED1")`},
		{"semicolon separator", "C0ALLOWED1;D0ALICE001", `entry 1 ("C0ALLOWED1;D0ALICE001")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := Parse(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, w)
		})
	}
}

func TestUnitAllows(t *testing.T) {
	w, err := Parse("C0ALLOWED1,D0ALICE001")
	require.NoError(t, err)

	assert.True(t, w.Allows("C0ALLOWED1"))
	assert.True(t, w.Allows("D0ALICE001"))
	assert.False(t, w.Allows("C0SECRET01"))
	assert.False(t, w.Allows("c0allowed1"), "match must be exact")
	assert.False(t, w.Allows(" C0ALLOWED1"), "match must be exact")
	assert.False(t, w.Allows("#allowed"), "names must be resolved to IDs first")
	assert.False(t, w.Allows(""))

	var none *Whitelist
	assert.True(t, none.Allows("C0SECRET01"), "no whitelist allows everything")
	assert.Equal(t, 0, none.Len())
}

func TestUnitFromEnv(t *testing.T) {
	t.Run("unset means no whitelist", func(t *testing.T) {
		// t.Setenv registers a restore of the original value; Unsetenv then
		// clears it for this test only.
		t.Setenv(EnvVar, "")
		require.NoError(t, os.Unsetenv(EnvVar))
		w, err := FromEnv()
		require.NoError(t, err)
		assert.Nil(t, w)
	})
	t.Run("set but empty is an error", func(t *testing.T) {
		t.Setenv(EnvVar, "")
		_, err := FromEnv()
		require.Error(t, err)
	})
	t.Run("malformed is an error", func(t *testing.T) {
		t.Setenv(EnvVar, "C0ALLOWED1,general")
		_, err := FromEnv()
		require.Error(t, err)
	})
	t.Run("valid", func(t *testing.T) {
		t.Setenv(EnvVar, "C0ALLOWED1, D0ALICE001")
		w, err := FromEnv()
		require.NoError(t, err)
		assert.Equal(t, 2, w.Len())
	})
}
