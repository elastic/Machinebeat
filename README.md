# About Machinebeat

> This Beat is beta. It is used in many different environments without issues for longe periods of time. However if you have issues with this beat you can't ask Elastic support, you will need to open an issue in this repository to get help.

The current version in master of Machinebeat is well working and tested. It will be developed further over time but can already be used. The downloadable binaries are expected to run without any major issues. Every attempt will be made to avoid breaking changes in future releases.

What is a beat? A beat is a lightweight data shipper written in GOLANG developed by [Elastic N.V.](https://www.elastic.co) and the community. It is open source and also implements a framework offered to the community to [build their own beats](https://www.elastic.co/guide/en/beats/devguide/current/new-beat.html). Those beats may offer special purpose data collection not offered by [existing beats](https://www.elastic.co/products/beats). Machinebeat is one of those special purpose beats.

The aim of Machinebeat is to build a lightweight data shipper that is able to pull metrics from machines used in industrial environments and ship it to [Elasticsearch](https://www.elastic.co/what-is/elasticsearch), or have [Logstash](https://www.elastic.co/products/logstash) or a message queue like [Kafka](https://kafka.apache.org/) inbetween, depending on the use case and architecture.

Today Machinebeat is able to connect to [OPC-UA](https://opcfoundation.org/) servers, MQTT Broker and IoT Cloud Services like [AWS IoT core](https://aws.amazon.com/iot-core/). It can pull data in real-time, subscribing to topics and feeding the data into Elasticsearch for near real-time monitoring purposes. This will enable machine operators to see behavior of their machines and analyse important metrics on the fly.

Machinebeat also supports [PLC4X](https://plc4x.apache.org/) protocols such as Modbus, S7, Ads, Bacnet, CBus, Eip and KNX. The PLC4X module is relatively new, we are looking for some real world users to check that the different protocols work as expected.

The ability to get machine metrics and other related information is a foundation for maintenance, quality assurance and optimization of industry 4.0 environments. This observability can help to identify potential issues early in the production cycle or optimize the use of ingredients for the products produced. There are many other problems that can be solved by monitoring machine metrics once the data is integrated into an analytics platform such as Elastic.

In a future version Machinebeat is envisaged to support additional protocols in order to cover a broader mix of different sensor metrics in the industry.

# Let's test it

You don't have an Elastic cluster up and running?
Use [Elasticsearch Service](https://www.elastic.co/cloud/elasticsearch-service/signup)

## Getting Started with Machinebeat

To start quickly and easy use the [pre-built binaries](https://github.com/elastic/Machinebeat/releases/) shared in this repository.
To get started choose the version that fits for your environment.

### ARM

Download the latest version of the binary here:
https://ela.st/machinebeat-arm

### Linux

Download the latest version of the binary here:
https://ela.st/machinebeat-linux

Or use 
```wget https://github.com/elastic/Machinebeat/releases/download/v.8.7.0/machinebeat_linux_8.7.0.zip```
and change the version number `8.7.0` to the latest

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
In order to get your own docker image. After finishing that process you can start your machinebeat container with
```
docker/docker-run.sh
```

Afterwardd you can check that it is working with 
```
docker logs machinebeat
```

### Configurations

#### OPCUA Module

Machinebeat uses the [GOPCUA](https://github.com/gopcua/opcua) project to connect and pull data or subscribe from OPS-UA servers. GOPCUA is an implementation of the OPC-UA specification written in GOLANG.
More about OPC-UA can be found on [Wikipedia](https://en.wikipedia.org/wiki/OPC_Unified_Architecture).

To enable the OPCUA Module rename the file `modules.d/opcua.yml.disabled` to `modules.d/opcua.yml`.

The current version is able to read and subscribe to nodes from an [OPC-UA address space](https://opcfoundation.org/developer-tools/specifications-unified-architecture/part-3-address-space-model/) specified in the `modules.d/opcua.yml` file from servers and transfer the collected data to Elasticsearch directly or via Logstash. The nodes and its leaves will be discovered automatically from mentioned YAML file like this:

```
- module: opcua
  metricsets: ["nodevalue"]
  enabled: true
  period: 2s
  
#The URL of your OPC UA Server
  endpoint: "opc.tcp://opcuaserver.com:48010"
  
```

Different nodes can be specified and read by Machinebeat.

#### MQTT Module

To enable the MQTT Module rename the file `modules.d/mqtt.yml.disabled` to `modules.d/mqtt.yml`.
Change the configuration based on your needs. It works with every MQTT broker and also IoT cloud services.

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
  
Make sure your certificate has the correct policies attached:
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

#### PLC4X Module

Machinebeat supports PLC4X protocols such as Modbus, S7, Ads, Bacnet, CBus, Eip and KNX. The PLC4X module is relatively new. We are looking for some real world users to check that the different protocols work as expected.
To enable the PLC4X Module rename the `file modules.d/plc4x.yml.disabled` to `modules.d/plc4x.yml`.
Change the configuration based on your needs.

## How to build on your own environment

1.) Download all dependencies from go.mod using `go get -u`
2.) You may need to overwrite some modules with the following versions that do not support `go.mod` in older versions:
```
go get k8s.io/client-go@kubernetes-1.14.8
go get k8s.io/api@kubernetes-1.14.8
go get k8s.io/apimachinery@kubernetes-1.14.8
```
3.) Run `go build` in the machinebeat repository
