# MQTT benchmarking tool

A simple MQTT (broker) benchmarking tool.

Installation by building from source or with this command:

```sh
go install github.com/banzai262/mqtt-benchmark-plus@main
```

The tool supports multiple concurrent clients, configurable message size, etc. If the broker address doesn't include `localhost`
the user can set `remote-user` and `remote-pwd` to monitor CPU and RAM usage from the broker host. Won't monitor if left empty.
Has no effect if the broker is on localhost.

```sh
$ ./mqtt-benchmark -h
Usage of ./mqtt-benchmark:
  -broker string
    	MQTT broker endpoint as scheme://host:port (default "tcp://localhost:1883")
  -broker-ca-cert string
    	Path to broker CA certificate in PEM format
  -client-cert string
    	Path to client certificate in PEM format
  -client-key string
    	Path to private clientKey in PEM format
  -topic-count int
        Number of topic to publish messages on (default: 10)
  -publishers int
    	Number of publishers per topic to start (default: 1 per topic)
  -subscribers
        Number of subscribers per topic to start (default: 1 per topic)
  -count int
    	Number of messages to send per client (default 100)
  -format string
    	Output format: text|json (default "text")
  -insecure
    	Skip TLS certificate verification
  -message-interval int
    	Time interval in milliseconds to publish message (default 1)
  -password string
    	MQTT client password (empty if auth disabled)
  -payload string
    	MQTT message payload. If empty, then payload is generated based on the size parameter
  -qos int
    	QoS for published messages (default 1)
  -quiet
    	Suppress logs while running
  -ramp-up-time int
    	Time in seconds to generate clients by default will not wait between load request
  -size int
    	Size of the messages payload (bytes) (default 0)
  -topic string
    	MQTT topic for outgoing messages (default "/test")
  -username string
    	MQTT client username (empty if auth disabled)
  -wait int
    	QoS 1 wait timeout in milliseconds (default 60000)
  -remote-user string
        Username to connect to the broker host machine via SSH (default "")
  -remote-pwd string
        Password to connect to the broker host machine via SSH (default "")
```

> NOTE: if `count=1` or there is 1 total publisher, the sample standard deviation will be returned as `0` (convention due to the [lack of NaN support in JSON](https://tools.ietf.org/html/rfc4627#section-2.4))

Two output formats supported: human-readable plain text and JSON.

Example use and output:

```sh
> mqtt-benchmark --broker tcp://broker.local:1883 --count 100 --size 100 --topic-count 100 --qos 2 --format text
....

======= CLIENT 27 =======
Ratio:               1.000 (1000/1000)
Bandwidth (msg/sec): 64383.209
CPU Usage (percent): 40.62
RAM Usage (percent): 37.00

========= TOTAL (100) =========
Total Ratio:                 1.000 (1000/1000)
Total Runtime (sec):         0.028
Time measurements (ms):      [0, 3, 15, 1, 0,...]
Msg time min (ms):           1.000
Msg time max (ms):           9.000
Msg time mean (ms):             5.003
Msg time std (ms):              1.631
Average Bandwidth Per Publisher (msg/sec): 46986.276
Total Bandwidth Publishers (msg/sec):   469862.758
Average Bandwidth Per Subscriber (msg/sec): 4268.882
Total Bandwidth Subscribers (msg/sec):   42688.821
Average CPU Usage (percent): 6.88
Average RAM Usage (percent): 37.00
```

With payload specified:

```sh
> mqtt-benchmark --broker tcp://broker.local:1883 --count 100 --clients 10 --qos 1 --topic house/bedroom/temperature --payload {\"temperature\":20,\"timeStamp\":1597314150}
....

======= CLIENT 0 =======
Ratio:               1.000 (100/100)
Runtime (s):         0.725
Msg time min (ms):   1.999
Msg time max (ms):   22.997
Msg time mean (ms):  6.955
Msg time std (ms):   3.523
Bandwidth (msg/sec): 137.839

========= TOTAL (1) =========
Total Ratio:                 1.000 (100/100)
Total Runtime (sec):         0.736
Average Runtime (sec):       0.725
Msg time min (ms):           1.999
Msg time max (ms):           22.997
Msg time mean mean (ms):     6.955
Msg time mean std (ms):      0.000
Average Bandwidth (msg/sec): 137.839
Total Bandwidth (msg/sec):   137.839
```

Similarly, in JSON:

```json
> mqtt-benchmark --broker tcp://broker.local:1883 --count 100 --size 100 --clients 100 --qos 2 --format json --quiet
{
    runs: [
        ...
        {
            "id": "0-0",
            "successes": 100,
            "failures": 0,
            "run_time": 0.5554133,
            "msgs_per_sec": 180.04610260503304,
            "CpuUsage": 10,
            "memory_usage": 10 
        }
    ],
    "totals": {
                "ratio": 1,
                "successes": 100,
                "failures": 0,
                "total_run_time": 0.5611372,
                "avg_run_time": 0.5554133,
                "time_measurements": [0,0,0,0,0,0,0,0,0,0,1,0,0,0,0,1,0,0,0,1,0,1,0],
                "msg_time_min": 0,
                "msg_time_max": 1,
                "msg_time_mean_avg": 0.17391304347826086,
                "msg_time_mean_std": 0.38755338788158983,
                "total_msgs_per_sec_pub": 180.04610260503304,
                "avg_msgs_per_sec_pub": 180.04610260503304,
                "total_msgs_per_sec_sub": 179.1635991686809,
                "avg_msgs_per_sec_sub": 179.1635991686809,
                "avg_cpu_usage": 0,
                "avg_memory_usage": 0
    }
}
```
