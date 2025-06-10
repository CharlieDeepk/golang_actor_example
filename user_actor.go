package main

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// // ClientMessage (what we expect from the client via WebSocket)
// type ClientMessage struct {
// 	Type      string `json:"type"`
// 	Recipient string `json:"recipient"` // For one-to-one chat
// 	Text      string `json:"text"`
// 	// ... other fields like "session_token" for auth
// }

// // ServerOutgoingMessage (what we send back to the client via WebSocket)
// type ServerOutgoingMessage struct {
// 	Type   string `json:"type"`
// 	Sender string `json:"sender,omitempty"`
// 	Text   string `json:"text,omitempty"`
// 	Status string `json:"status,omitempty"` // For user status updates
// }

// UserActor represents a connected user.
type UserActor struct {
	ID         string                     // Unique user ID (e.g., username)
	Conn       *websocket.Conn            // The WebSocket connection for this user
	ServerChan chan<- UserToServerMessage // Channel to send messages to the Server Actor
	Inbox      chan ServerToUserMessage   // Channel to receive messages from Server/GroupChatActors
	StopChan   chan struct{}              // Signal for user to stop
	Wg         sync.WaitGroup
}

// NewUserActor creates and starts a new UserActor.
func NewUserActor(serverChan chan<- UserToServerMessage, conn *websocket.Conn, userID string) *UserActor {
	u := &UserActor{
		ID:         userID,
		Conn:       conn,
		ServerChan: serverChan,
		Inbox:      make(chan ServerToUserMessage, 100), //buffered channel
		StopChan:   make(chan struct{}),
	}
	// go u.start()
	return u
}

// Start the UserActor's message processing and I/O loops.
func (u *UserActor) start() {
	u.Wg.Add(2) // for read and write goroutine

	// Goroutine to read messages from the WebSocket connection
	go u.readPump()
	// Goroutine to write messages from the WebSocket connection
	go u.writePump()

	log.Printf("UserActor for %s started...\n", u.ID)
}

func (u *UserActor) Stop() {
	log.Printf("Stopping UserActor %s...", u.ID)
	close(u.StopChan)
	u.Wg.Wait() // Wait for readPump and writePump to finish
	u.Conn.Close()
	log.Printf("UserActor %s stopped.", u.ID)
}

func (u *UserActor) Send(msg ServerToUserMessage) {
	select {
	case u.Inbox <- msg:
	case <-u.StopChan:
		log.Printf("UserActor %s inbox is closed, message dropped.", u.ID)
	default:
		log.Printf("UserActor %s inbox is full, message dropped.", u.ID)
	}
}

// readPump reads messages from the WebSocket and sends them to the ServerActor.
func (u *UserActor) readPump() {
	defer func() {
		u.Wg.Done()
		log.Printf("UserActor %s: Read pump exiting.\n", u.ID)
		// Notify server that this user disconnected
		u.ServerChan <- UserToServerMessage{
			FromUserID: u.ID,
			Message: Message{
				Type: "disconnect",
				Payload: map[string]string{
					"user_id": u.ID,
				},
			},
		}

	}()
	// u.Conn.SetReadLimit(512)
	// u.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	// u.Conn.SetPongHandler(func(string) error { u.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	for {
		select {
		case <-u.StopChan:
			return
		default:
			var msg Message
			err := u.Conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("UserActor %s websocket closed: %v", u.ID, err)
				} else {
					log.Printf("UserActor %s read error: %v", u.ID, err)
				}
				return
			}
			log.Printf("UserActor %s received from client: %+v", u.ID, msg)
			u.ServerChan <- UserToServerMessage{
				FromUserID: u.ID,
				Message:    msg,
			}
		}

	}
}

// writePump writes messages from the Inbox to the WebSocket connection
func (u *UserActor) writePump() {
	// ticker := time.NewTicker(50 * time.Second) // Ping interval
	defer func() {
		// ticker.Stop()
		u.Wg.Done()
		log.Printf("UserActor %s: Write pump exiting.\n", u.ID)
	}()

	for {
		select {
		case msg := <-u.Inbox:
			log.Printf("UserActor %s sending to client: %+v", u.ID, msg.Message)
			err := u.Conn.WriteJSON(msg.Message)
			if err != nil {
				log.Printf("UserActor %s write error: %v", u.ID, err)
				return
			}

		// case <-ticker.C:
		// 	// Send ping frame
		// 	if err := u.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		// 		log.Printf("UserActor %s: Ping write error: %v\n", u.id, err)
		// 		return
		// 	}
		case <-u.StopChan: // Listen for quit signal to stop writing
			return
		}
	}
}

//
// completed til here
//

// // handleUserMessage processes internal messages directed to this UserActor.
// func (u *UserActor) handleUserMessage(msg UserMessage) {
// 	switch msg.Type {
// 	case MessageTypeUserChat:
// 		// This is an incoming chat message from another user
// 		log.Printf("UserActor %s: Received chat from %s: %s\n", u.id, msg.SenderID, string(msg.Payload))

// 		var chatMsg struct {
// 			Text string `json:"text"`
// 		}
// 		if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
// 			log.Printf("UserActor %s: Failed to unmarshal chat payload: %v\n", u.id, err)
// 			return
// 		}

// 		u.writeChan <- ServerOutgoingMessage{
// 			Type:   "incoming_message",
// 			Sender: msg.SenderID,
// 			Text:   chatMsg.Text,
// 		}
// 	case MessageTypeConnect:
// 		// Could send a "you are connected" message to the client
// 		u.writeChan <- ServerOutgoingMessage{
// 			Type:   "status",
// 			Status: "connected",
// 		}
// 	default:
// 		log.Printf("UserActor %s: Unknown message type: %s\n", u.id, msg.Type)
// 	}
// }
