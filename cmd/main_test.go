/*
Copyright 2026.

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
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func Test_initLogger_ShouldDefaultToInfo_WhenNoNAuthLoggingFlagsAreSet(t *testing.T) {
	opts := zap.Options{
		Development: true,
	}

	logger, err := initLogger(&opts, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if !opts.Development {
		t.Fatal("expected local defaults to keep zap development mode")
	}
	if opts.Level == nil {
		t.Fatal("expected default log level to be configured")
	}
	if !opts.Level.Enabled(zapcore.InfoLevel) {
		t.Fatal("expected default log level to emit info logs")
	}
	if opts.Level.Enabled(zapcore.DebugLevel) {
		t.Fatal("expected default log level to suppress debug logs")
	}
	if logger.V(1).Enabled() {
		t.Fatal("expected default logger to suppress verbose messages")
	}
}

func Test_initLogger_ShouldConfigureJSONInfoLogging(t *testing.T) {
	var log bytes.Buffer
	opts := zap.Options{
		Development: true,
		DestWriter:  &log,
	}

	logger, err := initLogger(&opts, "json", "info")
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("structured message", "field", "value")

	var entry map[string]any
	if err := json.Unmarshal(log.Bytes(), &entry); err != nil {
		t.Fatalf("expected JSON log entry, got %q: %s", log.String(), err.Error())
	}

	if entry["level"] != "info" {
		t.Fatalf("expected info level, got %q", entry["level"])
	}
	if entry["msg"] != "structured message" {
		t.Fatalf("expected structured message, got %q", entry["msg"])
	}
	if entry["field"] != "value" {
		t.Fatalf("expected structured field, got %q", entry["field"])
	}
	if _, ok := entry["ts"]; !ok {
		t.Fatal("expected timestamp field")
	}
}

func Test_initLogger_ShouldDefaultToInfo_WhenOnlyJSONFormatIsSet(t *testing.T) {
	opts := zap.Options{
		Development: true,
	}

	if _, err := initLogger(&opts, "json", ""); err != nil {
		t.Fatal(err)
	}

	if !opts.Development {
		t.Fatal("expected JSON format to preserve existing zap development mode")
	}
	if opts.Level == nil {
		t.Fatal("expected JSON logging to configure a default log level")
	}
	if !opts.Level.Enabled(zapcore.InfoLevel) {
		t.Fatal("expected JSON logging to emit info logs by default")
	}
	if opts.Level.Enabled(zapcore.DebugLevel) {
		t.Fatal("expected JSON logging to suppress debug logs by default")
	}
}

func Test_initLogger_ShouldAcceptTextFormat(t *testing.T) {
	opts := zap.Options{
		Development: true,
	}

	if _, err := initLogger(&opts, "text", ""); err != nil {
		t.Fatal(err)
	}

	if !opts.Development {
		t.Fatal("expected text format to keep zap development mode")
	}
}

func Test_initLogger_ShouldEnableVerboseMessages_WhenDebugLevelIsConfigured(t *testing.T) {
	opts := zap.Options{
		Development: true,
	}

	logger, err := initLogger(&opts, "", "debug")
	if err != nil {
		t.Fatal(err)
	}

	if !logger.V(1).Enabled() {
		t.Fatal("expected debug logging to enable verbose messages")
	}
}

func Test_initLogger_ShouldPreserveControllerRuntimeLevel_WhenNAuthLevelIsUnset(t *testing.T) {
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
	}

	if _, err := initLogger(&opts, "", ""); err != nil {
		t.Fatal(err)
	}

	if !opts.Level.Enabled(zapcore.DebugLevel) {
		t.Fatal("expected an explicitly configured controller-runtime level to be preserved")
	}
}

func Test_initLogger_ShouldSupportWarnLevel(t *testing.T) {
	opts := zap.Options{
		Development: true,
	}

	if _, err := initLogger(&opts, "json", "warn"); err != nil {
		t.Fatal(err)
	}

	if opts.Level.Enabled(zapcore.InfoLevel) {
		t.Fatal("expected warn level to suppress info logs")
	}
	if !opts.Level.Enabled(zapcore.WarnLevel) {
		t.Fatal("expected warn level to emit warn logs")
	}
}

func Test_initLogger_ShouldRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		format string
		level  string
	}{
		{name: "format", format: "logfmt", level: "info"},
		{name: "console_alias", format: "console", level: "info"},
		{name: "level", format: "json", level: "trace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := zap.Options{
				Development: true,
			}

			if _, err := initLogger(&opts, tt.format, tt.level); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
