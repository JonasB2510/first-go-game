package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	screenWidth  = 1000
	screenHeight = 480
)

var (
	running  = true
	bkgColor = rl.NewColor(147, 211, 196, 255)

	grassSprite          rl.Texture2D
	fenceSprite          rl.Texture2D
	hillSprite           rl.Texture2D
	waterSprite          rl.Texture2D
	woodHouseWallsSprite rl.Texture2D
	woodHouseRoofSprite  rl.Texture2D
	tilledSprite         rl.Texture2D
	doorSprite           rl.Texture2D

	tex rl.Texture2D

	playerSprite rl.Texture2D

	playerSrc                                     rl.Rectangle
	playerDest                                    rl.Rectangle
	playerMoving                                  bool
	playerDir                                     int
	playerUp, playerDown, playerRight, playerLeft bool
	playerFrame                                   int
	maxFrames                                     = 4

	frameCount int

	tileDest   rl.Rectangle
	tileSrc    rl.Rectangle
	tileMap    []int
	srcMap     []string
	mapW, mapH int

	playerSpeed float32 = 3

	musicPaused bool
	music       rl.Music

	cam rl.Camera2D

	map_file    = "resource/maps/second.map"
	map_hotswap = true

	host_type  string
	server_url string

	position_error_count int

	// Add mutex for thread safety
	playersMutex  sync.RWMutex
	joinedPlayers = make(map[int]map[string]rl.Rectangle)
	joinPlayerID  int
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
type PlayerPos struct {
	ExcludeID int `json:"exclude_id"`
}

func drawScene() {
	//rl.DrawTexture(grassSprite, 100, 50, rl.White)

	for i := 0; i < len(tileMap); i++ {
		if tileMap[i] != 0 {
			tileDest.X = tileDest.Width * float32(i%mapW)
			tileDest.Y = tileDest.Height * float32(i/mapW)
			//fmt.Println(srcMap)
			if srcMap[i] == "g" {
				tex = grassSprite
			}
			if srcMap[i] == "f" {
				tex = fenceSprite
			}
			if srcMap[i] == "h" {
				tex = hillSprite
			}
			if srcMap[i] == "w" {
				tex = waterSprite
			}
			if srcMap[i] == "ww" {
				tex = woodHouseWallsSprite
			}
			if srcMap[i] == "wr" {
				tex = woodHouseRoofSprite
			}
			if srcMap[i] == "t" {
				tex = tilledSprite
			}
			if srcMap[i] == "d" {
				tex = doorSprite
			}

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

	// Thread-safe access to joinedPlayers - render other players
	playersMutex.RLock()
	for playerID, val := range joinedPlayers {
		// Don't render the local player twice (skip own ID)
		if playerID != joinPlayerID {
			rl.DrawTexturePro(playerSprite, val["playerSrc"], val["playerDest"], rl.NewVector2(val["playerDest"].Width, val["playerDest"].Height), 0, rl.White)
		}
	}
	playersMutex.RUnlock()

	// Always render the local player
	rl.DrawTexturePro(playerSprite, playerSrc, playerDest, rl.NewVector2(playerDest.Width, playerDest.Height), 0, rl.White)
}

func input() {
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		playerMoving = true
		playerDir = 1 //0
		playerUp = true
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		playerMoving = true
		playerDir = 2 //2
		playerLeft = true
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		playerMoving = true
		playerDir = 0 //0
		playerDown = true
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		playerMoving = true
		playerDir = 3 //3
		playerRight = true
	}
	if rl.IsKeyPressed(rl.KeyQ) {
		musicPaused = !musicPaused
	}
}

func update() {
	running = !rl.WindowShouldClose()

	//playerSrc.X = 0
	if playerFrame > (maxFrames - 1) {
		playerFrame = 0
	}
	playerSrc.X = ((playerSrc.Width * float32(playerFrame)) + ((playerSrc.Width * float32(maxFrames)) * float32(playerDir)))

	if playerMoving {
		//if playerUp {
		//	playerDest.Y -= playerSpeed
		//}
		//if playerLeft {
		//	playerDest.X -= playerSpeed
		//}
		//if playerDown {
		//	playerDest.Y += playerSpeed
		//}
		//if playerRight {
		//	playerDest.X += playerSpeed
		//}

		// Update local position in server's map if this is the host
		if host_type == "host" {
			updateLocalPlayerOnServer()
		}

		if host_type == "join" || host_type == "host" {
			data := MovementData{
				PlayerID:    joinPlayerID,
				PlayerUp:    playerUp,
				PlayerLeft:  playerLeft,
				PlayerDown:  playerDown,
				PlayerRight: playerRight,
			}
			sendDataMovement(server_url, data)
		}
		if frameCount%8 == 1 {
			playerFrame++
		}
	} else if frameCount%45 == 1 {
		playerFrame++
	}
	frameCount++
	playerSrc.Y = playerSrc.Height
	if !playerMoving && playerFrame > 1 {
		playerFrame = 0
	}
	playerExclude := PlayerPos{
		ExcludeID: joinPlayerID,
	}
	if host_type == "join" || host_type == "host" {
		sendDataPlayerPos(server_url, playerExclude)
	}

	rl.UpdateMusicStream(music)
	if musicPaused {
		rl.PauseMusicStream(music)
	} else {
		rl.ResumeMusicStream(music)
	}

	cam.Target = rl.NewVector2(float32(playerDest.X-(playerDest.Width/2)), float32(playerDest.Y-(playerDest.Height/2)))

	playerMoving = false
	playerUp, playerDown, playerRight, playerLeft = false, false, false, false
}

// New function to update local player position in server's map
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

	if map_hotswap {
		loadMap(map_file)
	}
	drawScene()

	rl.EndMode2D()
	rl.EndDrawing()
}

func loadMap(mapFile string) {
	tileMap = tileMap[:0] // Clear tileMap
	srcMap = srcMap[:0]   // Clear srcMap

	file, err := os.ReadFile(mapFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	remNewLines := strings.Replace(string(file), "\n", " ", -1)
	sliced := strings.Fields(remNewLines) // Changed from Split to Fields - this removes empty strings
	//fmt.Println(file)
	//fmt.Println("reloading map")
	mapW = -1
	mapH = -1
	for i := 0; i < len(sliced); i++ {
		if mapW == -1 {
			// Parse width
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue
			}
			mapW = int(s)
		} else if mapH == -1 {
			// Parse height
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue
			}
			mapH = int(s)
		} else if len(tileMap) < mapW*mapH {
			// Parse tile data (numbers)
			s, err := strconv.ParseInt(sliced[i], 10, 64)
			if err != nil {
				continue // Skip invalid entries
			}
			m := int(s)
			tileMap = append(tileMap, m)
		} else {
			// Parse texture data (letters/strings)
			srcMap = append(srcMap, sliced[i])
		}
	}
}

