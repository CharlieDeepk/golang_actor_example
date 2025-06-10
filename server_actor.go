package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket" // For WebSocket communication
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// Internal Message Types
const (
	MessageTypeConnect        = "connect"
	MessageTypeDisconnect     = "disconnect"
	MessageTypeUserChat       = "chat"            // Message from one user to another
	MessageTypeRegisterUser   = "register_user"   // Server registers a user's inbox
	MessageTypeUnregisterUser = "unregister_user" // Server unregisters a user
)

type ServerActor struct {
	UserActors      map[string]*UserActor
	GroupChatActors map[string]*GroupChatActor
	Inbox           chan UserToServerMessage // from UserActor
	StopChan        chan struct{}
	Wg              sync.WaitGroup
	mu              sync.RWMutex // For protecting UserActors and GroupChatActors map
	nextUserID      int64
	nextGroupID     int64
}

// NewServerActor creates and starts the main server actor.
func NewServerActor() *ServerActor {
	s := &ServerActor{
		UserActors:      make(map[string]*UserActor),
		GroupChatActors: make(map[string]*GroupChatActor),
		Inbox:           make(chan UserToServerMessage, 100),

		StopChan:    make(chan struct{}),
		nextUserID:  1,
		nextGroupID: 1,
	}
	// go s.start()
	return s
}

// Start the ServerActor .
func (sa *ServerActor) start() {
	sa.Wg.Add(1)
	go sa.run()
	log.Println("ServerActor started...")

}

// Stop sends a quit signal to the ServerActor.
func (sa *ServerActor) Stop() {
	log.Println("Stopping ServerActor...")
	close(sa.StopChan)
	sa.Wg.Wait()
	// stops all the active UserActor and GroupChatActor
	sa.mu.RLock()
	for _, ua := range sa.UserActors {
		ua.Stop()
	}
	for _, gca := range sa.GroupChatActors {
		gca.Stop()
	}
	sa.mu.RUnlock()
	log.Println("ServerActor Stopped...")
}

func (sa *ServerActor) run() {
	defer sa.Wg.Done()
	for {
		select {
		case msg := <-sa.Inbox:
			sa.handleServerMessage(msg)
		case <-sa.StopChan:
			return
		}
	}
}

// handle UserToServerMessage processes messages received by the ServerActor.
func (s *ServerActor) handleServerMessage(msg UserToServerMessage) {
	// s.mu.Lock() // Protect map access
	// defer s.mu.Unlock()

	switch msg.Message.Type {
	case "chat":
		s.handleChatMessage(msg.FromUserID, msg.Message)
	case "group_action":
		s.handleGroupAction(msg.FromUserID, msg.Message)
	case "disconnect":
		s.handleUserDisconnect(msg.FromUserID)
	default:
		log.Printf("ServerActor received unknown message type from user %s: %s", msg.FromUserID, msg.Message.Type)
	}
}

// SendMessage allows other components to send messages to the ServerActor.
func (sa *ServerActor) handleChatMessage(senderID string, chatMsg Message) {
	payload, ok := chatMsg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("ServerActor: Malformed chat message payload from %s", senderID)
		sa.sendErrorToUser(senderID, "Invalid chat message format.")
		return
	}
	content, contentOk := payload["content"].(string)
	recipientID, recipientIDOk := payload["recipient"].(string)
	groupID, groupIDOk := payload["group_id"].(string)
	if !contentOk || content == "" {
		sa.sendErrorToUser(senderID, "Chat message content is empty.")
		return
	}
	if groupIDOk && groupID != "" {
		// it is a group chat message
		sa.mu.RLock()
		group, exists := sa.GroupChatActors[groupID]
		sa.mu.RUnlock()
		if !exists {
			sa.sendErrorToUser(senderID, fmt.Sprintf("Group '%s' does not exist.", groupID))
			return
		}
		// now ill forward this message to GroupChatActor
		group.Send(ServerToGroupMessage{
			FromActorID: senderID,
			Message: Message{
				Type: "chat",
				Payload: map[string]interface{}{
					"sender_id": senderID,
					"content":   content,
					"group_id":  groupID,
				},
				Timestamp: time.Now(),
			},
		})
		log.Printf("Server: Forwarded chat from %s to group %s", senderID, groupID)
	} else {
		// Direct message (for now, just log or send back to sender for echo)
		log.Printf("Server: Received direct chat from %s: %s", senderID, content)
		if !recipientIDOk || recipientID == "" {
			sa.sendErrorToUser(senderID, "Chat message recipientID is empty.")
			return
		}
		sa.mu.RLock()
		recipientActor, recipientActorExists := sa.UserActors[recipientID]
		sa.mu.RUnlock()
		if !recipientActorExists {
			sa.sendErrorToUser(senderID, fmt.Sprintf("Recipient '%s' does not exist.", recipientID))
			return
		}

		recipientActor.Send(ServerToUserMessage{
			FromActorID: senderID,
			Message: Message{
				Type: "chat",
				Payload: map[string]interface{}{
					"sender_id": senderID,
					"content":   content,
				},
				Timestamp: time.Now(),
			},
		})
		log.Printf("Server: Forwarded chat from user: %s to user: %s", senderID, recipientID)
		//// Example: echo back to sender
		// sa.sendToUser(senderID, Message{
		// 	Type: "chat",
		// 	Payload: ChatMessage{
		// 		SenderID: "server",
		// 		Content:  fmt.Sprintf("You said: %s", content),
		// 	},
		// 	Timestamp: time.Now(),
		// })
	}
}

