package overlord

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// ResponseError writes an error response to the client
func ResponseError(w http.ResponseWriter, err string, code int) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(errorResponse{Error: err})
}

// ResponseJSON writes a JSON response to the client
func ResponseJSON(w http.ResponseWriter, json string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(json))
	w.WriteHeader(code)
}

// ResponseSuccess writes a success response to the client
func ResponseSuccess(w http.ResponseWriter, data json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(successResponse{Status: "success", Data: data})
}

// WebSocketSendError sends an error message to the client
func WebSocketSendError(ws *websocket.Conn, err string) {
	log.Println(err)
	msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, err)
	ws.WriteMessage(websocket.CloseMessage, msg)
	ws.Close()
}
