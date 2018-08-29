package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pions/webrtc"
	"github.com/pions/webrtc/pkg/ice"
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

type CmdConnect struct {
	Hostname           string
	Port               int
	Username           string
	Channel            string
	SessionDescription string
}

type WSConn struct {
	*websocket.Conn
}

type wsHandler struct {
	pc   *webrtc.RTCPeerConnection
	conn WSConn
}

func (h *wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	gconn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("Client connected %s\n", gconn.RemoteAddr())

	// wrap Gorilla conn with our conn so we can extend functionality
	h.conn = WSConn{gconn}

	// setup ping/pong to keep connection open
	go func() {
		c := time.Tick(PingInterval)
		for {
			select {
			case <-c:
				// WriteControl can be called concurrently
				if err := h.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(WriteWait)); err != nil {
					log.Printf("WS: ping client, err %s\n", err)
					return
				}
			}
		}
	}()

	for {
		msgType, raw, err := h.conn.ReadMessage()
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
				conn := CmdConnect{}
				err = json.Unmarshal(msg.Value, &conn)
				if err != nil {
					log.Println(err)
					return
				}
				err := h.connectHandler(conn)
				if err != nil {
					log.Printf("connectHandler error: %s\n", err)
					return
				}
			}
		} else {
			log.Printf("unknown message type - close websocket\n")
			h.conn.Close()
			return
		}
		log.Printf("WS end main loop\n")
	}
}

func (c *WSConn) writeMsg(val interface{}) error {
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

func (h *wsHandler) connectHandler(conn CmdConnect) error {
	var err error

	offer := conn.SessionDescription
	h.pc, err = NewPC(offer, h.rtcStateChangeHandler)
	if err != nil {
		return err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := h.pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	j, err := json.Marshal(answer.Sdp)
	if err != nil {
		return err
	}
	err = h.conn.writeMsg(Msg{Key: "sd_answer", Value: j})
	if err != nil {
		return err
	}

	return nil
}

func (h *wsHandler) rtcStateChangeHandler(connectionState ice.ConnectionState) {

	switch connectionState {
	case ice.ConnectionStateConnected:
		log.Printf("connected, config %s\n", h.pc.RemoteDescription().Sdp)
	case ice.ConnectionStateDisconnected:
		log.Printf("disconnected\n")
	}
}