func (sa *ServerActor) handleGroupAction(userID string, groupActionMsg Message) {
	payload, ok := groupActionMsg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("ServerActor: Malformed group action payload from %s", userID)
		sa.sendErrorToUser(userID, "Invalid group action format.")
		return

	}
	actionStr, actionOk := payload["action"].(string)
	groupID, groupIDOk := payload["group_id"].(string)
	groupName, groupNameOk := payload["group_name"].(string)

	if !actionOk {
		sa.sendErrorToUser(userID, "Missing group action type.")
		return
	}
	action := GroupActionType(actionStr)

	switch action {
	case CreateGroup:
		if !groupNameOk || groupName == "" {
			sa.sendErrorToUser(userID, "Group name is required to create a group.")
			return
		}
		sa.createGroup(userID, groupName)
	case JoinGroup:
		if !groupIDOk || groupID == "" {
			sa.sendErrorToUser(userID, "Group ID is required to join a group.")
			return
		}
		sa.joinGroup(userID, groupID)
	case LeaveGroup:
		if !groupIDOk || groupID == "" {
			sa.sendErrorToUser(userID, "Group ID is required to leave a group.")
			return
		}
		sa.leaveGroup(userID, groupID)
	default:
		sa.sendErrorToUser(userID, fmt.Sprintf("Unknown group action: %s", action))
	}

}

func (sa *ServerActor) createGroup(creatorID, groupName string) {

	sa.mu.Lock()
	defer sa.mu.Unlock()

	//check if group name already exists./// later we will not do on the basis of name
	for _, group := range sa.GroupChatActors {
		if group.Name == groupName {
			// group already exists
			sa.sendErrorToUser(creatorID, fmt.Sprintf("Group with name '%s' already exists.", groupName))
			return
		}
	}
	newGroupID := fmt.Sprintf("group-%d", sa.nextGroupID)
	sa.nextGroupID++

	groupActor := NewGroupChatActor(newGroupID, groupName, sa)
	sa.GroupChatActors[newGroupID] = groupActor
	groupActor.Start()

	//group participant is blank, so adding its creator by default
	if userActor, exists := sa.UserActors[creatorID]; exists {
		groupActor.AddMember(userActor)
		sa.sendSuccessToUser(creatorID, fmt.Sprintf("Group '%s' created with ID '%s'. You have joined.", groupName, newGroupID))
	} else {
		log.Printf("Error: Creator user %s not found when creating group %s.", creatorID, groupName)
	}
	log.Printf("Server: Group '%s' (ID: %s) created by %s.", groupName, newGroupID, creatorID)
}

func (sa *ServerActor) joinGroup(userID, groupID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	userActor, exists := sa.UserActors[userID]
	if !exists {
		sa.sendErrorToUser(userID, "You are not a registered user.")
		return
	}
	groupActor, groupExists := sa.GroupChatActors[groupID]
	if !groupExists {
		sa.sendErrorToUser(userID, fmt.Sprintf("Group '%s' does not exist.", groupID))
		return
	}
	// if the user is already a member of this group
	if _, isMember := groupActor.Members[userID]; isMember {
		sa.sendErrorToUser(userID, fmt.Sprintf("You are already a member of group '%s'.", groupActor.Name))
		return
	}
	groupActor.AddMember(userActor)
	sa.sendSuccessToUser(userID, fmt.Sprintf("Successfully joined group '%s'.", groupActor.Name))
	log.Printf("Server: User %s joined group %s.", userID, groupActor.Name)
}

