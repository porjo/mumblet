package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"github.com/pions/webrtc"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleutil"
)

const MumbleDefaultPort = 64738

type MumbleClient struct {
	Hostname string
	Port     int
	Username string
	//Password string
	Channel string

	client *gumble.Client
	config *gumble.Config

	logChan  chan string
	opusChan chan webrtc.RTCSample

	link gumble.Detacher
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
			m.logChan <- fmt.Sprintf("mumble client connected message: %s", *e.WelcomeMessage)
		},
		Disconnect: func(e *gumble.DisconnectEvent) {
			m.logChan <- fmt.Sprint("mumble client disconnected message")
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

	channel := m.client.Channels.Find(m.Channel)
	if channel != nil {
		m.client.Self.Move(channel)
	}
	m.client.Self.SetDeafened(false)

	m.link = m.config.AttachAudio(m)

	return nil
}

func (m *MumbleClient) Disconnect() error {
	return m.client.Disconnect()
}

func (m *MumbleClient) OnAudioStream(e *gumble.AudioStreamEvent) {
	fmt.Printf("onaudiostream \n")

	for p := range e.C {
		fmt.Printf("audio c range %d\n", len(p.OpusBuffer))
		rtcSample := webrtc.RTCSample{Data: p.OpusBuffer, Samples: p.OpusSamples}
		m.opusChan <- rtcSample
	}
}
