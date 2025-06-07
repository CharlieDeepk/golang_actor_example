package main

import (
	"log"
	"net/http"
)

func main() {
	serverActor := NewServerActor()

	// WebSocket handler
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// In a real application, you'd perform authentication here
		// and get a reliable UserID from a session or token.
		// For this example, we'll use a query parameter.
		userID := r.Header.Get("Phone-Number")
		if userID == "" {
			http.Error(w, "Phone-Number is required", http.StatusBadRequest)
			return
		}

		conn, err := serverActor.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		log.Printf("New WebSocket connection for user: %s\n", userID)

		// Create a new UserActor for this connection
		NewUserActor(serverActor, conn, userID)
		// The UserActor will register itself with the ServerActor

		// Keep the handler alive until the user disconnects
		// (The UserActor's start() method handles the lifecycle)
		// We could add a mechanism to wait for userActor.quit to be closed here
		// but the current implementation for UserActor stops itself cleanly.
	})

	log.Println("Chat server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