func sendDataMovement(server_url string, data MovementData) {
	//fmt.Println(data)
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Fehler beim JSON:", err)
		return
	}
	//fmt.Println(jsonData)

	resp, err := http.Post(server_url+"/data", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Fehler beim Senden:", err)
		position_error_count++
		if position_error_count > 15 {
			quit()
		}
		return
	}
	defer resp.Body.Close()
	respMsg, _ := io.ReadAll(resp.Body)
	//fmt.Println("Antwort vom Server:", string(respMsg), "Player Koords:", playerDest.X, playerDest.Y)
	playerPositionen := strings.Split(string(respMsg), ",")
	fmt.Println(playerPositionen)
	x, err := strconv.ParseFloat(playerPositionen[0], 64)
	if err != nil {
		panic(err)
	}
	y, err := strconv.ParseFloat(playerPositionen[1], 64)
	if err != nil {
		panic(err)
	}
	playerDest.X = float32(x)
	playerDest.Y = float32(y)
	fmt.Println(playerDest)
	position_error_count = 0
}

func sendDataRespawn(server_url string, data RespawnData) {
	//fmt.Println(data)
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Fehler beim JSON:", err)
		return
	}
	//fmt.Println(jsonData)

	resp, err := http.Post(server_url+"/respawn", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Fehler beim Senden:", err)
		position_error_count++
		if position_error_count > 15 {
			quit()
		}
		return
	}
	defer resp.Body.Close()
	respMsg, _ := io.ReadAll(resp.Body)
	joinPlayerID, err = strconv.Atoi(string(respMsg))
	if err != nil {
		fmt.Println("Fehler:", err)
	}
	//fmt.Println("Antwort vom Server:", string(respMsg), "Player Koords:", playerDest.X, playerDest.Y)
	position_error_count = 0
}

