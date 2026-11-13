// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestMultiNodeConnection_TransportFailoverAndCertificatePinning(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"reject", "stall"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			// Given
			server, attempts := newDatabaseTestServer(t, failure)
			port := databaseTestServerPort(t, server)
			fingerprint := sha256.Sum256(server.Certificate().Raw)
			info := &config.ConnectionInfo{
				Host: "localhost", DBPort: 1, AllowAllNodes: true,
				CertFingerprint: hex.EncodeToString(fingerprint[:]),
				Endpoints: []config.ConnectionEndpoint{
					{Node: "n11", Host: "localhost", DBPort: port, Eligible: true},
					{Node: "n12", Host: "127.0.0.1", DBPort: port, Eligible: true},
				},
			}
			database, err := NewExasolConnection(
				config.NewDeploymentDir(t.TempDir()),
				info,
				"sys",
				"secret",
				false,
			)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			// When
			err = database.Connect(ctx)

			// Then
			require.NoError(t, err)
			defer database.Close()
			require.Len(t, attempts(), 2)
			require.NotEqual(t, attempts()[0], attempts()[1])
		})
	}
}

func TestMultiNodeConnection_RejectsMismatchingCertificates(t *testing.T) {
	t.Parallel()

	// Given
	server, attempts := newDatabaseTestServer(t, "")
	port := databaseTestServerPort(t, server)
	info := &config.ConnectionInfo{
		Host: "localhost", DBPort: port, AllowAllNodes: true,
		CertFingerprint: strings.Repeat("0", 64),
		Endpoints: []config.ConnectionEndpoint{
			{Host: "localhost", DBPort: port, Eligible: true},
			{Host: "127.0.0.1", DBPort: port, Eligible: true},
		},
	}
	database, err := NewExasolConnection(
		config.NewDeploymentDir(t.TempDir()),
		info,
		"sys",
		"secret",
		false,
	)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// When
	err = database.Connect(ctx)

	// Then
	require.ErrorContains(t, err, "does not match the expected fingerprint")
	require.Empty(t, attempts(), "TLS must fail before any HTTP request or login")
}

func TestPreviousLauncher_ConnectsWithMultiNodeManifest(t *testing.T) {
	t.Parallel()

	previous := os.Getenv("EXASOL_PREVIOUS_LAUNCHER")
	if previous == "" {
		t.Skip("set EXASOL_PREVIOUS_LAUNCHER to verify a preceding launcher binary")
	}

	// Given
	server, _ := newDatabaseTestServer(t, "")
	port := databaseTestServerPort(t, server)
	fingerprint := sha256.Sum256(server.Certificate().Raw)
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, config.WriteDeploymentVersionMarker(deployment, "0.0.0"))
	require.NoError(t, os.MkdirAll(deployment.InstallationDir(), 0o700))
	require.NoError(t, os.WriteFile(deployment.InstallManifestPath(),
		[]byte("name: Test\ndatabaseConnection:\n  allowAllNodes: true\n"), 0o600))
	state := &config.ExasolPersonalState{VersionCheckEnabled: false}
	require.NoError(t, state.SetWorkflowStateAndWrite(&config.WorkflowStateRunning{}, deployment))
	require.NoError(
		t,
		config.WriteSecrets(deployment.Root(), &config.Secrets{DbPassword: "secret"}),
	)
	require.NoError(t, config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend: "tofu", DeploymentId: "compatibility-test",
		Connection: &config.DeploymentConnection{
			Host: "127.0.0.1", DBPort: port,
			CertFingerprint: hex.EncodeToString(fingerprint[:]),
		},
	}))

	// When
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	//nolint:gosec // G702: operator-provided executable; separate arguments, no shell.
	command := exec.CommandContext(
		ctx,
		previous,
		"connect",
		"--deployment-dir",
		deployment.Root(),
		"-c",
		"SELECT 1",
		"--json",
	)
	output, err := command.CombinedOutput()

	// Then
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "statementType")
}

