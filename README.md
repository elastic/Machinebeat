# About Machinebeat

Welcome to Machinebeat. This version of Machinebeat is a first prototype and it will be developed further over time. 

What is a beat? A beat is a lightweight data shipper written in GOLANG developed by [Elastic N.V.](https://www.elastic.co) and the community. It is open source and also implements a framework offered to the community to [build their own beats](https://www.elastic.co/guide/en/beats/devguide/current/new-beat.html). Those beats may offer special purpose data collection not offered by [existing beats](https://www.elastic.co/products/beats). Machinebeat is one of those special purpose beats.

The aim of machinebeat is, to build a leightweight data shipper that is able to pull metrics from machines used in industrial  environments and ship it to Elasticsearch or have [Logstash](https://www.elastic.co/products/logstash) or a message queue like [Kafka](https://kafka.apache.org/) inbetween, depending on the use case and architecture.

In first place Machinebeat is able to connect to [OPC-UA](https://opcfoundation.org/) Servers or other [PLCs](https://en.wikipedia.org/wiki/Programmable_logic_controller) to pull data in real-time, feeding it into Elasticsearch for near real-time monitoring purpose. This will enable machine operators to see behavior of their machines and analyse important metric on the fly. More about OPC-UA can also be found on [Wikipedia](https://en.wikipedia.org/wiki/OPC_Unified_Architecture)

Machinebeat uses the [GOPCUA](https://github.com/gopcua/opcua) project to connect and pull data from OPS-UA servers. GOPCUA is an implementation of the OPC-UA specification written in GOLANG.

The ability to get machine metrics and other related information is a foundation for maintenance, quality assurance and optimization of industry 4.0 environments. It can help to see possible issues early in the production cylce or optimize the use of incredients for the products produced. There are many other problems that can be solved monitoring machine metrics once they can be derived from them.

In a future version Machinebeat is supposed to support additional protocols like MQTT in order to cover a broader mix of different sensor metrics in the industry.

The current prototype is able to read nodes from an [OPC-UA address space](https://opcfoundation.org/developer-tools/specifications-unified-architecture/part-3-address-space-model/) specified in the machinebeat.yml file from Servers and transfer the collected data to Elasticsearch directly or via Logstash. The nodes and its leafs have to be specified in the above mentioned YAML file like this:

```
#The URL of your OPC UA Server
  endpoint: "opc.tcp://opcuaserver.com:48010"

  nodes:
  -  ns: 3
     id: "AirConditioner_1.State"
  -  ns: 3
     id: "AirConditioner_1.Humidity"
  -  ns: 3
     id: "AirConditioner_1.Temperature"
```

Different Nodes can be specified and read by machinebeat.

In a future version it is planned to have Machinebeat using the browsing service of OPC-UA to browse for nodes/views. It will then pull the subsequent leafs and metrics from those specified nodes or views into the config file for further editing and adjustments.

# Let's test it

You don't have an Elastic cluster up and running?
Use [Elasticsearch Service](https://www.elastic.co/cloud/elasticsearch-service/signup)

## Getting Started with Machinebeat

You can run the .exe directly if you are running on a windows system.
I also added a linux binary. Just run it from your command line.
Use -e to see the log output.
```
machinebeat.exe -e
```

If it not works in your environment you need to compile to your environment:
Ensure that this folder is at the following location:
`${GOPATH}/src/github.com/felix-lessoer/machinebeat`

### Requirements

* [Golang](https://golang.org/dl/) 1.12
* [Elastic Stack](https://cloud.elastic.co) >v7.0

### Init Project
To get running with Machinebeat and also install the
dependencies, run the following command:

```
make setup
```

It will create a clean git history for each major step. Note that you can always rewrite the history if you wish before pushing your changes.

To push Machinebeat in the git repository, run the following commands:

```
git remote set-url origin https://github.com/felix-lessoer/machinebeat
git push origin master
```

For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).

### Build

To build the binary for Machinebeat run the command below. This will generate a binary
in the same directory with the name machinebeat.

```
make
```


### Run

To run Machinebeat with debugging output enabled, run:

```
./machinebeat -c machinebeat.yml -e -d "*"
```


### Test

To test Machinebeat, run the following command:

```
make testsuite
```

alternatively:
```
make unit-tests
make system-tests
make integration-tests
make coverage-report
```

The test coverage is reported in the folder `./build/coverage/`

### Update

Each beat has a template for the mapping in elasticsearch and a documentation for the fields
which is automatically generated based on `fields.yml` by running the following command.

```
make update
```


### Cleanup

To clean  Machinebeat source code, run the following command:

```
make fmt
```

To clean up the build directory and generated artifacts, run:

```
make clean
```


### Clone

To clone Machinebeat from the git repository, run the following commands:

```
mkdir -p ${GOPATH}/src/github.com/felix-lessoer/machinebeat
git clone https://github.com/felix-lessoer/machinebeat ${GOPATH}/src/github.com/felix-lessoer/machinebeat
```


For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).


## Packaging

The beat frameworks provides tools to crosscompile and package your beat for different platforms. This requires [docker](https://www.docker.com/) and vendoring as described above. To build packages of your beat, run the following command:

```
make release
```

This will fetch and create all images required for the build process. The whole process to finish can take several minutes.
