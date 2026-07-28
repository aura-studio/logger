package logger

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	awsease "github.com/aura-studio/aws-ease"
	"github.com/sirupsen/logrus"
)

func TestTangoHookBlocksLogCall(t *testing.T) {
	received := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(received)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	hook, err := NewHook("tango", `{"level":"info","format":"message","url":"`+srv.URL+`","timeout":"1s"}`)
	if err != nil {
		t.Fatalf("new hook: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&JSONFormatter{JSONFormatter: logrus.JSONFormatter{DisableTimestamp: true}})
	logger.AddHook(hook)

	done := make(chan struct{})
	go func() {
		logger.WithField("#type", "track").Info("")
		close(done)
	}()

	<-received

	select {
	case <-done:
		t.Fatal("logger returned before HTTP response completed")
	default:
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("logger did not return after HTTP response completed")
	}
}

func TestTangoHookPostsFormattedMessage(t *testing.T) {
	var got struct {
		Line     string `json:"line"`
		Database string `json:"Database"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %s, want application/json", ct)
		}
		if token := r.Header.Get("X-Token"); token != "secret" {
			t.Fatalf("X-Token = %s, want secret", token)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	hook, err := NewHook("tango", `{
		"level":"info",
		"format":"message",
		"url":"`+srv.URL+`",
		"database":"tango_aurora",
		"headers":{"X-Token":"secret"}
	}`)
	if err != nil {
		t.Fatalf("new hook: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&JSONFormatter{JSONFormatter: logrus.JSONFormatter{DisableTimestamp: true}})
	logger.AddHook(hook)
	entry := logger.WithField("#type", "track").WithField("properties", map[string]interface{}{"coin": 7})
	entry.Data["Bytes"] = []byte(`{"stale":true}`)
	entry.Info("")

	var line map[string]interface{}
	if err := json.Unmarshal([]byte(got.Line), &line); err != nil {
		t.Fatalf("decode line: %v", err)
	}
	if line["#type"] != "track" {
		t.Fatalf("#type = %v, want track", line["#type"])
	}
	if got.Database != "tango_aurora" {
		t.Fatalf("Database = %q, want tango_aurora", got.Database)
	}
	if _, ok := line["Bytes"]; ok {
		t.Fatal("unexpected Bytes cache in HTTP payload")
	}
	properties, ok := line["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties type = %T, want map", line["properties"])
	}
	if properties["coin"] != float64(7) {
		t.Fatalf("coin = %v, want 7", properties["coin"])
	}
}

func TestTangoHookOmitsDatabaseWhenNotConfigured(t *testing.T) {
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	hook, err := NewHook("tango", `{"level":"info","format":"message","url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatalf("new hook: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&JSONFormatter{JSONFormatter: logrus.JSONFormatter{DisableTimestamp: true}})
	logger.AddHook(hook)
	logger.WithField("#type", "track").Info("")

	if _, ok := got["Database"]; ok {
		t.Fatal("legacy payload unexpectedly contains Database")
	}
}

// TestTangoHookURLUsesAwsEase 钉住 url 走 aws-ease 语义：scheme 决定后端，
// 非法 scheme 的错误能用 aws-ease 哨兵判别（http/lambda 的正向路径分别由
// TestTangoHookPostsFormattedMessage 与真实环境集成验证覆盖）。
func TestTangoHookURLUsesAwsEase(t *testing.T) {
	hook, err := NewHook("tango", `{"level":"info","format":"message","url":"ftp://nope"}`)
	if err != nil {
		t.Fatalf("new hook: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&JSONFormatter{JSONFormatter: logrus.JSONFormatter{DisableTimestamp: true}})
	entry := logger.WithField("#type", "track")

	fireErr := hook.Fire(entry)
	if !errors.Is(fireErr, awsease.ErrUnknownScheme) {
		t.Fatalf("Fire err = %v, want errors.Is awsease.ErrUnknownScheme", fireErr)
	}
}

func TestTangoHookSkipsWhenURLIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	hook, err := NewHook("tango", `{"level":"info","format":"message","url":""}`)
	if err != nil {
		t.Fatalf("new hook: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&JSONFormatter{JSONFormatter: logrus.JSONFormatter{DisableTimestamp: true}})
	logger.AddHook(hook)
	logger.WithField("#type", "track").Info("")
}
