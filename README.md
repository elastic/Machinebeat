# About Machinebeat

> This Beat is beta. It is used in many different environments without bigger issues for a longer period of time. However if you have issues with this beat you can't ask support. You need to open an issue in this repository to get help.

The current version in master of Machinebeat is a well working and tested. It will be developed further over time but can already be used. The downloadable binaries are expected to run without any mayor issues. The current state is beta. We try to avoid breaking changes in future releases!

What is a beat? A beat is a lightweight data shipper written in GOLANG developed by [Elastic N.V.](https://www.elastic.co) and the community. It is open source and also implements a framework offered to the community to [build their own beats](https://www.elastic.co/guide/en/beats/devguide/current/new-beat.html). Those beats may offer special purpose data collection not offered by [existing beats](https://www.elastic.co/products/beats). Machinebeat is one of those special purpose beats.

The aim of machinebeat is, to build a leightweight data shipper that is able to pull metrics from machines used in industrial  environments and ship it to Elasticsearch or have [Logstash](https://www.elastic.co/products/logstash) or a message queue like [Kafka](https://kafka.apache.org/) inbetween, depending on the use case and architecture.

Today Machinebeat is able to connect to [OPC-UA](https://opcfoundation.org/) Servers, MQTT Broker and IoT Clout Services like AWS IoT core. It can pull data in real-time, subscribing to topics and feeding the data into Elasticsearch for near real-time monitoring purpose. This will enable machine operators to see behavior of their machines and analyse important metric on the fly.
If you want to collect data from [PLCs](https://en.wikipedia.org/wiki/Programmable_logic_controller) check this out: [PLC4X at Github](https://github.com/apache/plc4x)

The ability to get machine metrics and other related information is a foundation for maintenance, quality assurance and optimization of industry 4.0 environments. It can help to see possible issues early in the production cylce or optimize the use of incredients for the products produced. There are many other problems that can be solved monitoring machine metrics once they can be derived from them.

In a future version Machinebeat is supposed to support additional protocols in order to cover a broader mix of different sensor metrics in the industry.

# Let's test it

You don't have an Elastic cluster up and running?
Use [Elasticsearch Service](https://www.elastic.co/cloud/elasticsearch-service/signup)

## Getting Started with Machinebeat
To start quickly and easy use the pre build binaries shared in this repo!
To get started choose the version that fits for your environment.

### ARM
Download the latest version of the binary here:
https://ela.st/machinebeat-arm

### Linux
Download the latest version of the binary here:
https://ela.st/machinebeat-linux

or use 
```wget https://github.com/elastic/Machinebeat/releases/download/v7.17.1/machinebeat_linux_7.17.1.zip```
and change the version number v7.17.1 to the latest

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

### Docker

We do not publish images of machinebeat. However we prepared a Dockerfile in order to build your own docker images.
Before you build the image you can already change the config files. That way the configuration will be part of the image. 
Of course you could also mount the config files later.

Just run 
```
docker build . -f docker/Dockerfile -t elastic/machinebeat
```
in order to get your own docker image. After finishing that process you can start your machinebeat container with
```
docker/docker-run.sh
```

Afterward you can check that its working with 
```
docker logs machinebeat
```
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

1.) Download all dependencies from go.mod using `go get -u`

2.) You may need to overwrite some modules with the following versions that do not support go.mod in older versions
```
go get k8s.io/client-go@kubernetes-1.14.8
go get k8s.io/api@kubernetes-1.14.8
go get k8s.io/apimachinery@kubernetes-1.14.8
```
3.) Run `go build` in the machinebeat repository
