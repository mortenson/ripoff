package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	listener, err := net.Listen("tcp", ":6767")
	if err != nil {
		log.Fatal("Error listening:", err)
	}
	defer func() {
		err := listener.Close()
		if err != nil {
			log.Println("Error closing listener", err)
		}
	}()
	fmt.Println("READY")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		go handleConnection(conn)
	}
}

type Request struct {
	Type      string   `json:"type"`
	ValueFunc string   `json:"valueFunc"`
	Args      []string `json:"args"`
}

type Response struct {
	Value string `json:"value"`
}

func handleConnection(conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Println("Error closing connection", err)
		}
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		message := scanner.Bytes()
		r := Request{}
		err := json.Unmarshal(message, &r)
		if err != nil {
			log.Println("Error parsing body:", err)
			return
		}
		if r.Type == "exit" {
			os.Exit(0)
			return
		}
		if len(r.Args) == 0 {
			log.Println("No args provided")
			return
		}
		var value string
		switch r.ValueFunc {
		case "sayHello":
			value = fmt.Sprintf("Hello %s", r.Args[0])
		case "sayGoodbye":
			value = fmt.Sprintf("Goodbye %s", r.Args[0])
		default:
			log.Println("Unknown value func:", r.ValueFunc)
			return
		}
		resp, err := json.Marshal(Response{
			Value: value,
		})
		if err != nil {
			log.Println("Could not marshal message:", r)
			return
		}
		_, err = conn.Write(append(resp, '\n'))
		if err != nil {
			log.Println("Could not send message:", r.ValueFunc)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Println("Scanner error:", err)
	}
}
