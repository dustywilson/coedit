package main

import (
	"fmt"
	"net/http"
	"time"
)

func init() {
	http.Handle("/coedit", newCoedit())
}

type coeditHandler struct {
	globalMessage chan []byte
	clients       map[chan []byte]bool
	newClient     chan chan []byte
	lostClient    chan chan []byte
}

func newCoedit() *coeditHandler {
	c := new(coeditHandler)
	c.clients = make(map[chan []byte]bool)
	c.globalMessage = make(chan []byte)
	c.newClient = make(chan chan []byte)
	c.lostClient = make(chan chan []byte)
	go c.handleClients()
	return c
}

func (c *coeditHandler) handleClients() {
	ticker := time.Tick(time.Second * 2)
	for {
		select {
		case cl := <-c.newClient:
			c.clients[cl] = true
			go func() {
				c.globalMessage <- []byte(fmt.Sprintf("A new client has connected.  Now at %d.", len(c.clients)))
			}()
		case cl := <-c.lostClient:
			// should maybe clean something up?
			delete(c.clients, cl)
			go func() {
				c.globalMessage <- []byte(fmt.Sprintf("A client has disconnected.  Now at %d.", len(c.clients)))
			}()
		case m := <-c.globalMessage:
			for cl := range c.clients {
				cl <- m
			}
		case <-ticker:
			go func() {
				c.globalMessage <- []byte("Tick")
			}()
		}
	}
}

func (c *coeditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Not supported", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan []byte)
	defer func() {
		c.lostClient <- messageChan
	}()
	c.newClient <- messageChan

	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		c.lostClient <- messageChan
	}()

	w.Header().Set("Content-type", "text/event-stream")
	w.Header().Set("Cache-control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-control-allow-origin", "*")

	for {
		m := <-messageChan
		fmt.Fprintf(w, "data: %s\n\n", m)
		f.Flush()
	}
}
