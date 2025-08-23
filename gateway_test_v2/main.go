package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/gorilla/websocket"
	"github.com/tawesoft/golib/v2/dialog"
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
	host_type           string
	server_url_ws       string
	gateway_server      string
	gateway_invite_code string
	websocket_client    *websocket.Conn
	websocket_gateway   *websocket.Conn

	// Multiplayer
	playersMutex      sync.RWMutex
	joinedPlayers     = make(map[string]map[string]rl.Rectangle)
	joinPlayerID_old  int
	joinPlayerID      string
	lastPlayerUpdate  time.Time
	lastMapUpdate     time.Time
	mapUpdateCooldown = 1000
)

type MovementData struct {
	PlayerID    string `json:"playerid"`
	PlayerUp    bool   `json:"playerUp"`
	PlayerLeft  bool   `json:"playerLeft"`
	PlayerDown  bool   `json:"playerDown"`
	PlayerRight bool   `json:"playerRight"`
}

type RespawnData struct {
	Respawn bool `json:"respawn"`
}

type GetPlayersData struct {
	ExcludeID int `json:"exclude_id"`
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
		if host_type == "host" || host_type == "gateway" {
			updateLocalPlayerOnServer()
		}

		// Send movement data via WebSocket
		if host_type == "join" || host_type == "host" || host_type == "gateway" || host_type == "gatewayjoin" {
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
	if host_type == "join" || host_type == "gatewayjoin" {
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

	if host_type == "host" || host_type == "gateway" {
		file, err := os.ReadFile(map_file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		remNewLines := strings.Replace(string(file), "\n", " ", -1)
		loadedMap = strings.Fields(remNewLines)
	} else if host_type == "join" || host_type == "gatewayjoin" {
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

func clientWebsocketConnect(websocket_url string, path string, invite_code string) {
	u := url.URL{Scheme: "ws", Host: websocket_url, Path: path}
	log.Printf("Connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	websocket_client = c
	if path == "/join" {
		dialog.Alert("test")
		response_data := make(map[string]string)
		response_data["command"] = "registerPlayer"
		response_data["invite_code"] = invite_code
		msg, err := json.Marshal(response_data)
		if err != nil {
			log.Printf("Error marshalling response: %v", err)
		}
		websocket_client.WriteMessage(websocket.TextMessage, msg)
	}

	// Start goroutine to handle incoming messages
	go handleWebSocketMessages()
}
func text(msg ...interface{}) string {
	return fmt.Sprintf("%+v", msg...)
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
				case "player_id":
					joinPlayerID = text(response["player_id"])
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
				joinPlayerID_old = id
				log.Printf("Assigned player ID: %d", joinPlayerID_old)
			}
		}
	}
}

func handlePlayerPositionsResponse(message []byte) {
	var response struct {
		Type    string                             `json:"type"`
		Players map[string]map[string]rl.Rectangle `json:"players"`
	}
	if err := json.Unmarshal(message, &response); err != nil {
		log.Println("Error parsing player positions:", err)
		return
	}

	playersMutex.Lock()
	if host_type == "host" || host_type == "gateway" {
		// Keep own player data, update others
		ownPlayerData := joinedPlayers[joinPlayerID]

		// Only update other players, keep own data
		for id, player := range response.Players {
			if id != joinPlayerID {
				if joinedPlayers[id] == nil {
					joinedPlayers[id] = make(map[string]rl.Rectangle)
				}
				for key, rect := range player {
					joinedPlayers[id][key] = rect
				}
			}
		}

		// Restore own player data if it was lost
		if ownPlayerData != nil && joinedPlayers[joinPlayerID] == nil {
			joinedPlayers[joinPlayerID] = ownPlayerData
		}
	} else {
		// Client: replace all with server data (join and gatewayjoin modes)
		joinedPlayers = make(map[string]map[string]rl.Rectangle)
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
	playerData["exclude_id"] = joinPlayerID
	playerData["player_id"] = joinPlayerID

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

func startGatewayConnection(gateway_url string) {
	u := url.URL{Scheme: "ws", Host: gateway_url, Path: "/host"}
	log.Printf("Connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	websocket_gateway = c
	data := map[string]string{
		"command": "registerHost",
	}

	// Convert map to JSON
	message, err := json.Marshal(data)
	if err != nil {
		log.Fatal("Error marshaling JSON:", err)
	}

	// Send JSON message
	err = websocket_gateway.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		log.Fatal("Error writing message:", err)
	}

	go gatewayConnectionHandler()

}
func gatewayConnectionHandler() {
	for {
		_, message, err := websocket_gateway.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			} else {
				log.Println("WebSocket connection closed")
				quit()
			}
			break
		}
		defer websocket_gateway.Close()
		var data map[string]string
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			continue
		}

		switch data["command"] {
		case "player_data":
			handlePlayerMovement(data, websocket_gateway)
		case "respawn":
			handlePlayerRespawn(data, websocket_gateway)
		case "get_players":
			handleGetPlayersWS(data, websocket_gateway)
		case "get_map":
			handleGetMapWS(data, websocket_gateway)
		case "registerHostResponse":
			fmt.Println("Lobby ID:", data["lobby_id"])
			gateway_invite_code = data["lobby_id"]

			// Now register the host as a player in the lobby
			registerData := map[string]string{
				"command":     "registerPlayer",
				"invite_code": gateway_invite_code,
			}
			msg, _ := json.Marshal(registerData)
			websocket_gateway.WriteMessage(websocket.TextMessage, msg)
		default:
			log.Printf("Unknown command: %s", data["command"])
		}
	}
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
		var playerID string

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
				data["player_id"] = playerID
				handlePlayerMovement(data, conn)
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
		if playerID != "" {
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

func handlePlayerMovement(data map[string]string, conn *websocket.Conn) {
	playersMutex.Lock()
	defer playersMutex.Unlock()
	var playerID string
	playerID = data["player_id"]
	//if err != nil {
	//	println("Error in handle player movement: %v", err)
	//}

	if _, exists := joinedPlayers[playerID]; !exists {
		log.Printf("Player %d not found for movement", playerID)
		fmt.Println(joinedPlayers)
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

	response := make(map[string]string) //fmt.Sprintf("%.0f,%.0f", currentRect.X, currentRect.Y)
	currentXstr := fmt.Sprintf("%f", currentRect.X)
	response["currentX"] = currentXstr
	currentYstr := fmt.Sprintf("%f", currentRect.Y)
	response["currentY"] = currentYstr
	response["player_id"] = playerID
	msg, err := json.Marshal(response)
	if err != nil {
		println("Json marshal error:", err)
	}
	conn.WriteMessage(websocket.TextMessage, msg)
}

func handlePlayerRespawn(data map[string]string, conn *websocket.Conn) string {
	if respawn, _ := strconv.ParseBool(data["respawn"]); respawn {
		var playerID string
		playerID = data["player_id"] //rand.IntN(1000000)
		//if err != "" {
		//	fmt.Printf("Error in respawn handler: %v", err)
		//}

		playersMutex.Lock()
		joinedPlayers[playerID] = make(map[string]rl.Rectangle)
		joinedPlayers[playerID]["playerDest"] = rl.NewRectangle(200, 200, 60, 60)
		joinedPlayers[playerID]["playerSrc"] = rl.NewRectangle(0, 0, 48, 48)
		playersMutex.Unlock()

		fmt.Printf("Player", playerID, "spawned. Total players:", len(joinedPlayers))

		// Send the player ID back to the client
		//conn.WriteMessage(websocket.TextMessage, []byte(strconv.Itoa(playerID)))
		return playerID
	}
	return ""
}

func handleGetPlayersWS(data map[string]string, conn *websocket.Conn) {
	excludeID := ""
	if excludeStr, exists := data["player_id"]; exists {
		id := excludeStr
		excludeID = id

	}

	playersMutex.RLock()
	playerList := make(map[string]map[string]rl.Rectangle)
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
		"type":      "player_positions",
		"players":   playerList,
		"player_id": excludeID,
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
		"command":   data["command"],
		"type":      data["type"],
		"map":       loadedMap,
		"player_id": data["player_id"],
	}
	jsonData, err := json.Marshal(response)
	//fmt.Println(data)
	if err != nil {
		panic(err)
	}
	conn.WriteMessage(websocket.TextMessage, jsonData)
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
	start_args := os.Args
	if len(start_args) < 2 {
		fmt.Println("Usage: program <host|join|gateway> [port|server_url]")
		os.Exit(1)
	}

	host_type = start_args[1]
	if host_type != "host" && host_type != "join" && host_type != "gateway" && host_type != "gatewayjoin" {
		fmt.Println("Mode must be 'host' or 'join'")
		os.Exit(1)
	}
	if host_type == "join" {
		if len(start_args) < 2 {
			fmt.Println("Please provide server URL for join mode")
			os.Exit(1)
		}
		server_url_ws = start_args[2]

		clientWebsocketConnect(server_url_ws, "/ws", "")

		// Wait a moment for connection to establish
		time.Sleep(100 * time.Millisecond)

		data := RespawnData{Respawn: true}
		sendDataRespawnWS(data)
	}
	if host_type == "gatewayjoin" {
		if len(start_args) < 3 {
			fmt.Println("Please provide server URL for join mode")
			os.Exit(1)
		}
		server_url_ws = start_args[2]

		gateway_invite_code = os.Args[3]
		clientWebsocketConnect(server_url_ws, "/join", gateway_invite_code)

		// Wait a moment for connection to establish
		time.Sleep(100 * time.Millisecond)

		data := RespawnData{Respawn: true}
		sendDataRespawnWS(data)
	}
	if host_type == "gateway" {
		if len(start_args) < 3 {
			fmt.Println("Please provide gateway url and local server port")
			os.Exit(1)
		}
		gateway_server = start_args[2]
		go startGatewayConnection(gateway_server)
		//dialog.Alert("to be implemented")
		time.Sleep(100 * time.Millisecond)

		clientWebsocketConnect(gateway_server, "/join", gateway_invite_code)

		// Wait for WebSocket connection
		time.Sleep(100 * time.Millisecond)

		data := RespawnData{Respawn: true}
		sendDataRespawnWS(data)

		// Wait for player ID assignment
		time.Sleep(100 * time.Millisecond)
	}

	if host_type == "host" {
		if len(start_args) < 3 {
			fmt.Println("Please provide port for host mode")
			os.Exit(1)
		}
		server_port := start_args[2]

		go startServer(server_port)

		server_url_ws = "localhost:" + server_port

		// Wait for server to start
		time.Sleep(200 * time.Millisecond)

		clientWebsocketConnect(server_url_ws, "/ws", "")

		// Wait for WebSocket connection
		time.Sleep(100 * time.Millisecond)

		data := RespawnData{Respawn: true}
		sendDataRespawnWS(data)

		// Wait for player ID assignment
		time.Sleep(100 * time.Millisecond)
	}

	loadMap()
}

func quit() {
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
		input()
		update()
		render()
	}
	quit()
}
