package main

import (
	"crypto/tls"
	"encoding/binary"

	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type LatencyClient struct {
	ClientID        string
	BrokerURL       string
	BrokerUser      string
	BrokerPass      string
	MsgTopic        string
	MsgQoS          byte
	MeasurementInterval int
	TLSConfig       *tls.Config
	Protocol        string
}

func (c *LatencyClient) Run(latencies *[]uint64, done chan bool) {
	c.calculateLatency(latencies, done)
}

func (c *LatencyClient) calculateLatency(latencies *[]uint64, done chan bool) {
	onConnected := func(client mqtt.Client) {
		// s'abonner au topic et définir la fonction de réception des messages
		// définir la loop et publier des messages sur le topic latency
		// quand on reçoit true sur done, on peut quitter la fonction
		onMessageReceived := func(c mqtt.Client, m mqtt.Message) {
			now := uint64(time.Now().UTC().UnixMilli())
			r := now - binary.LittleEndian.Uint64(m.Payload())

			*latencies = append(*latencies, r)
		}

		token := client.Subscribe(c.MsgTopic, c.MsgQoS, onMessageReceived)
		token.Wait()
		buf := make([]byte, 8)
		for {
			binary.LittleEndian.PutUint64(buf, uint64(time.Now().UTC().UnixMilli()))
			token = client.Publish(c.MsgTopic, c.MsgQoS, false, buf)
			token.Wait()
			select {
			case <-done:
				return
			default: 
				time.Sleep(time.Duration(time.Millisecond * time.Duration(c.MeasurementInterval)))
				continue
			}
		}
	}

	opts := mqtt.NewClientOptions().
		AddBroker(c.BrokerURL).
		SetClientID(c.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(onConnected).
		SetConnectionLostHandler(func(client mqtt.Client, reason error) {
			log.Printf("CLIENT %v lost connection to the broker: %v. Will reconnect...\n", c.ClientID, reason.Error())
		})
	if c.BrokerUser != "" && c.BrokerPass != "" {
		opts.SetUsername(c.BrokerUser)
		opts.SetPassword(c.BrokerPass)
	}
	if c.TLSConfig != nil {
		opts.SetTLSConfig(c.TLSConfig)
	}
	opts.SetKeepAlive(0)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()

	if token.Error() != nil {
		log.Printf("CLIENT %v had error connecting to the broker: %v\n", c.ClientID, token.Error())
	}
}
