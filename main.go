package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/montanaflynn/stats"
)

// MessageMqtt describes a message fro mqtt
type MessageMqtt struct {
	Topic     string
	QoS       byte
	Payload   []byte
	Sent      time.Time
	Delivered time.Time
	Error     bool
}

// RunResults describes results of a single client / run
type RunResults struct {
	ID          string  `json:"id"`
	Successes   int64   `json:"successes"`
	Failures    int64   `json:"failures"`
	RunTime     float64 `json:"run_time"`
	MsgsPerSec  float64 `json:"msgs_per_sec"`
	CpuUsage    float64 `json:"cpu_usage`
	MemoryUsage float64 `json:"memory_usage"`
}

// TotalResults describes results of all clients / runs
type TotalResults struct {
	Ratio                     float64   `json:"ratio"`
	Successes                 int64     `json:"successes"`
	Failures                  int64     `json:"failures"`
	TotalRunTime              float64   `json:"total_run_time"`
	AvgRunTime                float64   `json:"avg_run_time"`
	TimeMeasurements          []float64 `json:"time_measurements"`
	MsgTimeMin                float64   `json:"msg_time_min"`
	MsgTimeMax                float64   `json:"msg_time_max"`
	MsgTimeAvg                float64   `json:"msg_time_mean_avg"`
	MsgTimeStd                float64   `json:"msg_time_mean_std"`
	TotalMsgsPerSecPublisher  float64   `json:"total_msgs_per_sec_pub"`
	AvgMsgsPerSecPublisher    float64   `json:"avg_msgs_per_sec_pub"`
	TotalMsgsPerSecSubscriber float64   `json:"total_msgs_per_sec_sub"`
	AvgMsgsPerSecSubscriber   float64   `json:"avg_msgs_per_sec_sub"`
	AvgCpuUsage               float64   `json:"avg_cpu_usage"`
	AvgMemoryUsage            float64   `json:"avg_memory_usage"`
}

// JSONResults are used to export results as a JSON document
type JSONResults struct {
	Runs   []*RunResults `json:"runs"`
	Totals *TotalResults `json:"totals"`
}

