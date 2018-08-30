/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pions/webrtc/pkg/ice"
)

const PingInterval = 10 * time.Second
const WriteWait = 10 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type wsMsg struct {
	Key   string
	Value json.RawMessage
}

type CmdConnect struct {
	URL                string
	Hostname           string
	Port               int
	Username           string
	Channel            string
	SessionDescription string
}

type Conn struct {
	peer   *WebRTCPeer
	conn   *websocket.Conn
	mumble *MumbleClient

	errChan  chan error
	infoChan chan string
	msgChan  chan MumbleMsg
}

type wsHandler struct{}

func (h *wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	gconn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Conn{}
	c.errChan = make(chan error)
	c.infoChan = make(chan string)
	c.msgChan = make(chan MumbleMsg)
	// wrap Gorilla conn with our conn so we can extend functionality
	c.conn = gconn
	defer c.conn.Close()

	log.Printf("WS %x: client connected, addr %s\n", c.conn.RemoteAddr(), c.conn.RemoteAddr())

	// TODO handle log/errors better
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("log goroutine quitting...\n")
				return
			case err := <-c.errChan:
				log.Printf("errChan %s\n", err)
				j, err := json.Marshal(err.Error())
				if err != nil {
					log.Printf("marshal err %s\n", err)
				}
				m := wsMsg{Key: "error", Value: j}
				err = c.writeMsg(m)
				if err != nil {
					log.Printf("writemsg err %s\n", err)
				}
			case info := <-c.infoChan:
				log.Printf("infoChan %s\n", info)
				j, err := json.Marshal(info)
				if err != nil {
					log.Printf("marshal err %s\n", err)
				}
				m := wsMsg{Key: "info", Value: j}
				err = c.writeMsg(m)
				if err != nil {
					log.Printf("writemsg err %s\n", err)
				}
			case msg := <-c.msgChan:
				log.Printf("msgChan %s\n", msg)
				j, err := json.Marshal(msg)
				if err != nil {
					log.Printf("marshal err %s\n", err)
				}
				m := wsMsg{Key: "msg", Value: j}
				err = c.writeMsg(m)
				if err != nil {
					log.Printf("writemsg err %s\n", err)
				}
			}
		}
	}()

	// setup ping/pong to keep connection open
	go func() {
		pingCh := time.Tick(PingInterval)
		for {
			select {
			case <-ctx.Done():
				log.Printf("ws ping goroutine quitting...\n")
				return
			case <-pingCh:
				// WriteControl can be called concurrently
				err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(WriteWait))
				if err != nil {
					log.Printf("WS %x: ping client, err %s\n", c.conn.RemoteAddr(), err)
					return
				}
			}
		}
	}()

	for {
		msgType, raw, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("WS %x: ReadMessage err %s\n", c.conn.RemoteAddr(), err)
			break
		}

		log.Printf("WS %x: read message %s\n", c.conn.RemoteAddr(), string(raw))

		if msgType == websocket.TextMessage {
			var msg wsMsg
			err = json.Unmarshal(raw, &msg)
			if err != nil {
				c.errChan <- err
				continue
			}

			if msg.Key == "connect" {
				cmd := CmdConnect{}
				err = json.Unmarshal(msg.Value, &cmd)
				if err != nil {
					c.errChan <- err
					continue
				}
				err := c.connectHandler(ctx, cmd)
				if err != nil {
					log.Printf("connectHandler error: %s\n", err)
					c.errChan <- err
					continue
				}
			}

		} else {
			log.Printf("unknown message type - close websocket\n")
			break
		}
	}
	ctxCancel()
	log.Printf("WS %x: end handler\n", c.conn.RemoteAddr())
}

func (c *Conn) writeMsg(val interface{}) error {
	j, err := json.Marshal(val)
	if err != nil {
		return err
	}
	log.Printf("WS %x: write message %s\n", c.conn.RemoteAddr(), string(j))
	if err = c.conn.WriteMessage(websocket.TextMessage, j); err != nil {
		return err
	}

	return nil
}

func (c *Conn) connectHandler(ctx context.Context, cmd CmdConnect) error {
	var err error

	c.mumble = NewMumbleClient(ctx, c.msgChan, c.infoChan)
	if cmd.URL != "" {
		err = c.mumble.ParseURL(cmd.URL)
		if err != nil {
			return fmt.Errorf("cannot parse URL: %s", err)
		}
	} else {
		if cmd.Hostname == "" {
			return fmt.Errorf("hostname cannot be empty")
		}
		c.mumble.Hostname = cmd.Hostname
		if cmd.Username == "" {
			return fmt.Errorf("username cannot be empty")
		}
		c.mumble.Username = cmd.Username
		if cmd.Port == 0 {
			cmd.Port = MumbleDefaultPort
		}
		c.mumble.Port = cmd.Port
		if cmd.Channel == "" {
			return fmt.Errorf("channel cannot be empty")
		}
		c.mumble.Channels = []string{cmd.Channel}
	}

	offer := cmd.SessionDescription
	c.peer, err = NewPC(offer, c.rtcStateChangeHandler)
	if err != nil {
		return err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := c.peer.pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	j, err := json.Marshal(answer.Sdp)
	if err != nil {
		return err
	}
	err = c.writeMsg(wsMsg{Key: "sd_answer", Value: j})
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("opusChan read goroutine quitting...\n")
				return
			case <-c.mumble.closeChan:
				log.Printf("opusChan read goroutine quitting...\n")
				return
			case s := <-c.mumble.opusChan:
				c.peer.track.Samples <- s
			}
		}
	}()

	return nil
}

// WebRTC callback function
func (c *Conn) rtcStateChangeHandler(connectionState ice.ConnectionState) {

	var err error

	switch connectionState {
	case ice.ConnectionStateConnected:
		log.Printf("ice connected, config %s\n", c.peer.pc.RemoteDescription().Sdp)

		if c.mumble.IsConnected() {
			c.errChan <- fmt.Errorf("mumble client already connected")
			return
		}

		err = c.mumble.Connect()
		if err != nil {
			c.errChan <- err
			return
		}

	case ice.ConnectionStateDisconnected:
		log.Printf("ice disconnected\n")
		err = c.mumble.Disconnect()
		if err != nil {
			c.errChan <- err
			return
		}
		log.Printf("mumble disconnected\n")
	}
}
