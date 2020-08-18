package topic

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"strings"
	"time"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/beats/v7/metricbeat/mb"
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

func NewTLSConfig() *tls.Config {
	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	if config.CA != "" {
		logp.Info("[MQTT] Set the CA")
		pemCerts, err := ioutil.ReadFile(config.CA)
		if err == nil {
			certpool.AppendCertsFromPEM(pemCerts)
		}
	}

	tlsconfig := &tls.Config{
		// RootCAs = certs used to verify server cert.
		RootCAs: certpool,
		// ClientAuth = whether to request cert from server.
		// Since the server is set up for SSL, this happens
		// anyways.
		ClientAuth: tls.NoClientCert,
		// ClientCAs = certs used to validate client cert.
		ClientCAs: nil,
		// InsecureSkipVerify = verify that cert contents
		// match server. IP matches what is in cert etc.
		InsecureSkipVerify: true,
	}

	// Import client certificate/key pair
	if config.ClientCert != "" && config.ClientKey != "" {
		logp.Info("[MQTT] Set the Certs")
		cert, err := tls.LoadX509KeyPair(config.ClientCert, config.ClientKey)
		if err != nil {
			panic(err)
		}

		// Certificates = list of certs client sends to server.
		tlsconfig.Certificates = []tls.Certificate{cert}
	}

	// Create tls.Config with desired tls properties
	return tlsconfig
}

// Prepare MQTT client
func setupMqttClient(m *MetricSet) {
	logp.Info("[MQTT] Connect to broker URL: %s", m.BrokerURL)
	config = m

	mqttClientOpt := MQTT.NewClientOptions()
	mqttClientOpt.SetClientID(m.ClientID)
	mqttClientOpt.AddBroker(m.BrokerURL)

	mqttClientOpt.SetMaxReconnectInterval(1 * time.Second)
	mqttClientOpt.SetConnectionLostHandler(reConnectHandler)
	mqttClientOpt.SetOnConnectHandler(subscribeOnConnect)
	mqttClientOpt.SetAutoReconnect(true)

	if m.BrokerUsername != "" {
		logp.Info("[MQTT] Broker username: %s", m.BrokerUsername)
		mqttClientOpt.SetUsername(m.BrokerUsername)
	}

	if m.BrokerPassword != "" {
		mqttClientOpt.SetPassword(m.BrokerPassword)
	}

	if m.SSL == true {
		logp.Info("[MQTT] Configure session to use SSL")
		tlsconfig := NewTLSConfig()
		mqttClientOpt.SetTLSConfig(tlsconfig)
	}

	client = MQTT.NewClient(mqttClientOpt)

	connect(client)
}

func connect(client MQTT.Client) {
	if !connected {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			logp.Info("Failed to connect to broker, waiting 5 seconds and retrying")
			time.Sleep(5 * time.Second)
			connected = false
			reConnectHandler(client, token.Error())
			return
		}
		connected = client.IsConnected()
		logp.Info("MQTT Client connected: %t", client.IsConnected())
		return
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

	// default case
	var message = make(common.MapStr)
	message["content"] = string(msg.Payload())
	if config.DecodePaylod == true {
		message["fields"] = DecodePayload(msg.Payload())
	}

	if strings.HasPrefix(msg.Topic(), "$") {
		event["isSystemTopic"] = true
	} else {
		event["isSystemTopic"] = false
	}
	event["topic"] = msg.Topic()
	message["ID"] = msg.MessageID()
	message["retained"] = msg.Retained()
	event["message"] = message

	// Finally sending the message to elasticsearch
	mbEvent.ModuleFields = event
	events <- mbEvent

	logp.Debug("MQTT", "Event sent: %t")
}

// DefaultConnectionLostHandler does nothing
func reConnectHandler(client MQTT.Client, reason error) {
	logp.Warn("[MQTT] Connection lost: %s", reason.Error())
	connected = false
	connect(client)
}

// DecodePayload will try to decode the payload. If every check fails, it will
// return the payload as a string
func DecodePayload(payload []byte) common.MapStr {
	event := make(common.MapStr)

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
