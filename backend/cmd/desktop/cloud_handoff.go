package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	errInvalidCloudHandoffState = errors.New("invalid cloud handoff state")
	errCloudHandoffExpired      = errors.New("cloud handoff expired")
)

type desktopCloudHandoff struct {
	mu          sync.Mutex
	clientState string
	code        string
	state       string
	expiresAt   time.Time
	server      *http.Server
	closeOnce   sync.Once
}

func (h *desktopCloudHandoff) close() {
	if h == nil {
		return
	}
	h.closeOnce.Do(func() {
		if h.server != nil {
			_ = h.server.Close()
		}
	})
}

func (a *DesktopApp) beginCloudHandoff(clientState string) (string, error) {
	clientState = strings.TrimSpace(clientState)
	if clientState == "" || len(clientState) > 160 {
		return "", errInvalidCloudHandoffState
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen for cloud handoff: %w", err)
	}
	handoff := &desktopCloudHandoff{clientState: clientState, expiresAt: time.Now().Add(10 * time.Minute)}
	mux := http.NewServeMux()
	mux.HandleFunc("/cloud/callback", handoff.handleCallback)
	handoff.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	a.cloudHandoffMu.Lock()
	previous := a.cloudHandoff
	a.cloudHandoff = handoff
	a.cloudHandoffMu.Unlock()
	if previous != nil {
		previous.close()
	}
	go func() {
		_ = handoff.server.Serve(listener)
	}()
	time.AfterFunc(10*time.Minute, func() { a.expireCloudHandoff(handoff) })

	return (&url.URL{Scheme: "http", Host: listener.Addr().String(), Path: "/cloud/callback"}).String(), nil
}

func (h *desktopCloudHandoff) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		h.writeMessage(w, http.StatusBadRequest, "缺少登录回调参数")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if time.Now().After(h.expiresAt) {
		h.writeMessage(w, http.StatusGone, "登录回调已过期")
		return
	}
	if state != h.clientState {
		h.writeMessage(w, http.StatusBadRequest, "登录回调状态无效")
		return
	}
	if h.code != "" {
		h.writeMessage(w, http.StatusConflict, "登录回调已处理")
		return
	}
	h.code = code
	h.state = state
	h.writeMessage(w, http.StatusOK, "登录完成，可以关闭此窗口")
}

func (h *desktopCloudHandoff) writeMessage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}

func (a *DesktopApp) pollCloudHandoff(clientState string) (string, error) {
	clientState = strings.TrimSpace(clientState)
	a.cloudHandoffMu.Lock()
	handoff := a.cloudHandoff
	a.cloudHandoffMu.Unlock()
	if handoff == nil {
		return "", nil
	}
	handoff.mu.Lock()
	defer handoff.mu.Unlock()
	if clientState == "" || clientState != handoff.clientState {
		return "", errInvalidCloudHandoffState
	}
	if time.Now().After(handoff.expiresAt) {
		return "", errCloudHandoffExpired
	}
	if handoff.code == "" {
		return "", nil
	}
	payload, err := json.Marshal(map[string]string{"code": handoff.code, "state": handoff.state})
	if err != nil {
		return "", fmt.Errorf("encode cloud handoff: %w", err)
	}
	return string(payload), nil
}

func (a *DesktopApp) expireCloudHandoff(expected *desktopCloudHandoff) {
	a.cloudHandoffMu.Lock()
	if a.cloudHandoff == expected {
		a.cloudHandoff = nil
	}
	a.cloudHandoffMu.Unlock()
	expected.close()
}

func (a *DesktopApp) clearCloudHandoff() {
	a.cloudHandoffMu.Lock()
	handoff := a.cloudHandoff
	a.cloudHandoff = nil
	a.cloudHandoffMu.Unlock()
	if handoff != nil {
		handoff.close()
	}
}
