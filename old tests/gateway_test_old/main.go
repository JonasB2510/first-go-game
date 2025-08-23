package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/gorilla/websocket"
)

const (
	screenWidth  = 1000
	screenHeight = 480
)

var (
	running  = true
	bkgColor = rl.NewColor(147, 211, 196, 255)

	// Sprites
	grassSprite          rl.Texture2D
	fenceSprite          rl.Texture2D
	hillSprite           rl.Texture2D
	waterSprite          rl.Texture2D
	woodHouseWallsSprite rl.Texture2D
	woodHouseRoofSprite  rl.Texture2D
	tilledSprite         rl.Texture2D
	doorSprite           rl.Texture2D
	tex                  rl.Texture2D
	playerSprite         rl.Texture2D

	// Player state
	playerSrc                                     rl.Rectangle
	playerDest                                    rl.Rectangle
	playerMoving                                  bool
	playerDir                                     int
	playerUp, playerDown, playerRight, playerLeft bool
	playerFrame                                   int
	maxFrames                                     = 4
	frameCount                                    int
	playerSpeed                                   float32 = 3

	// Map
	tileDest   rl.Rectangle
	tileSrc    rl.Rectangle
	tileMap    []int
	srcMap     []string
	mapW, mapH int
	map_file   = "resource/maps/second.map"
	loadedMap  []string

	// Audio
	musicPaused bool
	music       rl.Music

	// Camera
	cam rl.Camera2D

	// Networking
	host_type        string
	server_url_ws    string
	gateway_server   string
	websocket_client *websocket.Conn

	// Multiplayer
	playersMutex      sync.RWMutex
	joinedPlayers     = make(map[int]map[string]rl.Rectangle)
	joinPlayerID      int
	lastPlayerUpdate  time.Time
	lastMapUpdate     time.Time
	mapUpdateCooldown = 1000

	serverID              string
	registeredWithGateway bool

	gatewayMutex sync.RWMutex
	gameServers  = make(map[string]*GameServer)
	gatewayPort  string

	gatewayMode         bool
	gatewayURL          string
	gatewayConn         *websocket.Conn
	availableServers    []GameServer
	serversMutex        sync.RWMutex
	showServerBrowser   bool
	selectedServerIndex int
	serverBrowserScroll int
	gatewayConnected    bool

	// UI state
	uiFont         rl.Font
	showingMessage bool
	messageText    string
	messageTimer   float32
)

type MovementData struct {
	PlayerID    int  `json:"playerid"`
	PlayerUp    bool `json:"playerUp"`
	PlayerLeft  bool `json:"playerLeft"`
	PlayerDown  bool `json:"playerDown"`
	PlayerRight bool `json:"playerRight"`
}

type RespawnData struct {
	Respawn bool `json:"respawn"`
}

type GetPlayersData struct {
	ExcludeID int `json:"exclude_id"`
}

type GameServer struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	Port        string    `json:"port"`
	PlayerCount int       `json:"player_count"`
	MaxPlayers  int       `json:"max_players"`
	GameMode    string    `json:"game_mode"`
	LastPing    time.Time `json:"last_ping"`
	Active      bool      `json:"active"`
}

type GatewayMessage struct {
	Type    string      `json:"type"`
	Command string      `json:"command"`
	Data    interface{} `json:"data"`
}

type ServerRegistration struct {
	ServerID   string `json:"server_id"`
	Address    string `json:"address"`
	Port       string `json:"port"`
	MaxPlayers int    `json:"max_players"`
	GameMode   string `json:"game_mode"`
}

type ServerListRequest struct {
	FilterGameMode string `json:"filter_game_mode,omitempty"`
	MinPlayers     int    `json:"min_players,omitempty"`
	MaxPlayers     int    `json:"max_players,omitempty"`
}

type ServerListResponse struct {
	Type    string       `json:"type"`
	Servers []GameServer `json:"servers"`
}

type JoinServerRequest struct {
	ServerID string `json:"server_id"`
}

type JoinServerResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Address string `json:"address"`
	Port    string `json:"port"`
	Message string `json:"message"`
}

type GatewayClientMessage struct {
	Type    string      `json:"type"`
	Command string      `json:"command"`
	Data    interface{} `json:"data"`
}

type ServerBrowserUI struct {
	WindowRect    rl.Rectangle
	ScrollOffset  int
	SelectedIndex int
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func drawScene() {
	// Draw map tiles
	for i := 0; i < len(tileMap); i++ {
		if tileMap[i] != 0 {
			tileDest.X = tileDest.Width * float32(i%mapW)
			tileDest.Y = tileDest.Height * float32(i/mapW)

			// Select texture based on source map
			switch srcMap[i] {
			case "g":
				tex = grassSprite
			case "f":
				tex = fenceSprite
			case "h":
				tex = hillSprite
			case "w":
				tex = waterSprite
			case "ww":
				tex = woodHouseWallsSprite
			case "wr":
				tex = woodHouseRoofSprite
			case "t":
				tex = tilledSprite
			case "d":
				tex = doorSprite
			}

			// Draw grass background for certain tiles
			if srcMap[i] == "ww" || srcMap[i] == "f" || srcMap[i] == "d" || srcMap[i] == "wr" {
				tileSrc.X = 0
				tileSrc.Y = tileSrc.Height * 5
				rl.DrawTexturePro(grassSprite, tileSrc, tileDest, rl.NewVector2(tileDest.Width, tileDest.Height), 0, rl.White)
			}

			tileSrc.X = tileSrc.Width * float32((tileMap[i]-1)%int(tex.Width/int32(tileSrc.Width)))
			tileSrc.Y = tileSrc.Height * float32((tileMap[i]-1)/int(tex.Width/int32(tileSrc.Width)))

			rl.DrawTexturePro(tex, tileSrc, tileDest, rl.NewVector2(tileDest.Width, tileDest.Height), 0, rl.White)
		}
	}

	// Draw other players
	playersMutex.RLock()
	for playerID, val := range joinedPlayers {
		if playerID != joinPlayerID {
			rl.DrawTexturePro(playerSprite, val["playerSrc"], val["playerDest"], rl.NewVector2(val["playerDest"].Width, val["playerDest"].Height), 0, rl.White)
		}
	}
	playersMutex.RUnlock()

	// Draw local player
	rl.DrawTexturePro(playerSprite, playerSrc, playerDest, rl.NewVector2(playerDest.Width, playerDest.Height), 0, rl.White)
}

func input() {
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		playerMoving = true
		playerDir = 1
		playerUp = true
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		playerMoving = true
		playerDir = 2
		playerLeft = true
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		playerMoving = true
		playerDir = 0
		playerDown = true
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		playerMoving = true
		playerDir = 3
		playerRight = true
	}
	if rl.IsKeyPressed(rl.KeyQ) {
		musicPaused = !musicPaused
	}
}

