package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const iterations = 100000

type Server struct {
	password string
	name     string
	inj      Injector
	reg      *ClientRegistry
}

func deriveProof(password string, salt, nonce []byte) string {
	key := pbkdf2.Key([]byte(password), salt, iterations, 32, sha256.New)
	mac := hmac.New(sha256.New, key)
	mac.Write(nonce)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	addr := c.RemoteAddr()
	r := bufio.NewScanner(c)
	r.Buffer(make([]byte, 64*1024), 1<<20)

	if !r.Scan() {
		return
	}
	var hello In
	if json.Unmarshal(r.Bytes(), &hello); hello.T != "hello" {
		s.send(c, map[string]any{"t": "error", "code": "bad_hello", "msg": "expected hello"})
		return
	}
	salt := make([]byte, 16)
	nonce := make([]byte, 16)
	rand.Read(salt)
	rand.Read(nonce)
	s.send(c, map[string]any{
		"t": "challenge", "salt": base64.StdEncoding.EncodeToString(salt),
		"nonce": base64.StdEncoding.EncodeToString(nonce), "iter": iterations,
	})
	if !r.Scan() {
		return
	}
	var auth In
	json.Unmarshal(r.Bytes(), &auth)
	want := deriveProof(s.password, salt, nonce)
	if auth.T != "auth" || subtle.ConstantTimeCompare([]byte(auth.Proof), []byte(want)) != 1 {
		s.send(c, map[string]any{"t": "error", "code": "auth_failed", "msg": "wrong password"})
		log.Printf("auth failed from %s (%s)", addr, hello.Name)
		return
	}
	s.send(c, map[string]any{"t": "auth_ok", "server": s.name, "ver": "0.1"})
	log.Printf("client connected: %s (%s)", hello.Name, addr)

	if s.reg != nil {
		cid := s.reg.Register(hello.Name, ipOf(addr))
		defer s.reg.Deregister(cid)
	}

	for r.Scan() {
		var m In
		if json.Unmarshal(r.Bytes(), &m) != nil {
			continue
		}
		switch m.T {
		case "ping":
			s.send(c, map[string]any{"t": "pong", "ts": m.Ts})
		case "bye":
			return
		default:
			applyEvent(s.inj, m)
		}
	}
	log.Printf("client disconnected: %s", addr)
}

func (s *Server) send(c net.Conn, v any) {
	b, _ := encode(v)
	c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	c.Write(b)
}

func (s *Server) Listen(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	log.Printf("control listening on tcp/%d", port)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func sanitizeName(n string) string { return strings.TrimSpace(n) }

// ipOf returns the host part of a net.Addr, dropping the port.
func ipOf(a net.Addr) string {
	if h, _, err := net.SplitHostPort(a.String()); err == nil {
		return h
	}
	return a.String()
}
