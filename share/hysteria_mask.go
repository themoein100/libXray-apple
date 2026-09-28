package share

import (
	"encoding/json"
	"strconv"

	"github.com/xtls/xray-core/infra/conf"
)

// buildHy2FinalMask builds Hysteria2 QUIC hop / bandwidth / salamander mask (shared by URI and Clash).
func buildHy2FinalMask(up, down, ports string, hopInterval *int32, obfsType, obfsPassword string) (*conf.FinalMask, error) {
	var quicParams *conf.QuicParamsConfig
	if up != "" || down != "" {
		quicParams = &conf.QuicParamsConfig{}
		quicParams.Congestion = "brutal"
		if up != "" {
			quicParams.BrutalUp = conf.Bandwidth(up)
		}
		if down != "" {
			quicParams.BrutalDown = conf.Bandwidth(down)
		}
	}

	var udpMasks []conf.Mask
	if obfsType == "salamander" && obfsPassword != "" {
		obfs := conf.Mask{Type: "salamander"}
		salamander := &conf.Salamander{Password: obfsPassword}
		salamanderRawMessage, err := convertJsonToRawMessage(salamander)
		if err != nil {
			return nil, err
		}
		obfs.Settings = &salamanderRawMessage
		udpMasks = append(udpMasks, obfs)
	}

	// Port hopping is a UDP mask since Xray-core v26.9.9 rather than a QUIC
	// parameter. It goes last: the core wraps the socket from the end of the
	// list, and hopping has to sit under salamander, as it did before.
	if ports != "" {
		hop, err := buildUDPHopMask(ports, hopInterval)
		if err != nil {
			return nil, err
		}
		udpMasks = append(udpMasks, hop)
	}

	if quicParams == nil && len(udpMasks) == 0 {
		return nil, nil
	}
	return &conf.FinalMask{QuicParams: quicParams, Udp: udpMasks}, nil
}

// defaultHopInterval is the Hysteria 2 default for `hop-interval`. The core
// now rejects a hop mask without an interval (minimum 5 s), where it used to
// fall back on its own.
const defaultHopInterval int32 = 30

func buildUDPHopMask(ports string, hopInterval *int32) (conf.Mask, error) {
	interval := defaultHopInterval
	if hopInterval != nil {
		interval = *hopInterval
	}
	// Written as the JSON the core parses: conf.Int32Range and conf.PortList
	// only unmarshal from their string forms, not from their own marshalling.
	settings := map[string]string{
		"mode":        "intervalRemote",
		"remotePorts": ports,
		"interval":    strconv.FormatInt(int64(interval), 10),
	}
	raw, err := convertJsonToRawMessage(settings)
	if err != nil {
		return conf.Mask{}, err
	}
	// Reject a malformed port spec here rather than when the core starts.
	var hop conf.UDPHop
	if err := json.Unmarshal(raw, &hop); err != nil {
		return conf.Mask{}, err
	}
	return conf.Mask{Type: "udphop", Settings: &raw}, nil
}