func update() {
	running = !rl.WindowShouldClose()

	if time.Since(lastMapUpdate) > time.Duration(mapUpdateCooldown)*time.Millisecond {
		loadMap()
		lastMapUpdate = time.Now()
	}

	// Update player animation
	if playerFrame > (maxFrames - 1) {
		playerFrame = 0
	}
	playerSrc.X = ((playerSrc.Width * float32(playerFrame)) + ((playerSrc.Width * float32(maxFrames)) * float32(playerDir)))

	if playerMoving {
		// Apply movement to local player
		if playerUp {
			playerDest.Y -= playerSpeed
		}
		if playerLeft {
			playerDest.X -= playerSpeed
		}
		if playerDown {
			playerDest.Y += playerSpeed
		}
		if playerRight {
			playerDest.X += playerSpeed
		}

		// Update local player in server's map if host
		if host_type == "host" {
			updateLocalPlayerOnServer()
		}

		// Send movement data via WebSocket
		if host_type == "join" || host_type == "host" {
			data := MovementData{
				PlayerID:    joinPlayerID,
				PlayerUp:    playerUp,
				PlayerLeft:  playerLeft,
				PlayerDown:  playerDown,
				PlayerRight: playerRight,
			}
			sendDataMovementWS(data)
		}

		if frameCount%8 == 1 {
			playerFrame++
		}
	} else if frameCount%45 == 1 {
		playerFrame++
	}

	// Request player positions periodically via WebSocket
	//if time.Since(lastPlayerUpdate) > 100*time.Millisecond {
	requestPlayerPositionsWS()
	//	lastPlayerUpdate = time.Now()
	//}
	if host_type == "join" {
		if time.Since(lastMapUpdate) > time.Duration(mapUpdateCooldown)*time.Millisecond {
			requestMapDataWS()
			time.Sleep(1 * time.Second)
			loadMap()
			lastMapUpdate = time.Now()
		}
	}
	frameCount++
	playerSrc.Y = playerSrc.Height
	if !playerMoving && playerFrame > 1 {
		playerFrame = 0
	}
	// Update music
	rl.UpdateMusicStream(music)
	if musicPaused {
		rl.PauseMusicStream(music)
	} else {
		rl.ResumeMusicStream(music)
	}

	// Update camera
	cam.Target = rl.NewVector2(float32(playerDest.X-(playerDest.Width/2)), float32(playerDest.Y-(playerDest.Height/2)))

	// Reset movement flags
	playerMoving = false
	playerUp, playerDown, playerRight, playerLeft = false, false, false, false
}

func updateLocalPlayerOnServer() {
	playersMutex.Lock()
	if _, exists := joinedPlayers[joinPlayerID]; exists {
		joinedPlayers[joinPlayerID]["playerDest"] = playerDest
		joinedPlayers[joinPlayerID]["playerSrc"] = playerSrc
	}
	playersMutex.Unlock()
}

func render() {
	rl.BeginDrawing()
	rl.ClearBackground(bkgColor)
	rl.BeginMode2D(cam)

	drawScene()

	rl.EndMode2D()
	rl.EndDrawing()
}

func loadMap() {
	tileMap = tileMap[:0]
	srcMap = srcMap[:0]

	if host_type == "host" {
		file, err := os.ReadFile(map_file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		remNewLines := strings.Replace(string(file), "\n", " ", -1)
		loadedMap = strings.Fields(remNewLines)
	} else if host_type == "join" {
		requestMapDataWS()
		//fmt.Println(loadedMap)
	}
	sliced := loadedMap

	mapW = -1
	mapH = -1
	for i := 0; i < len(sliced); i++ {
		if mapW == -1 {
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue
			}
			mapW = int(s)
		} else if mapH == -1 {
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue
			}
			mapH = int(s)
		} else if len(tileMap) < mapW*mapH {
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue
			}
			m := int(s)
			tileMap = append(tileMap, m)
		} else {
			srcMap = append(srcMap, sliced[i])
		}
	}
}
func startGatewayServer(port string) {
	gatewayPort = port

	// Start cleanup routine for inactive servers
	go cleanupInactiveServers()

	http.HandleFunc("/ws", handleGatewayWebSocket)
	http.HandleFunc("/api/servers", handleServerListHTTP)
	http.HandleFunc("/", serveGatewayHTML)

	fmt.Printf("Gateway server running on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleGatewayWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Gateway WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	log.Println("Gateway WebSocket connection established")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Gateway WebSocket error: %v", err)
			}
			break
		}

		var msg GatewayMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Gateway JSON unmarshal error: %v", err)
			continue
		}

		switch msg.Command {
		case "register_server":
			handleServerRegistration(msg.Data, conn)
		case "unregister_server":
			handleServerUnregistration(msg.Data, conn)
		case "server_ping":
			handleServerPing(msg.Data, conn)
		case "get_server_list":
			handleGetServerList(msg.Data, conn)
		case "join_server":
			handleJoinServer(msg.Data, conn)
		case "update_server_info":
			handleServerInfoUpdate(msg.Data, conn)
		default:
			log.Printf("Unknown gateway command: %s", msg.Command)
		}
	}
}

