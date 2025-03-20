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
	json.NewEncoder(w).Encode(standardResponse{
		Status: "error",
		Data:   err,
	})
}

// ResponseSuccess writes a success response to the client
func ResponseSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(standardResponse{
		Status: "success",
		Data:   data,
	})
}

// ResponseJSON writes a JSON response to the client with standard format
func ResponseJSON(w http.ResponseWriter, data string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	status := "success"
	if code != http.StatusOK {
		status = "error"
	}

	json.NewEncoder(w).Encode(standardResponseRaw{
		Status: status,
		Data:   json.RawMessage(data),
	})
}

// WebSocketSendError sends an error message to the client
func WebSocketSendError(ws *websocket.Conn, err string) {
	log.Println(err)
	msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, err)
	ws.WriteMessage(websocket.CloseMessage, msg)
	ws.Close()
}
