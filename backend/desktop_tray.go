package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// HasLegacyTrayHost checks the X11 tray selection without desktop-specific tools.
func (a *App) HasLegacyTrayHost() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	display := strings.TrimPrefix(os.Getenv("DISPLAY"), "unix")
	if !strings.HasPrefix(display, ":") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(display, ":"), ".")
	number, err := strconv.Atoi(parts[0])
	if err != nil || number < 0 || number > 65535 {
		return false
	}
	screen := 0
	if len(parts) > 2 {
		return false
	}
	if len(parts) == 2 {
		screen, err = strconv.Atoi(parts[1])
		if err != nil || screen < 0 || screen > 255 {
			return false
		}
	}
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".Xauthority")
		}
	}
	var cookie []byte
	if file, err := os.Open(path); err == nil {
		data, _ := io.ReadAll(io.LimitReader(file, 64*1024))
		_ = file.Close()
		cookie = x11Cookie(data, strconv.Itoa(number))
	}
	conn, err := net.DialTimeout("unix", "/tmp/.X11-unix/X"+strconv.Itoa(number), 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(750 * time.Millisecond))
	return x11TrayOwner(conn, cookie, "_NET_SYSTEM_TRAY_S"+strconv.Itoa(screen))
}

// x11Cookie returns the MIT-MAGIC-COOKIE-1 for a local display. An empty
// display field matches every display, as written by Mutter for Xwayland;
// an entry naming the display takes precedence.
func x11Cookie(data []byte, display string) []byte {
	var wildcard []byte
	reader := bytes.NewReader(data)
	readField := func() ([]byte, error) {
		var size uint16
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			return nil, err
		}
		field := make([]byte, size)
		_, err := io.ReadFull(reader, field)
		return field, err
	}
	for reader.Len() > 0 {
		var family uint16
		if binary.Read(reader, binary.BigEndian, &family) != nil {
			return nil
		}
		_, err := readField()
		if err != nil {
			return nil
		}
		number, err := readField()
		if err != nil {
			return nil
		}
		name, err := readField()
		if err != nil {
			return nil
		}
		cookie, err := readField()
		if err != nil {
			return nil
		}
		if (family != 256 && family != 65535) || string(name) != "MIT-MAGIC-COOKIE-1" {
			continue
		}
		if string(number) == display {
			return cookie
		}
		if len(number) == 0 && wildcard == nil {
			wildcard = cookie
		}
	}
	return wildcard
}

func x11TrayOwner(conn net.Conn, cookie []byte, selection string) bool {
	order := binary.LittleEndian
	padded := func(size int) int { return (size + 3) & ^3 }
	var auth []byte
	if len(cookie) > 0 {
		auth = []byte("MIT-MAGIC-COOKIE-1")
	}
	setup := make([]byte, 12+padded(len(auth))+padded(len(cookie)))
	setup[0] = 'l'
	order.PutUint16(setup[2:], 11)
	order.PutUint16(setup[6:], uint16(len(auth)))
	order.PutUint16(setup[8:], uint16(len(cookie)))
	copy(setup[12:], auth)
	copy(setup[12+padded(len(auth)):], cookie)
	if _, err := conn.Write(setup); err != nil {
		return false
	}
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 1 {
		return false
	}
	extra := int(order.Uint16(header[6:])) * 4
	if extra > 64*1024 {
		return false
	}
	if _, err := io.CopyN(io.Discard, conn, int64(extra)); err != nil {
		return false
	}
	request := make([]byte, 8+padded(len(selection)))
	request[0] = 16
	request[1] = 1
	order.PutUint16(request[2:], uint16(len(request)/4))
	order.PutUint16(request[4:], uint16(len(selection)))
	copy(request[8:], selection)
	if _, err := conn.Write(request); err != nil {
		return false
	}
	reply := make([]byte, 32)
	if _, err := io.ReadFull(conn, reply); err != nil || reply[0] != 1 {
		return false
	}
	atom := order.Uint32(reply[8:])
	if atom == 0 {
		return false
	}
	request = make([]byte, 8)
	request[0] = 23
	order.PutUint16(request[2:], 2)
	order.PutUint32(request[4:], atom)
	if _, err := conn.Write(request); err != nil {
		return false
	}
	if _, err := io.ReadFull(conn, reply); err != nil || reply[0] != 1 {
		return false
	}
	return order.Uint32(reply[8:]) != 0
}
