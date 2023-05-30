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

func (s *Worker) Mtr(in *proto.MtrRequest, server proto.MtrSbWorker_MtrServer) error {
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
		server.Send(&proto.MtrResponse{Response: &proto.MtrResponse_Error{Error: &proto.Error{
			Title:   "no target",
			Message: "message A no target message B",
		}}})
		return fmt.Errorf("no target")
	}
	if ipfilter.IsPrivateIP(target) {
		server.Send(&proto.MtrResponse{Response: &proto.MtrResponse_Error{Error: &proto.Error{
			Title:   "bad target",
			Message: "target is private ip address",
		}}})
		return fmt.Errorf("bad target")
	}

	server.Send(&proto.MtrResponse{Response: &proto.MtrResponse_Lookup{Lookup: &proto.HostLookupResult{
		Ip: target,
	}}})

	ctx, cancel := context.WithTimeout(server.Context(), time.Second*15)
	defer cancel()
	args := []string{"--no-dns", "--raw", "--report-cycles", "10", target}
	cmd := exec.CommandContext(ctx, "mtr", args...)
	stdout, err := cmd.StdoutPipe()
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("something bad happened: %v", err)
	}

	ina := bufio.NewScanner(stdout)
	for ina.Scan() {
		line := ina.Text()
		log.Println("line", line)

		result := parseMtrResult(line)
		if result != nil {
			server.Send(result)
		}
	}

	cmd.Wait()

	fmt.Println("done")
	return nil
}

func parseMtrResult(line string) *proto.MtrResponse {
	line = strings.Replace(strings.TrimSpace(line), "  ", " ", -1)

	pieces := strings.Split(line, " ")
	position, err := strconv.Atoi(pieces[1])
	if err != nil {
		log.Println(err)
		return nil
	}

	switch pieces[0] {
	case "d":
		return &proto.MtrResponse{
			Pos: uint32(position),
			Response: &proto.MtrResponse_Dns{
				Dns: &proto.MtrDnsLine{
					Hostname: pieces[2],
				},
			},
		}
	case "h":
		return &proto.MtrResponse{
			Pos: uint32(position),
			Response: &proto.MtrResponse_Host{
				Host: &proto.MtrHostLine{
					Ip: pieces[2],
				},
			},
		}
	case "p":
		rtt, err := strconv.ParseFloat(pieces[2], 32)
		if err != nil {
			log.Println(err)
			return nil
		}
		seq, err := strconv.Atoi(pieces[3])
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.MtrResponse{
			Pos: uint32(position),
			Response: &proto.MtrResponse_Ping{
				Ping: &proto.MtrPingLine{
					Rtt:    float32(rtt / 1000),
					Seqnum: uint32(seq),
				},
			},
		}
	case "x":
		seq, err := strconv.Atoi(pieces[2])
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.MtrResponse{
			Pos: uint32(position),
			Response: &proto.MtrResponse_Transmit{
				Transmit: &proto.MtrTransmitLine{
					Seqnum: uint32(seq),
				},
			},
		}
	default:
		return &proto.MtrResponse{
			Response: &proto.MtrResponse_Error{
				Error: &proto.Error{
					Message: fmt.Sprintf("Unknown reply: %s", line),
				},
			},
		}
	}

}
