package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"chase/internal/room"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingEvery  = 25 * time.Second
	maxMsgSize = 4096
)

var upgrader = websocket.Upgrader{ReadBufferSize: 2048, WriteBufferSize: 8192}

// wsClient adapts a gorilla connection to room.Client. Send never blocks: a
// client that cannot keep up is dropped rather than stalling the room actor.
type wsClient struct {
	conn *websocket.Conn
	out  chan []byte
	once sync.Once
	done chan struct{}
}

func (c *wsClient) Send(data []byte) {
	select {
	case c.out <- data:
	default:
		c.Close(websocket.CloseTryAgainLater, "too slow")
	}
}

func (c *wsClient) Close(code int, reason string) {
	c.once.Do(func() {
		_ = c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		close(c.done)
		_ = c.conn.Close()
	})
}

// GET /ws?role=screen&code=X
// GET /ws?role=host&code=X&key=K
// GET /ws?role=player&code=X&playerId=Y&token=Z
//
// Identity comes from the URL, so there is no join handshake message and the
// server can push the first state immediately. Rejections are reported in the
// close frame's reason because no write pump exists yet.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	q := r.URL.Query()
	reject := func(code int, reason string) {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		_ = conn.Close()
	}

	rm := s.mgr.Get(strings.ToUpper(q.Get("code")))
	if rm == nil {
		reject(room.CloseRoomGone, "that room no longer exists")
		return
	}
	v := room.Viewer{Role: q.Get("role"), PlayerID: q.Get("playerId")}
	secret := q.Get("key")
	if v.Role == "player" {
		secret = q.Get("token")
	}

	c := &wsClient{conn: conn, out: make(chan []byte, 32), done: make(chan struct{})}
	if err := rm.Attach(v, secret, c); err != nil {
		reject(room.CloseBadAuth, err.Error())
		return
	}
	go c.writePump()
	c.readPump(rm)
}

func (c *wsClient) readPump(rm *room.Room) {
	defer func() {
		rm.Detach(c)
		c.Close(websocket.CloseNormalClosure, "bye")
	}()
	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		rm.Incoming(c, data)
	}
}

func (c *wsClient) writePump() {
	t := time.NewTicker(pingEvery)
	defer t.Stop()
	for {
		select {
		case msg := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.Close(websocket.CloseAbnormalClosure, "write failed")
				return
			}
		case <-t.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.Close(websocket.CloseAbnormalClosure, "ping failed")
				return
			}
		case <-c.done:
			return
		}
	}
}
