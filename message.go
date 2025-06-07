package main

import "encoding/json"

// Message sent to UserActor
type UserMessage struct {
	Type     string // e.g., "connect", "disconnect", "chat", "send_to_user"
	SenderID string // ID of the sender (user, or system)
	TargetID string // ID of the target user (for one-to-one)
	// Payload  interface{} // The actual data (e.g., chat text, connection info)
	Payload json.RawMessage // Or interface{} for structured data
	Resp    chan error      // Optional: for synchronous acknowledgements
}

// Message sent to ServerActor
type ServerMessage struct {
	Type    string           // e.g., "register_user", "unregister_user", "user_online", "user_offline"
	UserID  string           // ID of the user involved
	UserRef chan UserMessage // Reference to the user's inbox channel
	// Payload interface{}      // Optional: additional data
	Payload json.RawMessage // Or interface{} for structured data
	Resp    chan error      // Optional: for synchronous acknowledgements
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
