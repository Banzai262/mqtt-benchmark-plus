package main

import (
	// "context"
	"crypto/tls"
	"encoding/binary"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SubscriberClient struct {
	ID            string
	ClientID      string
	BrokerURL     string
	BrokerUser    string
	BrokerPass    string
	MsgTopic      string
	TopicMsgCount int
	MsgQoS        byte
	TLSConfig     *tls.Config
	Quiet         bool
	Timeout       int
}

func (c *SubscriberClient) Run(res chan float64, latencies *[]uint64) {
	c.consume(res, latencies)
}

func (c *SubscriberClient) consume(res chan float64, latencies *[]uint64) {
	onConnected := func(client mqtt.Client) {
		if !c.Quiet {
			log.Printf("SUBSCRIBER %v is connected to the broker %v\n", c.ID, c.BrokerURL)
		}
		startTime := time.Now()
		ctr := 0
		timeout := time.Second * time.Duration(c.Timeout)
		msgChan := make(chan mqtt.Message)

		client.Subscribe(c.MsgTopic, c.MsgQoS, func(c mqtt.Client, m mqtt.Message) {
			timestamp := time.Now().UTC().UnixMilli()
			receivedTimestampBytes := m.Payload()[:8]
			receivedTimestamp := binary.LittleEndian.Uint64(receivedTimestampBytes)
			*latencies = append(*latencies, uint64(timestamp)-receivedTimestamp)
			msgChan <- m
		})

		timer := time.NewTimer(timeout)

		for {
			select {
			case <-msgChan:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(timeout)
				ctr++
				if float64(ctr) >= float64(c.TopicMsgCount)*0.99 { // for some reason the ctr sometimes goes up to just below the expected count
					duration := time.Since(startTime)
					res <- float64(ctr) / duration.Seconds()
					if !c.Quiet {
						log.Printf("SUBSCRIBER %v received every message, disconnecting", c.ID)
					}
					return
				}
			case <-timer.C:
				duration := time.Since(startTime).Seconds() - timeout.Seconds()
				res <- float64(ctr) / duration
				// res <- 0
				if !c.Quiet {
					log.Printf("SUBSCRIBER %v only received %v messages, can't calculate throughput", c.ID, ctr)
				}
				return
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
			log.Printf("SUBSCRIBER %v lost connection to the broker: %v. Will reconnect...\n", c.ClientID, reason.Error())
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
		log.Printf("SUBSCRIBER %v had error connecting to the broker: %v\n", c.ClientID, token.Error())
	}
}
