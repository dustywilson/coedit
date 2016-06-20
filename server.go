package coedit

import (
	"net/http"
	"sync"

	"github.com/gorilla/mux"
)

func init() {
	r := mux.NewRouter()
	http.Handle("/coedit/", r)
	r.PathPrefix("/coedit/lib/").Handler(http.StripPrefix("/coedit/lib/", http.FileServer(http.Dir("js"))))
	h := newCoeditHandler()
	r.Path("/coedit/{id}").Handler(h)
	r.Path("/coedit/{id}/{key}").Handler(h)
}

type coeditHandler struct {
	globalMessage chan string
	instancesLock sync.Mutex
	instances     map[string]*coeditInstance
}

func newCoeditHandler() *coeditHandler {
	h := new(coeditHandler)
	h.globalMessage = make(chan string)
	h.instances = make(map[string]*coeditInstance)
	go h.run()
	return h
}

func (h *coeditHandler) run() {
	for {
		select {
		case m := <-h.globalMessage:
			func() {
				h.instancesLock.Lock()
				defer h.instancesLock.Unlock()
				for _, c := range h.instances {
					go func(c *coeditInstance) {
						c.instanceMessage <- m
					}(c)
				}
			}()
		}
	}
}

func (h *coeditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var c *coeditInstance

	func() {
		h.instancesLock.Lock()
		defer h.instancesLock.Unlock()

		var ok bool
		c, ok = h.instances[id]
		if !ok {
			c = newCoeditInstance()
			h.instances[id] = c
		}
	}()

	c.ServeHTTP(w, r)
}
