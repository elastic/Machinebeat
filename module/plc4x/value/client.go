package plc4xvalue

import (
	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/apache/plc4x/plc4go/pkg/api"
	"github.com/apache/plc4x/plc4go/pkg/api/drivers"
	"github.com/apache/plc4x/plc4go/pkg/api/model"
	"github.com/apache/plc4x/plc4go/pkg/api/values"

	"errors"
	"fmt"
)

type Client struct {
	connection       plc4go.PlcConnection
	connectionResult plc4go.PlcConnectionConnectResult

	config    *MetricSet
	connected bool
	counter   int
}

type ResponseObject struct {
	node  Node
	value values.PlcValue
}

type Node struct {
	ID    string `config:"tag"`
	Label string `config:"label"`
	Name  string
}

func (client *Client) connect() (bool, error) {
	var err error
	var config = client.config

	//Implement connection logic
	// Create a new instance of the PlcDriverManager
	driverManager := plc4go.NewPlcDriverManager()

	// Register the Drivers
	drivers.RegisterModbusTcpDriver(driverManager)
	drivers.RegisterAdsDriver(driverManager)
	drivers.RegisterBacnetDriver(driverManager)
	drivers.RegisterCBusDriver(driverManager)
	drivers.RegisterEipDriver(driverManager)
	drivers.RegisterKnxDriver(driverManager)
	drivers.RegisterS7Driver(driverManager)

	// Get a connection to a remote PLC
	connectionRequestChanel := driverManager.GetConnection(config.Endpoint)
	// Wait for the driver to connect (or not)
	client.connectionResult = <-connectionRequestChanel

	// Check if something went wrong
	if client.connectionResult.GetErr() != nil {
		fmt.Printf("Error connecting to PLC: %s", client.connectionResult.GetErr().Error())
		return false, client.connectionResult.GetErr()
	}

	client.connection = client.connectionResult.GetConnection()

	if !client.connection.IsConnected() {
		return false, errors.New("The connection is not established")
	}

	client.connected = true
	logp.Info("[PLC4x] Connection established")
	return true, err
}

func (client *Client) read() ([]*ResponseObject, error) {
	var retVal []*ResponseObject

	logp.Info("[PLC4x] Start read node values")

	// Prepare a read-request
	for _, node := range client.config.Nodes {
		var response ResponseObject

		readRequest, err := client.connection.ReadRequestBuilder().
			AddTagAddress("tag", node.ID).
			Build()
		if err != nil {
			logp.Info("[PLC4x] Error preparing read-request")
			return retVal, err
		}

		// Execute a read-request
		rrc := readRequest.Execute()

		// Wait for the response to finish
		rrr := <-rrc
		if rrr.GetErr() != nil {
			logp.Info("[PLC4x] Error executing read-request: %s", rrr.GetErr().Error())
			return retVal, rrr.GetErr()
		}

		// Do something with the response
		if rrr.GetResponse().GetResponseCode("tag") != model.PlcResponseCode_OK {
			fmt.Printf("error an non-ok return code: %s", rrr.GetResponse().GetResponseCode("tag").GetName())
			return retVal, rrr.GetErr()
		}

		response.node = node
		response.value = rrr.GetResponse().GetValue("tag")
		fmt.Printf("Got result %f", response.value.GetFloat32())
		retVal = append(retVal, &response)
	}

	return retVal, nil
}

func (client *Client) closeConnection() {
	logp.Debug("Shutdown", "Will shutdown connection savely")
	client.connected = false

	//Fetch panic during shutdown. So that the beat can reconnect
	defer func() {
		if r := recover(); r != nil {
			logp.Info("The connection was already closed / terminated")
		}
	}()

	// Make sure the connection is closed at the end
	client.connection.BlockingClose()

	logp.Debug("Shutdown", "Shutdown successfully")
}