func handleServerRegistration(data interface{}, conn *websocket.Conn) {
	dataBytes, _ := json.Marshal(data)
	var reg ServerRegistration
	if err := json.Unmarshal(dataBytes, &reg); err != nil {
		log.Printf("Error parsing server registration: %v", err)
		return
	}

	gatewayMutex.Lock()
	server := &GameServer{
		ID:          reg.ServerID,
		Address:     reg.Address,
		Port:        reg.Port,
		PlayerCount: 0,
		MaxPlayers:  reg.MaxPlayers,
		GameMode:    reg.GameMode,
		LastPing:    time.Now(),
		Active:      true,
	}
	gameServers[reg.ServerID] = server
	gatewayMutex.Unlock()

	log.Printf("Server registered: %s (%s:%s)", reg.ServerID, reg.Address, reg.Port)

	response := map[string]interface{}{
		"type":    "registration_response",
		"success": true,
		"message": "Server registered successfully",
	}
	sendGatewayResponse(conn, response)
}

func handleServerUnregistration(data interface{}, conn *websocket.Conn) {
	dataBytes, _ := json.Marshal(data)
	var unreg struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal(dataBytes, &unreg); err != nil {
		log.Printf("Error parsing server unregistration: %v", err)
		return
	}

	gatewayMutex.Lock()
	delete(gameServers, unreg.ServerID)
	gatewayMutex.Unlock()

	log.Printf("Server unregistered: %s", unreg.ServerID)

	response := map[string]interface{}{
		"type":    "unregistration_response",
		"success": true,
		"message": "Server unregistered successfully",
	}
	sendGatewayResponse(conn, response)
}

func handleServerPing(data interface{}, conn *websocket.Conn) {
	dataBytes, _ := json.Marshal(data)
	var ping struct {
		ServerID    string `json:"server_id"`
		PlayerCount int    `json:"player_count"`
	}
	if err := json.Unmarshal(dataBytes, &ping); err != nil {
		log.Printf("Error parsing server ping: %v", err)
		return
	}

	gatewayMutex.Lock()
	if server, exists := gameServers[ping.ServerID]; exists {
		server.LastPing = time.Now()
		server.PlayerCount = ping.PlayerCount
		server.Active = true
	}
	gatewayMutex.Unlock()

	response := map[string]interface{}{
		"type": "ping_response",
		"time": time.Now().Unix(),
	}
	sendGatewayResponse(conn, response)
}

func handleGetServerList(data interface{}, conn *websocket.Conn) {
	gatewayMutex.RLock()
	var serverList []GameServer
	for _, server := range gameServers {
		if server.Active {
			serverList = append(serverList, *server)
		}
	}
	gatewayMutex.RUnlock()

	response := ServerListResponse{
		Type:    "server_list",
		Servers: serverList,
	}
	sendGatewayResponse(conn, response)
}

func handleJoinServer(data interface{}, conn *websocket.Conn) {
	dataBytes, _ := json.Marshal(data)
	var req JoinServerRequest
	if err := json.Unmarshal(dataBytes, &req); err != nil {
		log.Printf("Error parsing join server request: %v", err)
		return
	}

	gatewayMutex.RLock()
	server, exists := gameServers[req.ServerID]
	gatewayMutex.RUnlock()

	var response JoinServerResponse
	response.Type = "join_server_response"

	if !exists {
		response.Success = false
		response.Message = "Server not found"
	} else if !server.Active {
		response.Success = false
		response.Message = "Server is not active"
	} else if server.PlayerCount >= server.MaxPlayers {
		response.Success = false
		response.Message = "Server is full"
	} else {
		response.Success = true
		response.Address = server.Address
		response.Port = server.Port
		response.Message = "Server connection info provided"
	}

	sendGatewayResponse(conn, response)
}

func handleServerInfoUpdate(data interface{}, conn *websocket.Conn) {
	dataBytes, _ := json.Marshal(data)
	var update struct {
		ServerID    string `json:"server_id"`
		PlayerCount int    `json:"player_count"`
		GameMode    string `json:"game_mode,omitempty"`
	}
	if err := json.Unmarshal(dataBytes, &update); err != nil {
		log.Printf("Error parsing server info update: %v", err)
		return
	}

	gatewayMutex.Lock()
	if server, exists := gameServers[update.ServerID]; exists {
		server.PlayerCount = update.PlayerCount
		server.LastPing = time.Now()
		if update.GameMode != "" {
			server.GameMode = update.GameMode
		}
	}
	gatewayMutex.Unlock()
}

func sendGatewayResponse(conn *websocket.Conn, response interface{}) {
	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling gateway response: %v", err)
		return
	}
	conn.WriteMessage(websocket.TextMessage, jsonData)
}

func cleanupInactiveServers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gatewayMutex.Lock()
		for id, server := range gameServers {
			if time.Since(server.LastPing) > 60*time.Second {
				log.Printf("Removing inactive server: %s", id)
				delete(gameServers, id)
			}
		}
		gatewayMutex.Unlock()
	}
}

func handleServerListHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	gatewayMutex.RLock()
	var serverList []GameServer
	for _, server := range gameServers {
		if server.Active {
			serverList = append(serverList, *server)
		}
	}
	gatewayMutex.RUnlock()

	response := ServerListResponse{
		Type:    "server_list",
		Servers: serverList,
	}

	json.NewEncoder(w).Encode(response)
}

func serveGatewayHTML(w http.ResponseWriter, r *http.Request) {
	file := "gateway.html"
	if _, err := os.Stat(file); err == nil {
		// Datei existiert – einlesen
		inhalt, err := os.ReadFile(file)
		if err != nil {
			fmt.Println("Fehler beim Lesen:", err)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(inhalt))
	} else if os.IsNotExist(err) {
		fmt.Println("Datei existiert nicht.")
	} else {
		fmt.Println("Fehler beim Prüfen der Datei:", err)
	}
}

