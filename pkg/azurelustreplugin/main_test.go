/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitKlogFlags(t *testing.T) {
	// Arrange
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	// Act
	err := initKlogFlags(fs)

	// Assert
	require.NoError(t, err)

	expectedFlags := map[string]string{
		"logtostderr":                      "true",
		"legacy_stderr_threshold_behavior": "false",
		// klog's severityValue.String() returns the numeric value; INFO = 0
		"stderrthreshold": "0",
	}

	for name, want := range expectedFlags {
		f := fs.Lookup(name)
		require.NotNilf(t, f, "flag %q not found", name)
		assert.Equalf(t, want, f.Value.String(), "flag %q", name)
	}
}

func TestInitKlogFlags_UserOverride(t *testing.T) {
	// Arrange
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	err := initKlogFlags(fs)
	require.NoError(t, err)

	// Act — simulate user passing --stderrthreshold=WARNING on the command line
	err = fs.Parse([]string{"--stderrthreshold=WARNING"})

	// Assert
	require.NoError(t, err)
	f := fs.Lookup("stderrthreshold")
	require.NotNilf(t, f, "flag %q not found", "stderrthreshold")
	// WARNING = severity 1
	assert.Equalf(t, "1", f.Value.String(), "flag %q after user override", "stderrthreshold")
}

func TestInitKlogFlags_InvalidThreshold(t *testing.T) {
	// Arrange
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	err := initKlogFlags(fs)
	require.NoError(t, err)

	// Act — simulate user passing an invalid threshold
	err = fs.Parse([]string{"--stderrthreshold=GARBAGE"})

	// Assert
	assert.Error(t, err, "expected error for invalid stderrthreshold value")
}

func TestTrapClosedConnErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "closed connection error returns nil",
			err:     errors.New("accept tcp [::]:1234: use of closed network connection"),
			wantNil: true,
		},
		{
			name:    "other error is returned",
			err:     errors.New("bind: address already in use"),
			wantNil: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trapClosedConnErr(tt.err)
			if tt.wantNil {
				assert.NoError(t, got, "expected nil error for input: %v", tt.err)
			} else {
				assert.Error(t, got, "expected non-nil error for input: %v", tt.err)
			}
		})
	}
}

func TestExportMetrics_ServesPrometheusContent(t *testing.T) {
	// Bind a random port, then close it so exportMetrics can use it.
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to find free port")
	addr := l.Addr().String()
	require.NoError(t, l.Close(), "failed to close temp listener")

	// Set the flag and call exportMetrics.
	*metricsAddress = addr
	t.Cleanup(func() { *metricsAddress = "" })
	exportMetrics()

	// Poll until the server is ready (max 3 seconds).
	var resp *http.Response
	deadline := time.Now().Add(3 * time.Second)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet,
			fmt.Sprintf("http://%s/metrics", addr), nil)
		require.NoError(t, reqErr, "failed to create request")
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, err, "metrics endpoint never became ready")
	t.Cleanup(func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	})

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected HTTP 200 from /metrics")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")
	assert.Contains(t, string(body), "# HELP", "expected prometheus exposition format")
}

func TestExportMetrics_DisabledWhenEmpty(t *testing.T) {
	*metricsAddress = ""
	t.Cleanup(func() { *metricsAddress = "" })
	// Should return immediately without error or side effects.
	exportMetrics()
}
