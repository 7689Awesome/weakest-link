// Package httpapi is the network layer: REST for room bootstrap, a WebSocket
// endpoint for gameplay, and static file serving for the frontend.
package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"chase/internal/room"
)

type Server struct {
	mgr       *room.Manager
	staticDir string
}

func New(mgr *room.Manager, staticDir string) http.Handler {
	s := &Server{mgr: mgr, staticDir: staticDir}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rooms", s.createRoom)
	mux.HandleFunc("GET /api/rooms/{code}", s.roomInfo)
	mux.HandleFunc("POST /api/rooms/{code}/players", s.joinRoom)
	mux.HandleFunc("GET /ws", s.serveWS)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	// Short, memorable entry points: /host for the quizmaster, /screen for the TV.
	mux.HandleFunc("GET /host", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/host.html", http.StatusFound) })
	mux.HandleFunc("GET /screen", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/screen.html", http.StatusFound) })
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	return secureHeaders(mux)
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// POST /api/rooms -> {roomCode, hostKey}
//
// hostKey is the secret that grants quizmaster control. The room code is
// public (players need it to join) so it must never be enough on its own.
func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	rm, key, err := s.mgr.Create()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"roomCode": rm.Code, "hostKey": key})
}

// GET /api/rooms/{code} -> {exists, phase, playerCount, full}
func (s *Server) roomInfo(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	rm := s.mgr.Get(code)
	if rm == nil {
		writeJSON(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	info, ok := rm.Info()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exists": true, "phase": info.Phase, "playerCount": info.PlayerCount, "full": info.Full,
	})
}

// POST /api/rooms/{code}/players {name, rejoinId?, rejoinToken?} -> {playerId, playerToken}
func (s *Server) joinRoom(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	rm := s.mgr.Get(code)
	if rm == nil {
		writeErr(w, http.StatusNotFound, "that room does not exist")
		return
	}
	var body struct {
		Name        string `json:"name"`
		RejoinID    string `json:"rejoinId"`
		RejoinToken string `json:"rejoinToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	id, token, err := rm.Join(body.Name, body.RejoinID, body.RejoinToken)
	if err != nil {
		status := http.StatusConflict
		if err.Error() == "room closed" {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"playerId": id, "playerToken": token})
}