func sendDataPlayerPos(server_url string, data PlayerPos) {
	//fmt.Println(data)
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Fehler beim JSON:", err)
		return
	}
	//fmt.Println(jsonData)

	resp, err := http.Post(server_url+"/players", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Fehler beim Senden:", err)
		position_error_count++
		if position_error_count > 15 {
			quit()
		}
		return
	}
	defer resp.Body.Close()
	respMsg, _ := io.ReadAll(resp.Body)

	// Parse the response and update local joinedPlayers
	var receivedPlayers map[int]map[string]rl.Rectangle
	if err := json.Unmarshal(respMsg, &receivedPlayers); err != nil {
		fmt.Println("Error parsing player positions:", err)
		return
	}

	// Update local joinedPlayers with received data
	playersMutex.Lock()
	// For host: preserve own player data, update others
	// For client: replace all with server data
	if host_type == "host" {
		// Keep own player data, update others
		ownPlayerData := joinedPlayers[joinPlayerID]
		joinedPlayers = make(map[int]map[string]rl.Rectangle)
		if ownPlayerData != nil {
			joinedPlayers[joinPlayerID] = ownPlayerData
		}
		for id, player := range receivedPlayers {
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
		for id, player := range receivedPlayers {
			joinedPlayers[id] = make(map[string]rl.Rectangle)
			for key, rect := range player {
				joinedPlayers[id][key] = rect
			}
		}
	}
	playersMutex.Unlock()

	position_error_count = 0
}

