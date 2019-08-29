package topic

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/metricbeat/mb"
	"gopkg.in/vmihailenco/msgpack.v2"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

var (
	client    MQTT.Client
	connected bool = false
	config    *MetricSet
	reporter  mb.ReporterV2
	events    chan mb.Event
)

// Prepare MQTT client
func setupMqttClient(m *MetricSet) {
	mqttClientOpt := MQTT.NewClientOptions()
	mqttClientOpt.AddBroker(m.BrokerURL)
	logp.Info("BROKER url: %s", m.BrokerURL)
	mqttClientOpt.SetConnectionLostHandler(reConnectHandler)
	mqttClientOpt.SetOnConnectHandler(subscribeOnConnect)

	if m.BrokerUsername != "" && m.BrokerPassword != "" {
		logp.Info("BROKER username: %s", m.BrokerUsername)
		mqttClientOpt.SetUsername(m.BrokerUsername)
		mqttClientOpt.SetPassword(m.BrokerPassword)
	}

	client = MQTT.NewClient(mqttClientOpt)
	config = m
	connect(client)
}

func connect(client MQTT.Client) {
	if !connected {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			logp.Info("Failed to connect to broker, waiting 5 seconds and retrying")
			time.Sleep(5 * time.Second)
			reConnectHandler(client, token.Error())
			return
		}
		connected = true
		logp.Info("MQTT Client connected: %t", client.IsConnected())
	}
}

func subscribeOnConnect(client MQTT.Client) {
	subscriptions := ParseTopics(config.TopicsSubscribe, config.QoS)
	//bt.beatConfig.TopicsSubscribe

	// Mqtt client - Subscribe to every topic in the config file, and bind with message handler
	if token := client.SubscribeMultiple(subscriptions, onMessage); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	events = make(chan mb.Event, 500)
	logp.Info("Subscribed to configured topics")
}

// Mqtt message handler
func onMessage(client MQTT.Client, msg MQTT.Message) {
	logp.Debug("MQTT Module", "MQTT message received: %s", string(msg.Payload()))
	var mbEvent mb.Event
	event := make(common.MapStr)

	if config.DecodePaylod == true {
		event = DecodePayload(msg.Payload())
	} else {
		event["message"] = msg.Payload()
	}
	if strings.HasPrefix(msg.Topic(), "$") {
		event["isSystemTopic"] = true
	} else {
		event["isSystemTopic"] = false
	}
	event["name"] = msg.Topic()

	// Finally sending the message to elasticsearch
	mbEvent.MetricSetFields = event
	events <- mbEvent

	logp.Debug("MQTT Modul", "Event sent: %t")
}

// DefaultConnectionLostHandler does nothing
func reConnectHandler(client MQTT.Client, reason error) {
	logp.Warn("Connection lost: %s", reason.Error())
	connected = false
	connect(client)
}

// DecodePayload will try to decode the payload. If every check fails, it will
// return the payload as a string
func DecodePayload(payload []byte) common.MapStr {
	event := make(common.MapStr)

	// default case
	event["message"] = string(payload)

	// A msgpack payload must be a json-like object
	err := msgpack.Unmarshal(payload, &event)
	if err == nil {
		logp.Debug("mqttbeat", "Payload decoded - msgpack")
		return event
	}

	err = json.Unmarshal(payload, &event)
	if err == nil {
		logp.Debug("mqttbeat", "Payload decoded - json")
		return event
	}

	logp.Debug("mqttbeat", "decoded - text")
	return event
}

// ParseTopics will parse the config file and return a map with topic:QoS
func ParseTopics(topics []string, qos int) map[string]byte {
	subscriptions := make(map[string]byte)
	for _, value := range topics {
		// Finally, filling the subscriptions map
		subscriptions[value] = byte(qos)
		logp.Info("Subscribe to %v with QoS %v", value, qos)
	}
	return subscriptions
}
