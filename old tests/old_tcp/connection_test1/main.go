package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

var (
	playerDest  rl.Rectangle
	playerSpeed float32 = 3
)

type MovementData struct {
	PlayerUp    bool `json:"playerUp"`
	PlayerLeft  bool `json:"playerLeft"`
	PlayerDown  bool `json:"playerDown"`
	PlayerRight bool `json:"playerRight"`
}
type RespawnData struct {
	Respawn bool `json:"respawn"`
}

func startServer() {
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
		//fmt.Printf("\nEmpfangen: ", data.PlayerUp, data.PlayerLeft, data.PlayerDown, data.PlayerRight, "\n")
		if data.PlayerUp {
			playerDest.Y -= playerSpeed
		}
		if data.PlayerLeft {
			playerDest.X -= playerSpeed
		}
		if data.PlayerDown {
			playerDest.Y += playerSpeed
		}
		if data.PlayerRight {
			playerDest.X += playerSpeed
		}
		w.Write([]byte(strconv.Itoa(int(playerDest.X)) + " " + strconv.Itoa(int(playerDest.Y))))
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
			playerDest.X = 200
			playerDest.Y = 200
			//fmt.Println("respawned character")
		}
	})

	fmt.Println("Server läuft auf http://localhost:8080 ...")
	http.ListenAndServe(":8080", nil)
}

func sendData() {
	data := MovementData{
		PlayerUp:    false,
		PlayerLeft:  false,
		PlayerDown:  false,
		PlayerRight: false,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Fehler beim JSON:", err)
		return
	}

	resp, err := http.Post("http://localhost:8080/data", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Fehler beim Senden:", err)
		return
	}
	defer resp.Body.Close()
	respMsg, _ := io.ReadAll(resp.Body)
	fmt.Println("Antwort vom Server:", string(respMsg))
}
func init() {
	playerDest = rl.NewRectangle(200, 200, 60, 60)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Benutzung: go run main.go [host|join]")
		return
	}

	mode := os.Args[1]
	if mode == "host" {
		startServer()
	} else if mode == "join" {
		sendData()
	} else {
		fmt.Println("Ungültiger Modus. Benutze 'host' oder 'join'.")
	}
}
