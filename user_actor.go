package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientMessage (what we expect from the client via WebSocket)
type ClientMessage struct {
	Type      string `json:"type"`
	Recipient string `json:"recipient"` // For one-to-one chat
	Text      string `json:"text"`
	// ... other fields like "session_token" for auth
}

// ServerOutgoingMessage (what we send back to the client via WebSocket)
type ServerOutgoingMessage struct {
	Type   string `json:"type"`
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"` // For user status updates
}

// UserActor represents a connected user.
type UserActor struct {
	serverActor *ServerActor               // Reference to the main ServerActor
	conn        *websocket.Conn            // The WebSocket connection for this user
	id          string                     // Unique user ID (e.g., username)
	inbox       chan UserMessage           // Channel for messages directed to this user
	writeChan   chan ServerOutgoingMessage // Channel to write messages to the WebSocket
	quit        chan struct{}              // Signal for user to stop
}

// NewUserActor creates and starts a new UserActor.
func NewUserActor(serverActor *ServerActor, conn *websocket.Conn, userID string) *UserActor {
	u := &UserActor{
		serverActor: serverActor,
		conn:        conn,
		id:          userID,
		inbox:       make(chan UserMessage),
		writeChan:   make(chan ServerOutgoingMessage, 10), // Buffered channel for writes
		quit:        make(chan struct{}),
	}
	go u.start()
	return u
}

// Start the UserActor's message processing and I/O loops.
func (u *UserActor) start() {
	log.Printf("UserActor for %s started...\n", u.id)

	// Register with the ServerActor
	resp := make(chan error)
	u.serverActor.SendMessage(ServerMessage{
		Type:    MessageTypeRegisterUser,
		UserID:  u.id,
		UserRef: u.inbox,
		Resp:    resp,
	})
	if err := <-resp; err != nil {
		log.Printf("Failed to register user %s: %v\n", u.id, err)
		u.conn.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2) // One for reader, one for writer

	// Goroutine for reading messages from the WebSocket
	go func() {
		defer wg.Done()
		u.readPump()
	}()

	// Goroutine for writing messages to the WebSocket
	go func() {
		defer wg.Done()
		u.writePump()
	}()

	// Main actor loop for internal messages
	for {
		select {
		case msg := <-u.inbox:
			u.handleUserMessage(msg)
		case <-u.quit:
			log.Printf("UserActor for %s stopping...\n", u.id)
			// Unregister from ServerActor
			u.serverActor.SendMessage(ServerMessage{
				Type:   MessageTypeUnregisterUser,
				UserID: u.id,
				Resp:   resp, // Re-use resp channel
			})
			if err := <-resp; err != nil {
				log.Printf("Failed to unregister user %s: %v\n", u.id, err)
			}
			// Close WebSocket connection and channels
			u.conn.Close()
			close(u.inbox)
			close(u.writeChan)
			return
		}
	}
}

// readPump reads messages from the WebSocket and sends them to the ServerActor.
func (u *UserActor) readPump() {
	defer func() {
		u.quit <- struct{}{} // Signal main actor loop to stop on disconnect
		log.Printf("UserActor %s: Read pump stopped.\n", u.id)
	}()
	u.conn.SetReadLimit(512)
	u.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	u.conn.SetPongHandler(func(string) error { u.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	for {
		_, message, err := u.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("UserActor %s: Read error: %v\n", u.id, err)
			}
			break
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			log.Printf("UserActor %s: Failed to unmarshal client message: %v\n", u.id, err)
			// Send an error back to client or log
			continue
		}

		switch clientMsg.Type {
		case "chat_message":
			// Forward the message to the ServerActor for routing
			payload, _ := json.Marshal(struct { // Include sender info in payload
				Sender string `json:"sender"`
				Text   string `json:"text"`
			}{
				Sender: u.id,
				Text:   clientMsg.Text,
			})

			u.serverActor.SendMessage(ServerMessage{
				Type:    MessageTypeUserChat,
				UserID:  clientMsg.Recipient, // Recipient is the target ID
				Payload: payload,
			})
		default:
			log.Printf("UserActor %s: Unhandled client message type: %s\n", u.id, clientMsg.Type)
		}
	}
}

// writePump writes messages from the writeChan to the WebSocket.
func (u *UserActor) writePump() {
	ticker := time.NewTicker(50 * time.Second) // Ping interval
	defer func() {
		ticker.Stop()
		log.Printf("UserActor %s: Write pump stopped.\n", u.id)
	}()

	for {
		select {
		case message, ok := <-u.writeChan:
			if !ok {
				u.conn.WriteMessage(websocket.CloseMessage, []byte{}) // Channel closed
				return
			}
			w, err := u.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("UserActor %s: Error getting writer: %v\n", u.id, err)
				return
			}
			json.NewEncoder(w).Encode(message)
			if err := w.Close(); err != nil {
				log.Printf("UserActor %s: Error closing writer: %v\n", u.id, err)
				return
			}
		case <-ticker.C:
			// Send ping frame
			if err := u.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("UserActor %s: Ping write error: %v\n", u.id, err)
				return
			}
		case <-u.quit: // Listen for quit signal to stop writing
			return
		}
	}
}

// handleUserMessage processes internal messages directed to this UserActor.
func (u *UserActor) handleUserMessage(msg UserMessage) {
	switch msg.Type {
	case MessageTypeUserChat:
		// This is an incoming chat message from another user
		log.Printf("UserActor %s: Received chat from %s: %s\n", u.id, msg.SenderID, string(msg.Payload))

		var chatMsg struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
			log.Printf("UserActor %s: Failed to unmarshal chat payload: %v\n", u.id, err)
			return
		}

		u.writeChan <- ServerOutgoingMessage{
			Type:   "incoming_message",
			Sender: msg.SenderID,
			Text:   chatMsg.Text,
		}
	case MessageTypeConnect:
		// Could send a "you are connected" message to the client
		u.writeChan <- ServerOutgoingMessage{
			Type:   "status",
			Status: "connected",
		}
	default:
		log.Printf("UserActor %s: Unknown message type: %s\n", u.id, msg.Type)
	}
}
