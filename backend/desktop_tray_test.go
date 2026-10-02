package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

type fakeX11 struct {
	cookie    []byte // required MIT-MAGIC-COOKIE-1 value; empty accepts anyone
	setupSize uint16 // additional setup data, in 4-byte units
	atom      uint32
	owner     uint32
	truncate  int // close after this many replies, when positive
}

// serve implements the three exchanges used by x11TrayOwner.
func (f fakeX11) serve(conn net.Conn) {
	defer conn.Close()
	order := binary.LittleEndian
	padded := func(size int) int { return (size + 3) & ^3 }
	header := make([]byte, 12)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 'l' {
		return
	}
	nameSize, dataSize := int(order.Uint16(header[6:])), int(order.Uint16(header[8:]))
	auth := make([]byte, padded(nameSize)+padded(dataSize))
	if _, err := io.ReadFull(conn, auth); err != nil {
		return
	}
	name, data := auth[:nameSize], auth[padded(nameSize):padded(nameSize)+dataSize]
	if len(f.cookie) > 0 && (string(name) != "MIT-MAGIC-COOKIE-1" || !bytes.Equal(data, f.cookie)) {
		_, _ = conn.Write([]byte{0, 0, 11, 0, 0, 0, 0, 0})
		return
	}
	setup := make([]byte, 8+int(f.setupSize)*4)
	setup[0] = 1
	order.PutUint16(setup[6:], f.setupSize)
	if f.truncate == 1 {
		setup = setup[:4]
	}
	if _, err := conn.Write(setup); err != nil || f.truncate == 1 {
		return
	}
	request := make([]byte, 8)
	if _, err := io.ReadFull(conn, request); err != nil || request[0] != 16 {
		return
	}
	if _, err := io.CopyN(io.Discard, conn, int64(padded(int(order.Uint16(request[4:]))))); err != nil {
		return
	}
	reply := make([]byte, 32)
	reply[0] = 1
	order.PutUint32(reply[8:], f.atom)
	if f.truncate == 2 {
		reply = reply[:10]
	}
	if _, err := conn.Write(reply); err != nil || f.truncate == 2 || f.atom == 0 {
		return
	}
	if _, err := io.ReadFull(conn, request); err != nil || request[0] != 23 || order.Uint32(request[4:]) != f.atom {
		return
	}
	reply = make([]byte, 32)
	reply[0] = 1
	order.PutUint32(reply[8:], f.owner)
	_, _ = conn.Write(reply)
}

func trayOwnerAgainst(t *testing.T, server fakeX11, cookie []byte) bool {
	t.Helper()
	client, remote := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); server.serve(remote) }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	result := x11TrayOwner(client, cookie, "_NET_SYSTEM_TRAY_S0")
	client.Close()
	<-done
	return result
}

func TestX11TrayOwner(t *testing.T) {
	cookie := []byte("0123456789abcdef")
	for _, test := range []struct {
		name   string
		server fakeX11
		cookie []byte
		want   bool
	}{
		{"authenticated host", fakeX11{cookie: cookie, setupSize: 30, atom: 321, owner: 77}, cookie, true},
		{"open server", fakeX11{setupSize: 2, atom: 5, owner: 1}, nil, true},
		{"no owner", fakeX11{cookie: cookie, atom: 321}, cookie, false},
		{"unknown atom", fakeX11{cookie: cookie, owner: 77}, cookie, false},
		{"wrong cookie", fakeX11{cookie: cookie, atom: 321, owner: 77}, []byte("ffffffffffffffff"), false},
		{"missing cookie", fakeX11{cookie: cookie, atom: 321, owner: 77}, nil, false},
		{"oversized setup", fakeX11{setupSize: 0xffff, atom: 321, owner: 77}, nil, false},
		{"truncated setup", fakeX11{truncate: 1, atom: 321, owner: 77}, nil, false},
		{"truncated reply", fakeX11{truncate: 2, atom: 321, owner: 77}, nil, false},
	} {
		if got := trayOwnerAgainst(t, test.server, test.cookie); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func xauthEntry(family uint16, address, display, name, data string) []byte {
	var entry bytes.Buffer
	_ = binary.Write(&entry, binary.BigEndian, family)
	for _, field := range []string{address, display, name, data} {
		_ = binary.Write(&entry, binary.BigEndian, uint16(len(field)))
		entry.WriteString(field)
	}
	return entry.Bytes()
}

func TestX11Cookie(t *testing.T) {
	file := bytes.Join([][]byte{
		xauthEntry(0, "\x7f\x00\x00\x01", "0", "MIT-MAGIC-COOKIE-1", "network"),
		xauthEntry(256, "host", "1", "MIT-MAGIC-COOKIE-1", "other-display"),
		xauthEntry(256, "host", "0", "XDM-AUTHORIZATION-1", "other-scheme"),
		xauthEntry(256, "host", "0", "MIT-MAGIC-COOKIE-1", "local"),
		xauthEntry(65535, "", "0", "MIT-MAGIC-COOKIE-1", "wildcard"),
	}, nil)
	if got := x11Cookie(file, "0"); string(got) != "local" {
		t.Fatalf("display 0: %q", got)
	}
	if got := x11Cookie(file, "1"); string(got) != "other-display" {
		t.Fatalf("display 1: %q", got)
	}
	if got := x11Cookie(file, "2"); got != nil {
		t.Fatalf("unknown display: %q", got)
	}
	// Mutter writes Xwayland entries without a display number.
	mutter := bytes.Join([][]byte{
		xauthEntry(256, "host", "", "MIT-MAGIC-COOKIE-1", "any-display"),
		xauthEntry(256, "host", "3", "MIT-MAGIC-COOKIE-1", "display-3"),
	}, nil)
	if got := x11Cookie(mutter, "0"); string(got) != "any-display" {
		t.Fatalf("wildcard entry: %q", got)
	}
	if got := x11Cookie(mutter, "3"); string(got) != "display-3" {
		t.Fatalf("exact entry must win: %q", got)
	}
	// Truncated and corrupt files must not panic or return partial entries.
	for size := range file {
		_ = x11Cookie(file[:size], "0")
	}
	for _, data := range [][]byte{nil, {1}, {1, 0, 0xff, 0xff}, bytes.Repeat([]byte{0xff}, 64)} {
		if got := x11Cookie(data, "0"); got != nil {
			t.Fatalf("malformed input returned %q", got)
		}
	}
}

func TestLegacyTrayHostRejectsInvalidDisplay(t *testing.T) {
	a := &App{}
	t.Setenv("XAUTHORITY", t.TempDir()+"/missing")
	for _, display := range []string{"", "wayland-0", "remote:0", ":", ":x", ":-1", ":70000", ":0.x", ":0.0.0", ":0.900"} {
		t.Setenv("DISPLAY", display)
		if a.HasLegacyTrayHost() {
			t.Fatalf("accepted display %q", display)
		}
	}
}
