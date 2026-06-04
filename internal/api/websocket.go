package api

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const webSocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func acceptWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if !headerTokenContains(r.Header.Get("Connection"), "upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "WebSocket Upgrade 头无效")
		return nil, fmt.Errorf("missing websocket upgrade headers")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "WebSocket 版本无效")
		return nil, fmt.Errorf("unsupported websocket version")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "WebSocket Key 缺失")
		return nil, fmt.Errorf("missing websocket key")
	}
	sum := sha1.Sum([]byte(key + webSocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "当前响应不支持 WebSocket")
		return nil, err
	}
	_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func writeWebSocketText(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(length))
		header = append(header, buf...)
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func writeWebSocketClose(conn net.Conn) {
	_, _ = conn.Write([]byte{0x88, 0x00})
}

func headerTokenContains(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
