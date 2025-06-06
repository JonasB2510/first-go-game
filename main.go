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
	websocket_client *websocket.Conn

	// Multiplayer
	playersMutex     sync.RWMutex
	joinedPlayers    = make(map[int]map[string]rl.Rectangle)
	joinPlayerID     int
	lastPlayerUpdate time.Time
	lastMapUpdate    time.Time
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
	if time.Since(lastMapUpdate) > 10000*time.Millisecond {
		requestMapDataWS()
		time.Sleep(1 * time.Second)
		loadMap()
		lastMapUpdate = time.Now()
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
		fmt.Println("Usage: program <host|join> [port|server_url]")
		os.Exit(1)
	}

	host_type = start_args[1]
	if host_type != "host" && host_type != "join" {
		fmt.Println("Mode must be 'host' or 'join'")
		os.Exit(1)
	}
	if host_type == "join" {
		if len(start_args) < 3 {
			fmt.Println("Please provide server URL for join mode")
			os.Exit(1)
		}
		server_url_ws = start_args[2]

		clientWebsocketConnect(server_url_ws)

		// Wait a moment for connection to establish
		time.Sleep(100 * time.Millisecond)

		data := RespawnData{Respawn: true}
		sendDataRespawnWS(data)
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

		clientWebsocketConnect(server_url_ws)

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
