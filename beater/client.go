package beater

import (
	"context"
	"time"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/felix-lessoer/machinebeat/config"

	"github.com/wmnsk/gopcua/datatypes"
	"github.com/wmnsk/gopcua/services"
	"github.com/wmnsk/gopcua/uacp"
	"github.com/wmnsk/gopcua/uasc"
)

var (
	client   *uacp.Conn
	secChan  *uasc.SecureChannel
	session  *uasc.Session
	endpoint string

	connected = false
)

func connect(endpointURL string) error {
	var err error
	endpoint = endpointURL
	if !connected {
		logp.Info("Connecting to %v", endpoint)
		ctx := context.Background()
		//ctx, _ := context.WithCancel(ctx)

		client, err := uacp.Dial(ctx, endpoint)
		if err != nil {
			return err
		}
		logp.Debug("Connect", "Successfully established connection with %v", client.RemoteEndpoint())

		// Open SecureChannel on top of UACP Connection established above.
		cfg := uasc.NewClientConfigSecurityNone(3333, 3600000)
		secChan, err = uasc.OpenSecureChannel(ctx, client, cfg, 5*time.Second, 3)
		if err != nil {
			return err
		}
		logp.Debug("Connect", "Successfully opened secure channel with %v", secChan.RemoteEndpoint())

		//discover()

		sessCfg := uasc.NewClientSessionConfig(
			[]string{"de-DE"},
			datatypes.NewAnonymousIdentityToken("anonymous"),
		)
		session, err = uasc.CreateSession(ctx, secChan, sessCfg, 3, 5*time.Second)
		if err != nil {
			return err
		}

		logp.Debug("Connect", "Successfully created secure session with %v", secChan.RemoteEndpoint())

		if err := session.Activate(); err != nil {
			return err
		}
		logp.Debug("Connect", "Successfully activated secure session with %v", secChan.RemoteEndpoint())

		connected = true
		logp.Info("Connection established")
	}
	return err
}

func discover() error {
	// Send FindServersRequest to remote Endpoint.
	if err := secChan.FindServersRequest([]string{"ja-JP", "de-DE", "en-US"}, "gopcua-server"); err != nil {
		return err
	}
	logp.Debug("Discover", "Successfully sent FindServersRequest")

	// Send GetEndpointsRequest to remote Endpoint.
	if err := secChan.GetEndpointsRequest([]string{"ja-JP", "de-DE", "en-US"}, []string{"gopcua-server"}); err != nil {
		return err
	}
	logp.Debug("Discover", "Successfully sent GetEndpointsRequest")

	return nil
}

func collectData(node config.Node) (map[string]interface{}, error) {
	logp.Debug("Collect", "Collecting data from Node %v (NS = %v)", node.ID, node.Namespace)
	var retVal = make(map[string]interface{})
	var nodeId *datatypes.NodeID

	switch v := node.ID.(type) {
	case int:
		nodeId = datatypes.NewNumericNodeID(node.Namespace, node.ID.(uint32))
	case string:
		nodeId = datatypes.NewStringNodeID(node.Namespace, node.ID.(string))
	default:
		logp.Debug("Collect", "Configured node id %v has not a valid type. int and string is allowed. %v provided", node.ID, v)
	}
	if err := session.ReadRequest(
		2000, services.TimestampsToReturnBoth, datatypes.NewReadValueID(
			nodeId, datatypes.IntegerIDValue, "", 0, "",
		),
	); err != nil {
		return nil, err
	}
	logp.Debug("Collect", "Successfully sent ReadRequest")

	data := make([]byte, 1000)
	_, err := session.Read(data)
	if err != nil {
		return nil, err
	}
	logp.Debug("Collect", "Successfully read the response -- now decoding")
	msg, err := uasc.Decode(data)
	if err != nil {
		return nil, err
	}
	logp.Debug("Collect", "Decoding done. Raw message: %v", msg)

	switch m := msg.Service.(type) {
	case *services.ReadResponse:
		value, status := handleReadResponse(m)
		retVal["Node"] = node.ID
		if value.Value != nil {
			retVal["Value"] = value.Value
		}
		retVal["Status"] = status
		retVal["Value_Timestamp"] = m.Timestamp
	default:
		logp.Debug("Collect", "Response type unknown")
	}
	logp.Debug("Collect", "Data collection done")

	return retVal, nil
}

func handleReadResponse(resp *services.ReadResponse) (value *datatypes.Variant, status uint32) {
	//TODO: Return array of values not only the first one
	for _, r := range resp.Results.DataValues {
		return r.Value, r.Status
	}
	return nil, 0
}

func closeConnection() {
	session.Close()
	logp.Debug("Collect", "Successfully closed session with %v", secChan.RemoteEndpoint())
	secChan.Close()
	logp.Debug("Collect", "Successfully closed secure channel with %v", client.RemoteEndpoint())
	client.Close()
	logp.Debug("Collect", "Successfully shutdown connection")
}
