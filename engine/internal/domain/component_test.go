package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestParseComponentRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantKind string
		wantNS   string
		wantName string
		wantErr  bool
	}{
		{"valid", "kind:Component/default/web-app", "Component", "default", "web-app", false},
		{"api kind", "kind:API/platform/users-api", "API", "platform", "users-api", false},
		{"missing prefix", "Component/default/web-app", "", "", "", true},
		{"wrong prefix", "type:Component/default/web-app", "", "", "", true},
		{"two segments", "kind:Component/default", "", "", "", true},
		{"four segments", "kind:Component/default/web-app/extra", "", "", "", true},
		{"empty ref", "", "", "", "", true},
		{"empty name", "kind:Component/default/", "", "", "", true},
		{"empty namespace", "kind:Component//web-app", "", "", "", true},
		{"empty kind", "kind:/default/web-app", "", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, ns, n, err := domain.ParseComponentRef(tc.ref)
			if tc.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, domain.ErrInvalidComponentRef))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantKind, k)
			require.Equal(t, tc.wantNS, ns)
			require.Equal(t, tc.wantName, n)
		})
	}
}

func TestNewComponent(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		c, err := domain.NewComponent("kind:Component/default/web-app", "team:platform")
		require.NoError(t, err)
		require.Equal(t, "Component", c.Kind)
		require.Equal(t, "default", c.Namespace)
		require.Equal(t, "web-app", c.Name)
		require.Equal(t, "team:platform", c.Owner)
	})
	t.Run("propagates parse error", func(t *testing.T) {
		_, err := domain.NewComponent("bad-ref", "team:platform")
		require.Error(t, err)
		require.True(t, errors.Is(err, domain.ErrInvalidComponentRef))
	})
}
