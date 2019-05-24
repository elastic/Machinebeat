package beater

import (
	"fmt"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/felix-lessoer/machinebeat/config"
)

// Machinebeat configuration.
type Machinebeat struct {
	done   chan struct{}
	config config.Config
	client beat.Client
}

var collectorError = false
var threadCounter = 0

// New creates an instance of machinebeat.
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &Machinebeat{
		done:   make(chan struct{}),
		config: c,
	}
	return bt, nil
}

func establishConnection(endpoint string, retryCounter int) error {
	var err error
	for i := retryCounter; i > 0; i-- {
		err = connect(endpoint)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	logp.Critical("Tried to connect %v time(s). Without success.", retryCounter)
	return err
}

// Run starts machinebeat.
func (bt *Machinebeat) Run(b *beat.Beat) error {
	logp.Info("machinebeat is running! Hit CTRL-C to stop it.")

	err := establishConnection(bt.config.Endpoint, 1)
	if err != nil {
		return err
	}

	logp.Info("Connecting to event publisher")
	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}
	logp.Info("Start collecting now")
	ticker := time.NewTicker(bt.config.Period)
	for {
		select {
		case <-bt.done:
			return nil
		case <-ticker.C:
			if connected {
				if threadCounter < bt.config.MaxThreads {
					go collect(bt, b)
				} else {
					logp.Err("Max threads reached. This means that it takes too long to get the data from your OPC UA server. You should consider to increase the max Thread counter or the period of getting the data.")
				}
			} else {
				//It seems that there was an error, we will try to reconnect
				logp.Info("Lets wait a while before reconnect happens")
				time.Sleep(5 * time.Second)
				err := establishConnection(bt.config.Endpoint, bt.config.RetryOnErrorCount)
				if err != nil {
					logp.Info("Reconnect was not successful")
					return err
				}
			}
		}

	}
}

func collect(bt *Machinebeat, b *beat.Beat) error {
	threadCounter = threadCounter + 1
	logp.Debug("Collector", "Event collector instance started")
	event := beat.Event{
		Timestamp: time.Now(),
		Fields: common.MapStr{
			"type": b.Info.Name,
		},
	}
	data, err := collectData(bt.config.Nodes)
	if err != nil {
		logp.Info("error: %v", err)
		logp.Error(err)
		connected = false
		return err
	}
	for _, response := range data {
		event.Fields.Put("event.dataset", "NodeValue")
		event.Fields.Put("event.module", "OPCUA")
		if response.value.Status == 0 {
			event.Fields.Put("service.state", "OK")
		} else {
			event.Fields.Put("service.state", "ERROR")
		}
		event.Fields.Put("event.created", response.value.SourceTimestamp.String())
		event.Fields.Put("OPCUA.value", response.value.Value.Value)
		event.Fields.Put("OPCUA.node", response.node.Label)
	}

	bt.client.Publish(event)
	logp.Debug("Collector", "Event collector instance finished sucessfully.")
	threadCounter = threadCounter - 1
	return nil
}

// Stop stops machinebeat.
func (bt *Machinebeat) Stop() {
	closeConnection()
	bt.client.Close()
	close(bt.done)
}
