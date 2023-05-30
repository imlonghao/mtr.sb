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
	"regexp"
	"strconv"
	"time"
)

var (
	timeOutRegex  *regexp.Regexp
	bytesRegex    *regexp.Regexp
	seqRegex      *regexp.Regexp
	ttlRegex      *regexp.Regexp
	rttRegex      *regexp.Regexp
	sentRegex     *regexp.Regexp
	receivedRegex *regexp.Regexp
)

func init() {
	timeOutRegex = regexp.MustCompile(`^no answer yet for`)
	bytesRegex = regexp.MustCompile(`([0-9]+) bytes from`)
	seqRegex = regexp.MustCompile(`seq=([0-9]+)\b`)
	ttlRegex = regexp.MustCompile(`ttl=([0-9]+)\b`)
	rttRegex = regexp.MustCompile(`time=([0-9.]+) ms\b`)
	sentRegex = regexp.MustCompile(`([0-9]+) packets transmitted`)
	receivedRegex = regexp.MustCompile(`([0-9]+) received`)
}

func (s *Worker) Ping(in *proto.PingRequest, server proto.MtrSbWorker_PingServer) error {
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
		server.Send(&proto.PingResponse{Response: &proto.PingResponse_Error{Error: &proto.Error{
			Title:   "no target",
			Message: "message A no target message B",
		}}})
		return fmt.Errorf("no target")
	}
	if ipfilter.IsPrivateIP(target) {
		server.Send(&proto.PingResponse{Response: &proto.PingResponse_Error{Error: &proto.Error{
			Title:   "bad target",
			Message: "target is private ip address",
		}}})
		return fmt.Errorf("bad target")
	}

	server.Send(&proto.PingResponse{Response: &proto.PingResponse_Lookup{Lookup: &proto.HostLookupResult{
		Ip: target,
	}}})

	ctx, cancel := context.WithTimeout(server.Context(), time.Second*10)
	defer cancel()
	args := []string{"-n", "-i", "0.5", "-c", "10", "-O", target}
	cmd := exec.CommandContext(ctx, "ping", args...)
	stdout, err := cmd.StdoutPipe()
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("something bad happened: %v", err)
	}

	ina := bufio.NewScanner(stdout)
	for ina.Scan() {
		line := ina.Text()
		log.Println("line", line)

		result := parsePingResult(line)
		if result != nil {
			server.Send(result)
		}
	}

	cmd.Wait()

	fmt.Println("done")
	return nil
}

func parsePingResult(line string) *proto.PingResponse {
	seqStr := seqRegex.FindStringSubmatch(line)

	if timeOutRegex.MatchString(line) {
		seq, err := strconv.Atoi(seqStr[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.PingResponse{
			Response: &proto.PingResponse_Timeout{
				Timeout: &proto.PingTimeout{
					Seq: int32(seq),
				},
			},
		}
	}

	bytesMatch := bytesRegex.MatchString(line)
	rttMatch := rttRegex.MatchString(line)
	ttlMatch := ttlRegex.MatchString(line)
	if bytesMatch && rttMatch && ttlMatch {
		bytes, err := strconv.Atoi(bytesRegex.FindStringSubmatch(line)[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		ttl, err := strconv.Atoi(ttlRegex.FindStringSubmatch(line)[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		rtt, err := strconv.ParseFloat(rttRegex.FindStringSubmatch(line)[1], 32)
		if err != nil {
			log.Println(err)
			return nil
		}
		seq, err := strconv.Atoi(seqStr[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.PingResponse{
			Response: &proto.PingResponse_Reply{
				Reply: &proto.PingReply{
					Rtt:   float32(rtt),
					Ttl:   int32(ttl),
					Seq:   int32(seq),
					Bytes: int32(bytes),
				},
			},
		}
	}

	sentMatch := sentRegex.MatchString(line)
	receivedMatch := receivedRegex.MatchString(line)
	if sentMatch && receivedMatch {
		received, err := strconv.Atoi(receivedRegex.FindStringSubmatch(line)[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		sent, err := strconv.Atoi(sentRegex.FindStringSubmatch(line)[1])
		if err != nil {
			log.Println(err)
			return nil
		}
		return &proto.PingResponse{
			Response: &proto.PingResponse_Summary{
				Summary: &proto.PingSummary{
					Received: int32(received),
					Sent:     int32(sent),
				},
			},
		}
	}
	return nil
}
