package overlord

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type standardResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

type standardResponseRaw struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// ResponseError writes an error response to the client with standard format
func ResponseError(w http.ResponseWriter, err string, code int) {
	log.Printf("ERROR [%d]: %s", code, err)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(standardResponse{
		Status: "error",
		Data:   err,
	}); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}

// ResponseSuccess writes a success response to the client
func ResponseSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(standardResponse{
		Status: "success",
		Data:   data,
	}); err != nil {
		log.Printf("Failed to encode success response: %v", err)
	}
}

// ResponseJSON writes a JSON response to the client with standard format
func ResponseJSON(w http.ResponseWriter, data string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	status := "success"
	if code != http.StatusOK {
		status = "error"
	}

	if err := json.NewEncoder(w).Encode(standardResponseRaw{
		Status: status,
		Data:   json.RawMessage(data),
	}); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// WebSocketSendError sends an error message to the client
func WebSocketSendError(ws *websocket.Conn, err string) {
	log.Println(err)
	msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, err)
	if err := ws.WriteMessage(websocket.CloseMessage, msg); err != nil {
		log.Printf("Failed to write WebSocket close message: %v", err)
	}
	ws.Close()
}
