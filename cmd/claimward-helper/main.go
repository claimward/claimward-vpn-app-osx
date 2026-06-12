// Command claimward-helper is the privileged macOS helper for Claimward.
//
// It runs as a root LaunchDaemon and listens on a Unix socket. The unprivileged
// app sends it a tunnel spec; the helper creates the utun device and brings the
// WireGuard tunnel up/down via wireguard-go (operations that require root).
//
// Protocol: one JSON hproto.Request per connection, one hproto.Response back.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/claimward/claimward-vpn-app-osx/internal/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/client"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"
	"github.com/claimward/claimward-vpn-client/pkg/routeclient"
	"github.com/claimward/claimward-vpn-client/pkg/wgkey"
	"github.com/claimward/claimward-vpn-client/pkg/wgtun"
)

type helper struct {
	log         *slog.Logger
	mu          sync.Mutex
	tun         *wgtun.Tunnel
	assigned    string
	watchCancel context.CancelFunc
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	socket := os.Getenv("CLAIMWARD_HELPER_SOCKET")
	if socket == "" {
		socket = hproto.DefaultSocketPath
	}

	// Clean up a stale socket from a previous run.
	if _, err := os.Stat(socket); err == nil {
		_ = os.Remove(socket)
	}

	ln, err := net.Listen("unix", socket)
	if err != nil {
		log.Error("listen", "socket", socket, "err", err)
		os.Exit(1)
	}
	// MVP: world-accessible socket on the loopback of the machine. Harden with a
	// dedicated group + 0660 (and verify peer creds) before shipping.
	if err := os.Chmod(socket, 0o666); err != nil {
		log.Warn("chmod socket", "err", err)
	}

	h := &helper{log: log}

	// Cleanup on signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		h.shutdown()
		_ = ln.Close()
		_ = os.Remove(socket)
		os.Exit(0)
	}()

	log.Info("claimward-helper listening", "socket", socket)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Error("accept", "err", err)
			return
		}
		go h.handle(conn)
	}
}

func (h *helper) handle(conn net.Conn) {
	defer conn.Close()
	var req hproto.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeResp(conn, hproto.Response{Error: "bad request: " + err.Error()})
		return
	}

	switch req.Action {
	case hproto.ActionConnect:
		writeResp(conn, h.connect(req.Connect))
	case hproto.ActionUp:
		writeResp(conn, h.up(req.Tunnel))
	case hproto.ActionDown:
		writeResp(conn, h.down())
	case hproto.ActionStatus:
		writeResp(conn, h.status())
	case hproto.ActionUpdateRoutes:
		writeResp(conn, h.updateRoutes(req.AllowedIPs))
	default:
		writeResp(conn, hproto.Response{Error: "unknown action: " + req.Action})
	}
}