// Client-side gateway integration functions
func connectToGateway(gatewayURL string) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: gatewayURL, Path: "/ws"}
	log.Printf("Connecting to gateway %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Gateway dial error:", err)
	}

	return c
}

func registerServerWithGateway(gatewayConn *websocket.Conn, serverID, address, port string) {
	registration := ServerRegistration{
		ServerID:   serverID,
		Address:    address,
		Port:       port,
		MaxPlayers: 10, // You can make this configurable
		GameMode:   "default",
	}

	msg := GatewayMessage{
		Type:    "gateway_message",
		Command: "register_server",
		Data:    registration,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling server registration: %v", err)
		return
	}

	gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
}

func sendServerPing(gatewayConn *websocket.Conn, serverID string, playerCount int) {
	ping := struct {
		ServerID    string `json:"server_id"`
		PlayerCount int    `json:"player_count"`
	}{
		ServerID:    serverID,
		PlayerCount: playerCount,
	}

	msg := GatewayMessage{
		Type:    "gateway_message",
		Command: "server_ping",
		Data:    ping,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling server ping: %v", err)
		return
	}

	gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
}

func requestServerList(gatewayConn *websocket.Conn) {
	msg := GatewayMessage{
		Type:    "gateway_message",
		Command: "get_server_list",
		Data:    struct{}{},
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling server list request: %v", err)
		return
	}

	gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
}

// Integration example for the main init() function:
/*
if host_type == "gateway" {
	if len(start_args) < 3 {
		fmt.Println("Please provide port for gateway mode")
		os.Exit(1)
	}
	gateway_port := start_args[2]
	startGatewayServer(gateway_port)
	return // Gateway doesn't run the game loop
}

if host_type == "host" {
	// ... existing host code ...

	// If you want this server to register with a gateway:
	if len(start_args) >= 4 {
		gateway_url := start_args[3] // e.g., "localhost:8080"
		go func() {
			gatewayConn := connectToGateway(gateway_url)
			defer gatewayConn.Close()

			serverID := fmt.Sprintf("server_%d", time.Now().Unix())
			registerServerWithGateway(gatewayConn, serverID, "localhost", server_port)

			// Send periodic pings
			ticker := time.NewTicker(30 * time.Second)
			for range ticker.C {
				playersMutex.RLock()
				playerCount := len(joinedPlayers)
				playersMutex.RUnlock()

				sendServerPing(gatewayConn, serverID, playerCount)
			}
		}()
	}
}
*/
func clientWebsocketConnect(websocket_url string) {
	u := url.URL{Scheme: "ws", Host: websocket_url, Path: "/ws"}
	log.Printf("Connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	websocket_client = c

	// Start goroutine to handle incoming messages
	go handleWebSocketMessages()
}

func handleWebSocketMessages() {
	defer websocket_client.Close()
	for {
		_, message, err := websocket_client.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		// Try to parse as JSON first
		var response map[string]interface{}
		if err := json.Unmarshal(message, &response); err == nil {
			// Check message type to handle different JSON responses
			if msgType, exists := response["type"]; exists {
				switch msgType {
				case "player_positions":
					handlePlayerPositionsResponse(message)
				case "map_data":
					handleMapDataResponse(message)
				default:
					log.Printf("Unknown JSON message type: %v", msgType)
				}
			} else {
				// Fallback to original handling if no type field
				handlePlayerPositionsResponse(message)
			}
		} else {
			// Handle simple string responses
			messageStr := string(message)
			if messageStr == "true" {
				log.Println("Respawn confirmed")
			} else if id, err := strconv.Atoi(messageStr); err == nil {
				joinPlayerID = id
				log.Printf("Assigned player ID: %d", joinPlayerID)
			}
		}
	}
}

func handlePlayerPositionsResponse(message []byte) {
	var response struct {
		Type    string                          `json:"type"`
		Players map[int]map[string]rl.Rectangle `json:"players"`
	}

	if err := json.Unmarshal(message, &response); err != nil {
		log.Println("Error parsing player positions:", err)
		return
	}

	playersMutex.Lock()
	if host_type == "host" {
		// Keep own player data, update others
		ownPlayerData := joinedPlayers[joinPlayerID]
		joinedPlayers = make(map[int]map[string]rl.Rectangle)
		if ownPlayerData != nil {
			joinedPlayers[joinPlayerID] = ownPlayerData
		}
		for id, player := range response.Players {
			if id != joinPlayerID {
				joinedPlayers[id] = make(map[string]rl.Rectangle)
				for key, rect := range player {
					joinedPlayers[id][key] = rect
				}
			}
		}
	} else {
		// Client: replace all with server data
		joinedPlayers = make(map[int]map[string]rl.Rectangle)
		for id, player := range response.Players {
			joinedPlayers[id] = make(map[string]rl.Rectangle)
			for key, rect := range player {
				joinedPlayers[id][key] = rect
			}
		}
	}
	playersMutex.Unlock()
}
func handleMapDataResponse(message []byte) {
	var response struct {
		Type    string        `json:"type"`
		Command string        `json:"command"`
		Map     []interface{} `json:"map"` // or []string, []map[string]interface{}, etc.
	}
	err := json.Unmarshal(message, &response)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	var stringMap []string
	for _, item := range response.Map {
		if str, ok := item.(string); ok {
			stringMap = append(stringMap, str)
		} else {
			fmt.Printf("Warning: non-string item found: %v\n", item)
		}
	}

	loadedMap = stringMap
}

func requestPlayerPositionsWS() {
	if websocket_client == nil {
		return
	}

	playerData := make(map[string]string)
	playerData["command"] = "get_players"
	playerData["exclude_id"] = strconv.Itoa(joinPlayerID)

	jsonData, err := json.Marshal(playerData)
	if err != nil {
		log.Println("Error marshaling get_players request:", err)
		return
	}

	err = websocket_client.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		log.Println("Error sending get_players request:", err)
	}
}

