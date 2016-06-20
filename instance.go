package coedit

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
)

type client struct {
	messageChan chan string
	name        string
	key         string
}

type keyValue struct {
	value     string
	actor     string
	clientKey string
	datetime  time.Time
}

type coeditInstance struct {
	instanceMessage chan string
	clients         map[chan string]client
	clientKey       map[string]client
	clientsLock     sync.Mutex
	newClient       chan client
	lostClient      chan chan string
	data            map[string]keyValue
	dataLock        sync.Mutex
	focus           map[chan string]string
}

func newCoeditInstance() *coeditInstance {
	c := new(coeditInstance)
	c.clients = make(map[chan string]client)
	c.clientKey = make(map[string]client)
	c.instanceMessage = make(chan string)
	c.newClient = make(chan client)
	c.lostClient = make(chan chan string)
	c.data = make(map[string]keyValue)
	c.focus = make(map[chan string]string)
	go c.handleClients()
	return c
}

func (c *coeditInstance) handleClients() {
	ticker := time.Tick(time.Second * 5)
	for {
		select {
		case nc := <-c.newClient:
			func() {
				c.clientsLock.Lock()
				defer c.clientsLock.Unlock()
				cl := nc.messageChan
				name := nc.name
				nc.key = uuid.NewV4().String()
				c.clients[cl] = nc
				c.clientKey[nc.key] = nc
				go func() {
					nc.messageChan <- fmt.Sprintf(`{"action":"setClientKey", "clientKey":"%s"}`, nc.key)
					c.instanceMessage <- fmt.Sprintf(`{"action":"fyiNewClient", "name":"%s", "newCount":%d}`, name, len(c.clients)) // FIXME: not proper handling of user-supplied data
					c.dataLock.Lock()
					defer c.dataLock.Unlock()
					for key, v := range c.data {
						out := fmt.Sprintf(`{"action":"update", "key":"%s", "actor":"%s", "value":"%s", "datetime":%d}`, key, v.actor, v.value, v.datetime.Unix()) // FIXME: not proper handling of user-supplied data
						nc.messageChan <- out
					}
				}()
			}()
		case cl := <-c.lostClient:
			func() {
				c.clientsLock.Lock()
				defer c.clientsLock.Unlock()
				name := c.clients[cl].name
				key := c.clients[cl].key
				delete(c.clientKey, key)
				delete(c.clients, cl)
				go func() {
					c.instanceMessage <- fmt.Sprintf(`{"action":"fyiLostClient", "name":"%s", "newCount":%d}`, name, len(c.clients)) // FIXME: not proper handling of user-supplied data
				}()
			}()
		case m := <-c.instanceMessage:
			func() {
				c.clientsLock.Lock()
				defer c.clientsLock.Unlock()
				for cl := range c.clients {
					cl <- m
				}
			}()
		case <-ticker:
			go func() {
				c.instanceMessage <- fmt.Sprintf(`{"action":"tick", "time":%d}`, time.Now().Unix())
			}()
		}
	}
}

func (c *coeditInstance) clientByKey(key string) client {
	c.clientsLock.Lock()
	defer c.clientsLock.Unlock()
	return c.clientKey[key]
}

func (c *coeditInstance) varHandler(w http.ResponseWriter, r *http.Request, id string, clientKey string, key string) {
	u := c.clientByKey(clientKey).name
	if u == "" {
		w.WriteHeader(http.StatusBadRequest) // ???: pick correct status
		return
	}
	log.Printf("varHandler for id:[%s] u:[%s] clientKey:[%s] v:[%s]\n", id, u, clientKey, key)
	if r.Method == "DELETE" {
		c.dataLock.Lock()
		defer c.dataLock.Unlock()
		delete(c.data, key)
		out := fmt.Sprintf(`{"action":"delete", "key":"%s", "actor":"%s", "datetime":%d}`, key, u, time.Now().Unix()) // FIXME: not proper handling of user-supplied data
		c.instanceMessage <- out
		fmt.Fprint(w, out)
	} else if r.Method == "POST" {
		c.dataLock.Lock()
		defer c.dataLock.Unlock()
		err := r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest) // ???: pick correct status
			fmt.Fprintf(w, "error: %s\n", err)
			return
		}
		c.data[key] = keyValue{
			value:     r.FormValue("v"),
			actor:     u,
			clientKey: key,
			datetime:  time.Now(),
		}
		out := fmt.Sprintf(`{"action":"update", "key":"%s", "actor":"%s", "value":"%s", "datetime":%d}`, key, c.data[key].actor, c.data[key].value, c.data[key].datetime.Unix()) // FIXME: not proper handling of user-supplied data
		c.instanceMessage <- out
		fmt.Fprint(w, out)
	} else if r.Method == "PUT" {
		c.dataLock.Lock()
		defer c.dataLock.Unlock()
		defer r.Body.Close()
		v, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest) // ???: pick correct status
			fmt.Fprintf(w, "error: %s\n", err)
			return
		}
		c.data[key] = keyValue{
			value:     string(v),
			actor:     u,
			clientKey: key,
			datetime:  time.Now(),
		}
		out := fmt.Sprintf(`{"action":"update", "key":"%s", "actor":"%s", "value":"%s", "datetime":%d}`, key, u, c.data[key].value, c.data[key].datetime.Unix()) // FIXME: not proper handling of user-supplied data
		c.instanceMessage <- out
		fmt.Fprint(w, out)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *coeditInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	clientKey := r.URL.Query().Get("ck")
	key := mux.Vars(r)["key"]
	log.Printf("TEST [%s] [%s]\n", clientKey, key)
	if clientKey != "" && key != "" {
		c.varHandler(w, r, id, clientKey, key)
		return
	}

	name := r.URL.Query().Get("n")
	log.Printf("ServeHTTP for [%s] name [%s]\n", id, name)

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Not supported", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan string)
	defer func() {
		c.lostClient <- messageChan
	}()
	c.newClient <- client{
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
