package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	server := NewServer()

	errChan := make(chan error)
	sigsCa := make(chan os.Signal, 1)
	signal.Notify(sigsCa, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigsCa
		signal.Stop(sigsCa)
		log.Println("Shutting down server...")
		server.saveStats()
		server.listener.Close()
		os.Exit(0)
	}()

	go func() {
		// Shut down server on first error
		if err := <-errChan; err != nil {
			log.Printf("Server shutting down due to error: %v", err)
			server.saveStats()
			server.listener.Close()
			os.Exit(1) // Exit the program
		}
	}()

	go processStdin(server)

	server.Start(errChan)
}

func getMessage(input []string) string {
	var message bytes.Buffer

	for i := 0; i <= len(input)-1; i++ {
		if i < len(input) {
			message.WriteString(input[i] + " ")
		} else {
			message.WriteString(input[i])
		}
	}

	return message.String()
}

func sendDisable(client *Client, message string) {
	sendServerMessage(client, message)
	client.sendPacket(`{"type":"DISABLE_ANCHOR"}`)
	client.disconnect()
}

func sendServerMessage(client *Client, message string) {
	if message == "" {
		message = "You have been disconnected by the server. Try to connect again in a bit!"
	}
	client.sendPacket(`{"type":"SERVER_MESSAGE","message":"` + message + `"}`)
}

func getClientID(clientID string) uint64 {
	converted, err := strconv.ParseUint(clientID, 10, 64)
	if err != nil {
		log.Println("Given text was not a valid clientID.")
		return 0
	}

	return converted
}

func processStdin(s *Server) {
	var reader bufio.Reader = *bufio.NewReader(os.Stdin)
	for {
		input, err := reader.ReadString('\n')

		if err != nil {
			log.Println("Error reading from stdin:", err)
			continue
		}

		// remove new line delimiter
		input = strings.Replace(input, "\n", "", 1)

		// split on space
		splitInput := strings.Split(input, " ")

		switch splitInput[0] {
		case "roomCount":
			s.mu.Lock()
			log.Println("Room count:", len(s.rooms))
			s.mu.Unlock()
		case "clientCount":
			s.mu.Lock()
			log.Println("Client count:", len(s.onlineClients))
			s.mu.Unlock()
		case "quiet":
			s.mu.Lock()
			s.quietMode = !s.quietMode
			log.Println("Quiet mode:", s.quietMode)
			s.mu.Unlock()
		case "stats":
			s.mu.Lock()
			log.Println("Online Count:", strconv.FormatInt(int64(len(s.onlineClients)), 10), "| Games Complete: "+strconv.FormatInt(int64(s.gamesCompleted), 10))
			s.mu.Unlock()
		case "list":
			s.mu.Lock()
			for _, room := range s.rooms {
				room.mu.Lock()
				log.SetFlags(0)
				log.Println("Room", room.id+":")
				for _, client := range room.clients {
					client.mu.Lock()
					log.Println("  Client", fmt.Sprint(client.id)+":", client.state)
					client.mu.Unlock()
				}
				log.SetFlags(log.LstdFlags)
				room.mu.Unlock()
			}
			s.mu.Unlock()
		case "disable":
			targetClientId := getClientID(splitInput[1])
			if targetClientId == 0 {
				continue
			}

			s.mu.Lock()
			client := s.onlineClients[targetClientId]
			s.mu.Unlock()

			if client != nil {
				client.mu.Unlock()
				log.Println("[Server] DISABLE_ANCHOR packet ->", client.id)
				client.mu.Unlock()
				go sendDisable(client, getMessage(splitInput[2:]))
				continue
			}

			log.Println("Client", targetClientId, "not found")
		case "disableAll":
			log.Println("[Server] DISABLE_ANCHOR packet -> All")
			s.mu.Lock()
			for _, client := range s.onlineClients {
				go sendDisable(client, getMessage(splitInput[1:]))
			}
			s.mu.Unlock()
		case "message":
			targetClientId := getClientID(splitInput[1])
			if targetClientId == 0 {
				continue
			}

			s.mu.Lock()
			client := s.onlineClients[targetClientId]
			s.mu.Unlock()

			if client != nil {
				client.mu.Lock()
				log.Println("[Server] SERVER_MESSAGE packet ->", client.id)
				client.mu.Unlock()
				go sendServerMessage(client, getMessage(splitInput[2:]))
				continue
			}

			log.Println("Client", targetClientId, "not found")
		case "messageAll":
			log.Println("[Server] SERVER_MESSAGE packet -> All")
			s.mu.Lock()
			for _, client := range s.onlineClients {
				go sendServerMessage(client, getMessage(splitInput[1:]))
			}
			s.mu.Unlock()
		case "deleteRoom":
			s.mu.Lock()
			targetRoomID := splitInput[1]

			room := s.rooms[targetRoomID]

			if room != nil {
				room.mu.Lock()
				for _, client := range s.onlineClients {
					client.mu.Lock()
					if client.room.id == targetRoomID {
						go sendDisable(client, "Deleting your room. Goodbye!")
					}
					client.mu.Unlock()
				}
				room.mu.Unlock()
				delete(s.rooms, targetRoomID)
			} else {
				log.Println("Client", targetRoomID, "not found")
			}

			s.mu.Unlock()
		case "stop":
			s.mu.Lock()
			for _, client := range s.onlineClients {
				go sendServerMessage(client, "Server restarting. Check back in a bit!")
			}
			s.mu.Unlock()

			s.saveStats()
			s.listener.Close()

			os.Exit(0)
		default:
			log.Printf("Available commands:\nhelp: Show this help message\nstats: Print server stats\nquiet: Toggle quiet mode\nroomCount: Show the number of rooms\nclientCount: Show the number of clients\nlist: List all rooms and clients\nstop <message>: Stop the server\nmessage <clientId> <message>: Send a message to a client\nmessageAll <message>: Send a message to all clients\ndisable <clientId> <message>: Disable anchor on a client\ndisableAll <message>: Disable anchor on all clients\ndeleteRoom <roomID>: Disables anchor on all online clients in the room and deletes it\n")
		}
	}
}