func sendDataRespawnWS(data RespawnData) {
	playerData := make(map[string]string)
	playerData["command"] = "respawn"
	playerData["respawn"] = strconv.FormatBool(data.Respawn)

	jsonData, err := json.Marshal(playerData)
	if err != nil {
		panic(err)
	}
	websocket_client.WriteMessage(websocket.TextMessage, jsonData)
}

func sendDataMovementWS(data MovementData) {
	playerData := make(map[string]string)
	playerData["command"] = "player_data"
	playerData["playerUp"] = strconv.FormatBool(data.PlayerUp)
	playerData["playerLeft"] = strconv.FormatBool(data.PlayerLeft)
	playerData["playerDown"] = strconv.FormatBool(data.PlayerDown)
	playerData["playerRight"] = strconv.FormatBool(data.PlayerRight)

	jsonData, err := json.Marshal(playerData)
	if err != nil {
		panic(err)
	}
	websocket_client.WriteMessage(websocket.TextMessage, jsonData)
}
func requestMapDataWS() {
	mapRequest := make(map[string]string)
	mapRequest["command"] = "get_map"

	jsonData, err := json.Marshal(mapRequest)
	if err != nil {
		panic(err)
	}
	websocket_client.WriteMessage(websocket.TextMessage, jsonData)
}

func startServer(port string) {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade error:", err)
			return
		}
		defer conn.Close()

		log.Println("WebSocket connection established")
		var playerID int

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				} else {
					log.Println("WebSocket connection closed")
				}
				break
			}

			var data map[string]string
			err = json.Unmarshal(message, &data)
			if err != nil {
				log.Printf("JSON unmarshal error: %v", err)
				continue
			}

			switch data["command"] {
			case "player_data":
				handlePlayerMovement(data, playerID, conn)
			case "respawn":
				playerID = handlePlayerRespawn(data, conn)
			case "get_players":
				handleGetPlayersWS(data, conn)
			case "get_map":
				handleGetMapWS(data, conn)
			default:
				log.Printf("Unknown command: %s", data["command"])
			}
		}

		// Clean up player when disconnected
		if playerID != 0 {
			playersMutex.Lock()
			delete(joinedPlayers, playerID)
			playersMutex.Unlock()
			log.Printf("Player %d disconnected and removed", playerID)
		}
	})
	file := "index.html"

	if _, err := os.Stat(file); err == nil {
		// Datei existiert – einlesen
		inhalt, err := os.ReadFile(file)
		if err != nil {
			fmt.Println("Fehler beim Lesen:", err)
			return
		}
		//fmt.Println("Dateiinhalt:", string(inhalt))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			//fmt.Fprint(w, []byte(inhalt))
			w.Write([]byte(inhalt))
		})
	} else if os.IsNotExist(err) {
		fmt.Println("Datei existiert nicht.")
	} else {
		fmt.Println("Fehler beim Prüfen der Datei:", err)
	}

	fmt.Println("Server running on http://localhost:" + port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func handlePlayerMovement(data map[string]string, playerID int, conn *websocket.Conn) {
	playersMutex.Lock()
	defer playersMutex.Unlock()

	if _, exists := joinedPlayers[playerID]; !exists {
		log.Printf("Player %d not found for movement", playerID)
		return
	}

	currentRect := joinedPlayers[playerID]["playerDest"]
	currentRectSrc := joinedPlayers[playerID]["playerSrc"]
	currentPlayerDir := 0

	if up, _ := strconv.ParseBool(data["playerUp"]); up {
		currentRect.Y -= playerSpeed
		currentPlayerDir = 1
	}
	if left, _ := strconv.ParseBool(data["playerLeft"]); left {
		currentRect.X -= playerSpeed
		currentPlayerDir = 2
	}
	if down, _ := strconv.ParseBool(data["playerDown"]); down {
		currentRect.Y += playerSpeed
		currentPlayerDir = 0
	}
	if right, _ := strconv.ParseBool(data["playerRight"]); right {
		currentRect.X += playerSpeed
		currentPlayerDir = 3
	}

	currentRectSrc.Y = currentRectSrc.Height
	currentRectSrc.X = ((currentRectSrc.Width * float32(playerFrame)) + ((currentRectSrc.Width * float32(maxFrames)) * float32(currentPlayerDir)))

	joinedPlayers[playerID]["playerDest"] = currentRect
	joinedPlayers[playerID]["playerSrc"] = currentRectSrc

	response := fmt.Sprintf("%.0f,%.0f", currentRect.X, currentRect.Y)
	conn.WriteMessage(websocket.TextMessage, []byte(response))
}

func handlePlayerRespawn(data map[string]string, conn *websocket.Conn) int {
	if respawn, _ := strconv.ParseBool(data["respawn"]); respawn {
		playerID := rand.IntN(1000000)

		playersMutex.Lock()
		joinedPlayers[playerID] = make(map[string]rl.Rectangle)
		joinedPlayers[playerID]["playerDest"] = rl.NewRectangle(200, 200, 60, 60)
		joinedPlayers[playerID]["playerSrc"] = rl.NewRectangle(0, 0, 48, 48)
		playersMutex.Unlock()

		fmt.Printf("Player %d spawned. Total players: %d\n", playerID, len(joinedPlayers))

		// Send the player ID back to the client
		conn.WriteMessage(websocket.TextMessage, []byte(strconv.Itoa(playerID)))
		return playerID
	}
	return -1
}

func handleGetPlayersWS(data map[string]string, conn *websocket.Conn) {
	excludeID := 0
	if excludeStr, exists := data["exclude_id"]; exists {
		if id, err := strconv.Atoi(excludeStr); err == nil {
			excludeID = id
		}
	}

	playersMutex.RLock()
	playerList := make(map[int]map[string]rl.Rectangle)
	for id, player := range joinedPlayers {
		if id != excludeID {
			playerList[id] = make(map[string]rl.Rectangle)
			for key, rect := range player {
				playerList[id][key] = rect
			}
		}
	}
	playersMutex.RUnlock()

	// Wrap the player data in a response structure with type
	response := map[string]interface{}{
		"type":    "player_positions",
		"players": playerList,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Println("Error marshaling player positions:", err)
		return
	}

	conn.WriteMessage(websocket.TextMessage, jsonData)
}
func handleGetMapWS(data map[string]string, conn *websocket.Conn) {
	data["type"] = "map_data"
	response := map[string]interface{}{
		"command": data["command"],
		"type":    data["type"],
		"map":     loadedMap,
	}
	jsonData, err := json.Marshal(response)
	//fmt.Println(data)
	if err != nil {
		panic(err)
	}
	conn.WriteMessage(websocket.TextMessage, jsonData)
}
func registerWithGateway(gatewayURL, serverPort string) {
	gatewayConn = connectToGateway(gatewayURL)
	if gatewayConn == nil {
		log.Println("Failed to connect to gateway")
		return
	}

	serverID = fmt.Sprintf("server_%d", time.Now().Unix())
	registerServerWithGateway(gatewayConn, serverID, "localhost", serverPort)
	registeredWithGateway = true

	// Start periodic ping routine
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			if registeredWithGateway && gatewayConn != nil {
				playersMutex.RLock()
				playerCount := len(joinedPlayers)
				playersMutex.RUnlock()

				sendServerPing(gatewayConn, serverID, playerCount)
			}
		}
	}()

	// Handle gateway messages
	go func() {
		for {
			if gatewayConn == nil {
				break
			}
			_, message, err := gatewayConn.ReadMessage()
			if err != nil {
				log.Println("Gateway connection error:", err)
				registeredWithGateway = false
				break
			}

			var response map[string]interface{}
			if err := json.Unmarshal(message, &response); err == nil {
				log.Printf("Gateway response: %v", response)
			}
		}
	}()
}