func main() {
	var (
		broker              = flag.String("broker", "tcp://localhost:1883", "MQTT broker endpoint as scheme://host:port")
		topic               = flag.String("topic", "/test", "MQTT topic for outgoing messages")
		payload             = flag.String("payload", "", "MQTT message payload. If empty, then payload is generated based on the size parameter")
		username            = flag.String("username", "", "MQTT client username (empty if auth disabled)")
		password            = flag.String("password", "", "MQTT client password (empty if auth disabled)")
		qos                 = flag.Int("qos", 1, "QoS for published messages")
		wait                = flag.Int("wait", 60000, "QoS 1 wait timeout in milliseconds")
		size                = flag.Int("size", 0, "Size of the messages payload (bytes)") // previous default value was 100
		count               = flag.Int("count", 100, "Number of messages to send per client")
		topicCount          = flag.Int("topic-count", 10, "Number of topic to publish messages on (Default: 10)")
		publishersPerTopic  = flag.Int("publishers", 1, "Number of publishers per topic to start (Default: 1 per topic)")
		subscribersPerTopic = flag.Int("subscribers", 1, "Number of subscribers per topic to start (Default: 1 per topic)")
		format              = flag.String("format", "text", "Output format: text|json")
		quiet               = flag.Bool("quiet", false, "Suppress logs while running")
		//clientPrefix         = flag.String("client-prefix", "mqtt-benchmark", "MQTT client id prefix (suffixed with '-<client-num>'")
		clientCert      = flag.String("client-cert", "", "Path to client certificate in PEM format")
		clientKey       = flag.String("client-key", "", "Path to private clientKey in PEM format")
		brokerCaCert    = flag.String("broker-ca-cert", "", "Path to broker CA certificate in PEM format")
		insecure        = flag.Bool("insecure", false, "Skip TLS certificate verification")
		rampUpTimeInSec = flag.Int("ramp-up-time", 0, "Time in seconds to generate clients by default will not wait between load request")
		messageInterval = flag.Int("message-interval", 1000, "Time interval in milliseconds to publish message")
		remoteUser      = flag.String("remote-user", "", "Username of the remote host where the broker is running")
		remotePwd       = flag.String("remote-pwd", "", "Password of the remote host where the broker is running")
	)

	flag.Parse()
	remote := false
	if !strings.Contains(*broker, "localhost") {
		remote = true
	}

	if *topicCount < 1 {
		log.Fatalf("Invalid arguments: number of clients should be >= 1, given: %v", *topicCount)
	}

	if *publishersPerTopic < 1 {
		log.Fatalf("Invalid arguments: number of publishers should be >= 1, given: %v", *publishersPerTopic)
	}

	if *subscribersPerTopic < 1 {
		log.Fatalf("Invalid arguments: number of subscribers should be >= 1, given: %v", *subscribersPerTopic)
	}

	if *count < 1 {
		log.Fatalf("Invalid arguments: messages count should be > 1, given: %v", *count)
	}

	if *clientCert != "" && *clientKey == "" {
		log.Fatal("Invalid arguments: private clientKey path missing")
	}

	if *clientCert == "" && *clientKey != "" {
		log.Fatalf("Invalid arguments: certificate path missing")
	}

	var tlsConfig *tls.Config
	if *clientCert != "" && *clientKey != "" {
		tlsConfig = generateTLSConfig(*clientCert, *clientKey, *brokerCaCert, *insecure)
	}

	resCh := make(chan *RunResults)
	subTpChannel := make(chan float64)

	latencies := []uint64{}
	time.Sleep(time.Duration(time.Second * 5))

	start := time.Now()
	sleepTime := float64(*rampUpTimeInSec) / float64(*publishersPerTopic)

	latenciesPointers := []*[]uint64{}

	for t := 0; t < *topicCount; t++ {
		for i := 0; i < *subscribersPerTopic; i++ {
			array := []uint64{}
			if !*quiet {
				log.Println("Starting SUBSCRIBER", fmt.Sprintf("%v-%v", t, i))
			}
			c := &SubscriberClient{
				ID:            fmt.Sprintf("%v-%v", t, i),
				ClientID:      fmt.Sprintf("subscriber-%v-%v-%v", t, i, time.Now().UTC().UnixMilli()),
				BrokerURL:     *broker,
				BrokerUser:    *username,
				BrokerPass:    *password,
				MsgTopic:      *topic + "-" + strconv.Itoa(t),
				TopicMsgCount: *publishersPerTopic * *count,
				MsgQoS:        byte(*qos),
				TLSConfig:     tlsConfig,
				Quiet:         *quiet,
				Timeout:       15,
			}
			latenciesPointers = append(latenciesPointers, &array)
			go c.Run(subTpChannel, &array)
			time.Sleep(time.Duration(sleepTime*1000) * time.Millisecond)
		}
	}

	for t := 0; t < *topicCount; t++ {
		for i := 0; i < *publishersPerTopic; i++ {
			if !*quiet {
				log.Println("Starting PUBLISHER", fmt.Sprintf("%v-%v", t, i))
			}
			c := &PublisherClient{
				ID:              fmt.Sprintf("%v-%v", t, i),
				ClientID:        fmt.Sprintf("publisher-%v-%v-%v", t, i, time.Now().UTC().UnixMilli()), // mqtt-benchmark-<topic number>-<publisher number>
				BrokerURL:       *broker,
				BrokerUser:      *username,
				BrokerPass:      *password,
				MsgTopic:        *topic + "-" + strconv.Itoa(t),
				MsgPayload:      *payload,
				MsgSize:         *size,
				MsgCount:        *count,
				MsgQoS:          byte(*qos),
				Quiet:           *quiet,
				WaitTimeout:     time.Duration(*wait) * time.Millisecond,
				TLSConfig:       tlsConfig,
				MessageInterval: *messageInterval,
				RemoteUser:      *remoteUser,
				RemotePwd:       *remotePwd,
				Remote:          remote,
			}
			go c.Run(resCh)
			time.Sleep(time.Duration(sleepTime*1000) * time.Millisecond)
		}
	}
	// collect the results
	results := make([]*RunResults, *publishersPerTopic**topicCount)
	for i := 0; i < *publishersPerTopic**topicCount; i++ {
		results[i] = <-resCh
	}
	totalTime := time.Since(start)

	subThroughputs := make([]float64, *subscribersPerTopic**topicCount)
	for i := 0; i < *subscribersPerTopic**topicCount; i++ {
		subThroughputs[i] = <-subTpChannel
	}

	for _, arrayPointer := range latenciesPointers {
		latencies = append(latencies, *arrayPointer...)
	}

	totals := calculateTotalResults(results, totalTime, *publishersPerTopic**topicCount, latencies, subThroughputs)

	// print stats
	printResults(results, totals, *format)
}

