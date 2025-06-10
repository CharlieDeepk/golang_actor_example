package main

import (
	"log"
	"sync"
	"time"
)

type GroupChatActor struct {
	ID       string
	Name     string
	Members  map[string]*UserActor // Map of UserID to UserActor example: { 8051215998: *UserActor(8051215998), ... }
	Inbox    chan ServerToGroupMessage
	StopChan chan struct{}
	Wg       sync.WaitGroup
	server   *ServerActor // Reference to the server to send messages back (e.g., errors)
	mu       sync.RWMutex // Mutex for concurrent access to members map
}

func NewGroupChatActor(id, name string, server *ServerActor) *GroupChatActor {
	gca := &GroupChatActor{
		ID:       id,
		Name:     name,
		Members:  make(map[string]*UserActor),
		Inbox:    make(chan ServerToGroupMessage, 100),
		StopChan: make(chan struct{}),
		server:   server,
	}
	return gca
}

func (gca *GroupChatActor) Start() {
	gca.Wg.Add(1)
	go gca.run()
	log.Printf("GroupChatActor %s ('%s') started.", gca.ID, gca.Name)

}

func (gca *GroupChatActor) Stop() {
	log.Printf("Stopping GroupChatActor %s...", gca.ID)
	close(gca.StopChan)
	gca.Wg.Wait()
	log.Printf("GroupChatActor %s stopped.", gca.ID)
}

func (gca *GroupChatActor) Send(msg ServerToGroupMessage) {
	select {
	case gca.Inbox <- msg:
	case <-gca.StopChan:
		log.Printf("GroupChatActor %s inbox is closed, message dropped.", gca.ID)
	default:
		log.Printf("GroupChatActor %s inbox is full, message dropped.", gca.ID)
	}
}

// func to add members to GroupChatActor's Members
func (gca *GroupChatActor) AddMember(userActor *UserActor) {
	gca.mu.Lock()
	defer gca.mu.Unlock()
	gca.Members[userActor.ID] = userActor
	log.Printf("User %s added to group %s.", userActor.ID, gca.Name)
	// Notify all members about the new member
	gca.broadcast(Message{
		Type: "group_update",
		Payload: map[string]string{
			"group_id":   gca.ID,
			"group_name": gca.Name,
			"action":     "member_joined",
			"user_id":    userActor.ID,
		},
	})

}

// func to remove members from GroupChatActor's Members
func (gca *GroupChatActor) RemoveMember(userID string) {
	gca.mu.Lock()
	defer gca.mu.Unlock()
	if _, exists := gca.Members[userID]; exists {
		delete(gca.Members, userID)
		log.Printf("User %s removed from group %s.", userID, gca.Name)
		// Notify all members about the member leaving
		gca.broadcast(Message{
			Type: "group_update",
			Payload: map[string]string{
				"group_id":   gca.ID,
				"group_name": gca.Name,
				"action":     "member_left",
				"user_id":    userID,
			},
		})
	}
}

func (gca *GroupChatActor) run() {
	defer gca.Wg.Done()
	for {
		select {
		case msg := <-gca.Inbox:
			switch msg.Message.Type {
			case "chat":
				chatMsg, ok := msg.Message.Payload.(map[string]interface{})
				if !ok {
					log.Printf("GroupChatActor %s received mal-formed chat message payload.", gca.ID)
					continue
				}
				senderID := msg.FromActorID
				content := chatMsg["content"].(string)

				// message to be broadcast
				broadcastMsg := Message{
					Type: "chat",
					Payload: ChatMessage{
						SenderID: senderID,
						Content:  content,
						GroupID:  gca.ID,
					},
					Timestamp: time.Now(),
				}
				gca.broadcast(broadcastMsg)
			default:
				log.Printf("GroupChatActor %s received unknown message type: %s", gca.ID, msg.Message.Type)

			}
		case <-gca.StopChan:
			return

		}
	}
}

func (gca *GroupChatActor) broadcast(msg Message) {
	gca.mu.RLock()
	defer gca.mu.RUnlock()
	for _, member := range gca.Members {
		// here i will send message to all the participants of this group
		// from actor id will be this group actor's ID
		member.Send(ServerToUserMessage{
			FromActorID: gca.ID,
			Message:     msg,
		})
	}
}
