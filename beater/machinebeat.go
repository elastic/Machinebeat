package beater

import (
	"fmt"
	"strings"
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

// Run starts machinebeat.
func (bt *Machinebeat) Run(b *beat.Beat) error {
	logp.Info("machinebeat is running! Hit CTRL-C to stop it.")

	err := connect(bt.config.Endpoint)

	if err != nil {
		return err
	}

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
			if !collectorError {
				go collect(bt, b)
			} else {
				//It seems that there was an error, we will try to reconnect
				err := connect(bt.config.Endpoint)

				if err != nil {
					return err
				}

				collectorError = false
			}
		}

	}
}

func collect(bt *Machinebeat, b *beat.Beat) error {
	logp.Debug("Collector", "Event collector instance started")
	event := beat.Event{
		Timestamp: time.Now(),
		Fields: common.MapStr{
			"type": b.Info.Name,
		},
	}
	for _, node := range bt.config.Nodes {
		data, err := collectData(node)
		if err != nil {
			logp.Info("error: %v", err)
			logp.Error(err)
			collectorError = true
			return err
		}

		for name, value := range data {
			var fieldId = []string{node.Label, name}
			event.Fields.Put(strings.Join(fieldId, "."), value)
		}
	}
	bt.client.Publish(event)
	logp.Debug("Collector", "Event collector instance finished sucessfully.")
	return nil
}

// Stop stops machinebeat.
func (bt *Machinebeat) Stop() {
	closeConnection()
	bt.client.Close()
	close(bt.done)
}