func initGatewayClient(gateway_url string) {
	gatewayMode = true
	gatewayURL = gateway_url
	showServerBrowser = true
	selectedServerIndex = -1

	// Load default font
	uiFont = rl.GetFontDefault()

	connectToGatewayClient()
}

func connectToGatewayClient() {
	u := url.URL{Scheme: "ws", Host: gatewayURL, Path: "/ws"}
	log.Printf("Connecting to gateway %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("Gateway connection failed: %v", err)
		showMessage("Failed to connect to gateway server")
		return
	}

	gatewayConn = c
	gatewayConnected = true

	// Start message handler
	go handleGatewayClientMessages()

	// Request initial server list
	requestServerListFromGateway()

	log.Println("Connected to gateway successfully")
	showMessage("Connected to gateway - Loading servers...")
}

func handleGatewayClientMessages() {
	if gatewayConn == nil {
		return
	}

	defer func() {
		gatewayConn.Close()
		gatewayConnected = false
	}()

	for {
		_, message, err := gatewayConn.ReadMessage()
		if err != nil {
			log.Printf("Gateway read error: %v", err)
			showMessage("Lost connection to gateway")
			break
		}

		var response map[string]interface{}
		if err := json.Unmarshal(message, &response); err != nil {
			log.Printf("Gateway message parse error: %v", err)
			continue
		}

		switch response["type"] {
		case "server_list":
			handleServerListResponse(message)
		case "join_server_response":
			handleJoinServerResponse(message)
		}
	}
}

func handleServerListResponse(message []byte) {
	var response ServerListResponse
	if err := json.Unmarshal(message, &response); err != nil {
		log.Printf("Error parsing server list: %v", err)
		return
	}

	serversMutex.Lock()
	availableServers = response.Servers
	serversMutex.Unlock()

	log.Printf("Received %d servers from gateway", len(response.Servers))
	if len(response.Servers) == 0 {
		showMessage("No servers available")
	} else {
		showMessage(fmt.Sprintf("Found %d available servers", len(response.Servers)))
	}
}

func handleJoinServerResponse(message []byte) {
	var response JoinServerResponse
	if err := json.Unmarshal(message, &response); err != nil {
		log.Printf("Error parsing join server response: %v", err)
		return
	}

	if response.Success {
		// Connect to the server
		server_address := response.Address + ":" + response.Port
		showMessage("Connecting to server: " + server_address)

		// Switch from gateway mode to join mode
		gatewayMode = false
		showServerBrowser = false
		host_type = "join"
		server_url_ws = server_address

		// Close gateway connection
		if gatewayConn != nil {
			gatewayConn.Close()
			gatewayConn = nil
		}

		// Connect to the game server
		go func() {
			time.Sleep(500 * time.Millisecond) // Brief delay
			clientWebsocketConnect(server_url_ws)
			time.Sleep(100 * time.Millisecond)

			data := RespawnData{Respawn: true}
			sendDataRespawnWS(data)
		}()

		log.Printf("Joining server: %s", server_address)
	} else {
		showMessage("Failed to join server: " + response.Message)
		log.Printf("Join failed: %s", response.Message)
	}
}

func requestServerListFromGateway() {
	if !gatewayConnected || gatewayConn == nil {
		return
	}

	msg := GatewayClientMessage{
		Type:    "gateway_message",
		Command: "get_server_list",
		Data:    struct{}{},
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling server list request: %v", err)
		return
	}

	gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
}

func joinServerThroughGateway(serverID string) {
	if !gatewayConnected || gatewayConn == nil {
		showMessage("Not connected to gateway")
		return
	}

	request := JoinServerRequest{
		ServerID: serverID,
	}

	msg := GatewayClientMessage{
		Type:    "gateway_message",
		Command: "join_server",
		Data:    request,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling join server request: %v", err)
		return
	}

	gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
	showMessage("Requesting to join server...")
}

