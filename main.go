package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	serverActor := NewServerActor()
	serverActor.start()

	http.HandleFunc("/ws", serverActor.ServeWS)

	// WebSocket handler
	// http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
	// 	// In a real application, you'd perform authentication here
	// 	// and get a reliable UserID from a session or token.
	// 	// For this example, we'll use a query parameter.
	// 	userID := r.Header.Get("Phone-Number")
	// 	if userID == "" {
	// 		http.Error(w, "Phone-Number is required", http.StatusBadRequest)
	// 		return
	// 	}

	// 	conn, err := serverActor.upgrader.Upgrade(w, r, nil)
	// 	if err != nil {
	// 		log.Println(err)
	// 		return
	// 	}
	// 	log.Printf("New WebSocket connection for user: %s\n", userID)

	// 	// Create a new UserActor for this connection
	// 	NewUserActor(serverActor, conn, userID)
	// 	// The UserActor will register itself with the ServerActor

	// 	// Keep the handler alive until the user disconnects
	// 	// (The UserActor's start() method handles the lifecycle)
	// 	// We could add a mechanism to wait for userActor.quit to be closed here
	// 	// but the current implementation for UserActor stops itself cleanly.
	// })

	log.Println("Chat server starting on :8080")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal("HTTP server failed: %v", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	serverActor.Stop()
	log.Println("Server gracefully shut down.")
}
