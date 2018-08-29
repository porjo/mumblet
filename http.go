package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
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
	peer   *WebRTCPeer
	conn   WSConn
	mumble *MumbleClient

	errChan  chan error
	infoChan chan string
}

func NewWSHandler() *wsHandler {
	h := &wsHandler{}
	h.errChan = make(chan error)
	h.infoChan = make(chan string)
	return h
}

func (h *wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	gconn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("ws client connected %s\n", gconn.RemoteAddr())

	// wrap Gorilla conn with our conn so we can extend functionality
	h.conn = WSConn{gconn}
	defer h.conn.Close()

	// TODO handle errors better
	go func() {
		for {
			select {
			case err := <-h.errChan:
				log.Printf("errChan %s\n", err)
				j, err := json.Marshal(err.Error())
				if err != nil {
					log.Printf("marshal err %s\n", err)
				}
				m := Msg{Key: "error", Value: j}
				err = h.conn.writeMsg(m)
				if err != nil {
					log.Printf("writemsg err %s\n", err)
				}
			case info := <-h.infoChan:
				log.Printf("infoChan %s\n", info)
				j, err := json.Marshal(info)
				if err != nil {
					log.Printf("marshal err %s\n", err)
				}
				m := Msg{Key: "info", Value: j}
				err = h.conn.writeMsg(m)
				if err != nil {
					log.Printf("writemsg err %s\n", err)
				}
			}
		}
	}()

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
			continue
		}

		log.Printf("WS: read message %s\n", string(raw))

		if msgType == websocket.TextMessage {
			var msg Msg
			err = json.Unmarshal(raw, &msg)
			if err != nil {
				h.errChan <- err
				continue
			}

			if msg.Key == "connect" {
				conn := CmdConnect{}
				err = json.Unmarshal(msg.Value, &conn)
				if err != nil {
					h.errChan <- err
					continue
				}
				err := h.connectHandler(conn)
				if err != nil {
					log.Printf("connectHandler error: %s\n", err)
					h.errChan <- err
					continue
				}
			}

		} else {
			log.Printf("unknown message type - close websocket\n")
			continue
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

	h.mumble = &MumbleClient{}
	h.mumble.logChan = h.infoChan
	if conn.Username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	h.mumble.Username = conn.Username
	if conn.Hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	h.mumble.Hostname = conn.Hostname
	if conn.Port == 0 {
		conn.Port = MumbleDefaultPort
	}
	h.mumble.Port = conn.Port
	if conn.Channel == "" {
		return fmt.Errorf("channel cannot be empty")
	}
	h.mumble.Channel = conn.Channel

	offer := conn.SessionDescription
	h.peer, err = NewPC(offer, h.rtcStateChangeHandler)
	if err != nil {
		return err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := h.peer.pc.CreateAnswer(nil)
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

	for {
		select {
		case s := <-h.mumble.opusChan:
			fmt.Printf("opusChan s %d\n", len(s.Data))
			h.peer.track.Samples <- s
		}
	}

	return nil
}

// WebRTC callback function
func (h *wsHandler) rtcStateChangeHandler(connectionState ice.ConnectionState) {

	var err error

	switch connectionState {
	case ice.ConnectionStateConnected:
		log.Printf("ice connected, config %s\n", h.peer.pc.RemoteDescription().Sdp)

		if h.mumble.IsConnected() {
			h.errChan <- fmt.Errorf("mumble client already connected")
			return
		}

		err = h.mumble.Connect()
		if err != nil {
			h.errChan <- err
			return
		}
	case ice.ConnectionStateDisconnected:
		log.Printf("ice disconnected\n")
		err = h.mumble.Disconnect()
		if err != nil {
			h.errChan <- err
			return
		}
	}
}
