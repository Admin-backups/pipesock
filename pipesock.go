package main

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"time"
)

type Hub struct {
	Connections map[*Socket]bool
	Pipe        chan string
}

type Message struct {
	Time    time.Time
	Message string
}

type Broadcast struct {
	Time     time.Time
	Messages []*Message
}

func readLoop() {
	go (func() {
		r := bufio.NewReader(os.Stdin)
		for {
			str, err := r.ReadString('\n')

			if err != nil {
				log.Println("Read Line Error:", err)
				continue
			}

			if len(str) == 0 {
				continue
			}

			if passThrough {
				fmt.Println(str)
			}

			//Broadcast
			h.Pipe <- str
		}
	})()
}

func (h *Hub) BroadcastLoop() {
	var currentMessages []*Message
	for {
		select {

		// Pipe in
		case str := <-h.Pipe:
			newMessage := &Message{time.Now(), str}
			currentMessages = append(currentMessages, newMessage)

			//Broadcast
		case <-time.After(time.Duration(delayMillis) * time.Millisecond):
			if len(currentMessages) > 0 {
				broadcast := &Broadcast{time.Now(), currentMessages}
				broadcastJSON, err := json.Marshal(broadcast)
				if err != nil {
					log.Println("Buffer JSON Error: ", err)
					return
				}
				for s, _ := range h.Connections {
					err := websocket.Message.Send(s.Ws, string(broadcastJSON))
					if err != nil {
						log.Println(err)
						s.Ws.Close()
						delete(h.Connections, s)
					}
				}
				// Push onto buffer, or grow if not yet at max
				if len(broadcastBuffer) == bufferSize {
					for i := 1; i < bufferSize; i++ {
						broadcastBuffer[i-1] = broadcastBuffer[i]
					}
					broadcastBuffer[bufferSize] = broadcast
				} else {
					broadcastBuffer = append(broadcastBuffer, broadcast)
				}
				currentMessages = currentMessages[:0]
			}
		}
	}
}

func (s *Socket) ReceiveMessage() {
	if len(broadcastBuffer) > 0 {
		broadcastBufferJSON, err := json.Marshal(broadcastBuffer)
		if err != nil {
			log.Println("Buffer JSON Error: ", err)
			return
		}

		websocket.Message.Send(s.Ws, string(broadcastBufferJSON))
	}

	for {
		var x []byte
		err := websocket.Message.Receive(s.Ws, &x)
		if err != nil {
			break
		}
	}
	s.Ws.Close()
}

type Socket struct {
	Ws *websocket.Conn
}

func IndexServer(w http.ResponseWriter, req *http.Request) {
	var filePath string
	if req.URL.Path == "/" {
		filePath = fmt.Sprintf("%s/.pipesock/%s/index.html", homePath, viewPath)
	} else {
		filePath = fmt.Sprintf("%s/.pipesock/%s%s", homePath, viewPath, req.URL.Path)
	}
	log.Println(filePath)
	http.ServeFile(w, req, filePath)
}

func wsServer(ws *websocket.Conn) {
	s := &Socket{ws}
	h.Connections[s] = true
	s.ReceiveMessage()
}

var (
	h                             Hub
	homePath, viewPath            string
	port, bufferSize, delayMillis int
	passThrough                   bool
	broadcastBuffer               []*Broadcast
)

func init() {
	flag.IntVar(&port, "port", 9193, "Port for the pipesock to sit on.")
	flag.IntVar(&port, "p", 9193, "Port for the pipesock to sit on (shorthand).")

	flag.BoolVar(&passThrough, "through", false, "Pass output to STDOUT.")
	flag.BoolVar(&passThrough, "t", false, "Pass output to STDOUT (shothand).")

	flag.StringVar(&viewPath, "view", "default", "Directory in ~/.pipesock to use as view.")
	flag.StringVar(&viewPath, "v", "default", "Directory in ~/.pipesock to use as view. (shorthand).")

	flag.IntVar(&bufferSize, "num", 20, "Number of previous broadcasts to keep in memory.")
	flag.IntVar(&bufferSize, "n", 20, "Number of previous broadcasts to keep in memory (shorthand).")

	flag.IntVar(&delayMillis, "delay", 2000, "Delay between broadcasts of bundled events in ms.")
	flag.IntVar(&delayMillis, "d", 2000, "Delay between broadcasts of bundled events in ms (shorthand).")

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	homePath = usr.HomeDir

	// Set up hub
	h.Connections = make(map[*Socket]bool)
	h.Pipe = make(chan string, 1)
}

func main() {
	flag.Parse()

	go h.BroadcastLoop()
	go readLoop()

	http.Handle("/ws", websocket.Handler(wsServer))
	http.HandleFunc("/", IndexServer)

	portString := fmt.Sprintf(":%d", port)
	err := http.ListenAndServe(portString, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
