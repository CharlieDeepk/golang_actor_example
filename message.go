package main

import (
	"encoding/json"
	"time"
)

// genral msg
type Message struct {
	Type      string      `json:"type"` // like "chat", "group_action", "error"
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// chat msg
type ChatMessage struct {
	SenderID string `json:"sender_id"`
	Content  string `json:"content"`
	GroupID  string `json:"group_id,omitempty"` // opt for group chat
}

// group action msg
type GroupActionType string

const (
	CreateGroup GroupActionType = "create_group"
	JoinGroup   GroupActionType = "join_group"
	LeaveGroup  GroupActionType = "leave_group"
)

type GroupAction struct {
	Action    GroupActionType `json:"action"`
	UserID    string          `json:"user_id"`
	GroupID   string          `json:"group_id"`
	GroupName string          `json:"group_name,omitempty"` // For create group
}

// Message sent to UserActor from ServerActor
type ServerToUserMessage struct {
	FromActorID string // ID of the actor sending this (example: Server, GroupChatActor)
	Message     Message
}

// Message sent to ServerActor from UserActor
type UserToServerMessage struct {
	FromUserID string // ID of the UserActor
	Message    Message
}

// Message sent to server from GroupActor
type GroupChatToServerMessage struct {
	FromGroupID string // ID of the GroupActor
	Message     Message
}

// Message sent to GroupActor from ServerActor
// later modify ServerToGroupMessage: ServerToGroupChatMessage
type ServerToGroupMessage struct {
	FromActorID string // UserID of the sender to grp
	Message     Message
}

// Helper to safely get string from json.RawMessage (assuming it's a string)
func GetStringFromRawMessage(r json.RawMessage, key string) string {
	var m map[string]string
	json.Unmarshal(r, &m)
	return m[key]
}

// Example message from client to server
// {
//     "type": "chat_message",
//     "recipient": "recipient_username",
//     "text": "Hello there!"
// }

// // Example message from server to client
// {
//     "type": "incoming_message",
//     "sender": "sender_username",
//     "text": "Hello there!"
// }

// {
//     "type": "user_status",
//     "username": "some_user",
//     "status": "online" // or "offline"
// }
