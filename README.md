# About Machinebeat

> This Beat is experimental and may be changed or removed completely in a future release. Do not use in production.

The current version in master of Machinebeat is a well working and tested prototype. It will be developed further over time but can already be used. The downloadable binaries are expected to run without any mayor issues. The current state is experimental. Breaking changes are possible in future releases!

What is a beat? A beat is a lightweight data shipper written in GOLANG developed by [Elastic N.V.](https://www.elastic.co) and the community. It is open source and also implements a framework offered to the community to [build their own beats](https://www.elastic.co/guide/en/beats/devguide/current/new-beat.html). Those beats may offer special purpose data collection not offered by [existing beats](https://www.elastic.co/products/beats). Machinebeat is one of those special purpose beats.

The aim of machinebeat is, to build a leightweight data shipper that is able to pull metrics from machines used in industrial  environments and ship it to Elasticsearch or have [Logstash](https://www.elastic.co/products/logstash) or a message queue like [Kafka](https://kafka.apache.org/) inbetween, depending on the use case and architecture.

Today Machinebeat is able to connect to [OPC-UA](https://opcfoundation.org/) Servers, MQTT Broker and IoT Clout Services like AWS IoT core. It can pull data in real-time, subscribing to topics and feeding the data into Elasticsearch for near real-time monitoring purpose. This will enable machine operators to see behavior of their machines and analyse important metric on the fly.
If you want to collect data from [PLCs](https://en.wikipedia.org/wiki/Programmable_logic_controller) check this out: [PLC4X at Github](https://github.com/apache/plc4x)

The ability to get machine metrics and other related information is a foundation for maintenance, quality assurance and optimization of industry 4.0 environments. It can help to see possible issues early in the production cylce or optimize the use of incredients for the products produced. There are many other problems that can be solved monitoring machine metrics once they can be derived from them.

In a future version Machinebeat is supposed to support additional protocols in order to cover a broader mix of different sensor metrics in the industry.

# Latest release news 24. March 2020

The OPC UA module does support automatic browsing now. So you don't need to know the structure of the data in your OPC UA server.
For users that have been used Machinebeat < 7.6:

There is a breaking change in the configuration. If you configure your nodes directly you need to put namespace (ns) into the ID.

A valid ID looks like this:

`id: "ns=2;s=Dynamic/RandomFloat"`

# Let's test it

You don't have an Elastic cluster up and running?
Use [Elasticsearch Service](https://www.elastic.co/cloud/elasticsearch-service/signup)

## Getting Started with Machinebeat
To start quickly and easy use the pre build binaries shared in this repo!
To get started choose the version that fits for your environment.

### Linux
Download the latest version of the binary here:
https://ela.st/machinebeat-linux

Start the beat with the following command in your terminal.
```
./machinebeat -e -c machinebeat.yml
```
Use -e to see the log output.
Use -c to set a different config file.

### Windows

Download the latest version of the binary here:
https://ela.st/machinebeat-windows

Start the beat with the following command in your CMD or PowerShell. There is also a PowerShell Script to add the beat as a service.
```
machinebeat.exe -e -c machinebeat.yml
```
Use -e to see the log output.
Use -c to set a different config file.

### Configurations

#### OPCUA Module
Machinebeat uses the [GOPCUA](https://github.com/gopcua/opcua) project to connect and pull data or subscribe from OPS-UA servers. GOPCUA is an implementation of the OPC-UA specification written in GOLANG.
More about OPC-UA can be found on [Wikipedia](https://en.wikipedia.org/wiki/OPC_Unified_Architecture)

To enable the OPCUA Module rename the file modules.d/opcua.yml.disabled to modules.d/opcua.yml

The current version is able to read and subscribe nodes from an [OPC-UA address space](https://opcfoundation.org/developer-tools/specifications-unified-architecture/part-3-address-space-model/) specified in the modules.d/opcua.yml file from Servers and transfer the collected data to Elasticsearch directly or via Logstash. The nodes and its leafs will be discovered automatically mentioned YAML file like this:

```

- module: opcua
  metricsets: ["nodevalue"]
  enabled: true
  period: 2s
  
#The URL of your OPC UA Server
  endpoint: "opc.tcp://opcuaserver.com:48010"
  
```

Different Nodes can be specified and read by machinebeat.

#### MQTT Module
To enable the MQTT Module rename the file modules.d/mqtt.yml.disabled to modules.d/mqtt.yml
Change the configuration based on your needs. It works with every MQTT Broker and also IoT Cloud Services.

Example configuration to collect from AWS IoT core:
```
- module: mqtt
  metricsets: ["topic"]
  enabled: true
  period: 1s
  host: "tcps://<yourAWSEndpoint>:8883/mqtt"
  clientID: "<yourAWSclientID>"
  topics: ["#"]
  decode_payload: true
  ssl: true
  CA: "<pathToAWSRootCA>"
  clientCert: "<pathToAWSyourIoTCertificate>"
  clientKey: ""<pathToAWSyourIoTPrivateKey>""
```
Your client id from IoT console -> things:
```
arn:aws:iot:us-east-2:<AWS account>:thing/<clientID>
```
  
Make sure your certificate has correct policies attached:
```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "iot:*",
      "Resource": "*"
    }
  ]
}
```

## How to build on your own env

If it not works in your environment you need to compile to your environment:
Ensure that this folder is at the following location:
`${GOPATH}/src/github.com/elastic/machinebeat`

### Requirements

* [Golang](https://golang.org/dl/) 1.12
* [Elastic Stack](https://cloud.elastic.co) >v7.6

### Init Project
To get running with Machinebeat and also install the
dependencies, run the following command:

```
make setup
```

It will create a clean git history for each major step. Note that you can always rewrite the history if you wish before pushing your changes.

To push Machinebeat in the git repository, run the following commands:

```
git remote set-url origin https://github.com/elastic/machinebeat
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
mkdir -p ${GOPATH}/src/github.com/elastic/machinebeat
git clone https://github.com/elastic/machinebeat ${GOPATH}/src/github.com/elastic/machinebeat
```


For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).


## Packaging

The beat frameworks provides tools to crosscompile and package your beat for different platforms. This requires [docker](https://www.docker.com/) and vendoring as described above. To build packages of your beat, run the following command:

```
make release
```

This will fetch and create all images required for the build process. The whole process to finish can take several minutes.
