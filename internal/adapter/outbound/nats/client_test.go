package nats

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestConnection_ListAccountStreams_ShouldReturnExistingStreamNames(t *testing.T) {
	server := runNatsServerFromConfig(t, fmt.Sprintf(`
listen: 127.0.0.1:-1
jetstream: enabled
store_dir: %q
no_auth_user: foo
accounts: {
  A: {
    jetstream: enabled
    users: [ {user: foo} ]
  },
}
`, filepath.Join(t.TempDir(), "store")))

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateStream(context.Background(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	_, err = js.CreateStream(context.Background(), jetstream.StreamConfig{Name: "INVOICES", Subjects: []string{"invoices.>"}})
	require.NoError(t, err)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Equal(t, []string{"INVOICES", "ORDERS"}, names)
}

func TestConnection_ListAccountStreams_ShouldReturnEmpty_WhenNoStreamsExist(t *testing.T) {
	server := runNatsServerFromConfig(t, fmt.Sprintf(`
listen: 127.0.0.1:-1
jetstream: enabled
store_dir: %q
no_auth_user: foo
accounts: {
  A: {
    jetstream: enabled
    users: [ {user: foo} ]
  },
}
`, filepath.Join(t.TempDir(), "store")))

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldReturnEmpty_WhenJetStreamDisabledForAccount(t *testing.T) {
	server := runNatsServerFromConfig(t, fmt.Sprintf(`
listen: 127.0.0.1:-1
jetstream: enabled
store_dir: %q
no_auth_user: foo
accounts: {
  A: {
    users: [ {user: foo} ]
  },
}
`, filepath.Join(t.TempDir(), "store")))

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldReturnEmpty_WhenJetStreamDisabledOnServer(t *testing.T) {
	server := runNatsServerFromConfig(t, `
listen: 127.0.0.1:-1
no_auth_user: foo
accounts: {
  A: {
    users: [ {user: foo} ]
  },
}
`)

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldFail_WhenConnectionIsLost(t *testing.T) {
	server := runNatsServerFromConfig(t, fmt.Sprintf(`
listen: 127.0.0.1:-1
jetstream: enabled
store_dir: %q
no_auth_user: foo
accounts: {
  A: {
    jetstream: enabled
    users: [ {user: foo} ]
  },
}
`, filepath.Join(t.TempDir(), "store")))

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)

	conn := &connection{conn: nc}
	nc.Close()

	names, err := conn.ListAccountStreams()
	require.Error(t, err)
	require.Nil(t, names)
}

func runNatsServerFromConfig(t *testing.T, config string) *natsserver.Server {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "nats.conf")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	opts, err := natsserver.ProcessConfigFile(configPath)
	require.NoError(t, err)

	server, err := natsserver.NewServer(opts)
	require.NoError(t, err)

	go server.Start()
	require.True(t, server.ReadyForConnections(10*time.Second), "nats-server did not become ready in time")

	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})

	return server
}
