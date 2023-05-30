package worker

import (
	"bufio"
	"context"
	"fmt"
	"git.esd.cc/imlonghao/mtr.sb/pkgs/ipfilter"
	"git.esd.cc/imlonghao/mtr.sb/proto"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func (s *Worker) Traceroute(in *proto.TracerouteRequest, server proto.MtrSbWorker_TracerouteServer) error {
	host := in.GetHost()
	protocol := in.GetProtocol()
	log.Printf("Received: %v @ %d", host, protocol)

	ips, _ := net.LookupIP(host)
	target := ""
	for _, ip := range ips {
		switch protocol {
		case proto.Protocol_ANY:
			target = ip.String()
			goto END
		case proto.Protocol_IPV4:
			if ipv4 := ip.To4(); ipv4 != nil {
				target = ip.String()
				goto END
			}
		case proto.Protocol_IPV6:
			if ipv4 := ip.To4(); ipv4 == nil {
				target = ip.String()
				goto END
			}
		}
	}
END:
	if target == "" {
		server.Send(&proto.TracerouteResponse{Response: &proto.TracerouteResponse_Error{Error: &proto.Error{
			Title:   "no target",
			Message: "message A no target message B",
		}}})
		return fmt.Errorf("no target")
	}
	if ipfilter.IsPrivateIP(target) {
		server.Send(&proto.TracerouteResponse{Response: &proto.TracerouteResponse_Error{Error: &proto.Error{
			Title:   "bad target",
			Message: "target is private ip address",
		}}})
		return fmt.Errorf("bad target")
	}

	server.Send(&proto.TracerouteResponse{Response: &proto.TracerouteResponse_Lookup{Lookup: &proto.HostLookupResult{
		Ip: target,
	}}})

	ctx, cancel := context.WithTimeout(server.Context(), time.Second*10)
	defer cancel()
	args := []string{"-n", "-q", "1", target}
	cmd := exec.CommandContext(ctx, "traceroute", args...)
	stdout, err := cmd.StdoutPipe()
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("something bad happened: %v", err)
	}

	ina := bufio.NewScanner(stdout)
	for ina.Scan() {
		line := ina.Text()
		log.Println("line", line)

		result := parseTracerouteResult(line)
		if result != nil {
			server.Send(result)
		}
	}

	cmd.Wait()

	fmt.Println("done")
	return nil
}

func parseTracerouteResult(line string) *proto.TracerouteResponse {
	line = strings.Replace(strings.TrimSpace(line), "  ", " ", -1)

	if strings.HasPrefix(line, "traceroute to") {
		return nil
	}

	pieces := strings.Split(line, " ")
	seqStr := pieces[0]
	seq, err := strconv.Atoi(seqStr)
	if err != nil {
		log.Println(err)
		return nil
	}

	if len(pieces) >= 2 && pieces[1] == "*" {
		return &proto.TracerouteResponse{
			Response: &proto.TracerouteResponse_Timeout{
				Timeout: &proto.PingTimeout{
					Seq: int32(seq),
				},
			},
		}
	}

	if len(pieces) >= 3 {
		rtt, err := strconv.ParseFloat(pieces[2], 32)
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.TracerouteResponse{
			Response: &proto.TracerouteResponse_Reply{
				Reply: &proto.TracerouteReply{
					Ip:  pieces[1],
					Seq: int32(seq),
					Rtt: float32(rtt),
				},
			},
		}
	}

	return &proto.TracerouteResponse{
		Response: &proto.TracerouteResponse_Error{
			Error: &proto.Error{
				Message: fmt.Sprintf("Unknown reply: %s", line),
			},
		},
	}
}