// connect does the whole server-facing flow as root (exempt from macOS Local
// Network privacy): enroll, bring up the tunnel, and watch for route pushes.
func (h *helper) connect(spec *hproto.ConnectSpec) hproto.Response {
	if spec == nil {
		return hproto.Response{Error: "missing connect spec"}
	}
	pair, err := wgkey.ParsePrivate(spec.PrivateKey)
	if err != nil {
		return hproto.Response{Error: "private key: " + err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := client.New(spec.ServerURL).Enroll(ctx, spec.Bearer, pair.Public, protocol.DeviceInfo{
		Name: spec.DeviceName, OS: "darwin", Platform: "app-osx",
	})
	if err != nil {
		return hproto.Response{Error: "enroll: " + err.Error()}
	}
	cfg, err := client.TunnelConfig(resp, pair.Private)
	if err != nil {
		return hproto.Response{Error: err.Error()}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watchCancel != nil {
		h.watchCancel()
		h.watchCancel = nil
	}
	if h.tun != nil {
		_ = h.tun.Close()
		h.tun = nil
	}
	tun, err := wgtun.Up(cfg)
	if err != nil {
		return hproto.Response{Error: err.Error()}
	}
	h.tun = tun
	h.assigned = resp.AssignedIP

	if resp.GRPCEndpoint != "" {
		wctx, wcancel := context.WithCancel(context.Background())
		h.watchCancel = wcancel
		ep, bearer, pub := resp.GRPCEndpoint, spec.Bearer, pair.Public.String()
		go func() {
			_ = routeclient.Watch(wctx, ep, bearer, pub, func(u routeclient.Update) {
				h.mu.Lock()
				t := h.tun
				h.mu.Unlock()
				if t == nil {
					return
				}
				if e := t.UpdateRoutes(u.AllowedIPs); e != nil {
					h.log.Error("apply pushed routes", "err", e)
				} else {
					h.log.Info("routes updated", "serial", u.Serial, "allowed_ips", u.AllowedIPs)
				}
			})
		}()
	}
	h.log.Info("connected", "interface", tun.Name(), "assigned", resp.AssignedIP, "routes", resp.AllowedIPs)
	return hproto.Response{OK: true, Connected: true, Interface: tun.Name(), AssignedIP: resp.AssignedIP}
}

func (h *helper) up(spec *hproto.TunnelSpec) hproto.Response {
	if spec == nil {
		return hproto.Response{Error: "missing tunnel spec"}
	}
	cfg, err := toConfig(spec)
	if err != nil {
		return hproto.Response{Error: err.Error()}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tun != nil {
		_ = h.tun.Close()
		h.tun = nil
	}
	tun, err := wgtun.Up(cfg)
	if err != nil {
		return hproto.Response{Error: err.Error()}
	}
	h.tun = tun
	h.log.Info("tunnel up", "interface", tun.Name(), "address", spec.Address)
	return hproto.Response{OK: true, Connected: true, Interface: tun.Name()}
}

func (h *helper) down() hproto.Response {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watchCancel != nil {
		h.watchCancel()
		h.watchCancel = nil
	}
	if h.tun != nil {
		_ = h.tun.Close()
		h.tun = nil
		h.assigned = ""
		h.log.Info("tunnel down")
	}
	return hproto.Response{OK: true, Connected: false}
}

func (h *helper) updateRoutes(allowedIPs []string) hproto.Response {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tun == nil {
		return hproto.Response{Error: "no active tunnel"}
	}
	if err := h.tun.UpdateRoutes(allowedIPs); err != nil {
		return hproto.Response{Error: err.Error()}
	}
	h.log.Info("routes updated", "allowed_ips", allowedIPs, "interface", h.tun.Name())
	return hproto.Response{OK: true, Connected: true, Interface: h.tun.Name()}
}

func (h *helper) status() hproto.Response {
	h.mu.Lock()
	defer h.mu.Unlock()
	resp := hproto.Response{OK: true, Connected: h.tun != nil}
	if h.tun != nil {
		resp.Interface = h.tun.Name()
		resp.AssignedIP = h.assigned
	}
	return resp
}

func (h *helper) shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tun != nil {
		_ = h.tun.Close()
		h.tun = nil
	}
}

func toConfig(spec *hproto.TunnelSpec) (wgtun.Config, error) {
	priv, err := wgkey.ParsePrivate(spec.PrivateKey)
	if err != nil {
		return wgtun.Config{}, fmt.Errorf("private key: %w", err)
	}
	serverPub, err := wgkey.ParsePublic(spec.ServerPublicKey)
	if err != nil {
		return wgtun.Config{}, fmt.Errorf("server public key: %w", err)
	}
	return wgtun.Config{
		PrivateKey:      priv.Private,
		ServerPublicKey: serverPub,
		Endpoint:        spec.Endpoint,
		AllowedIPs:      spec.AllowedIPs,
		Address:         spec.Address,
		DNS:             spec.DNS,
		MTU:             spec.MTU,
		Keepalive:       spec.Keepalive,
	}, nil
}

func writeResp(conn net.Conn, resp hproto.Response) {
	_ = json.NewEncoder(conn).Encode(resp)
}