func newDatabaseTestServer(t *testing.T, failure string) (*httptest.Server, func() []string) {
	t.Helper()

	var mutex sync.Mutex
	var attempts []string
	var server *httptest.Server
	server = httptest.NewTLSServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			mutex.Lock()
			attempts = append(attempts, request.Host)
			fail := failure != "" && len(attempts) == 1
			mutex.Unlock()
			if fail && failure == "stall" {
				<-request.Context().Done()
				return
			}
			if fail {
				http.Error(writer, "node unavailable", http.StatusServiceUnavailable)
				return
			}
			connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()

			key, ok := server.Certificate().PublicKey.(*rsa.PublicKey)
			if !ok {
				t.Error("database test server requires an RSA certificate")
				return
			}
			for {
				var command map[string]any
				if err := connection.ReadJSON(&command); err != nil {
					return
				}
				var data any
				switch command["command"] {
				case "login":
					data = map[string]any{
						"publicKeyModulus":  fmt.Sprintf("%x", key.N),
						"publicKeyExponent": strconv.FormatInt(int64(key.E), 16),
					}
				case "execute":
					data = map[string]any{
						"numResults": 1,
						"results": []any{map[string]any{
							"resultType": "resultSet",
							"resultSet": map[string]any{
								"numColumns":       1,
								"numRows":          1,
								"numRowsInMessage": 1,
								"columns": []any{
									map[string]any{
										"name":     "VALUE",
										"dataType": map[string]any{"type": "VARCHAR", "size": 100},
									},
								},
								"data": []any{[]string{"1"}},
							},
						}},
					}
				default:
					data = map[string]any{}
				}
				if err := connection.WriteJSON(
					map[string]any{"status": "ok", "responseData": data},
				); err != nil {
					return
				}
				if command["command"] == "disconnect" {
					return
				}
			}
		}),
	)
	t.Cleanup(server.Close)

	return server, func() []string {
		mutex.Lock()
		defer mutex.Unlock()

		return append([]string(nil), attempts...)
	}
}

func databaseTestServerPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	address, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "database test server requires a TCP listener")

	return address.Port
}

//nolint:tparallel // Subtests rewrite the same saved installation manifest.
func TestLauncher_MultiNodeEligibility(t *testing.T) {
	t.Parallel()

	launcher := os.Getenv("EXASOL_TEST_LAUNCHER")
	if launcher == "" {
		t.Skip("set EXASOL_TEST_LAUNCHER to verify the launcher connection workflow")
	}

	// Given
	server, _ := newDatabaseTestServer(t, "")
	port := databaseTestServerPort(t, server)
	fingerprint := sha256.Sum256(server.Certificate().Raw)
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, config.WriteDeploymentVersionMarker(deployment, "0.0.0"))
	require.NoError(t, os.MkdirAll(deployment.InstallationDir(), 0o700))
	state := &config.ExasolPersonalState{VersionCheckEnabled: false}
	require.NoError(t, state.SetWorkflowStateAndWrite(&config.WorkflowStateRunning{}, deployment))
	require.NoError(
		t,
		config.WriteSecrets(deployment.Root(), &config.Secrets{DbPassword: "secret"}),
	)
	require.NoError(t, config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend: "tofu", DeploymentId: "multi-node-cli-test",
		Connection: &config.DeploymentConnection{
			Host: "127.0.0.2", DBPort: port,
			CertFingerprint: hex.EncodeToString(fingerprint[:]),
		},
		Nodes: map[string]config.DeploymentNode{
			"n11": {
				PublicIp: "127.0.0.2",
				Database: config.DeploymentDatabase{DbPort: strconv.Itoa(port)},
			},
			"n12": {
				PublicIp: "127.0.0.1",
				Database: config.DeploymentDatabase{DbPort: strconv.Itoa(port)},
			},
		},
	}))

	//nolint:paralleltest // Subtests rewrite the same saved installation manifest.
	for _, allNodes := range []bool{true, false} {
		t.Run(strconv.FormatBool(allNodes), func(t *testing.T) {
			// Given
			require.NoError(t, os.WriteFile(
				deployment.InstallManifestPath(),
				fmt.Appendf(
					nil,
					"name: Test\ndatabaseConnection:\n  allowAllNodes: %t\n",
					allNodes,
				),
				0o600,
			))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			//nolint:gosec // G702: operator-provided executable; separate arguments, no shell.
			command := exec.CommandContext(
				ctx,
				launcher,
				"connect",
				"--deployment-dir",
				deployment.Root(),
				"-c",
				"SELECT 1",
				"--json",
			)
			// When
			output, err := command.CombinedOutput()
			// Then
			if allNodes {
				require.NoError(t, err, "%s", output)
				require.Contains(t, string(output), "statementType")
			} else {
				require.Error(
					t,
					err,
					"unmarked deployment must not connect to the available secondary",
				)
				require.Contains(t, string(output), "127.0.0.2")
			}
		})
	}
}
