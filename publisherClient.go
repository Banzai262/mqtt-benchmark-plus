package main

import (
	// "context"
	"crypto/tls"
	"log"
	"math"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/montanaflynn/stats"
	// "github.com/sfreiberg/simplessh"
	"github.com/eugenmayer/go-sshclient/sshwrapper"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// PublisherClient implements an MQTT client running benchmark test
type PublisherClient struct {
	ID              string
	ClientID        string
	BrokerURL       string
	BrokerUser      string
	BrokerPass      string
	MsgTopic        string
	MsgPayload      string
	MsgSize         int
	MsgCount        int
	MsgQoS          byte
	Quiet           bool
	WaitTimeout     time.Duration
	TLSConfig       *tls.Config
	MessageInterval int
	Protocol        string
	RemoteUser      string
	RemotePwd       string
	Remote          bool
}

type Pair[T, U any] struct {
	First  T
	Second U
}

func getRandom(id string) rand.Rand {
	sum := 0

	for _, c := range id {
		sum += int(c)
	}
	return *rand.New(rand.NewSource(int64(sum)))
}

// Run runs benchmark tests and writes results in the provided channel
func (c *PublisherClient) Run(res chan *RunResults) {
	pubMsgsMqtt := make(chan *MessageMqtt)
	donePub := make(chan float64)
	runResults := new(RunResults)

	runResults.ID = c.ID
	cpuUsage := []float64{}
	ramUsage := []float64{}
	ctr := 0
	url, _ := extractHostnameFromURL(c.BrokerURL)

	ram, _ := mem.VirtualMemory()
	tmp, _ := cpu.Percent(0, false) // to initiate CPU usage measurements

	// Oold ssh client that used to work
	// sshClient, err := simplessh.ConnectWithPassword(url, c.RemoteUser, c.RemotePwd)
	// if err != nil {
	// 	log.Printf("error when creating ssh client: %v", err.Error())
	// }
	// defer sshClient.Close()

	sshApi, err := sshwrapper.DefaultSshApiSetup(url, 22, c.RemoteUser, "")
	sshApi.Password = c.RemotePwd
	err = sshApi.DefaultSshPasswordSetup()
	if err != nil {
		log.Fatal(err)
	}
	defer sshApi.Close()

	// start generator
	msgs := c.genMessagesMqttV2()
	started := time.Now()
	// start publisher
	go c.pubMessagesMqttV2(msgs, pubMsgsMqtt, donePub)

	for {
		select {
		case m := <-pubMsgsMqtt:
			if m.Error {
				log.Printf("PUBLISHER %v ERROR publishing message: %v: at %v\n", c.ID, m.Topic, m.Sent.Unix())
				runResults.Failures++
			} else {
				// log.Printf("Message published: %v: sent: %v delivered: %v flight time: %v\n", m.Topic, m.Sent, m.Delivered, m.Delivered.Sub(m.Sent))
				runResults.Successes++

				ctr++
				if ctr%50 == 0 {
					if c.Remote {
						// cpu, _ := getRemoteCPUUsage(*sshApi)
						// memory, _ := getRemoteMemoryUsage(*sshApi)
						// cpuUsage = append(cpuUsage, cpu)
						// ramUsage = append(ramUsage, memory)
						go getRemoteCPUUsage(*sshApi, &cpuUsage)
						go getRemoteMemoryUsage(*sshApi, &ramUsage)
					} else {
						tmp, _ = cpu.Percent(0, false)
						cpuUsage = append(cpuUsage, tmp[0])
						ramUsage = append(ramUsage, ram.UsedPercent)
					}
				}

			}
		case t := <-donePub:
			// calculate results

			duration := time.Since(started)
			runResults.RunTime = duration.Seconds() - float64((c.MsgCount/100)*20)
			runResults.MsgsPerSec = float64(runResults.Successes) / t
			runResults.CpuUsage, _ = stats.Mean(cpuUsage)
			runResults.MemoryUsage, _ = stats.Mean(ramUsage)

			if math.IsNaN(runResults.CpuUsage) {
				runResults.CpuUsage = 0
			}
			if math.IsNaN(runResults.MemoryUsage) {
				runResults.MemoryUsage = 0
			}

			// report results and exit
			res <- runResults
			return
		}
	}
}

func extractHostnameFromURL(inputURL string) (string, error) {
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return "", err
	}

	// The hostname could be an IP address or a domain name
	hostname := parsedURL.Hostname()
	return hostname, nil
}

