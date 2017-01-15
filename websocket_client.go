package main

// A reduced copy of the websocket-chat example from
// https://github.com/golang-samples/websocket

import (
	"fmt"
	"golang.org/x/net/websocket"
	"io"
)

const channelBufSize = 100

var maxId int = 0

// Chat client.
type WSClient struct {
	id     int
	ws     *websocket.Conn
	server *Server
	ch     chan *ClientInfo
	doneCh chan bool
}

// Create new chat client.
func NewWSClient(ws *websocket.Conn, server *Server) *WSClient {

	if ws == nil {
		panic("ws cannot be nil")
	}

	if server == nil {
		panic("server cannot be nil")
	}

	maxId++
	ch := make(chan *ClientInfo, channelBufSize)
	doneCh := make(chan bool)

	return &WSClient{maxId, ws, server, ch, doneCh}
}

func (c *WSClient) Conn() *websocket.Conn {
	return c.ws
}

func (c *WSClient) Write(msg *ClientInfo) {
	select {
	case c.ch <- msg:
	default:
		c.server.Del(c)
		err := fmt.Errorf("client %d is disconnected.", c.id)
		c.server.Err(err)
	}
}

func (c *WSClient) Done() {
	c.doneCh <- true
}

// Listen Write and Read request via chanel
func (c *WSClient) Listen() {
	go c.listenWrite()
	c.listenRead()
}

// Listen write request via chanel
func (c *WSClient) listenWrite() {
	for {
		select {

		// send message to the client
		case msg := <-c.ch:
			websocket.JSON.Send(c.ws, msg)

		// receive done request
		case <-c.doneCh:
			c.server.Del(c)
			c.doneCh <- true // for listenRead method
			return
		}
	}
}

// Listen read request via chanel
func (c *WSClient) listenRead() {
	for {
		select {

		// receive done request
		case <-c.doneCh:
			c.server.Del(c)
			c.doneCh <- true // for listenWrite method
			return

		// read data from websocket connection
		default:
			var msg ClientInfo
			err := websocket.JSON.Receive(c.ws, &msg)
			if err == io.EOF {
				c.doneCh <- true
			}
		}
	}
}
