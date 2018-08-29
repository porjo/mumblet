package main

import (
	"fmt"

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
			fmt.Printf("Received text message: %s\n", e.Message)
		},
		Connect: func(e *gumble.ConnectEvent) {
			fmt.Printf("mumble client connected message: %s\n", e.WelcomeMessage)
		},
	})

	// Connect to the server:

	m.client, err = gumble.Dial(m.Hostname+":"+string(m.Port), m.config)
	if err != nil {
		return err
	}

	return nil
}
