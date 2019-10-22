package websocket

import (
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Client is the individual connection associated with a computer
type Client struct {
	ID   string
	Conn *websocket.Conn
	Pool *Pool
	sync.Mutex
}

// Message is the message written for Broadcast
type ChanMessage struct {
	Type int    `json:"type"`
	Body string `json:"body"`
}

func (c *Client) Read() {
	defer func() {
		// writes to channel
		c.Pool.Unregister <- c
		c.Conn.Close()
	}()

	for {
		messageType, p, err := c.Conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}

		// db.Create(&Message{Content: string(p), UserID: 1, ChannelID: 1})

		message := ChanMessage{Type: messageType, Body: string(p)}
		c.Pool.Broadcast <- message
		fmt.Printf("Message Received: %+v\n", message)
	}
}
