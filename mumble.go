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

	logChan  chan string
	opusChan chan webrtc.RTCSample

	link gumble.Detacher
	ctx  context.Context
}

func NewMumbleClient(ctx context.Context, logChan chan string) *MumbleClient {
	client := &MumbleClient{}
	client.logChan = logChan
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
			m.logChan <- e.Message
		},
		Connect: func(e *gumble.ConnectEvent) {
			m.logChan <- fmt.Sprintf("mumble client, connected message: %s", *e.WelcomeMessage)
		},
		Disconnect: func(e *gumble.DisconnectEvent) {
			m.logChan <- fmt.Sprint("mumble client, disconnected message")
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

	m.Username = u.User.Username()
	u.Path = strings.Trim(u.Path, " /")
	m.Channels = strings.Split(u.Path, "/")

	fmt.Printf("m %+v\n", m)

	return nil
}