func showMessage(text string) {
	messageText = text
	showingMessage = true
	messageTimer = 3.0 // Show for 3 seconds
}

// Enhanced input handling for gateway mode
func inputGateway() {
	if !showServerBrowser {
		return
	}

	// Navigation
	if rl.IsKeyPressed(rl.KeyUp) && selectedServerIndex > 0 {
		selectedServerIndex--
	}

	serversMutex.RLock()
	serverCount := len(availableServers)
	serversMutex.RUnlock()

	if rl.IsKeyPressed(rl.KeyDown) && selectedServerIndex < serverCount-1 {
		selectedServerIndex++
	}

	// Join server
	if rl.IsKeyPressed(rl.KeyEnter) && selectedServerIndex >= 0 && selectedServerIndex < serverCount {
		serversMutex.RLock()
		if selectedServerIndex < len(availableServers) {
			serverID := availableServers[selectedServerIndex].ID
			serversMutex.RUnlock()
			joinServerThroughGateway(serverID)
		} else {
			serversMutex.RUnlock()
		}
	}

	// Refresh server list
	if rl.IsKeyPressed(rl.KeyR) {
		requestServerListFromGateway()
		showMessage("Refreshing server list...")
	}

	// Exit to main menu (you can implement this)
	if rl.IsKeyPressed(rl.KeyEscape) {
		running = false
	}
}

// Enhanced update function for gateway mode
func updateGateway() {
	running = !rl.WindowShouldClose()

	// Update message timer
	if showingMessage && messageTimer > 0 {
		messageTimer -= rl.GetFrameTime()
		if messageTimer <= 0 {
			showingMessage = false
		}
	}

	// Auto-refresh server list every 10 seconds
	static_last_refresh := time.Now()
	if time.Since(static_last_refresh) > 10*time.Second {
		if gatewayConnected {
			requestServerListFromGateway()
		}
		static_last_refresh = time.Now()
	}
}

// Enhanced render function for server browser
func renderServerBrowser() {
	rl.BeginDrawing()
	rl.ClearBackground(rl.NewColor(30, 30, 40, 255))

	// Title
	titleText := "Game Server Browser"
	titleSize := 30
	titleWidth := rl.MeasureText(titleText, int32(titleSize))
	rl.DrawText(titleText, (screenWidth-titleWidth)/2, 50, int32(titleSize), rl.White)

	// Instructions
	instructText := "Use UP/DOWN to select, ENTER to join, R to refresh, ESC to exit"
	instrSize := 16
	instrWidth := rl.MeasureText(instructText, int32(instrSize))
	rl.DrawText(instructText, (screenWidth-instrWidth)/2, 100, int32(instrSize), rl.Gray)

	// Connection status
	statusText := "Disconnected"
	statusColor := rl.Red
	if gatewayConnected {
		statusText = "Connected to Gateway"
		statusColor = rl.Green
	}
	rl.DrawText(statusText, 20, 20, 16, statusColor)

	// Server list
	serversMutex.RLock()
	servers := make([]GameServer, len(availableServers))
	copy(servers, availableServers)
	serversMutex.RUnlock()

	startY := 150
	rowHeight := 80
	newStartY := startY + 100

	if len(servers) == 0 {
		noServersText := "No servers available - Press R to refresh"
		noServersWidth := rl.MeasureText(noServersText, 20)
		rl.DrawText(noServersText, (screenWidth-noServersWidth)/2, int32(newStartY), 20, rl.Gray)
	} else {
		for i, server := range servers {
			y := startY + i*rowHeight

			// Skip if off screen
			if y > screenHeight {
				break
			}

			// Server box background
			boxColor := rl.NewColor(50, 50, 60, 255)
			if i == selectedServerIndex {
				boxColor = rl.NewColor(70, 70, 100, 255)
			}

			// Status-based coloring
			if server.PlayerCount >= server.MaxPlayers {
				boxColor = rl.NewColor(80, 40, 40, 255) // Full - reddish
			} else if server.PlayerCount == 0 {
				boxColor = rl.NewColor(40, 80, 40, 255) // Empty - greenish
			}

			rl.DrawRectangle(50, int32(y), screenWidth-100, int32(rowHeight-10), boxColor)

			// Server name
			serverNameText := server.ID
			rl.DrawText(serverNameText, 70, int32(y+10), 18, rl.White)

			// Server details
			addressText := fmt.Sprintf("Address: %s:%s", server.Address, server.Port)
			rl.DrawText(addressText, 70, int32(y+35), 14, rl.LightGray)

			playersText := fmt.Sprintf("Players: %d/%d", server.PlayerCount, server.MaxPlayers)
			rl.DrawText(playersText, 70, int32(y+55), 14, rl.LightGray)

			// Game mode
			gameModeText := fmt.Sprintf("Mode: %s", server.GameMode)
			gameModeWidth := rl.MeasureText(gameModeText, 14)
			rl.DrawText(gameModeText, screenWidth-100-gameModeWidth, int32(y+35), 14, rl.LightGray)

			// Status indicator
			statusIndicatorText := "●"
			statusIndicatorColor := rl.Green
			if !server.Active {
				statusIndicatorColor = rl.Red
			} else if server.PlayerCount >= server.MaxPlayers {
				statusIndicatorColor = rl.Orange
			}
			rl.DrawText(statusIndicatorText, screenWidth-80, int32(y+15), 20, statusIndicatorColor)
		}
	}

	// Show message if any
	if showingMessage {
		messageWidth := rl.MeasureText(messageText, 16)
		messageX := (screenWidth - messageWidth) / 2
		messageY := screenHeight - 100

		// Message background
		rl.DrawRectangle(messageX-10, int32(messageY-5), messageWidth+20, 30, rl.NewColor(0, 0, 0, 180))
		rl.DrawText(messageText, messageX, int32(messageY), 16, rl.White)
	}

	rl.EndDrawing()
}

