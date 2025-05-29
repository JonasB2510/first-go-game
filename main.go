package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
)

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
				tileSrc.Y = tileSrc.Height * 6
				rl.DrawTexturePro(grassSprite, tileSrc, tileDest, rl.NewVector2(tileDest.Width, tileDest.Height), 0, rl.White)
			}

			tileSrc.X = tileSrc.Width * float32((tileMap[i]-1)%int(tex.Width/int32(tileSrc.Width)))
			tileSrc.Y = tileSrc.Height * float32((tileMap[i]-1)/int(tex.Width/int32(tileSrc.Width)))

			rl.DrawTexturePro(tex, tileSrc, tileDest, rl.NewVector2(tileDest.Width, tileDest.Height), 0, rl.White)
		}
	}

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
	//fmt.Println(tileMap)

	// Remove unnecessary trimming since we're controlling the append logic above
	/*if len(tileMap) > mapW*mapH {
		tileMap = tileMap[:len(tileMap)-1] // Trim to exact size needed
	}*/

	/*for i := 0; i < (mapW * mapH); i++ {
		tileMap = append(tileMap, 56)
	}*/
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
	musicPaused = false
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

	for running {
		input()
		update()
		render()
	}
	quit()
}
