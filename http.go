package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pions/webrtc"
)

const PingInterval = 30 * time.Second
const WriteWait = 10 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Msg struct {
	Key   string
	Value json.RawMessage
}

type Connect struct {
	Hostname           string
	Port               int
	Username           string
	Channel            string
	SessionDescription string
}

type Conn struct {
	*websocket.Conn
}

type wsHandler struct {
	WebRoot string
	pc      *webrtc.RTCPeerConnection

	conn Conn
}

func (ws *wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	gconn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("Client connected %s\n", gconn.RemoteAddr())

	// wrap Gorilla conn with our conn so we can extend functionality
	ws.conn = Conn{gconn}

	// setup ping/pong to keep connection open
	go func() {
		c := time.Tick(PingInterval)
		for {
			select {
			case <-c:
				// WriteControl can be called concurrently
				if err := ws.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(WriteWait)); err != nil {
					log.Printf("WS: ping client, err %s\n", err)
					return
				}
			}
		}
	}()

	for {
		msgType, raw, err := ws.conn.ReadMessage()
		if err != nil {
			log.Printf("WS: ReadMessage err %s\n", err)
			return
		}

		log.Printf("WS: read message %s\n", string(raw))

		if msgType == websocket.TextMessage {
			var msg Msg
			err = json.Unmarshal(raw, &msg)
			if err != nil {
				log.Printf("json unmarshal error: %s\n", err)
				return
			}

			if msg.Key == "connect" {
				conn := Connect{}
				err = json.Unmarshal(msg.Value, &conn)
				if err != nil {
					log.Println(err)
					return
				}
				err := ws.connectHandler(conn)
				if err != nil {
					log.Printf("connectHandler error: %s\n", err)
					return
				}
			}
		} else {
			log.Printf("unknown message type - close websocket\n")
			ws.conn.Close()
			return
		}
		log.Printf("WS end main loop\n")
	}
}

func (c *Conn) writeMsg(val interface{}) error {
	j, err := json.Marshal(val)
	if err != nil {
		return err
	}
	log.Printf("WS: write message %s\n", string(j))
	if err = c.WriteMessage(websocket.TextMessage, j); err != nil {
		return err
	}

	return nil
}

func (ws *wsHandler) connectHandler(conn Connect) error {
	var err error

	fmt.Printf("connmsg %+v\n", conn)

	var offer []byte
	offer, err = base64.StdEncoding.DecodeString(conn.SessionDescription)
	if err != nil {
		return err
	}
	ws.pc, err = NewPC(string(offer))
	if err != nil {
		return err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := ws.pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// Get the LocalDescription and take it to base64 so we can paste in browser
	answerb64 := base64.StdEncoding.EncodeToString([]byte(answer.Sdp))

	j, err := json.Marshal(answerb64)
	if err != nil {
		return err
	}
	err = ws.conn.writeMsg(Msg{Key: "sd_answer", Value: j})
	if err != nil {
		return err
	}

	return nil
}
