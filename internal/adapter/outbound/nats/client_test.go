package nats

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestConnection_ListAccountStreams_ShouldReturnExistingStreamNames(t *testing.T) {
	server := runNatsServer(t, natsServerConfig{
		serverJetStream:  true,
		accountJetStream: true,
	})
	nc := connectTestAccount(t, server)

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
	server := runNatsServer(t, natsServerConfig{
		serverJetStream:  true,
		accountJetStream: true,
	})
	nc := connectTestAccount(t, server)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldReturnEmpty_WhenJetStreamDisabledForAccount(t *testing.T) {
	server := runNatsServer(t, natsServerConfig{
		serverJetStream: true,
	})
	nc := connectTestAccount(t, server)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldReturnEmpty_WhenJetStreamDisabledOnServer(t *testing.T) {
	server := runNatsServer(t, natsServerConfig{})
	nc := connectTestAccount(t, server)

	conn := &connection{conn: nc}

	names, err := conn.ListAccountStreams()
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestConnection_ListAccountStreams_ShouldFail_WhenConnectionIsLost(t *testing.T) {
	server := runNatsServer(t, natsServerConfig{
		serverJetStream:  true,
		accountJetStream: true,
	})
	nc := connectTestAccount(t, server)

	conn := &connection{conn: nc}
	nc.Close()

	names, err := conn.ListAccountStreams()
	require.Error(t, err)
	require.Nil(t, names)
}

type natsServerConfig struct {
	serverJetStream  bool
	accountJetStream bool
}

func runNatsServer(t *testing.T, cfg natsServerConfig) *natsserver.Server {
	t.Helper()

	account := natsserver.NewAccount("A")
	opts := &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
		JetStream:             cfg.serverJetStream,
		StoreDir:              filepath.Join(t.TempDir(), "store"),
		Accounts:              []*natsserver.Account{account},
		Users: []*natsserver.User{{
			Username: "foo",
			Password: "bar",
			Account:  account,
		}},
	}

	server, err := natsserver.NewServer(opts)
	require.NoError(t, err)

	go server.Start()
	require.True(t, server.ReadyForConnections(10*time.Second), "nats-server did not become ready in time")

	if cfg.accountJetStream {
		registeredAccount, err := server.LookupAccount(account.Name)
		require.NoError(t, err)
		require.NoError(t, registeredAccount.EnableJetStream(nil, nil))
	}

	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})

	return server
}

func connectTestAccount(t *testing.T, server *natsserver.Server) *nats.Conn {
	t.Helper()

	nc, err := nats.Connect(server.ClientURL(), nats.UserInfo("foo", "bar"))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	return nc
}