// func getRemoteCPUUsage(sshClient simplessh.Client) (float64, error) {
func getRemoteCPUUsage(sshClient sshwrapper.SshApi, usage *[]float64) {
	// cpu, _ := sshClient.Exec("top -bn1 | awk '/Cpu/ {print 100 - $8}'")
	cpu, _, _ := sshClient.Run("top -bn1 | awk '/Cpu/ {print 100 - $8}'")
	cpuUsage, _ := strconv.ParseFloat(strings.TrimSpace(string(cpu)), 64)
	*usage = append(*usage, cpuUsage)

	// return cpuUsage, nil
}

// func getRemoteMemoryUsage(sshClient simplessh.Client) (float64, error) {
func getRemoteMemoryUsage(sshClient sshwrapper.SshApi, usage *[]float64) {
	// mem, _ := sshClient.Exec("free | awk '/Mem/ {print $3/ $2 * 100}'")
	mem, _, _ := sshClient.Run("free | awk '/Mem/ {print $3/ $2 * 100}'")
	memUsage, _ := strconv.ParseFloat(strings.TrimSpace(string(mem)), 64)
	*usage = append(*usage, memUsage)

	// return memUsage, nil
}

// generate all messages and add them to a list
func (c *PublisherClient) genMessagesMqttV2() *[]MessageMqtt {
	var msgs []MessageMqtt
	random := getRandom(c.ID)
	minRand := 7000   // byte
	maxRand := 600000 // byte
	size := c.MsgSize
	for i := 0; i < c.MsgCount; i++ {
		if c.MsgSize == 0 {
			size = random.Intn(maxRand-minRand) + minRand

		}
		m := MessageMqtt{
			Topic:   c.MsgTopic,
			QoS:     c.MsgQoS,
			Payload: make([]byte, size),
		}
		msgs = append(msgs, m)
	}
	return &msgs
}

func (c *PublisherClient) pubMessagesMqttV2(msgs *[]MessageMqtt, out chan *MessageMqtt, donePub chan float64) {
	onConnected := func(client mqtt.Client) {
		if !c.Quiet {
			log.Printf("PUBLISHER %v is connected to the broker %v\n", c.ID, c.BrokerURL)
		}
		ctr := 0
		globalTime := time.Now()

		if c.MessageInterval > 0 {

			// ticker provides a more precise interval
			ticker := time.NewTicker(time.Millisecond * time.Duration(c.MessageInterval))
			defer ticker.Stop()
			for range ticker.C {
				msg := (*msgs)[ctr]
				msg.Sent = time.Now()
				for i := 0; i < 8; i++ {
					msg.Payload[i] = byte(uint64(msg.Sent.UTC().UnixMilli()) >> (8 * (i)))
				}
				client.Publish(msg.Topic, msg.QoS, false, msg.Payload)
				msg.Delivered = time.Now()
				msg.Error = false

				out <- &msg

				if !c.Quiet {
					if ctr > 0 && ctr%100 == 0 {
						log.Printf("PUBLISHER %v published %v messages and keeps publishing...\n", c.ID, ctr)
					}
				}
				ctr++
				if ctr >= c.MsgCount {
					break
				}
			}
		} else { // for interval of 0
			for _, msg := range *msgs {
				msg.Sent = time.Now()
				for i := 0; i < 8; i++ {
					msg.Payload[i] = byte(uint64(msg.Sent.UTC().UnixMilli()) >> (8 * (i)))
				}
				client.Publish(msg.Topic, msg.QoS, false, msg.Payload)
				msg.Delivered = time.Now()
				msg.Error = false

				out <- &msg

				if !c.Quiet {
					if ctr > 0 && ctr%100 == 0 {
						log.Printf("PUBLISHER %v published %v messages and keeps publishing...\n", c.ID, ctr)
					}
				}
				ctr++
				time.Sleep(time.Millisecond * time.Duration(c.MessageInterval))
			}
		}

		donePub <- time.Since(globalTime).Seconds()
		if !c.Quiet {
			log.Printf("PUBLISHER %v is done publishing in %v\n", c.ID, time.Since(globalTime).Seconds())
		}
	}

	opts := mqtt.NewClientOptions().
		AddBroker(c.BrokerURL).
		SetClientID(c.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(onConnected).
		SetConnectionLostHandler(func(client mqtt.Client, reason error) {
			log.Printf("PUBLISHER %v lost connection to the broker: %v. Will reconnect...\n", c.ID, reason.Error())
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
		log.Printf("PUBLISHER %v had error connecting to the broker: %v\n", c.ID, token.Error())
	}
}
