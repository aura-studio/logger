package logger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		Line string `json:"line"`
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
