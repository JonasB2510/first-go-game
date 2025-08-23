package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	lobbies      = make(map[string]*Lobby)
	lobbiesMutex sync.RWMutex // Mutex für die lobbies map
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type SafeConnection struct {
	conn       *websocket.Conn
	writeMutex sync.Mutex
	readMutex  sync.Mutex
}

func NewSafeConnection(conn *websocket.Conn) *SafeConnection {
	return &SafeConnection{
		conn: conn,
	}
}

func (sc *SafeConnection) WriteMessage(messageType int, data []byte) error {
	sc.writeMutex.Lock()
	defer sc.writeMutex.Unlock()
	return sc.conn.WriteMessage(messageType, data)
}

func (sc *SafeConnection) ReadMessage() (int, []byte, error) {
	sc.readMutex.Lock()
	defer sc.readMutex.Unlock()
	return sc.conn.ReadMessage()
}

func (sc *SafeConnection) Close() error {
	sc.writeMutex.Lock()
	defer sc.writeMutex.Unlock()
	return sc.conn.Close()
}

type Lobby struct {
	Host         *SafeConnection
	HostPlayerID string // Add this to track host's player ID
	Clients      map[string]*SafeConnection
	clientsMutex sync.RWMutex // Mutex für die clients map
}

func NewLobby(host *SafeConnection) *Lobby {
	return &Lobby{
		Host:    host,
		Clients: make(map[string]*SafeConnection),
	}
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

func closeLobby(lobby_id string) {
	lobbiesMutex.Lock()
	defer lobbiesMutex.Unlock()

	if lobby, exists := lobbies[lobby_id]; exists {
		lobby.clientsMutex.RLock()
		for _, client_conn := range lobby.Clients {
			client_conn.Close()
		}
		lobby.clientsMutex.RUnlock()

		delete(lobbies, lobby_id)
		fmt.Println("Lobby closed:", lobby_id)
	}
}

// Helper function to safely get string value from interface{}
func getStringValue(data map[string]interface{}, key string) string {
	if val, exists := data[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func hostHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Host upgrade error:", err)
		return
	}

	safeConn := NewSafeConnection(conn)
	defer safeConn.Close()
	log.Println("Host WebSocket connection established")
	var lobbyID string

	// Keep the connection alive and handle messages
	for {
		_, message, err := safeConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Host WebSocket error: %v", err)
			} else {
				log.Println("Host WebSocket connection closed")
			}
			break
		}

		// Use map[string]interface{} for flexible JSON parsing
		var data map[string]interface{}
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			log.Printf("Raw message: %s", string(message))
			continue
		}

		command := getStringValue(data, "command")
		playerID := getStringValue(data, "player_id")

		switch command {
		case "registerHost":
			id := uuid.New()
			lobbyID = id.String()

			// Create the lobby BEFORE trying to access it
			lobbiesMutex.Lock()
			lobbies[lobbyID] = NewLobby(safeConn)
			lobbies[lobbyID].HostPlayerID = "" // Will be set when host registers as player
			lobbiesMutex.Unlock()

			fmt.Println("Lobby created:", lobbyID)

			response_data := map[string]string{
				"command":  "registerHostResponse",
				"lobby_id": lobbyID,
			}

			msg, err := json.Marshal(response_data)
			if err != nil {
				log.Printf("Error marshalling response: %v", err)
				continue
			}

			// Set up cleanup when connection closes
			defer closeLobby(lobbyID)

			if err := safeConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error writing message: %v", err)
			}

		case "registerPlayer":
			// Host is registering as a player in their own lobby
			lobbiesMutex.RLock()
			lobby, exists := lobbies[lobbyID]
			lobbiesMutex.RUnlock()

			if exists {
				lobby.HostPlayerID = playerID
				fmt.Printf("Host registered as player %s in lobby %s\n", playerID, lobbyID)

				// Send player ID confirmation back to host
				response_data := map[string]string{
					"type":      "player_id",
					"player_id": playerID,
				}
				msg, _ := json.Marshal(response_data)
				safeConn.WriteMessage(websocket.TextMessage, msg)
			}

		default:
			// Forward message to clients only, but not back to host
			if playerID != "" {
				lobbiesMutex.RLock()
				lobby, lobbyExists := lobbies[lobbyID]
				lobbiesMutex.RUnlock()

				if lobbyExists {
					// Don't forward messages from host back to host
					if playerID == lobby.HostPlayerID {
						// This is a message from the host, forward only to clients
						lobby.clientsMutex.RLock()
						for clientID, clientConn := range lobby.Clients {
							if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
								log.Printf("Error forwarding message to client %s: %v", clientID, err)
								// Remove disconnected client
								lobby.clientsMutex.RUnlock()
								lobby.clientsMutex.Lock()
								delete(lobby.Clients, clientID)
								lobby.clientsMutex.Unlock()
								lobby.clientsMutex.RLock()
							}
						}
						lobby.clientsMutex.RUnlock()
					} else {
						// This is a message from a client, forward to specific recipient
						lobby.clientsMutex.RLock()
						clientConn, clientExists := lobby.Clients[playerID]
						lobby.clientsMutex.RUnlock()

						if clientExists {
							// Forward the original message as-is
							if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
								log.Printf("Error forwarding message to client %s: %v", playerID, err)
								// Remove disconnected client
								lobby.clientsMutex.Lock()
								delete(lobby.Clients, playerID)
								lobby.clientsMutex.Unlock()
							}
						} else {
							log.Printf("Client %s not found in lobby %s", playerID, lobbyID)
						}
					}
				} else {
					log.Printf("Lobby %s not found", lobbyID)
				}
			} else {
				log.Printf("Received message without player_id: %s", string(message))
			}
		}
	}
}

func joinHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Join upgrade error:", err)
		return
	}

	safeConn := NewSafeConnection(conn)
	defer safeConn.Close()
	log.Println("Client WebSocket connection established")

	var playerID string = ""
	var playerLobbyID string = ""

	// Keep the connection alive and handle messages
	for {
		_, message, err := safeConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Client WebSocket error: %v", err)
			} else {
				log.Println("Client WebSocket connection closed")
			}
			// Clean up player from lobby when connection closes
			if playerID != "" && playerLobbyID != "" {
				lobbiesMutex.RLock()
				lobby, exists := lobbies[playerLobbyID]
				lobbiesMutex.RUnlock()

				if exists {
					lobby.clientsMutex.Lock()
					delete(lobby.Clients, playerID)
					lobby.clientsMutex.Unlock()
					fmt.Printf("Player %s removed from lobby %s\n", playerID, playerLobbyID)
				}
			}
			break
		}

		// Use map[string]interface{} for flexible JSON parsing
		var data map[string]interface{}
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			log.Printf("Raw message: %s", string(message))
			continue
		}

		command := getStringValue(data, "command")

		switch command {
		case "registerPlayer":
			inviteCode := getStringValue(data, "invite_code")
			if inviteCode == "" {
				log.Println("No invite_code provided for registerPlayer")
				continue
			}

			// Check if lobby exists
			lobbiesMutex.RLock()
			lobby, exists := lobbies[inviteCode]
			lobbiesMutex.RUnlock()

			if !exists {
				log.Printf("Lobby %s not found", inviteCode)
				// Send error response to client
				errorResponse := map[string]string{
					"type":  "error",
					"error": "Lobby not found",
				}
				errorMsg, _ := json.Marshal(errorResponse)
				safeConn.WriteMessage(websocket.TextMessage, errorMsg)
				continue
			}

			id := uuid.New()
			playerID = id.String()
			playerLobbyID = inviteCode

			lobby.clientsMutex.Lock()
			lobby.Clients[playerID] = safeConn
			lobby.clientsMutex.Unlock()

			response_data := map[string]string{
				"type":      "player_id",
				"player_id": playerID,
			}

			msg, err := json.Marshal(response_data)
			if err != nil {
				log.Printf("Error marshalling response: %v", err)
				continue
			}

			fmt.Printf("Added player %s to lobby %s\n", playerID, playerLobbyID)
			safeConn.WriteMessage(websocket.TextMessage, msg)

		default:
			if playerID == "" || playerLobbyID == "" {
				log.Printf("Message received before player registration: %s", string(message))
				continue
			}

			// Add player_id to the data and forward to host
			data["player_id"] = playerID

			msg, err := json.Marshal(data)
			if err != nil {
				log.Printf("Error marshalling message: %v", err)
				continue
			}

			// Check if lobby and host still exist
			lobbiesMutex.RLock()
			lobby, exists := lobbies[playerLobbyID]
			lobbiesMutex.RUnlock()

			if exists && lobby.Host != nil {
				if err := lobby.Host.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("Error forwarding message to host: %v", err)
				} else {
					if command != "get_players" {
						fmt.Printf("Forwarded command %s from player %s to host\n", command, playerID)
					}
				}
			} else {
				log.Printf("Host not found for lobby %s", playerLobbyID)
			}
		}
	}
}
