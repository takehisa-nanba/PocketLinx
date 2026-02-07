package api

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
)

type portProxy struct {
	listener net.Listener
	target   string
	stop     chan struct{}
	mu       sync.RWMutex
	conns    map[net.Conn]struct{}
	connsMu  sync.Mutex
}

func (s *Server) syncProxies() {
	containers, err := s.engine.List()
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 127.0.0.x 方式: コンテナ固有のループバックIPで待ち受ける
	// 10.10.0.x -> 127.0.0.x にマッピング
	desiredProxies := make(map[string]string) // "winIP:port" -> "targetIP:targetPort"

	for _, c := range containers {
		if c.Status != "Running" || c.IP == "" {
			continue
		}

		// 10.10.0.x の x を抽出して 127.0.0.x を作成
		var lastOctet int
		fmt.Sscanf(c.IP, "10.10.0.%d", &lastOctet)
		winIP := fmt.Sprintf("127.0.0.%d", lastOctet)

		for _, p := range c.Ports {
			key := fmt.Sprintf("%s:%d", winIP, p.Host)
			desiredProxies[key] = fmt.Sprintf("%s:%d", c.IP, p.Container)
		}
	}

	// 1. 不要になったプロキシを削除
	for key, proxy := range s.proxies {
		if _, stillNeeded := desiredProxies[key]; !stillNeeded {
			fmt.Printf("[Proxy] Closing %s: Container stopped\n", key)
			proxy.listener.Close()
			close(proxy.stop)

			proxy.connsMu.Lock()
			for conn := range proxy.conns {
				conn.Close()
			}
			proxy.connsMu.Unlock()

			delete(s.proxies, key)
		}
	}

	// 2. 新しいプロキシを開始 (住所が違うので衝突しない)
	for key, target := range desiredProxies {
		if _, exists := s.proxies[key]; !exists {
			fmt.Printf("[Proxy] Starting proxy on %s -> %s\n", key, target)
			l, err := net.Listen("tcp", key)
			if err != nil {
				fmt.Printf("[Proxy] Failed to listen on %s: %v\n", key, err)
				continue
			}

			p := &portProxy{
				listener: l,
				target:   target,
				stop:     make(chan struct{}),
				conns:    make(map[net.Conn]struct{}),
			}
			s.proxies[key] = p

			go s.runProxy(p)
		}
	}
}

func (s *Server) runProxy(p *portProxy) {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.stop:
				return
			default:
				return
			}
		}

		p.connsMu.Lock()
		p.conns[conn] = struct{}{}
		p.connsMu.Unlock()

		go func(c net.Conn) {
			defer func() {
				c.Close()
				p.connsMu.Lock()
				delete(p.conns, c)
				p.connsMu.Unlock()
			}()

			p.mu.RLock()
			currentTarget := p.target
			p.mu.RUnlock()

			cmd := exec.Command("wsl.exe", "-d", "pocketlinx", "-u", "root", "--", "socat", "-", fmt.Sprintf("TCP:%s", currentTarget))

			wc, _ := cmd.StdinPipe()
			rc, _ := cmd.StdoutPipe()

			if err := cmd.Start(); err != nil {
				return
			}

			done := make(chan struct{}, 2)
			go func() {
				io.Copy(wc, c)
				wc.Close()
				done <- struct{}{}
			}()
			go func() {
				io.Copy(c, rc)
				done <- struct{}{}
			}()
			<-done
			cmd.Process.Kill()
		}(conn)
	}
}
