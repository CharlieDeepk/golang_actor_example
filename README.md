How it Works (Actor-wise)
Client Connects: A client connects to /ws with a Phone-Number from header.
main creates UserActor: The http.HandleFunc receives the WebSocket upgrade request. After upgrading, it instantiates a UserActor for that specific client, passing it the WebSocket connection and a reference to the ServerActor.
UserActor Starts and Registers: The UserActor starts its start() goroutine. The first thing it does is send a MessageTypeRegisterUser message to the ServerActor's inbox, including its userID and a reference to its own inbox channel (UserRef).
ServerActor Registers User: The ServerActor receives the MessageTypeRegisterUser message and stores the userID -> UserActor.inbox mapping in its connectedUsers map.
UserActor readPump: Simultaneously, the UserActor's readPump goroutine continuously reads messages from the WebSocket connection (from the client).
Client Sends Chat Message: When a client sends a chat message (e.g., {"type": "chat_message", "recipient": "bob", "text": "Hi Bob!"}), the UserActor's readPump receives it.
UserActor Forwards to ServerActor: The UserActor then transforms this client message into an internal ServerMessage of type MessageTypeUserChat, setting UserID to the recipient from the client message and including the original sender and text in the Payload. It sends this ServerMessage to the ServerActor's inbox.
ServerActor Routes Message: The ServerActor receives the MessageTypeUserChat message. It looks up the recipient's UserActor.inbox in its connectedUsers map. If found, it creates a UserMessage (of type MessageTypeUserChat) and sends it to the target UserActor's inbox.
Target UserActor Receives and Writes: The target UserActor receives the UserMessage in its inbox. It then uses its writeChan to send a ServerOutgoingMessage (e.g., {"type": "incoming_message", "sender": "alice", "text": "Hi Bob!"}) to its writePump goroutine.
UserActor writePump Sends to Client: The writePump receives the ServerOutgoingMessage and writes it as a JSON WebSocket frame to the connected client.
Client Disconnects: If a client disconnects, the readPump will encounter an error. It then sends a signal to the UserActor's main loop via the quit channel.
UserActor Unregisters and Cleans Up: The UserActor receives the quit signal, sends a MessageTypeUnregisterUser message to the ServerActor, closes its own channels, and closes the WebSocket connection.
ServerActor Unregisters User: The ServerActor removes the userID from its connectedUsers map.
Advantages of this Actor-like Approach:
Concurrency: Each UserActor runs in its own goroutine, allowing for highly concurrent message processing.
Safety: No shared mutable state between goroutines; all communication is explicit via channels, eliminating race conditions.
Scalability: You can easily scale by adding more UserActors. The ServerActor acts as a central router but doesn't do heavy processing.
Maintainability: The code is modular, with clear responsibilities for each actor type.
Testability: Individual actors can be tested in isolation by sending messages to their inboxes and observing outputs.
Graceful Shutdown: The quit channels allow for controlled termination of actors.
Further Enhancements:
Authentication & Authorization: Integrate a proper authentication system (e.g., JWT) to get the userID from a token, not a query parameter.
Error Handling: More robust error handling for network issues, JSON parsing, etc., with appropriate error messages to clients.
Persistence: Store chat history in a database. This would likely involve another actor (e.g., PersistenceActor) that the ServerActor or UserActor sends messages to.
Presence (Online/Offline Status): The ServerActor's connectedUsers map already gives you basic presence. You could extend this to notify friends when a user comes online/offline.
Group Chats: Introduce a GroupActor that manages a specific group, allowing multiple UserActors to join and send messages to it.
Message Buffering/Offline Messages: If a target user is offline, the ServerActor could buffer messages or send them to a PersistenceActor for later delivery.
Heartbeats/Timeouts: Implement more robust ping/pong mechanisms and connection timeouts to detect disconnected clients quickly.
Load Balancing: For very large scale, you'd run multiple instances of this server behind a load balancer. The ServerActor's connectedUsers map would then need to be distributed or externalized (e.g., Redis).
Logging & Monitoring: Comprehensive logging and metrics for performance monitoring.
This actor-based design provides a solid, scalable, and idiomatic Go foundation for building a real-time chat server.
