package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMock_Copy_RecordsCall verifies that Copy records the call in CopyLog.
func TestMock_Copy_RecordsCall(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-gastown"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	err := m.Copy(ctx, id, "/tmp/taxiway-env-123", "/home/lab/.config/taxiway/env")
	require.NoError(t, err)

	require.Len(t, m.CopyLog, 1)
	require.Equal(t, id, m.CopyLog[0].ID)
	require.Equal(t, "/tmp/taxiway-env-123", m.CopyLog[0].Src)
	require.Equal(t, "/home/lab/.config/taxiway/env", m.CopyLog[0].Dst)
}

// TestMock_Copy_MultipleCalls verifies ordering is preserved.
func TestMock_Copy_MultipleCalls(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-gastown"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	require.NoError(t, m.Copy(ctx, id, "/tmp/file1", "/dest1"))
	require.NoError(t, m.Copy(ctx, id, "/tmp/file2", "/dest2"))

	require.Len(t, m.CopyLog, 2)
	require.Equal(t, "/dest1", m.CopyLog[0].Dst)
	require.Equal(t, "/dest2", m.CopyLog[1].Dst)
}

// TestDryRun_Copy_DoesNotCallInner verifies dry-run prints and returns nil
// without forwarding to the inner driver.
func TestDryRun_Copy_DoesNotCallInner(t *testing.T) {
	ctx := context.Background()
	inner := newMock(t)
	dr := NewDryRun(inner)
	id := "taxiway-test"

	err := dr.Copy(ctx, id, "/tmp/src", "/id/dst")
	require.NoError(t, err)

	// Inner mock must NOT have received the call.
	require.Empty(t, inner.CopyLog)
}
