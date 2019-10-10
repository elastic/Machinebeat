module github.com/elastic/machinebeat

go 1.13

require (
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/elastic/beats v0.0.0-00010101000000-000000000000
	github.com/gopcua/opcua v0.1.6
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gopkg.in/vmihailenco/msgpack.v2 v2.9.1
)

replace (
	github.com/dop251/goja => github.com/andrewkroh/goja v0.0.0-20190128172624-dd2ac4456e20
	github.com/elastic/beats => github.com/andrewkroh/beats v0.0.0-20190820143154-8b3f90af6bb3
	github.com/fsnotify/fsevents => github.com/elastic/fsevents v0.0.0-20181029231046-e1d381a4d270
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.1
)
