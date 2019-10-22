package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/SwanHub/chat-app/backend/pkg/websocket"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// Channel struct yes
type Channel struct {
	AdminID   int       `json:"admin"`
	Name      string    `json:"name"`
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"long"`
	Messages  []Message `gorm:"foreignkey:ChannelID"`
	gorm.Model
}

// Message struct which will contain all messages created by users.
type Message struct {
	Content   string `json:"content"`
	UserID    uint
	ChannelID uint
	gorm.Model
}

// User ... They are at the pinnacle of the relational hierarchy.
type User struct {
	Name     string    `json:"name"`
	Messages []Message `gorm:"foreignkey:UserID"`
	gorm.Model
}

func initialMigration() {
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		fmt.Println(err.Error())
		panic("Failed to connect to the database")
	}
	defer db.Close()

	db.AutoMigrate(&Channel{}, &User{}, &Message{})
}

// Evidently, I can create a struct in a lower level and access the db in the higher up shit. Then use that struct to create messages...
func serveWs(pool *websocket.Pool, w http.ResponseWriter, r *http.Request) {
	fmt.Println("Websocket Endpoint Hit ...")
	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		fmt.Fprintf(w, "%+v\n", err)
	}

	client := &websocket.Client{
		Conn: conn,
		Pool: pool,
	}

	pool.Register <- client
	client.Read()
}

func showUser(w http.ResponseWriter, r *http.Request) {
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	vars := mux.Vars(r)
	id := vars["id"]

	var user User
	db.Where("ID = ?", id).Find(&user)
	db.Model(&user).Association("Messages").Find(&user.Messages)
	json.NewEncoder(w).Encode(user)
}

func showChannel(w http.ResponseWriter, r *http.Request) {
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	vars := mux.Vars(r)
	id := vars["id"]

	var channel Channel
	db.Where("ID = ?", id).Find(&channel)
	db.Model(&channel).Association("Messages").Find(&channel.Messages)

	json.NewEncoder(w).Encode(channel)
}

func allChannels(w http.ResponseWriter, r *http.Request) {
	// Access this directory's database
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	// Print all channels in JSON to the screen
	var channels []Channel

	db.Find(&channels)
	json.NewEncoder(w).Encode(channels)
}

func hsin(theta float64) float64 {
	return math.Pow(math.Sin(theta/2), 2)
}

func distance(lat1, lon1, lat2, lon2 float64) float64 {
	// convert to radians
	// must cast radius as float to multiply later
	var la1, lo1, la2, lo2, r float64
	la1 = lat1 * math.Pi / 180
	lo1 = lon1 * math.Pi / 180
	la2 = lat2 * math.Pi / 180
	lo2 = lon2 * math.Pi / 180

	r = 6378100 // Earth radius in METERS

	// calculate
	h := hsin(la2-la1) + math.Cos(la1)*math.Cos(la2)*hsin(lo2-lo1)

	return 2 * r * math.Asin(math.Sqrt(h))
}

func channelsNearMe(w http.ResponseWriter, r *http.Request) {
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	// Print all channels in JSON to the screen
	var channels []Channel
	db.Find(&channels)

	// Set current location (in the future, this will be a computed value)
	vars := mux.Vars(r)
	lat, _ := strconv.ParseFloat(vars["lat"], 64)
	long, _ := strconv.ParseFloat(vars["long"], 64)
	radius, _ := strconv.ParseFloat(vars["radius"], 64)

	var filteredChannels []Channel
	for i := 0; i < len(channels); i++ {
		if distance(channels[i].Latitude, channels[i].Longitude, lat, long) < radius {
			filteredChannels = append(filteredChannels, channels[i])
		}
	}

	fmt.Print("\ncurrent location: ", lat, long)
	json.NewEncoder(w).Encode(filteredChannels)
}

func setupBasicRoutes() {
	// initiate websocket pool
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/user/{id}", showUser).Methods("GET")
	myRouter.HandleFunc("/channel/{id}", showChannel).Methods("GET")
	myRouter.HandleFunc("/channel/{name}/{lat}/{long}", func(w http.ResponseWriter, r *http.Request) {
		// initiate pool
		pool := websocket.NewPool()
		go pool.Start()

		// connect to db.
		db, err := gorm.Open("sqlite3", "test.db")
		if err != nil {
			panic("Could not connect to the database")
		}
		defer db.Close()

		// create new channel with the name given in parameter
		vars := mux.Vars(r)
		name := vars["name"]
		lat, _ := strconv.ParseFloat(vars["lat"], 64)
		long, _ := strconv.ParseFloat(vars["long"], 64)

		fmt.Println(name, lat, long)

		db.Create(&Channel{Name: name, Latitude: lat, Longitude: long})
		var channel Channel
		db.Where("name = ?", name).Find(&channel)

		// Create a new websocket associated with the id of the new channel in the database
		endpoint := fmt.Sprintf("/ws/%d", channel.Model.ID)
		myRouter.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			serveWs(pool, w, r)
		})

		json.NewEncoder(w).Encode(channel)
	}).Methods("POST")

	// Get all current channels
	myRouter.HandleFunc("/channels", allChannels).Methods("GET")
	myRouter.HandleFunc("/nearme/{lat}/{long}/{radius}", channelsNearMe).Methods("GET")

	// open all current websockets
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	// Print all channels in JSON to the screen
	var channels []Channel
	db.Find(&channels)

	for i := 0; i < len(channels); i++ {
		pool := websocket.NewPool()
		go pool.Start()

		endpoint := fmt.Sprintf("/ws/%d", channels[i].Model.ID)

		myRouter.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			serveWs(pool, w, r)
		})
	}

	//Listen on 8080 port
	log.Fatal(http.ListenAndServe(":8080", myRouter))
}

func main() {
	fmt.Println("Distributed Chat System Over the Airwaves")

	initialMigration()

	setupBasicRoutes()
}
