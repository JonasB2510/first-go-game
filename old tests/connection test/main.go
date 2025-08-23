package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// upgrader konvertiert HTTP-Verbindungen zu WebSocket-Verbindungen
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Erlaube alle Ursprünge (für Produktion solltest du das einschränken)
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP-Verbindung zu WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade Fehler:", err)
		return
	}
	defer conn.Close()

	for {
		// Nachricht lesen
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read Fehler:", err)
			break
		}
		log.Printf("Empfangen: %s", message)

		// Nachricht zurücksenden (Echo)
		err = conn.WriteMessage(messageType, message)
		if err != nil {
			log.Println("Write Fehler:", err)
			break
		}
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	log.Println("Server startet auf :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
