package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket" // For WebSocket communication
)

// Internal Message Types
const (
	MessageTypeConnect        = "connect"
	MessageTypeDisconnect     = "disconnect"
	MessageTypeUserChat       = "chat"            // Message from one user to another
	MessageTypeRegisterUser   = "register_user"   // Server registers a user's inbox
	MessageTypeUnregisterUser = "unregister_user" // Server unregisters a user
)

type ServerActor struct {
	inbox          chan ServerMessage
	connectedUsers map[string]chan UserMessage // Map UserID to UserActor's inbox
	quit           chan struct{}
	upgrader       websocket.Upgrader // WebSocket upgrader instance
	mu             sync.RWMutex       // For protecting connectedUsers map
}

// NewServerActor creates and starts the main server actor.
func NewServerActor() *ServerActor {
	s := &ServerActor{
		inbox:          make(chan ServerMessage),
		connectedUsers: make(map[string]chan UserMessage),
		quit:           make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all origins for simplicity
		},
	}
	go s.start()
	return s
}

// Start the ServerActor's message processing loop.
func (s *ServerActor) start() {
	log.Println("ServerActor started...")
	for {
		select {
		case msg := <-s.inbox:
			s.handleServerMessage(msg)
		case <-s.quit:
			log.Println("ServerActor stopping...")
			// Close all connected user inboxes or send disconnect messages
			s.mu.Lock()
			for _, userInbox := range s.connectedUsers {
				close(userInbox) // This will cause UserActors to exit
			}
			s.connectedUsers = make(map[string]chan UserMessage) // Clear map
			s.mu.Unlock()
			close(s.inbox)
			return
		}
	}
}

// handleServerMessage processes messages received by the ServerActor.
func (s *ServerActor) handleServerMessage(msg ServerMessage) {
	s.mu.Lock() // Protect map access
	defer s.mu.Unlock()

	switch msg.Type {
	case MessageTypeRegisterUser:
		log.Printf("ServerActor: Registering user %s\n", msg.UserID)
		s.connectedUsers[msg.UserID] = msg.UserRef
		if msg.Resp != nil {
			msg.Resp <- nil // Acknowledge registration
		}
	case MessageTypeUnregisterUser:
		log.Printf("ServerActor: Unregistering user %s\n", msg.UserID)
		delete(s.connectedUsers, msg.UserID)
		if msg.Resp != nil {
			msg.Resp <- nil // Acknowledge unregistration
		}
	case MessageTypeUserChat: // This is a message from a user to another user, forwarded by the ServerActor
		targetUserInbox, found := s.connectedUsers[msg.UserID] // msg.UserID here is the target
		if found {
			log.Printf("ServerActor: Forwarding chat from %s to %s\n", msg.Payload, msg.UserID) // Payload should contain sender info
			targetUserInbox <- UserMessage{
				Type: MessageTypeUserChat,
				// SenderID: msg.Payload.GetString("sender"), // Assuming payload has a sender field
				SenderID: GetStringFromRawMessage(msg.Payload, "sender"), // Assuming payload has a sender field
				Payload:  msg.Payload,
			}
		} else {
			log.Printf("ServerActor: Target user %s not found. Message dropped.\n", msg.UserID)
			// In a real app, you'd send an error back to the sender
		}
	default:
		log.Printf("ServerActor: Unknown message type: %s\n", msg.Type)
	}
}

// SendMessage allows other components to send messages to the ServerActor.
func (s *ServerActor) SendMessage(msg ServerMessage) {
	s.inbox <- msg
}

// Stop sends a quit signal to the ServerActor.
func (s *ServerActor) Stop() {
	s.quit <- struct{}{}
}