func calculateTotalResults(results []*RunResults, totalTime time.Duration, sampleSize int, latencies []uint64, subTp []float64) *TotalResults {
	totals := new(TotalResults)
	totals.TotalRunTime = totalTime.Seconds()

	msgsPerSecs := make([]float64, len(results))
	runTimes := make([]float64, len(results))
	bws := make([]float64, len(results))
	cpuUsage := make([]float64, len(results))
	ramUsage := make([]float64, len(results))
	// totals.MsgTimeMin = results[0].MsgTimeMin

	for _, v := range subTp {
		totals.TotalMsgsPerSecSubscriber += v
	}

	for i, res := range results {
		totals.Successes += res.Successes
		totals.Failures += res.Failures
		totals.TotalMsgsPerSecPublisher += res.MsgsPerSec

		// if res.MsgTimeMin < totals.MsgTimeMin {
		// 	totals.MsgTimeMin = res.MsgTimeMin
		// }

		// if res.MsgTimeMax > totals.MsgTimeMax {
		// 	totals.MsgTimeMax = res.MsgTimeMax
		// }

		msgsPerSecs[i] = res.MsgsPerSec
		runTimes[i] = res.RunTime
		bws[i] = res.MsgsPerSec
		cpuUsage[i] = res.CpuUsage
		ramUsage[i] = res.MemoryUsage
	}
	latenciesFloat64 := stats.LoadRawData(latencies[:])
	totals.Ratio = float64(totals.Successes) / float64(totals.Successes+totals.Failures)
	totals.AvgMsgsPerSecPublisher, _ = stats.Mean(msgsPerSecs)
	totals.AvgMsgsPerSecSubscriber, _ = stats.Mean(subTp)
	totals.AvgRunTime, _ = stats.Mean(runTimes)
	totals.TimeMeasurements = latenciesFloat64
	totals.MsgTimeMin, _ = stats.Min(latenciesFloat64)
	totals.MsgTimeMax, _ = stats.Max(latenciesFloat64)
	totals.MsgTimeAvg, _ = stats.Mean(latenciesFloat64)
	totals.MsgTimeStd, _ = stats.StandardDeviationSample(latenciesFloat64)
	totals.AvgCpuUsage, _ = stats.Mean(cpuUsage)
	totals.AvgMemoryUsage, _ = stats.Mean(ramUsage)

	return totals
}

func printResults(results []*RunResults, totals *TotalResults, format string) {
	switch format {
	case "json":
		jr := JSONResults{
			Runs:   results,
			Totals: totals,
		}
		data, err := json.Marshal(jr)
		if err != nil {
			log.Fatalf("Error marshalling results: %v", err)
		}
		var out bytes.Buffer
		_ = json.Indent(&out, data, "", "\t")

		fmt.Println(out.String())
	default:
		for _, res := range results {
			fmt.Printf("======= PUBLISHER %v =======\n", res.ID)
			fmt.Printf("Ratio:               %.3f (%d/%d)\n", float64(res.Successes)/float64(res.Successes+res.Failures), res.Successes, res.Successes+res.Failures)
			// fmt.Printf("Runtime (s):         %.3f\n", res.RunTime)
			fmt.Printf("Bandwidth (msg/sec): %.3f\n", res.MsgsPerSec)
			fmt.Printf("CPU Usage (percent): %.2f\n", res.CpuUsage)
			fmt.Printf("RAM Usage (percent): %.2f\n\n", res.MemoryUsage)
		}
		fmt.Printf("========= TOTAL (%d) =========\n", len(results))
		fmt.Printf("Total Ratio:                 %.3f (%d/%d)\n", totals.Ratio, totals.Successes, totals.Successes+totals.Failures)
		fmt.Printf("Total Runtime (sec):         %.3f\n", totals.TotalRunTime)
		fmt.Printf("Time measurements (ms): 	%.3f", totals.TimeMeasurements)
		fmt.Printf("Msg time min (ms):           %.3f\n", totals.MsgTimeMin)
		fmt.Printf("Msg time max (ms):           %.3f\n", totals.MsgTimeMax)
		fmt.Printf("Msg time mean (ms):     	%.3f\n", totals.MsgTimeAvg)
		fmt.Printf("Msg time std (ms):      	%.3f\n", totals.MsgTimeStd)
		fmt.Printf("Average Bandwidth Per Publisher (msg/sec): %.3f\n", totals.AvgMsgsPerSecPublisher)
		fmt.Printf("Total Bandwidth Publishers (msg/sec):   %.3f\n", totals.TotalMsgsPerSecPublisher)
		fmt.Printf("Average Bandwidth Per Subscriber (msg/sec): %.3f\n", totals.AvgMsgsPerSecSubscriber)
		fmt.Printf("Total Bandwidth Subscribers (msg/sec):   %.3f\n", totals.TotalMsgsPerSecSubscriber)
		fmt.Printf("Average CPU Usage (percent): %.2f\n", totals.AvgCpuUsage)
		fmt.Printf("Average RAM Usage (percent): %.2f\n", totals.AvgMemoryUsage)
	}
}

func generateTLSConfig(certFile string, keyFile string, caFile string, insecure bool) *tls.Config {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("Error reading certificate files: %v", err)
	}

	var caCertPool *x509.CertPool = nil
	if caFile != "" {
		caCert, err := ioutil.ReadFile(caFile)
		if err != nil {
			log.Fatalf("Error reading CA certificate file: %v", err)
		}

		caCertPool = x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM(caCert)
		if !ok {
			log.Fatalf("Error parsing CA certificate %v", certFile)
		}
	}

	cfg := tls.Config{
		ClientAuth:         tls.NoClientCert,
		ClientCAs:          nil,
		InsecureSkipVerify: insecure,
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
	}

	return &cfg
}
