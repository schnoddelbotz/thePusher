package main

// A reduced copy of the websocket-chat example from
// https://github.com/golang-samples/websocket

import (
	"golang.org/x/net/websocket"
	"log"
	"net/http"
)

// Chat server.
type Server struct {
	pattern   string
	clients   map[int]*WSClient
	addCh     chan *WSClient
	delCh     chan *WSClient
	sendAllCh chan *ClientInfo
	doneCh    chan bool
	errCh     chan error
}

// Create new chat server.
func NewWebSocketServer(pattern string) *Server {
	clients := make(map[int]*WSClient)
	addCh := make(chan *WSClient)
	delCh := make(chan *WSClient)
	sendAllCh := make(chan *ClientInfo)
	doneCh := make(chan bool)
	errCh := make(chan error)

	return &Server{
		pattern,
		clients,
		addCh,
		delCh,
		sendAllCh,
		doneCh,
		errCh,
	}
}

func (s *Server) Add(c *WSClient) {
	s.addCh <- c
}

func (s *Server) Del(c *WSClient) {
	s.delCh <- c
}

func (s *Server) SendAll(msg *ClientInfo) {
	s.sendAllCh <- msg
}

func (s *Server) Done() {
	s.doneCh <- true
}

func (s *Server) Err(err error) {
	s.errCh <- err
}

func (s *Server) sendAll(msg *ClientInfo) {
	for _, c := range s.clients {
		c.Write(msg)
	}
}

// Listen and serve.
// It serves client connection and broadcast request.
func (s *Server) Listen() {
	//websocket handler
	onConnected := func(ws *websocket.Conn) {
		defer func() {
			err := ws.Close()
			if err != nil {
				s.errCh <- err
			}
		}()

		client := NewWSClient(ws, s)
		s.Add(client)
		client.Listen()
	}
	http.Handle(s.pattern, websocket.Handler(onConnected))

	for {
		select {

		// Add new a client
		case c := <-s.addCh:
			log.Println("Added new WS client")
			s.clients[c.id] = c
			log.Println("Now", len(s.clients), "clients connected.")
			//s.sendPastMessages(c)

		// del a client
		case c := <-s.delCh:
			log.Println("Delete WS client")
			delete(s.clients, c.id)

		// broadcast message for all clients
		case msg := <-s.sendAllCh:
			//log.Println("Send WS all:", msg)
			//s.messages = append(s.messages, msg)
			s.sendAll(msg)

		case err := <-s.errCh:
			log.Println("WS Error:", err.Error())

		case <-s.doneCh:
			return
		}
	}
}
