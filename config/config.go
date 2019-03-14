// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

import "time"

//TODO: Add number of errors and time before reconnect
type Config struct {
	Period            time.Duration `config:"period"`
	Endpoint          string        `config:"endpoint"`
	Nodes             []Node        `config:"nodes"`
	RetryOnErrorCount int           `config:"retryOnError"`
}

type Node struct {
	Namespace uint16      `config:"ns"`
	ID        interface{} `config:"id"`
	Label     string      `config:"label"`
}

var DefaultConfig = Config{
	Period:            1 * time.Second,
	Endpoint:          "opc.tcp://localhost:4840",
	RetryOnErrorCount: 5,
}
