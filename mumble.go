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
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/pions/webrtc"
	"github.com/porjo/gumble/gumble"
	"github.com/porjo/gumble/gumbleutil"
	_ "github.com/porjo/gumble/opus"
)

const MumbleDefaultPort = 64738

type MumbleClient struct {
	Hostname string
	Port     int
	Username string
	//Password string
	Channels []string

	client *gumble.Client
	config *gumble.Config

	logChan   chan string
	stateChan chan gumble.State
	msgChan   chan MumbleMsg
	opusChan  chan webrtc.RTCSample

	link gumble.Detacher
	ctx  context.Context
}

type MumbleMsg struct {
	Sender  string
	Message string
}

func NewMumbleClient(ctx context.Context, msgChan chan MumbleMsg, logChan chan string, stateChan chan gumble.State) *MumbleClient {
	client := &MumbleClient{}
	client.logChan = logChan
	client.msgChan = msgChan
	client.stateChan = stateChan
	client.ctx = ctx
	client.opusChan = make(chan webrtc.RTCSample)
	return client
}

func (m *MumbleClient) IsConnected() bool {
	return m.client != nil
}

func (m *MumbleClient) Connect() error {
	var err error
	m.config = gumble.NewConfig()
	m.config.Username = m.Username

	// Attach event listeners to the configuration:

	m.config.Attach(gumbleutil.Listener{
		TextMessage: func(e *gumble.TextMessageEvent) {
			msg := MumbleMsg{
				Sender:  e.Sender.Name,
				Message: e.Message,
			}
			m.msgChan <- msg
		},
		Connect: func(e *gumble.ConnectEvent) {
			log.Printf("mumble client connected\n")
			m.stateChan <- e.Client.State()
		},
		Disconnect: func(e *gumble.DisconnectEvent) {
			log.Printf("mumble client disconnected\n")
			// non blocking channel write, as receiving goroutine may already have quit
			select {
			case m.stateChan <- e.Client.State():

			default:
			}
		},
	})

	// Connect to the server:
	port := strconv.Itoa(m.Port)
	host := m.Hostname + ":" + port

	//m.client, err = gumble.Dial(host, m.config)
	// FIXME accept self-signed
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	m.client, err = gumble.DialWithDialer(&net.Dialer{}, host, m.config, tlsConfig)
	if err != nil {
		return err
	}

	if len(m.client.Channels) > 0 {
		channel := m.client.Channels.Find(m.Channels...)
		if channel == nil {
			m.logChan <- fmt.Sprintf("channel(s) '%s' not found, defaulting to root", m.Channels)
		} else {
			m.client.Self.Move(channel)
			m.logChan <- fmt.Sprintf("joined channel '%s'", channel.Name)
		}
	}
	m.client.Self.SetSelfMuted(true)

	m.link = m.config.AttachAudio(m)

	return nil
}

func (m *MumbleClient) Disconnect() error {
	if m.client != nil {
		return m.client.Disconnect()
	}

	return nil
}

func (m *MumbleClient) OnAudioStream(e *gumble.AudioStreamEvent) {
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				log.Printf("opusChan write gorouting quitting...\n")
				return
			case p := <-e.C:
				rtcSample := webrtc.RTCSample{Data: p.OpusBuffer, Samples: p.OpusSamples}
				m.opusChan <- rtcSample
			}
		}
	}()
}

func (m *MumbleClient) ParseURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	if u.Scheme != "mumble" {
		return fmt.Errorf("invalid scheme")
	}

	h, p, _ := net.SplitHostPort(u.Host)

	if h == "" {
		m.Hostname = u.Host
	} else {
		m.Hostname = h
	}
	if p == "" {
		m.Port = MumbleDefaultPort
	} else {
		m.Port, _ = strconv.Atoi(p)
	}

	if u.User.Username() == "" {
		return fmt.Errorf("username cannot be blank")
	}
	m.Username = u.User.Username()
	u.Path = strings.Trim(u.Path, " /")
	if u.Path != "" {
		m.Channels = strings.Split(u.Path, "/")
	}

	return nil
}