// Modified main loop functions
func inputMain() {
	if gatewayMode && showServerBrowser {
		inputGateway()
	} else {
		input() // Original input function
	}
}

func updateMain() {
	if gatewayMode && showServerBrowser {
		updateGateway()
	} else {
		update() // Original update function
	}
}

func renderMain() {
	if gatewayMode && showServerBrowser {
		renderServerBrowser()
	} else {
		render() // Original render function
	}
}

func init() {
	rl.InitWindow(screenWidth, screenHeight, "Simple Game")
	rl.SetExitKey(0)
	rl.SetTargetFPS(60)

	// Load textures
	grassSprite = rl.LoadTexture("resource/tilesets/grass.png")
	fenceSprite = rl.LoadTexture("resource/tilesets/fences.png")
	hillSprite = rl.LoadTexture("resource/tilesets/hills.png")
	waterSprite = rl.LoadTexture("resource/tilesets/water.png")
	woodHouseWallsSprite = rl.LoadTexture("resource/tilesets/wood_walls.png")
	woodHouseRoofSprite = rl.LoadTexture("resource/tilesets/wood_roof.png")
	tilledSprite = rl.LoadTexture("resource/tilesets/tilled.png")
	doorSprite = rl.LoadTexture("resource/tilesets/doors.png")
	playerSprite = rl.LoadTexture("resource/tilesets/player.png")

	// Initialize rectangles
	tileDest = rl.NewRectangle(0, 0, 16, 16)
	tileSrc = rl.NewRectangle(0, 0, 16, 16)
	playerSrc = rl.NewRectangle(0, 0, 48, 48)
	playerDest = rl.NewRectangle(200, 200, 60, 60)

	// Initialize audio
	rl.InitAudioDevice()
	music = rl.LoadMusicStream("resource/music/music.mp3")
	musicPaused = true
	rl.PlayMusicStream(music)

	// Initialize camera
	cam = rl.NewCamera2D(
		rl.NewVector2(float32(screenWidth/2), float32(screenHeight/2)),
		rl.NewVector2(float32(playerDest.X-(playerDest.Width/2)), float32(playerDest.Y-(playerDest.Height/2))),
		0.0, 1.5)
	//start_args := os.Args
	//if len(start_args) < 2 {
	//	fmt.Println("Usage: program <host|join|gateway> [port|server_url]")
	//	os.Exit(1)
	//}

	start_args := os.Args
	if len(start_args) < 2 {
		fmt.Println("Usage: program <host|join|gateway> [port|server_url] [gateway_url]")
		os.Exit(1)
	}

	host_type = start_args[1]
	if host_type != "host" && host_type != "join" && host_type != "gateway" {
		fmt.Println("Mode must be 'host', 'join', or 'gateway'")
		os.Exit(1)
	}

	if host_type == "join" {
		if len(start_args) < 3 {
			fmt.Println("Please provide server URL or 'gateway:URL' for join mode")
			os.Exit(1)
		}

		server_arg := start_args[2]

		// Check if joining through gateway
		if strings.HasPrefix(server_arg, "gateway:") {
			gateway_url := strings.TrimPrefix(server_arg, "gateway:")
			initGatewayClient(gateway_url)
		} else {
			// Direct server connection (existing code)
			server_url_ws = server_arg
			clientWebsocketConnect(server_url_ws)
			time.Sleep(100 * time.Millisecond)
			data := RespawnData{Respawn: true}
			sendDataRespawnWS(data)
		}
	}
	if host_type == "gateway" {
		if len(start_args) < 3 {
			fmt.Println("Please provide port for gateway mode")
			os.Exit(1)
		}
		gateway_port := start_args[2]
		startGatewayServer(gateway_port)
		return // Gateway doesn't run the game loop
	}

	if host_type == "host" {
		// ... existing host code ...

		// If you want this server to register with a gateway:
		if len(start_args) >= 4 {
			gateway_url := start_args[3] // e.g., "localhost:8080"
			go func() {
				gatewayConn := connectToGateway(gateway_url)
				defer gatewayConn.Close()

				server_port := start_args[3]

				serverID := fmt.Sprintf("server_%d", time.Now().Unix())
				registerServerWithGateway(gatewayConn, serverID, "localhost", server_port)

				// Send periodic pings
				ticker := time.NewTicker(30 * time.Second)
				for range ticker.C {
					playersMutex.RLock()
					playerCount := len(joinedPlayers)
					playersMutex.RUnlock()

					sendServerPing(gatewayConn, serverID, playerCount)
				}
			}()
		}
	}

	loadMap()
}

func quit() {
	if registeredWithGateway && gatewayConn != nil {
		// Unregister from gateway
		unreg := struct {
			ServerID string `json:"server_id"`
		}{
			ServerID: serverID,
		}

		msg := GatewayMessage{
			Type:    "gateway_message",
			Command: "unregister_server",
			Data:    unreg,
		}

		jsonData, _ := json.Marshal(msg)
		gatewayConn.WriteMessage(websocket.TextMessage, jsonData)
		gatewayConn.Close()
	}

	if websocket_client != nil {
		websocket_client.Close()
	}
	if websocket_client != nil {
		websocket_client.Close()
	}
	rl.UnloadTexture(grassSprite)
	rl.UnloadTexture(fenceSprite)
	rl.UnloadTexture(hillSprite)
	rl.UnloadTexture(waterSprite)
	rl.UnloadTexture(woodHouseWallsSprite)
	rl.UnloadTexture(woodHouseRoofSprite)
	rl.UnloadTexture(tilledSprite)
	rl.UnloadTexture(doorSprite)
	rl.UnloadTexture(playerSprite)
	rl.UnloadMusicStream(music)
	rl.CloseAudioDevice()
	rl.CloseWindow()
}

func main() {

	rl.SetWindowTitle("Simple Game: " + host_type)
	lastPlayerUpdate = time.Now()

	for running {
		inputMain()
		updateMain()
		renderMain()
	}
	quit()
}