func (sa *ServerActor) leaveGroup(userID, groupID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	_, userExists := sa.UserActors[userID]
	if !userExists {
		sa.sendErrorToUser(userID, "You are not a registered user.")
		return
	}
	groupActor, groupExists := sa.GroupChatActors[groupID]
	if !groupExists {
		sa.sendErrorToUser(userID, fmt.Sprintf("Group '%s' does not exist.", groupID))
		return
	}

	// checks if this user is in this group then only we will remove this user
	if _, isMember := groupActor.Members[userID]; !isMember {
		sa.sendErrorToUser(userID, fmt.Sprintf("You are not a member of group '%s'.", groupActor.Name))
		return
	}
	groupActor.RemoveMember(userID)
	sa.sendSuccessToUser(userID, fmt.Sprintf("Successfully left group '%s'.", groupActor.Name))
	log.Printf("Server: User %s left group %s.", userID, groupActor.Name)

	// If the group becomes empty, then stop/delete the GroupChatActor
	if len(groupActor.Members) == 0 {
		log.Printf("Group %s is now empty. Stopping GroupChatActor.", groupActor.Name)
		groupActor.Stop()
		delete(sa.GroupChatActors, groupID)
	}
}

// func to delete their account
func (sa *ServerActor) handleUserDisconnect(userID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if userActor, exists := sa.UserActors[userID]; exists {
		log.Printf("Server: User %s disconnected.", userID)
		userActor.Stop() // Ensure user actor's goroutines are stopped
		delete(sa.UserActors, userID)

		// removing user from all its group participation
		for _, group := range sa.GroupChatActors {
			if _, isMember := group.Members[userID]; isMember {
				group.RemoveMember(userID)
				// If the group becomes empty after this, consider stopping/deleting it
				if len(group.Members) == 0 {
					log.Printf("Group %s is now empty after user %s disconnected. Stopping GroupChatActor.", group.Name, userID)
					group.Stop()
					delete(sa.GroupChatActors, group.ID)
				}
			}
		}
	}
}

// sendToUser sends a message to a specific UserActor's inbox
func (sa *ServerActor) sendToUser(userID string, msg Message) {
	sa.mu.RLock()
	userActor, exists := sa.UserActors[userID]
	sa.mu.RUnlock()

	if !exists {
		log.Printf("Error: User %s not found to send message.", userID)
		return
	}
	userActor.Send(ServerToUserMessage{
		FromActorID: "server",
		Message:     msg,
	})

}

// sendErrorToUser send error to user specifically
func (sa *ServerActor) sendErrorToUser(userID, errorMessage string) {
	sa.sendToUser(userID, Message{
		Type: "error",
		Payload: map[string]string{
			"message": errorMessage,
		},
		Timestamp: time.Now(),
	})
}

// sendSuccessToUser send success to user specifically
func (sa *ServerActor) sendSuccessToUser(userID, successMessage string) {
	sa.sendToUser(userID, Message{
		Type: "success",
		Payload: map[string]string{
			"message": successMessage,
		},
		Timestamp: time.Now(),
	})
}

// websocket handler
func (sa *ServerActor) ServeWS(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// implement get phone number from request header: later
	// userID := fmt.Sprintf("user-%d", sa.nextUserID)
	// sa.nextUserID++
	userID := r.Header.Get("Phone-Number")
	if userID == "" {
		http.Error(w, "Phone-Number is required", http.StatusBadRequest)
		return
	}

	log.Printf("New client connected: %s", userID)
	userActor := NewUserActor(sa.Inbox, conn, userID)

	sa.mu.Lock()
	sa.UserActors[userID] = userActor
	sa.mu.Unlock()

	userActor.start()

	userActor.Send(ServerToUserMessage{
		FromActorID: "server",
		Message: Message{
			Type: "welcome",
			Payload: map[string]string{
				"user_id": userID,
				"message": "Welcome to the chat server!",
			},
			Timestamp: time.Now(),
		},
	})
}
