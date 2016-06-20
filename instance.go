package coedit

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type newClient struct {
	messageChan chan []byte
	name        string
}

type coeditInstance struct {
	instanceMessage chan []byte
	clients         map[chan []byte]string
	newClient       chan newClient
	lostClient      chan chan []byte
	data            map[string]string
	focus           map[chan []byte]string
}

func newCoeditInstance() *coeditInstance {
	c := new(coeditInstance)
	c.clients = make(map[chan []byte]string)
	c.instanceMessage = make(chan []byte)
	c.newClient = make(chan newClient)
	c.lostClient = make(chan chan []byte)
	c.data = make(map[string]string)
	c.focus = make(map[chan []byte]string)
	go c.handleClients()
	return c
}

func (c *coeditInstance) handleClients() {
	ticker := time.Tick(time.Second * 5)
	for {
		select {
		case nc := <-c.newClient:
			cl := nc.messageChan
			name := nc.name
			c.clients[cl] = name
			go func() {
				c.instanceMessage <- []byte(fmt.Sprintf("A new client [%s] has connected.  Now at %d.", name, len(c.clients)))
			}()
		case cl := <-c.lostClient:
			// should maybe clean something up?
			name := c.clients[cl]
			delete(c.clients, cl)
			go func() {
				c.instanceMessage <- []byte(fmt.Sprintf("A client [%s] has disconnected.  Now at %d.", name, len(c.clients)))
			}()
		case m := <-c.instanceMessage:
			for cl := range c.clients {
				cl <- m
			}
		case <-ticker:
			go func() {
				c.instanceMessage <- []byte("Tick")
			}()
		}
	}
}

func (c *coeditInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	name := r.URL.Query().Get("n")
	log.Printf("ServeHTTP for [%s] name [%s]\n", id, name)

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Not supported", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan []byte)
	defer func() {
		c.lostClient <- messageChan
	}()
	c.newClient <- newClient{
		messageChan: messageChan,
		name:        name,
	}

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
