package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	lobbies = make(map[string]*Lobby)
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Lobby struct {
	Host    *websocket.Conn
	Clients map[string]*websocket.Conn
}

func main() {
	setupRoutes()
	log.Printf("Server starting on port %s", os.Args[1])
	http.ListenAndServe("0.0.0.0:"+os.Args[1], nil)
}

func setupRoutes() {
	http.HandleFunc("/host", hostHandler)
	http.HandleFunc("/join", joinHandler)
}

func hostHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Host upgrade error:", err)
		return
	}
	defer conn.Close()
	log.Println("Host WebSocket connection established")

	// Keep the connection alive and handle messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Host WebSocket error: %v", err)
			} else {
				log.Println("Host WebSocket connection closed")
			}
			break
		}
		//fmt.Printf("Host message: %s\n", string(message))
		// Add your host message handling logic here
		var data map[string]string
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			continue
		}
		response_data := make(map[string]string)

		switch data["command"] {
		case "registerHost":
			id := uuid.New()
			lobbyID := id.String()

			// Create the lobby BEFORE trying to access it
			lobbies[lobbyID] = &Lobby{
				Host:    conn,
				Clients: make(map[string]*websocket.Conn),
			}
			fmt.Println("Lobby created:", lobbyID)

			response_data["command"] = "registerHostResponse"
			response_data["lobby_id"] = lobbyID

			msg, err := json.Marshal(response_data)
			if err != nil {
				log.Printf("Error marshalling response: %v", err)
				return
			}

			// Set up cleanup when connection closes
			defer delete(lobbies, lobbyID)
			defer fmt.Println("Lobby closed:", lobbyID)

			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error writing message: %v", err)
			}
		default:
			log.Printf("Unknown command: %s", data["command"])
		}
	}
}

func joinHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Join upgrade error:", err)
		return
	}
	defer conn.Close()
	log.Println("Client WebSocket connection established")

	// Keep the connection alive and handle messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Client WebSocket error: %v", err)
			} else {
				log.Println("Client WebSocket connection closed")
			}
			break
		}
		//fmt.Printf("Client message: %s\n", string(message))
		// Add your client message handling logic here
		var data map[string]string
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			continue
		}
		response_data := make(map[string]string)
		switch data["command"] {
		case "registerPlayer":
			id := uuid.New()
			playerID := id.String()
			lobbies[data["invite_code"]].Clients[playerID] = conn
			response_data["type"] = "registerHostResponse"
			response_data["player_id"] = playerID

			msg, err := json.Marshal(response_data)
			if err != nil {
				log.Printf("Error marshalling response: %v", err)
				return
			}
			fmt.Println("added player to: %v" + playerID)
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}
}
