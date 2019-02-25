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
	ticker := time.NewTicker(bt.config.Period)
	for {
		select {
		case <-bt.done:
			return nil
		case <-ticker.C:
		}
		for _, node := range bt.config.Nodes {
			data, err := collectData(node)
			if err != nil {
				return err
			}
			event := beat.Event{
				Timestamp: time.Now(),
				Fields: common.MapStr{
					"type": b.Info.Name,
				},
			}
			for name, value := range data {
				event.Fields.Put(name, value)
			}
			bt.client.Publish(event)
		}
		logp.Info("Event sent")
	}
}

// Stop stops machinebeat.
func (bt *Machinebeat) Stop() {
	closeConnection()
	bt.client.Close()
	close(bt.done)
}