func startServer(port string) {
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Could not read body", http.StatusInternalServerError)
			return
		}
		var data MovementData
		if err := json.Unmarshal(body, &data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Use the correct player ID from the request data
		playerID := data.PlayerID

		// Thread-safe access to joinedPlayers
		playersMutex.Lock()
		defer playersMutex.Unlock()

		// Check if player exists in our map
		if _, exists := joinedPlayers[playerID]; !exists {
			http.Error(w, "Player not found", http.StatusNotFound)
			fmt.Printf("Player %d not found. Available players: %v\n", playerID, getPlayerIDs())
			return
		}

		// Get current position
		currentRect := joinedPlayers[playerID]["playerDest"]
		currentRectSrc := joinedPlayers[playerID]["playerSrc"]
		currentPlayerDir := 0
		// Apply movement based on the received data
		if data.PlayerUp {
			currentRect.Y -= playerSpeed
			currentPlayerDir = 1
		}
		if data.PlayerLeft {
			currentRect.X -= playerSpeed
			currentPlayerDir = 2
		}
		if data.PlayerDown {
			currentRect.Y += playerSpeed
			currentPlayerDir = 0
		}
		if data.PlayerRight {
			currentRect.X += playerSpeed
			currentPlayerDir = 3
		}
		currentRectSrc.Y = currentRectSrc.Height
		currentRectSrc.X = ((currentRectSrc.Width * float32(playerFrame)) + ((currentRectSrc.Width * float32(maxFrames)) * float32(currentPlayerDir)))

		// Update the position in the map
		joinedPlayers[playerID]["playerDest"] = currentRect
		joinedPlayers[playerID]["playerSrc"] = currentRectSrc

		fmt.Printf("Player %d moved to: %.2f, %.2f\n", playerID, currentRect.X, currentRect.Y)

		// Return the updated position
		response := fmt.Sprintf("%.0f,%.0f", currentRect.X, currentRect.Y)
		w.Write([]byte(response))
	})

	http.HandleFunc("/respawn", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Could not read body", http.StatusInternalServerError)
			return
		}
		var data RespawnData
		if err := json.Unmarshal(body, &data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if data.Respawn {
			playerID := rand.IntN(1000000)

			// Thread-safe access to joinedPlayers
			playersMutex.Lock()
			// Initialize the inner map first
			joinedPlayers[playerID] = make(map[string]rl.Rectangle)
			joinedPlayers[playerID]["playerDest"] = rl.NewRectangle(200, 200, 60, 60)
			joinedPlayers[playerID]["playerSrc"] = rl.NewRectangle(0, 0, 48, 48)
			playersMutex.Unlock()

			fmt.Printf("Player %d spawned. Total players: %d\n", playerID, len(joinedPlayers))
			w.Write([]byte(strconv.Itoa(playerID)))
		}
	})

	http.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Could not read body", http.StatusInternalServerError)
			return
		}
		var data PlayerPos
		if err := json.Unmarshal(body, &data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Thread-safe access and proper copying
		playersMutex.RLock()
		playerList := make(map[int]map[string]rl.Rectangle)
		for id, player := range joinedPlayers {
			if id != data.ExcludeID {
				playerList[id] = make(map[string]rl.Rectangle)
				for key, rect := range player {
					playerList[id][key] = rect
				}
			}
		}
		playersMutex.RUnlock()

		jsonData, err := json.Marshal(playerList)
		if err != nil {
			http.Error(w, "Error serializing player list", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})

	fmt.Println("Server läuft auf http://localhost:" + port)
	// Fix: Use just ":port" format, not full URL
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// Helper function to get player IDs for debugging
func getPlayerIDs() []int {
	var ids []int
	for id := range joinedPlayers {
		ids = append(ids, id)
	}
	return ids
}

func init() {
	rl.InitWindow(screenWidth, screenHeight, "Simple Game")
	rl.SetExitKey(0)
	rl.SetTargetFPS(60)

	grassSprite = rl.LoadTexture("resource/tilesets/grass.png")
	fenceSprite = rl.LoadTexture("resource/tilesets/fences.png")
	hillSprite = rl.LoadTexture("resource/tilesets/hills.png")
	waterSprite = rl.LoadTexture("resource/tilesets/water.png")
	woodHouseWallsSprite = rl.LoadTexture("resource/tilesets/wood_walls.png")
	woodHouseRoofSprite = rl.LoadTexture("resource/tilesets/wood_roof.png")
	tilledSprite = rl.LoadTexture("resource/tilesets/tilled.png")
	doorSprite = rl.LoadTexture("resource/tilesets/doors.png")

	tileDest = rl.NewRectangle(0, 0, 16, 16)
	tileSrc = rl.NewRectangle(0, 0, 16, 16)

	playerSprite = rl.LoadTexture("resource/tilesets/player.png")

	playerSrc = rl.NewRectangle(0, 0, 48, 48)
	playerDest = rl.NewRectangle(200, 200, 60, 60)

	rl.InitAudioDevice()
	music = rl.LoadMusicStream("resource/music/music.mp3")
	musicPaused = true
	rl.PlayMusicStream(music)

	cam = rl.NewCamera2D(rl.NewVector2(float32(screenWidth/2), float32(screenHeight/2)), rl.NewVector2(float32(playerDest.X-(playerDest.Width/2)), float32(playerDest.Y-(playerDest.Height/2))), 0.0, 1.5)

	loadMap(map_file)
}

func quit() {
	rl.UnloadTexture(grassSprite)
	rl.UnloadTexture(playerSprite)
	rl.UnloadMusicStream(music)
	rl.CloseAudioDevice()
	rl.CloseWindow()
}

func main() {
	position_error_count = 0
	start_args := os.Args
	if len(start_args) < 2 {
		fmt.Println("Usage: program <host|join> [port|server_url]")
		os.Exit(1)
	}

	if start_args[1] == "host" || start_args[1] == "join" {
		host_type = start_args[1]
	} else {
		host_type = "none"
	}

	if host_type == "join" {
		if len(start_args) < 3 {
			fmt.Println("Please provide server URL for join mode")
			os.Exit(1)
		}
		server_url = os.Args[2]
		data := RespawnData{
			Respawn: true,
		}
		sendDataRespawn(server_url, data)
	}

	if host_type == "host" {
		if len(start_args) < 3 {
			fmt.Println("Please provide port for host mode")
			os.Exit(1)
		}
		server_port := os.Args[2]
		go startServer(server_port)

		// Host also joins as a player
		server_url = "http://localhost:" + server_port
		// Wait a moment for server to start
		time.Sleep(100 * time.Millisecond)
		data := RespawnData{
			Respawn: true,
		}
		sendDataRespawn(server_url, data)
	}

	rl.SetWindowTitle("Simple Game: " + host_type)

	for running {
		input()
		update()
		render()
	}
	quit()
}
